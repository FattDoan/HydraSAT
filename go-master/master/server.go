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
// The worker identifies itself; the master hands back a task payload.
func (s *MasterServer) GetTask(ctx context.Context, req *pb.WorkerIdentity) (*pb.TaskPayload, error) {
	// Register the worker (or refresh its heartbeat).
	s.workerTracker.RegisterWorker(req.WorkerId, req.Hostname)

	fmt.Printf("Worker [%s@%s] requested a task\n", req.WorkerId, req.Hostname)

	select {
	case task := <-s.taskManager.TaskQueue():
		s.workerTracker.AssignTask(req.WorkerId, task.TaskId, task.Literals, float64(task.TimeoutSec))
		fmt.Printf("Task %d assigned to worker %s: cube=%v\n", task.TaskId, req.WorkerId, task.Literals)
		return task, nil

	case <-time.After(5 * time.Second):
		// No work available right now - tell the worker to come back later.
		s.workerTracker.SetIdle(req.WorkerId)
		return &pb.TaskPayload{TaskId: -1}, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SubmitResult is called by a worker once ganak has finished (or timed out).
func (s *MasterServer) SubmitResult(ctx context.Context, req *pb.TaskResult) (*pb.Empty, error) {
	cube, exists := s.taskManager.GetAndRemoveTask(req.TaskId)
	if !exists {
		return nil, fmt.Errorf("received result for unknown task ID %d", req.TaskId)
	}

	s.workerTracker.UpdateWorkerResult(req.WorkerId, req.TimedOut)

	if req.TimedOut {
		fmt.Printf("Worker %s TIMEOUT on task %d: cube=%v — splitting\n",
			req.WorkerId, req.TaskId, cube)
		s.taskManager.EnqueueCubes(SplitCube(cube))
	} else {
		val := new(big.Int)
		if _, ok := val.SetString(req.Count, 10); !ok {
			fmt.Printf("Worker %s: failed to parse count %q for task %d\n",
				req.WorkerId, req.Count, req.TaskId)
		} else {
			s.countMutex.Lock()
			s.totalCount.Add(s.totalCount, val)
			s.countMutex.Unlock()
			fmt.Printf("Worker %s completed task %d: count=%s\n",
				req.WorkerId, req.TaskId, val.Text(10))
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

// ReportStatus receives a live CPU/RAM heartbeat from a worker while ganak runs.
func (s *MasterServer) ReportStatus(ctx context.Context, req *pb.WorkerStatus) (*pb.Empty, error) {
	s.workerTracker.UpdateLiveStats(req.WorkerId, req.CpuUsage, req.MemoryMb, req.MemoryPct)
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

// Start - Start the gRPC server
func (s *MasterServer) Start(port string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	server := grpc.NewServer()
	pb.RegisterSolverServiceServer(server, s)

	// Start server in goroutine
	go func() {
		if err := server.Serve(lis); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for completion
	<-s.done

	server.GracefulStop()
	return nil
}
