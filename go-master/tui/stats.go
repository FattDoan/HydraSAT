package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	pb "HydraSAT/proto"
)

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

type SystemStats struct {
	Workers           map[string]*WorkerInfo
	ActiveTasks       int32
	CompletedTasks    int32
	QueuedTasks       int32
	TotalCount        string
	Uptime            float64
	TotalWorkers      int32
	BusyWorkers       int32
	AvgTaskSec        float64 // running average of completed task durations
	CurrentTimeoutSec float64 // current dynamic timeout (0 = no timeout)
	DynamicTimeout    bool    // whether dynamic timeout is on
}

type statsMsg SystemStats
type errMsg error

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
				TaskTimeout:    resp.CurrentTimeoutSec,
			}
		}

		return statsMsg(SystemStats{
			Workers:           workers,
			ActiveTasks:       resp.ActiveTasks,
			CompletedTasks:    resp.CompletedTasks,
			QueuedTasks:       resp.QueuedTasks,
			TotalCount:        resp.TotalCount,
			Uptime:            resp.UptimeSec,
			TotalWorkers:      resp.TotalWorkers,
			BusyWorkers:       resp.BusyWorkers,
			AvgTaskSec:        resp.AvgTaskSec,
			CurrentTimeoutSec: resp.CurrentTimeoutSec,
			DynamicTimeout:    resp.DynamicTimeout,
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
