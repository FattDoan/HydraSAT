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

	taskManager    *TaskManager
	workerTracker  *WorkerTracker
	timeoutTracker *TimeoutTracker
	resultLogger   *ResultLogger
	cfg            *HeuristicConfig
	cnfFile        string // for logging

	totalCount     *big.Int
	countMutex     sync.Mutex
	done           chan struct{}
	startTime      time.Time
	completedTasks int32
	totalTasks     int32 // incremented when cubes are enqueued

	broadcaster *TimeoutBroadcaster
}

func NewMasterServer(cnfData *CNFData, cnfFile string, cfg *HeuristicConfig, logger *ResultLogger) *MasterServer {
	tt := NewTimeoutTracker(cfg)
	return &MasterServer{
		taskManager:    NewTaskManager(cnfData, tt),
		workerTracker:  NewWorkerTracker(),
		timeoutTracker: tt,
		resultLogger:   logger,
		broadcaster:    NewTimeoutBroadcaster(),
		cfg:            cfg,
		cnfFile:        cnfFile,
		totalCount:     big.NewInt(0),
		done:           make(chan struct{}),
		startTime:      time.Now(),
	}
}

func (s *MasterServer) LoadInitialTasks(cubes [][]int32) {
	atomic.AddInt32(&s.totalTasks, int32(len(cubes)))
	s.taskManager.EnqueueCubes(cubes)
}

func (s *MasterServer) GetTotalCount() *big.Int {
	s.countMutex.Lock()
	defer s.countMutex.Unlock()
	return new(big.Int).Set(s.totalCount)
}

// GetTask is called by a worker that is ready for work.
func (s *MasterServer) GetTask(ctx context.Context, req *pb.WorkerIdentity) (*pb.TaskPayload, error) {
	s.workerTracker.RegisterWorker(req.WorkerId, req.Hostname)
	fmt.Printf("Worker [%s@%s] requested a task\n", req.WorkerId, req.Hostname)

	select {
	case task := <-s.taskManager.TaskQueue():
		// INJECT THE FRESHEST TIMEOUT HERE before sending to the worker
		task.TimeoutSec = s.timeoutTracker.CurrentTimeoutSec()

		s.taskManager.RecordAssignment(task.TaskId, req.Hostname)
		s.workerTracker.AssignTask(req.WorkerId, req.Hostname, task.TaskId, task.Literals, float64(task.TimeoutSec))

		fmt.Printf("Task %d assigned to worker %s@%s: cube=%v  timeout=%ds\n",
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
	hostname, cube, exists := s.taskManager.GetAndRemoveTask(req.TaskId)
	if !exists {
		return nil, fmt.Errorf("received result for unknown task ID %d", req.TaskId)
	}

	s.workerTracker.UpdateWorkerResult(req.WorkerId, hostname, req.TimedOut)

	if req.TimedOut {
		fmt.Printf("Worker %s@%s TIMEOUT on task %d: cube=%v — splitting\n",
			req.WorkerId, hostname, req.TaskId, cube)
		newCubes := SplitCube(cube)
		atomic.AddInt32(&s.totalTasks, int32(len(newCubes)))
		s.taskManager.EnqueueCubes(newCubes)
	} else {
		// Record duration for dynamic timeout BEFORE parsing count
		if req.DurationSec > 0 {
			// 1. Get the old timeout BEFORE recording the new duration
			oldTimeout := s.timeoutTracker.CurrentTimeoutSec()

			// 2. Record the duration
			s.timeoutTracker.RecordCompletion(req.DurationSec)

			// 3. Get the new timeout
			newTimeout := s.timeoutTracker.CurrentTimeoutSec()

			// 4. ONLY broadcast if the integer value actually changed!
			if newTimeout != oldTimeout {
				s.broadcaster.Broadcast(newTimeout)
			}
		}

		val := new(big.Int)
		if _, ok := val.SetString(req.Count, 10); !ok {
			fmt.Printf("Worker %s@%s: failed to parse count %q for task %d\n",
				req.WorkerId, hostname, req.Count, req.TaskId)
		} else {
			s.countMutex.Lock()
			s.totalCount.Add(s.totalCount, val)
			s.countMutex.Unlock()
			fmt.Printf("Worker %s@%s completed task %d: count=%s  avg=%.1fs  next_timeout=%ds\n",
				req.WorkerId, hostname, req.TaskId, val.Text(10),
				s.timeoutTracker.AvgTaskSec(),
				s.timeoutTracker.CurrentTimeoutSec())
		}
		atomic.AddInt32(&s.completedTasks, 1)
	}

	if s.taskManager.IsEmpty() {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}

	return &pb.Empty{}, nil
}

// ReportStatus receives a live CPU/RAM heartbeat from a worker.
func (s *MasterServer) ReportStatus(ctx context.Context, req *pb.WorkerStatus) (*pb.Empty, error) {
	hostname, _ := s.taskManager.HostnameForTask(req.TaskId)
	s.workerTracker.UpdateLiveStats(req.WorkerId, hostname, req.CpuUsage, req.MemoryMb, req.MemoryPct)
	return &pb.Empty{}, nil
}

// GetMasterStats is called by the TUI every 500 ms.
func (s *MasterServer) GetMasterStats(ctx context.Context, _ *pb.Empty) (*pb.MasterStatsResponse, error) {
	uptime := time.Since(s.startTime).Seconds()

	s.countMutex.Lock()
	totalCount := s.totalCount.Text(10)
	s.countMutex.Unlock()

	avgSec := s.timeoutTracker.AvgTaskSec()
	currentTimeout := s.timeoutTracker.CurrentTimeoutSec()

	// Convert the sentinel "no timeout" back to 0 for the TUI
	currentTimeoutF := float64(currentTimeout)
	if currentTimeout == math.MaxInt32 {
		currentTimeoutF = 0
	}

	return &pb.MasterStatsResponse{
		ActiveTasks:       s.taskManager.ActiveCount(),
		CompletedTasks:    atomic.LoadInt32(&s.completedTasks),
		QueuedTasks:       s.taskManager.QueuedCount(),
		TotalCount:        totalCount,
		UptimeSec:         uptime,
		TotalWorkers:      s.workerTracker.TotalWorkerCount(),
		BusyWorkers:       s.workerTracker.BusyWorkerCount(),
		Workers:           s.workerTracker.GetAllWorkerStats(),
		AvgTaskSec:        avgSec,
		CurrentTimeoutSec: currentTimeoutF,
		DynamicTimeout:    s.cfg.DynamicTimeout,
	}, nil
}

// Start listens on port and blocks until all tasks are done.
func (s *MasterServer) Start(port string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterSolverServiceServer(grpcServer, s)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	fmt.Printf("gRPC server listening on :%s\n", port)

	<-s.done

	wallTime := time.Since(s.startTime).Seconds()
	fmt.Println("All tasks complete — holding for 3s so TUI captures final stats…")

	// Write result log
	if s.resultLogger != nil {
		s.countMutex.Lock()
		count := s.totalCount.Text(10)
		s.countMutex.Unlock()

		s.resultLogger.LogResult(
			s.cnfFile,
			count,
			atomic.LoadInt32(&s.completedTasks),
			atomic.LoadInt32(&s.totalTasks),
			wallTime,
			s.timeoutTracker.AvgTaskSec(),
			s.timeoutTracker.CurrentTimeoutSec(),
			s.workerTracker.TotalWorkerCount(),
		)
	}

	time.Sleep(3 * time.Second)
	grpcServer.GracefulStop()
	return nil
}

// SubscribeTimeoutUpdates streams timeout updates to a worker for as long
// as the worker holds the stream open (i.e. while it has a task running).
func (s *MasterServer) SubscribeTimeoutUpdates(req *pb.WorkerIdentity, stream pb.SolverService_SubscribeTimeoutUpdatesServer) error {
	ch := s.broadcaster.Subscribe(req.WorkerId)
	defer s.broadcaster.Unsubscribe(req.WorkerId)

	// Send the current timeout immediately so the worker is in sync
	current := s.timeoutTracker.CurrentTimeoutSec()
	if err := stream.Send(&pb.TimeoutUpdate{TimeoutSec: current}); err != nil {
		return err
	}

	for {
		select {
		case update, ok := <-ch:
			if !ok {
				return nil // broadcaster closed (master shutting down)
			}
			if err := stream.Send(update); err != nil {
				return err // worker disconnected
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}
