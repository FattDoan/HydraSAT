package main

import (
	"sync"
	"sync/atomic"

	pb "HydraSAT/proto"
)

// taskEntry is what the checkbook remembers about an in-flight task.
// Storing hostname here lets SubmitResult and ReportStatus resolve the full
// composite worker key (hostname + workerID) even though those RPCs only
// carry workerID in the proto.
type taskEntry struct {
	cube     []int32
	hostname string
}

type TaskManager struct {
	taskQueue   chan *pb.TaskPayload
	checkbook   map[int64]taskEntry // taskID -> (cube, hostname)
	mu          sync.Mutex
	nextID      int64
	activeTasks int32
	cnfData     *CNFData
}

func NewTaskManager(cnfData *CNFData) *TaskManager {
	return &TaskManager{
		taskQueue: make(chan *pb.TaskPayload, 10000),
		checkbook: make(map[int64]taskEntry),
		cnfData:   cnfData,
	}
}

// TaskQueue exposes the read side of the channel so the server can select on it.
func (tm *TaskManager) TaskQueue() <-chan *pb.TaskPayload {
	return tm.taskQueue
}

// EnqueueCubes builds a TaskPayload for every cube and sends it to the queue.
func (tm *TaskManager) EnqueueCubes(cubes [][]int32) {
	for _, cube := range cubes {
		tm.taskQueue <- tm.makePayload(cube)
	}
}

// RecordAssignment stores the hostname for a task that has just been handed to
// a worker. Called by the server after AssignTask so that SubmitResult and
// ReportStatus (which only carry workerID, not hostname) can resolve the full
// composite key.
func (tm *TaskManager) RecordAssignment(taskID int64, hostname string) {
	tm.mu.Lock()
	if e, ok := tm.checkbook[taskID]; ok {
		e.hostname = hostname
		tm.checkbook[taskID] = e
	}
	tm.mu.Unlock()
}

// GetAndRemoveTask removes a task from the checkbook and returns its cube and
// the hostname of the worker it was assigned to.
// Returns ("", nil, false) if the task ID is unknown.
func (tm *TaskManager) GetAndRemoveTask(taskID int64) (string, []int32, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	entry, exists := tm.checkbook[taskID]
	if exists {
		delete(tm.checkbook, taskID)
		atomic.AddInt32(&tm.activeTasks, -1)
	}
	return entry.hostname, entry.cube, exists
}

// HostnameForTask returns the hostname stored for an in-flight task without
// removing it. Used by ReportStatus to resolve the composite worker key.
func (tm *TaskManager) HostnameForTask(taskID int64) (string, bool) {
	tm.mu.Lock()
	entry, ok := tm.checkbook[taskID]
	tm.mu.Unlock()
	return entry.hostname, ok
}

// IsEmpty reports whether all tasks have been completed.
func (tm *TaskManager) IsEmpty() bool {
	return atomic.LoadInt32(&tm.activeTasks) == 0 && len(tm.taskQueue) == 0
}

// ActiveCount returns the number of tasks currently assigned to workers.
func (tm *TaskManager) ActiveCount() int32 {
	return atomic.LoadInt32(&tm.activeTasks)
}

// QueuedCount returns the number of tasks waiting in the queue.
func (tm *TaskManager) QueuedCount() int32 {
	return int32(len(tm.taskQueue))
}

// makePayload assigns an ID to a cube, registers it in the checkbook,
// and wraps it in the protobuf TaskPayload the worker expects.
// hostname is left empty here and filled in by RecordAssignment once the
// server knows which worker received the task.
func (tm *TaskManager) makePayload(cube []int32) *pb.TaskPayload {
	id := atomic.AddInt64(&tm.nextID, 1)

	tm.mu.Lock()
	tm.checkbook[id] = taskEntry{cube: cube}
	tm.mu.Unlock()

	atomic.AddInt32(&tm.activeTasks, 1)

	return &pb.TaskPayload{
		TaskId:      id,
		NumVars:     tm.cnfData.NumVars,
		NumClauses:  tm.cnfData.NumClauses,
		FormulaBody: tm.cnfData.Body,
		Literals:    cube,
		TimeoutSec:  30,
	}
}

// SplitCube takes a cube and produces two child cubes by branching on the next
// variable after the current maximum. This is the simplest possible splitting
// heuristic; replace with something smarter once cube generation is wired in.
func SplitCube(cube []int32) [][]int32 {
	var maxVar int32
	for _, lit := range cube {
		if lit < 0 {
			lit = -lit
		}
		if lit > maxVar {
			maxVar = lit
		}
	}
	next := maxVar + 1
	return [][]int32{
		append(append([]int32{}, cube...), next),
		append(append([]int32{}, cube...), -next),
	}
}
