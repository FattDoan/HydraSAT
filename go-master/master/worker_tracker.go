package main

import (
	"sync"
	"time"

	pb "HydraSAT/proto"
)

// workerState holds everything the master knows about a single worker.
type workerState struct {
	workerID       string
	hostname       string
	status         string // "IDLE" | "BUSY" | "TIMEOUT"
	currentTaskID  int64
	currentCube    []int32
	tasksCompleted int32
	tasksFailed    int32
	lastSeen       time.Time
	taskStartTime  time.Time
	taskTimeout    float64 // seconds
	cpuUsage       float64 // live, from ReportStatus
	memoryMB       float64
	memoryPct      float64
	mu             sync.RWMutex
}

// WorkerTracker is the master's in-memory view of the worker fleet.
type WorkerTracker struct {
	workers map[string]*workerState // keyed by workerKey(hostname, workerID)
	mu      sync.RWMutex
}

func NewWorkerTracker() *WorkerTracker {
	return &WorkerTracker{
		workers: make(map[string]*workerState),
	}
}

// workerKey builds a collision-free map key from the two identity fields.
// Using hostname+workerID means two workers on different machines can share
// the same workerID string (e.g. both named "worker-01") without overwriting
// each other in the map.
func workerKey(hostname, workerID string) string {
	return hostname + "\x00" + workerID
}

// RegisterWorker upserts a worker entry. Static identity only — no CPU/RAM here
// because those are only meaningful while ganak is actually running.
func (wt *WorkerTracker) RegisterWorker(workerID, hostname string) {
	key := workerKey(hostname, workerID)

	wt.mu.Lock()
	w, exists := wt.workers[key]
	if !exists {
		wt.workers[key] = &workerState{
			workerID:      workerID,
			hostname:      hostname,
			status:        "IDLE",
			currentTaskID: -1,
			lastSeen:      time.Now(),
		}
		wt.mu.Unlock()
		return
	}
	wt.mu.Unlock()

	w.mu.Lock()
	w.lastSeen = time.Now()
	w.mu.Unlock()
}

// AssignTask marks a worker as BUSY and records what it's working on.
func (wt *WorkerTracker) AssignTask(workerID, hostname string, taskID int64, cube []int32, timeoutSec float64) {
	w := wt.get(hostname, workerID)
	if w == nil {
		return
	}
	w.mu.Lock()
	w.status = "BUSY"
	w.currentTaskID = taskID
	w.currentCube = cube
	w.taskStartTime = time.Now()
	w.taskTimeout = timeoutSec
	w.lastSeen = time.Now()
	w.mu.Unlock()
}

// SetIdle marks a worker as idle (no task assigned).
func (wt *WorkerTracker) SetIdle(workerID, hostname string) {
	w := wt.get(hostname, workerID)
	if w == nil {
		return
	}
	w.mu.Lock()
	w.status = "IDLE"
	w.currentTaskID = -1
	w.currentCube = nil
	w.lastSeen = time.Now()
	w.mu.Unlock()
}

// UpdateWorkerResult is called after SubmitResult to reflect success or timeout.
func (wt *WorkerTracker) UpdateWorkerResult(workerID, hostname string, timedOut bool) {
	w := wt.get(hostname, workerID)
	if w == nil {
		return
	}
	w.mu.Lock()
	if timedOut {
		w.status = "TIMEOUT"
		w.tasksFailed++
	} else {
		w.status = "IDLE"
		w.tasksCompleted++
	}
	w.currentTaskID = -1
	w.currentCube = nil
	w.lastSeen = time.Now()
	w.mu.Unlock()
}

// UpdateLiveStats receives a heartbeat from a worker's ReportStatus call.
func (wt *WorkerTracker) UpdateLiveStats(workerID, hostname string, cpu, memMB, memPct float64) {
	w := wt.get(hostname, workerID)
	if w == nil {
		return
	}
	w.mu.Lock()
	w.cpuUsage = cpu
	w.memoryMB = memMB
	w.memoryPct = memPct
	w.lastSeen = time.Now()
	w.mu.Unlock()
}

// GetAllWorkerStats snapshots every worker for the TUI response.
func (wt *WorkerTracker) GetAllWorkerStats() []*pb.WorkerStats {
	wt.mu.RLock()
	defer wt.mu.RUnlock()

	stats := make([]*pb.WorkerStats, 0, len(wt.workers))
	for _, w := range wt.workers {
		w.mu.RLock()
		var elapsed float64
		if !w.taskStartTime.IsZero() {
			elapsed = time.Since(w.taskStartTime).Seconds()
		}
		stats = append(stats, &pb.WorkerStats{
			WorkerId:       w.workerID,
			Hostname:       w.hostname,
			Status:         w.status,
			CurrentTaskId:  w.currentTaskID,
			CurrentCube:    w.currentCube,
			TasksCompleted: w.tasksCompleted,
			TasksFailed:    w.tasksFailed,
			LastSeenUnix:   w.lastSeen.Unix(),
			TaskElapsedSec: elapsed,
			TaskTimeout:    w.taskTimeout,
			CpuUsage:       w.cpuUsage,
			MemoryUsageMb:  w.memoryMB,
			MemoryUsagePct: w.memoryPct,
		})
		w.mu.RUnlock()
	}
	return stats
}

// BusyWorkerCount returns the number of workers currently solving a task.
func (wt *WorkerTracker) BusyWorkerCount() int32 {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	var count int32
	for _, w := range wt.workers {
		w.mu.RLock()
		if w.status == "BUSY" {
			count++
		}
		w.mu.RUnlock()
	}
	return count
}

// TotalWorkerCount returns the total number of registered workers.
func (wt *WorkerTracker) TotalWorkerCount() int32 {
	wt.mu.RLock()
	defer wt.mu.RUnlock()
	return int32(len(wt.workers))
}

// get returns a worker by composite key under a read lock.
func (wt *WorkerTracker) get(hostname, workerID string) *workerState {
	wt.mu.RLock()
	w := wt.workers[workerKey(hostname, workerID)]
	wt.mu.RUnlock()
	return w
}
