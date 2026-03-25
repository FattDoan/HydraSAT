package main

import (
	"context"
	"fmt"
	"log"
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
	totalCount     *big.Int
	countMutex     sync.Mutex
	done           chan struct{}
	startTime      time.Time
	completedTasks int32
}

func NewMasterServer(cnfData *CNFData) *MasterServer {
	return &MasterServer{
		taskManager:   NewTaskManager(cnfData),
		workerTracker: NewWorkerTracker(),
		totalCount:    big.NewInt(0),
		done:          make(chan struct{}),
		startTime:     time.Now(),
	}
}

func (s *MasterServer) LoadInitialTasks(cubes [][]int32) {
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
		// Store hostname in the checkbook so SubmitResult/ReportStatus can
		// resolve the composite worker key without a proto change.
		s.taskManager.RecordAssignment(task.TaskId, req.Hostname)
		s.workerTracker.AssignTask(req.WorkerId, req.Hostname, task.TaskId, task.Literals, float64(task.TimeoutSec))
		fmt.Printf("Task %d assigned to worker %s@%s: cube=%v\n",
			task.TaskId, req.WorkerId, req.Hostname, task.Literals)
		return task, nil

	case <-time.After(5 * time.Second):
		s.workerTracker.SetIdle(req.WorkerId, req.Hostname)
		return &pb.TaskPayload{TaskId: -1}, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SubmitResult is called by a worker once ganak has finished (or timed out).
// The count is committed to totalCount BEFORE we check IsEmpty, so the TUI
// will always see the fully-accumulated total when it polls during the grace period.
func (s *MasterServer) SubmitResult(ctx context.Context, req *pb.TaskResult) (*pb.Empty, error) {
	hostname, cube, exists := s.taskManager.GetAndRemoveTask(req.TaskId)
	if !exists {
		return nil, fmt.Errorf("received result for unknown task ID %d", req.TaskId)
	}

	s.workerTracker.UpdateWorkerResult(req.WorkerId, hostname, req.TimedOut)

	if req.TimedOut {
		fmt.Printf("Worker %s@%s TIMEOUT on task %d: cube=%v — splitting\n",
			req.WorkerId, hostname, req.TaskId, cube)
		s.taskManager.EnqueueCubes(SplitCube(cube))
	} else {
		val := new(big.Int)
		if _, ok := val.SetString(req.Count, 10); !ok {
			fmt.Printf("Worker %s@%s: failed to parse count %q for task %d\n",
				req.WorkerId, hostname, req.Count, req.TaskId)
		} else {
			s.countMutex.Lock()
			s.totalCount.Add(s.totalCount, val)
			s.countMutex.Unlock()
			fmt.Printf("Worker %s@%s completed task %d: count=%s (running total: %s)\n",
				req.WorkerId, hostname, req.TaskId, val.Text(10), s.totalCount.Text(10))
		}
		atomic.AddInt32(&s.completedTasks, 1)
	}

	// Non-blocking send: only the first IsEmpty() winner fires done.
	if s.taskManager.IsEmpty() {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}

	return &pb.Empty{}, nil
}

// ReportStatus receives a live CPU/RAM heartbeat from a worker while ganak runs.
func (s *MasterServer) ReportStatus(ctx context.Context, req *pb.WorkerStatus) (*pb.Empty, error) {
	// Resolve hostname from the task the worker is currently working on.
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

	return &pb.MasterStatsResponse{
		ActiveTasks:    s.taskManager.ActiveCount(),
		CompletedTasks: atomic.LoadInt32(&s.completedTasks),
		QueuedTasks:    s.taskManager.QueuedCount(),
		TotalCount:     totalCount,
		UptimeSec:      uptime,
		TotalWorkers:   s.workerTracker.TotalWorkerCount(),
		BusyWorkers:    s.workerTracker.BusyWorkerCount(),
		Workers:        s.workerTracker.GetAllWorkerStats(),
	}, nil
}

// Start listens on port and blocks until all tasks are done.
//
// After the last result is committed we wait 3 seconds before calling
// GracefulStop. This gives the TUI (polling every 500 ms) several more chances
// to call GetMasterStats and capture the final accumulated count and the full
// worker list before the connection drops.
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

	fmt.Println("All tasks complete — holding for 3s so TUI captures final stats…")
	time.Sleep(3 * time.Second)

	grpcServer.GracefulStop()
	return nil
}
