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


# How long to keep retrying before giving up permanently.
# Between CNF files the master is still UP (gRPC server stays running),
# so UNAVAILABLE between files is just a transient hiccup that resolves
# in under a second. We retry for up to GIVE_UP_AFTER_SEC before exiting.
GIVE_UP_AFTER_SEC = 120   # 2 minutes of continuous failure = real problem
RETRY_BASE_SEC    = 2.0   # first retry delay
RETRY_MAX_SEC     = 15.0  # cap on exponential back-off


class SATWorker:
    def __init__(self, master_address: str, worker_id: str):
        self.master_address = master_address
        self.worker_id = worker_id
        self.hostname = socket.gethostname()
        self.process = psutil.Process(os.getpid())
        self.ganak_path = self._resolve_ganak()
        self._stub = None  # set in run() before any task is dispatched

    # ── setup ──────────────────────────────────────────────────────────────

    def _resolve_ganak(self) -> str:
        system = shutil.which("ganak") or "/usr/local/bin/ganak"
        repo = Path(__file__).parent.parent / "src/external/ganak-linux-amd64/ganak"
        if os.path.isfile(system) and os.access(system, os.X_OK):
            return system
        if repo.exists():
            return str(repo.resolve())
        return "ganak"

    # ── stats ──────────────────────────────────────────────────────────────

    def _collect_stats(self) -> tuple[float, float, float]:
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
            return cpu / n_cpus, mem_mb, mem_pct
        except Exception as e:
            print(f"[{self.worker_id}] stats error: {e}")
            return 0.0, 0.0, 0.0

    # ── status reporter ────────────────────────────────────────────────────

    def _start_status_reporter(
        self,
        task_id: int,
        stop_event: threading.Event,
        interval: float = 2.0,
    ) -> threading.Thread:
        def _report():
            while not stop_event.is_set():
                cpu, mem_mb, mem_pct = self._collect_stats()
                elapsed = time.time() - start_ts
                try:
                    self._stub.ReportStatus(solver_pb2.WorkerStatus(
                        worker_id=self.worker_id,
                        task_id=task_id,
                        elapsed_time=elapsed,
                        cpu_usage=cpu,
                        memory_mb=mem_mb,
                        memory_pct=mem_pct,
                    ))
                except grpc.RpcError:
                    pass  # transient; the main loop handles persistent failures
                stop_event.wait(interval)

        start_ts = time.time()
        t = threading.Thread(target=_report, daemon=True)
        t.start()
        return t

    # ── timeout subscriber ─────────────────────────────────────────────────

    def _start_timeout_subscriber(
        self,
        task_id: int,
        proc: subprocess.Popen,
        start_ts: float,
        stop_event: threading.Event,
        killed_event: threading.Event,
    ) -> threading.Thread:
        def _listen():
            try:
                stream = self._stub.SubscribeTimeoutUpdates(
                    solver_pb2.WorkerIdentity(
                        worker_id=self.worker_id,
                        hostname=self.hostname,
                    )
                )
                for update in stream:
                    if stop_event.is_set():
                        break

                    new_timeout = update.timeout_sec
                    if new_timeout <= 0 or new_timeout >= 2_147_483_647:
                        print(f"[{self.worker_id}] task {task_id}: timeout update = no limit")
                        continue

                    elapsed = time.time() - start_ts
                    if elapsed >= new_timeout:
                        print(
                            f"[{self.worker_id}] task {task_id}: timeout updated to {new_timeout}s "
                            f"but already elapsed {elapsed:.1f}s — killing ganak"
                        )
                        killed_event.set()
                        try:
                            proc.kill()
                        except Exception:
                            pass
                        break
                    else:
                        remaining = new_timeout - elapsed
                        print(
                            f"[{self.worker_id}] task {task_id}: timeout updated to {new_timeout}s "
                            f"(elapsed {elapsed:.1f}s, {remaining:.1f}s remaining)"
                        )
            except grpc.RpcError as e:
                if not stop_event.is_set():
                    print(f"[{self.worker_id}] timeout stream error: {e}")

        t = threading.Thread(target=_listen, daemon=True)
        t.start()
        return t

    # ── solving ────────────────────────────────────────────────────────────

    def _solve(self, task) -> solver_pb2.TaskResult:
        new_clauses = task.num_clauses + len(task.literals)
        header = f"p cnf {task.num_vars} {new_clauses}"
        cube_clauses = "\n".join(f"{lit} 0" for lit in task.literals)
        formula = f"{header}\n{task.formula_body}\n{cube_clauses}\n"

        subprocess_timeout = (
            None if task.timeout_sec >= 2_147_483_647 else task.timeout_sec
        )

        start_ts = time.time()
        proc = subprocess.Popen(
            [self.ganak_path, "/dev/stdin"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )

        stop_event = threading.Event()
        killed_event = threading.Event()

        self._start_timeout_subscriber(
            task_id=task.task_id,
            proc=proc,
            start_ts=start_ts,
            stop_event=stop_event,
            killed_event=killed_event,
        )

        try:
            stdout, _ = proc.communicate(input=formula, timeout=subprocess_timeout)
            duration = time.time() - start_ts
            stop_event.set()

            if killed_event.is_set():
                print(f"[{self.worker_id}] task {task.task_id} killed by timeout update after {duration:.1f}s")
                return solver_pb2.TaskResult(
                    task_id=task.task_id, worker_id=self.worker_id,
                    count="0", duration_sec=duration, timed_out=True,
                )

            count = parse_ganak_unweighted_count(stdout)
            return solver_pb2.TaskResult(
                task_id=task.task_id, worker_id=self.worker_id,
                count=count, duration_sec=duration, timed_out=False,
            )

        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
            stop_event.set()
            duration = time.time() - start_ts
            print(f"[{self.worker_id}] task {task.task_id} timed out after {duration:.1f}s")
            return solver_pb2.TaskResult(
                task_id=task.task_id, worker_id=self.worker_id,
                count="0", duration_sec=duration, timed_out=True,
            )

    # ── connection + retry logic ───────────────────────────────────────────

    def _run_with_channel(self, channel) -> bool:
        """
        Inner work loop for one channel lifetime.
        Returns True  → caller should reconnect and retry.
        Returns False → caller should exit (master sent permanent shutdown).
        """
        self._stub = solver_pb2_grpc.SolverServiceStub(channel)
        print(f"[{self.worker_id}] connected to {self.master_address} (hostname: {self.hostname})")

        # Reset failure tracking on a fresh connection
        consecutive_failures = 0
        failure_start: float | None = None

        while True:
            try:
                task = self._stub.GetTask(solver_pb2.WorkerIdentity(
                    worker_id=self.worker_id,
                    hostname=self.hostname,
                ))

                # Successful RPC — reset failure tracking
                consecutive_failures = 0
                failure_start = None

                if task.task_id == -1:
                    # No work right now (queue empty between files, or maxWorkers cap).
                    # Sleep briefly and poll again — do NOT exit.
                    time.sleep(2)
                    continue

                timeout_display = (
                    "no limit" if task.timeout_sec >= 2_147_483_647
                    else f"{task.timeout_sec}s"
                )
                print(
                    f"[{self.worker_id}] received task {task.task_id} "
                    f"(timeout: {timeout_display}, cube: {list(task.literals)})"
                )

                stop_reporter = threading.Event()
                self._start_status_reporter(task.task_id, stop_reporter)

                result = self._solve(task)

                stop_reporter.set()
                self._stub.SubmitResult(result)
                print(
                    f"[{self.worker_id}] submitted task {task.task_id}: "
                    f"count={result.count} timed_out={result.timed_out}"
                )

            except grpc.RpcError as e:
                code = e.code()

                if code == grpc.StatusCode.UNAVAILABLE:
                    # The master is temporarily unreachable.
                    # This is NORMAL between CNF files during the 3-second grace
                    # period, or during a brief restart. Retry with backoff.
                    now = time.time()
                    if failure_start is None:
                        failure_start = now
                        print(f"[{self.worker_id}] master unavailable — retrying…")

                    elapsed_failing = now - failure_start
                    if elapsed_failing > GIVE_UP_AFTER_SEC:
                        print(
                            f"[{self.worker_id}] master has been unavailable for "
                            f"{elapsed_failing:.0f}s — giving up"
                        )
                        return False  # permanent failure

                    consecutive_failures += 1
                    delay = min(RETRY_BASE_SEC * (1.5 ** consecutive_failures), RETRY_MAX_SEC)
                    time.sleep(delay)
                    # Signal to the outer loop to rebuild the channel
                    return True

                else:
                    # Other RPC errors (deadline exceeded, internal, etc.)
                    # — transient, back off and retry within the same channel
                    print(f"[{self.worker_id}] RPC error ({code}): {e} — retrying in 5s")
                    time.sleep(5)

            except Exception as e:
                import traceback
                print(f"[{self.worker_id}] unexpected error: {e}")
                traceback.print_exc()
                return False  # unknown error — exit cleanly

    # ── main entry point ───────────────────────────────────────────────────

    def run(self):
        """
        Outer connection loop. Re-establishes the gRPC channel whenever the
        inner loop signals that the connection was lost and should be retried.
        Workers only truly exit when:
          - The master has been continuously unavailable for GIVE_UP_AFTER_SEC
          - An unexpected non-RPC exception occurs in the inner loop
        """
        while True:
            try:
                with grpc.insecure_channel(self.master_address) as channel:
                    should_reconnect = self._run_with_channel(channel)

                if not should_reconnect:
                    print(f"[{self.worker_id}] shutting down")
                    break

                # Brief pause before rebuilding the channel
                print(f"[{self.worker_id}] reconnecting to {self.master_address}…")
                time.sleep(2)

            except Exception as e:
                import traceback
                print(f"[{self.worker_id}] channel error: {e}")
                traceback.print_exc()
                time.sleep(5)


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

    print(f"[worker] Master address: {args.master}")
    SATWorker(args.master, args.id).run()
