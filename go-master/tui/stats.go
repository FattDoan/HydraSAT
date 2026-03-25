package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	pb "HydraSAT/proto"
)

// WorkerInfo is the TUI-local representation of a single worker's state.
type WorkerInfo struct {
	ID             string
	Hostname       string
	Status         string
	CurrentTaskID  int64
	CurrentCube    []int32
	TasksCompleted int32
	TasksFailed    int32
	CPUUsage       float64
	MemoryUsageMB  float64
	MemoryUsagePct float64
	LastSeen       time.Time
	TaskElapsedSec float64
	TaskTimeout    float64
}

// SystemStats is a snapshot of the master's global state for one render frame.
type SystemStats struct {
	Workers        map[string]*WorkerInfo
	ActiveTasks    int32
	CompletedTasks int32
	QueuedTasks    int32
	TotalCount     string
	Uptime         float64
	TotalWorkers   int32
	BusyWorkers    int32
}

type statsMsg SystemStats
type errMsg error

// StatsCollector polls the master via gRPC and stores the latest snapshot.
type StatsCollector struct {
	client     pb.SolverServiceClient
	stats      SystemStats
	mu         sync.RWMutex
	errorCount int
}

func NewStatsCollector(client pb.SolverServiceClient) *StatsCollector {
	return &StatsCollector{
		client: client,
		stats:  SystemStats{Workers: make(map[string]*WorkerInfo)},
	}
}

// FetchStats returns a Bubble Tea command that calls GetMasterStats once.
func (sc *StatsCollector) FetchStats() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := sc.client.GetMasterStats(ctx, &pb.Empty{})
		if err != nil {
			sc.mu.Lock()
			sc.errorCount++
			count := sc.errorCount
			sc.mu.Unlock()

			// Surface last-known stats for a couple of cycles before giving up.
			if count < 3 {
				sc.mu.RLock()
				last := sc.stats
				sc.mu.RUnlock()
				return statsMsg(last)
			}
			return errMsg(fmt.Errorf("master disconnected (finished or crashed): %w", err))
		}

		sc.mu.Lock()
		sc.errorCount = 0
		sc.mu.Unlock()

		workers := make(map[string]*WorkerInfo, len(resp.Workers))
		for _, ws := range resp.Workers {
			// Use the same composite key as the master tracker so that two
			// workers on different hosts with the same workerID string don't
			// overwrite each other in this map.
			key := ws.Hostname + "\x00" + ws.WorkerId
			workers[key] = &WorkerInfo{
				ID:             ws.WorkerId,
				Hostname:       ws.Hostname,
				Status:         ws.Status,
				CurrentTaskID:  ws.CurrentTaskId,
				CurrentCube:    ws.CurrentCube,
				TasksCompleted: ws.TasksCompleted,
				TasksFailed:    ws.TasksFailed,
				CPUUsage:       ws.CpuUsage,
				MemoryUsageMB:  ws.MemoryUsageMb,
				MemoryUsagePct: ws.MemoryUsagePct,
				LastSeen:       time.Unix(ws.LastSeenUnix, 0),
				TaskElapsedSec: ws.TaskElapsedSec,
				TaskTimeout:    ws.TaskTimeout,
			}
		}

		return statsMsg(SystemStats{
			Workers:        workers,
			ActiveTasks:    resp.ActiveTasks,
			CompletedTasks: resp.CompletedTasks,
			QueuedTasks:    resp.QueuedTasks,
			TotalCount:     resp.TotalCount,
			Uptime:         resp.UptimeSec,
			TotalWorkers:   resp.TotalWorkers,
			BusyWorkers:    resp.BusyWorkers,
		})
	}
}

func (sc *StatsCollector) Update(msg statsMsg) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.stats = SystemStats(msg)
}

func (sc *StatsCollector) GetStats() SystemStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.stats
}
