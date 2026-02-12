import subprocess
import argparse
import time
import os
import grpc
import solver_pb2
import solver_pb2_grpc
from parser import parse_ganak_unweighted_count


# When master calls SolveCube, grpc on this worker receives the binary data,
# decodes it into Python object request and hands to this function
class SATWorker:
    def __init__(self, master_address, worker_id):
        self.ganak_path = "/usr/local/bin/ganak"
        self.master_address = master_address
        self.worker_id = worker_id

    def solve_task(self, task):
        """Builds the CNF formula and runs the Ganak subprocess."""
        new_nb_clauses = task.num_clauses + len(task.literals)
        header = f"p cnf {task.num_vars} {new_nb_clauses}"
        cube_str = "\n".join(f"{lit} 0" for lit in task.literals)

        # Format the DIMACS file
        new_formula = f"{header}\n{task.formula_body}\n{cube_str}\n"

        start_time = time.time()
        try:
            result = subprocess.run(
                [self.ganak_path, "/dev/stdin"],
                input=new_formula,
                capture_output=True,
                text=True,
                timeout=task.timeout_sec
            )

            duration = time.time() - start_time
            count_str = parse_ganak_unweighted_count(result.stdout)

            return solver_pb2.CountResponse(
                count=count_str,
                duration_sec=duration,
                timed_out=False,
                task_id=task.task_id,
                worker_id=self.worker_id
            )

        except subprocess.TimeoutExpired:
            print(f"Task {task.task_id} timed out after {task.timeout_sec}s")
            return solver_pb2.CountResponse(
                count="0",
                duration_sec=float(task.timeout_sec),
                timed_out=True,
                task_id=task.task_id,
                worker_id=self.worker_id
            )


    def run(self):
        """ Main loop: Ask for work -> Solve -> Submit -> """
        # Connect to master
        with grpc.insecure_channel(self.master_address) as channel:
            stub = solver_pb2_grpc.SolverServiceStub(channel)
            print(f"Worker {self.worker_id}: Connected to master at {self.master_address}")

            while True:
                try:
                    request = solver_pb2.RegisterRequest(worker_id=self.worker_id)
                    task = stub.GetTask(request)

                    # If TaskId is 0, then Master has no more tasks to assign now
                    if task.task_id == -1:
                        print(f"Worker {self.worker_id}: No more tasks available, waiting...")
                        time.sleep(2)  # Wait for a while before asking again
                        continue

                    print(f"Worker {self.worker_id}: Received task {task.task_id}")

                    # Solve the task
                    result = self.solve_task(task)
                    print(f"[*] Task {task.task_id} result: {result.count}. Submitting...")

                    # Submit the result back to Master
                    stub.SubmitResult(result)
                    print(f"Worker {self.worker_id}: Submitted result for task {task.task_id}")

                except grpc.RpcError as e:
                    print(f"Worker {self.worker_id}: RPC error: {e}")
                    time.sleep(5)  # Wait before retrying
                except Exception as e:
                    print(f"Worker {self.worker_id}: Unexpected error: {e}")
                    time.sleep(5)  # Wait before retrying


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--master",
                        type=str,
                        default="master:50051",
                        help="Master's address (IP:Port)")

    parser.add_argument("--id",
                        type=str,
                        default="worker-01",
                        help="Unique ID for this worker")

    args = parser.parse_args()

    
    worker = SATWorker(args.master, args.id)
    worker.run()
