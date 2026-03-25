import argparse
import os
import shutil
import socket
import subprocess
import threading
import time
from pathlib import Path
 
import grpc
import psutil
import solver_pb2
import solver_pb2_grpc
from parser import parse_ganak_unweighted_count


class SATWorker:
    def __init__(self, master_address: str, worker_id: str):
        self.master_address = master_address
        self.worker_id = worker_id
        self.hostname = socket.gethostname()
        self.process = psutil.Process(os.getpid())
        self.ganak_path = self._resolve_ganak()
 
    # ── setup ─────────────────────────────────────────────────────────────────
 
    def _resolve_ganak(self) -> str:
        """Return the path to the ganak binary, preferring system installs."""
        system = shutil.which("ganak") or "/usr/local/bin/ganak"
        repo = Path(__file__).parent.parent / "src/external/ganak-linux-amd64/ganak"
        if os.path.isfile(system) and os.access(system, os.X_OK):
            return system
        if repo.exists():
            return str(repo.resolve())
        return "ganak"
 
    # ── stats ──────────────────────────────────────────────────────────────────
 
    def _collect_stats(self) -> tuple[float, float, float]:
        """Return (cpu_pct, mem_mb, mem_pct) for this process + children."""
        try:
            cpu = self.process.cpu_percent(interval=0.1)
            mem_bytes = self.process.memory_info().rss
 
            for child in self.process.children(recursive=True):
                try:
                    cpu += child.cpu_percent(interval=None)
                    mem_bytes += child.memory_info().rss
                except psutil.NoSuchProcess:
                    pass
 
            mem_mb = mem_bytes / (1024 * 1024)
            system_mem = psutil.virtual_memory()
            mem_pct = (mem_bytes / system_mem.total) * 100
 
            n_cpus = psutil.cpu_count(logical=True) or 1
            cpu_norm = cpu / n_cpus
 
            return cpu_norm, mem_mb, mem_pct
        except Exception as e:
            print(f"[{self.worker_id}] stats error: {e}")
            return 0.0, 0.0, 0.0
 
    # ── status reporter ────────────────────────────────────────────────────────
 
    def _start_status_reporter(
        self,
        stub: solver_pb2_grpc.SolverServiceStub,
        task_id: int,
        stop_event: threading.Event,
        interval: float = 2.0,
    ) -> threading.Thread:
        """Spin up a background thread that calls ReportStatus every `interval` seconds."""
 
        def _report():
            while not stop_event.is_set():
                cpu, mem_mb, mem_pct = self._collect_stats()
                elapsed = time.time() - start_ts
                try:
                    stub.ReportStatus(solver_pb2.WorkerStatus(
                        worker_id=self.worker_id,
                        task_id=task_id,
                        elapsed_time=elapsed,
                        cpu_usage=cpu,
                        memory_mb=mem_mb,
                        memory_pct=mem_pct,
                    ))
                except grpc.RpcError as e:
                    print(f"[{self.worker_id}] ReportStatus error: {e}")
                stop_event.wait(interval)
 
        start_ts = time.time()
        t = threading.Thread(target=_report, daemon=True)
        t.start()
        return t
 
    # ── solving ────────────────────────────────────────────────────────────────
 
    def _solve(self, task) -> solver_pb2.TaskResult:
        """Construct the sub-formula, run ganak, return a TaskResult."""
        new_clauses = task.num_clauses + len(task.literals)
        header = f"p cnf {task.num_vars} {new_clauses}"
        cube_clauses = "\n".join(f"{lit} 0" for lit in task.literals)
        formula = f"{header}\n{task.formula_body}\n{cube_clauses}\n"
 
        start = time.time()
        try:
            proc = subprocess.run(
                [self.ganak_path, "/dev/stdin"],
                input=formula,
                capture_output=True,
                text=True,
                timeout=task.timeout_sec,
            )
            duration = time.time() - start
            count = parse_ganak_unweighted_count(proc.stdout)
            return solver_pb2.TaskResult(
                task_id=task.task_id,
                worker_id=self.worker_id,
                count=count,
                duration_sec=duration,
                timed_out=False,
            )
 
        except subprocess.TimeoutExpired:
            print(f"[{self.worker_id}] task {task.task_id} timed out after {task.timeout_sec}s")
            return solver_pb2.TaskResult(
                task_id=task.task_id,
                worker_id=self.worker_id,
                count="0",
                duration_sec=float(task.timeout_sec),
                timed_out=True,
            )
 
    # ── main loop ──────────────────────────────────────────────────────────────
 
    def run(self):
        with grpc.insecure_channel(self.master_address) as channel:
            stub = solver_pb2_grpc.SolverServiceStub(channel)
            print(f"[{self.worker_id}] connected to {self.master_address} (hostname: {self.hostname})")
 
            while True:
                try:
                    # 1. Ask the master for a task.
                    task = stub.GetTask(solver_pb2.WorkerIdentity(
                        worker_id=self.worker_id,
                        hostname=self.hostname,
                    ))
 
                    if task.task_id == -1:
                        # No work right now — back off briefly.
                        time.sleep(2)
                        continue
 
                    print(f"[{self.worker_id}] received task {task.task_id} "
                          f"(timeout: {task.timeout_sec}s, cube: {list(task.literals)})")
 
                    # 2. Start live CPU/RAM reporting in the background.
                    stop_reporter = threading.Event()
                    self._start_status_reporter(stub, task.task_id, stop_reporter)
 
                    # 3. Solve.
                    result = self._solve(task)
 
                    # 4. Stop the reporter and submit the result.
                    stop_reporter.set()
                    stub.SubmitResult(result)
                    print(f"[{self.worker_id}] submitted task {task.task_id}: count={result.count}")
 
                except grpc.RpcError as e:
                    if e.code() == grpc.StatusCode.UNAVAILABLE:
                        print(f"[{self.worker_id}] master offline — shutting down")
                        break
                    print(f"[{self.worker_id}] RPC error: {e}")
                    time.sleep(5)
 
                except Exception as e:
                    import traceback
                    print(f"[{self.worker_id}] unexpected error: {e}")
                    traceback.print_exc()
                    break
 
 
if __name__ == "__main__":
    env_master = os.getenv("MASTER_ADDR")
 
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--master",
        default=env_master or "master:50051",
        help="Master address (host:port)",
    )
    parser.add_argument(
        "--id",
        default=os.getenv("WORKER_ID", "worker-01"),
        help="Unique worker ID",
    )
    args = parser.parse_args()
 
    print(f"Master address: {args.master}")
    SATWorker(args.master, args.id).run()

