package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	pb "HydraSAT/proto"
)

type MasterServer struct {
	pb.UnimplementedSolverServiceServer

	// ── long-lived ────────────────────────────────────────────────────────────
	grpcServer    *grpc.Server
	workerTracker *WorkerTracker // workers stay connected across files
	broadcaster   *TimeoutBroadcaster
	resultLogger  *ResultLogger
	cfg           *HeuristicConfig

	// ── per-problem (all guarded by mu) ──────────────────────────────────────
	mu             sync.RWMutex
	taskManager    *TaskManager
	timeoutTracker *TimeoutTracker
	totalCount     *big.Int
	countMu        sync.Mutex
	done           chan struct{}
	startTime      time.Time
	completedTasks int32
	timedOutTasks  int32
	totalTasks     int32
	cnfFile        string
}

// NewMasterServer creates the server shell. Call Listen() next, then Solve()
// for each CNF file, then Shutdown() when the batch is done.
func NewMasterServer(cfg *HeuristicConfig, logger *ResultLogger) *MasterServer {
	return &MasterServer{
		workerTracker: NewWorkerTracker(),
		broadcaster:   NewTimeoutBroadcaster(),
		resultLogger:  logger,
		cfg:           cfg,
		// per-problem fields are nil until the first Solve() call
	}
}

// Listen binds the gRPC server to port. Non-blocking — returns immediately
// after the listener is up. Call Shutdown() to stop it.
func (s *MasterServer) Listen(port string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to listen on :%s: %w", port, err)
	}

	s.grpcServer = grpc.NewServer()
	pb.RegisterSolverServiceServer(s.grpcServer, s)

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			log.Printf("gRPC server stopped: %v", err)
		}
	}()

	fmt.Printf("[master] gRPC server listening on :%s\n", port)
	return nil
}

// Solve resets the per-problem state for cnfData, loads initialCubes, and
// blocks until every task is complete. It then writes the result log and
// waits 3 seconds so the TUI captures the final stats — but it does NOT
// stop the gRPC server, so workers stay connected for the next file.
func (s *MasterServer) Solve(cnfData *CNFData, cnfFile string, initialCubes [][]int32) error {
	// ── Reset per-problem state ───────────────────────────────────────────────
	tt := NewTimeoutTracker(s.cfg)

	s.mu.Lock()
	s.taskManager = NewTaskManager(cnfData, tt)
	s.timeoutTracker = tt
	s.totalCount = big.NewInt(0)
	s.done = make(chan struct{}, 1) // buffered so SubmitResult never blocks
	s.startTime = time.Now()
	s.cnfFile = cnfFile
	atomic.StoreInt32(&s.completedTasks, 0)
	atomic.StoreInt32(&s.timedOutTasks, 0)
	atomic.StoreInt32(&s.totalTasks, 0)
	s.mu.Unlock()

	// ── Broadcast the initial timeout to any already-subscribed workers ───────
	s.broadcaster.Broadcast(tt.CurrentTimeoutSec())

	// ── Load initial cubes ────────────────────────────────────────────────────
	atomic.AddInt32(&s.totalTasks, int32(len(initialCubes)))
	s.mu.RLock()
	tm := s.taskManager
	s.mu.RUnlock()
	tm.EnqueueCubes(initialCubes)

	fmt.Printf("[master] Solving %s  (%d initial cubes)\n", cnfFile, len(initialCubes))

	// ── Wait for completion ───────────────────────────────────────────────────
	s.mu.RLock()
	doneCh := s.done
	s.mu.RUnlock()
	<-doneCh

	wallTime := time.Since(s.startTime).Seconds()
	fmt.Printf("[master] %s done in %.1fs — holding 3s for TUI…\n", cnfFile, wallTime)

	// ── Log result ────────────────────────────────────────────────────────────
	if s.resultLogger != nil {
		s.countMu.Lock()
		count := s.totalCount.Text(10)
		s.countMu.Unlock()

		s.resultLogger.LogResult(
			s.cfg.BenchmarkLabel,
			s.cfg.MaxWorkers,
			cnfFile,
			count,
			atomic.LoadInt32(&s.completedTasks),
			atomic.LoadInt32(&s.timedOutTasks),
			atomic.LoadInt32(&s.totalTasks),
			wallTime,
			tt.AvgTaskSec(),
			tt.CurrentTimeoutSec(),
			s.workerTracker.TotalWorkerCount(),
		)
	}

	// Grace period: TUI polls every 500 ms — 3 s gives it 6 more snapshots
	// with the correct final count before workers see "no tasks" and go idle.
	time.Sleep(3 * time.Second)
	return nil
}

func (s *MasterServer) LoadInitialTasks(cubes [][]int32) {
	atomic.AddInt32(&s.totalTasks, int32(len(cubes)))
	s.taskManager.EnqueueCubes(cubes)
}

// GetTotalCount returns a copy of the current total model count.
func (s *MasterServer) GetTotalCount() *big.Int {
	s.countMu.Lock()
	defer s.countMu.Unlock()
	if s.totalCount == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(s.totalCount)
}

// Shutdown stops the gRPC server. Call once after all Solve() calls are done.
func (s *MasterServer) Shutdown() {
	if s.grpcServer != nil {
		fmt.Println("[master] Shutting down gRPC server…")
		s.grpcServer.GracefulStop()
	}
}

// GetTask is called by a worker that is ready for work.
func (s *MasterServer) GetTask(ctx context.Context, req *pb.WorkerIdentity) (*pb.TaskPayload, error) {
	s.workerTracker.RegisterWorker(req.WorkerId, req.Hostname)

	// MaxWorkers cap — return early before even looking at the queue
	if s.cfg.MaxWorkers > 0 && int(s.workerTracker.BusyWorkerCount()) >= s.cfg.MaxWorkers {
		return &pb.TaskPayload{TaskId: -1}, nil
	}

	// Snapshot per-problem pointers under read lock
	s.mu.RLock()
	tm := s.taskManager
	tt := s.timeoutTracker
	s.mu.RUnlock()

	// If Solve() hasn't been called yet (startup) or has just reset, tm is nil.
	if tm == nil {
		return &pb.TaskPayload{TaskId: -1}, nil
	}

	fmt.Printf("[master] Worker [%s@%s] requested a task\n", req.WorkerId, req.Hostname)

	select {
	case task := <-tm.TaskQueue():
		task.TimeoutSec = tt.CurrentTimeoutSec()
		tm.RecordAssignment(task.TaskId, req.Hostname)
		s.workerTracker.AssignTask(req.WorkerId, req.Hostname, task.TaskId, task.Literals, float64(task.TimeoutSec))
		fmt.Printf("[master] Task %d → %s@%s  cube=%v  timeout=%ds\n",
			task.TaskId, req.WorkerId, req.Hostname, task.Literals, task.TimeoutSec)
		return task, nil

	case <-time.After(5 * time.Second):
		s.workerTracker.SetIdle(req.WorkerId, req.Hostname)
		return &pb.TaskPayload{TaskId: -1}, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SubmitResult is called by a worker once ganak has finished (or timed out).
func (s *MasterServer) SubmitResult(ctx context.Context, req *pb.TaskResult) (*pb.Empty, error) {
	s.mu.RLock()
	tm := s.taskManager
	tt := s.timeoutTracker
	doneCh := s.done
	s.mu.RUnlock()

	if tm == nil {
		return nil, fmt.Errorf("SubmitResult called before any problem was loaded (task %d)", req.TaskId)
	}

	hostname, cube, exists := tm.GetAndRemoveTask(req.TaskId)
	if !exists {
		return nil, fmt.Errorf("received result for unknown task ID %d", req.TaskId)
	}

	s.workerTracker.UpdateWorkerResult(req.WorkerId, hostname, req.TimedOut)

	if req.TimedOut {
		atomic.AddInt32(&s.timedOutTasks, 1)
		fmt.Printf("[master] Worker %s@%s TIMEOUT task %d: cube=%v — splitting (JW)\n",
			req.WorkerId, hostname, req.TaskId, cube)
		newCubes := SplitCubeSmart(cube, tm.cnfData.Clauses)
		atomic.AddInt32(&s.totalTasks, int32(len(newCubes)))
		tm.EnqueueCubes(newCubes)

	} else {
		if req.DurationSec > 0 {
			oldTimeout := tt.CurrentTimeoutSec()
			tt.RecordCompletion(req.DurationSec)
			newTimeout := tt.CurrentTimeoutSec()
			if newTimeout != oldTimeout {
				s.broadcaster.Broadcast(newTimeout)
			}
		}

		val := new(big.Int)
		if _, ok := val.SetString(req.Count, 10); !ok {
			fmt.Printf("[master] Worker %s@%s: failed to parse count %q for task %d\n",
				req.WorkerId, hostname, req.Count, req.TaskId)
		} else {
			s.countMu.Lock()
			s.totalCount.Add(s.totalCount, val)
			s.countMu.Unlock()
			fmt.Printf("[master] Worker %s@%s task %d: count=%s  avg=%.1fs  next_timeout=%ds\n",
				req.WorkerId, hostname, req.TaskId, val.Text(10),
				tt.AvgTaskSec(), tt.CurrentTimeoutSec())
		}
		atomic.AddInt32(&s.completedTasks, 1)
	}

	if tm.IsEmpty() {
		select {
		case doneCh <- struct{}{}:
		default:
		}
	}

	return &pb.Empty{}, nil
}

// ReportStatus receives a live CPU/RAM heartbeat from a worker.
func (s *MasterServer) ReportStatus(ctx context.Context, req *pb.WorkerStatus) (*pb.Empty, error) {
	s.mu.RLock()
	tm := s.taskManager
	s.mu.RUnlock()

	var hostname string
	if tm != nil {
		hostname, _ = tm.HostnameForTask(req.TaskId)
	}
	s.workerTracker.UpdateLiveStats(req.WorkerId, hostname, req.CpuUsage, req.MemoryMb, req.MemoryPct)
	return &pb.Empty{}, nil
}

// GetMasterStats is called by the TUI every 500 ms.
func (s *MasterServer) GetMasterStats(ctx context.Context, _ *pb.Empty) (*pb.MasterStatsResponse, error) {
	s.mu.RLock()
	tt := s.timeoutTracker
	startTime := s.startTime
	s.mu.RUnlock()

	uptime := time.Since(startTime).Seconds()

	s.countMu.Lock()
	totalCount := ""
	if s.totalCount != nil {
		totalCount = s.totalCount.Text(10)
	} else {
		totalCount = "0"
	}
	s.countMu.Unlock()

	var avgSec float64
	var currentTimeoutF float64
	var dynamicTimeout bool

	if tt != nil {
		avgSec = tt.AvgTaskSec()
		ct := tt.CurrentTimeoutSec()
		currentTimeoutF = float64(ct)
		if ct == math.MaxInt32 {
			currentTimeoutF = 0
		}
		dynamicTimeout = s.cfg.DynamicTimeout
	}

	s.mu.RLock()
	tm := s.taskManager
	s.mu.RUnlock()

	var activeTasks, completedTasks, queuedTasks int32
	if tm != nil {
		activeTasks = tm.ActiveCount()
		queuedTasks = tm.QueuedCount()
	}
	completedTasks = atomic.LoadInt32(&s.completedTasks)

	return &pb.MasterStatsResponse{
		ActiveTasks:       activeTasks,
		CompletedTasks:    completedTasks,
		QueuedTasks:       queuedTasks,
		TotalCount:        totalCount,
		UptimeSec:         uptime,
		TotalWorkers:      s.workerTracker.TotalWorkerCount(),
		BusyWorkers:       s.workerTracker.BusyWorkerCount(),
		Workers:           s.workerTracker.GetAllWorkerStats(),
		AvgTaskSec:        avgSec,
		CurrentTimeoutSec: currentTimeoutF,
		DynamicTimeout:    dynamicTimeout,
	}, nil
}

// SubscribeTimeoutUpdates streams timeout updates to a worker.
func (s *MasterServer) SubscribeTimeoutUpdates(req *pb.WorkerIdentity, stream pb.SolverService_SubscribeTimeoutUpdatesServer) error {
	ch := s.broadcaster.Subscribe(req.WorkerId)
	defer s.broadcaster.Unsubscribe(req.WorkerId)

	// Send the current timeout immediately
	s.mu.RLock()
	tt := s.timeoutTracker
	s.mu.RUnlock()

	if tt != nil {
		if err := stream.Send(&pb.TimeoutUpdate{TimeoutSec: tt.CurrentTimeoutSec()}); err != nil {
			return err
		}
	}

	for {
		select {
		case update, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(update); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}
