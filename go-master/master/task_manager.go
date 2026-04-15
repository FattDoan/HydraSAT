package main

import (
	"sync"
	"sync/atomic"

	pb "HydraSAT/proto"
)

// taskEntry is what the checkbook remembers about an in-flight task.
type taskEntry struct {
	cube     []int32
	hostname string
}

type TaskManager struct {
	taskQueue      chan *pb.TaskPayload
	checkbook      map[int64]taskEntry // taskID -> (cube, hostname)
	mu             sync.Mutex
	nextID         int64
	activeTasks    int32
	cnfData        *CNFData
	timeoutTracker *TimeoutTracker
}

func NewTaskManager(cnfData *CNFData, tt *TimeoutTracker) *TaskManager {
	return &TaskManager{
		taskQueue:      make(chan *pb.TaskPayload, 10000),
		checkbook:      make(map[int64]taskEntry),
		cnfData:        cnfData,
		timeoutTracker: tt,
	}
}

func (tm *TaskManager) TaskQueue() <-chan *pb.TaskPayload {
	return tm.taskQueue
}

func (tm *TaskManager) EnqueueCubes(cubes [][]int32) {
	for _, cube := range cubes {
		tm.taskQueue <- tm.makePayload(cube)
	}
}

func (tm *TaskManager) RecordAssignment(taskID int64, hostname string) {
	tm.mu.Lock()
	if e, ok := tm.checkbook[taskID]; ok {
		e.hostname = hostname
		tm.checkbook[taskID] = e
	}
	tm.mu.Unlock()
}

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

func (tm *TaskManager) HostnameForTask(taskID int64) (string, bool) {
	tm.mu.Lock()
	entry, ok := tm.checkbook[taskID]
	tm.mu.Unlock()
	return entry.hostname, ok
}

func (tm *TaskManager) IsEmpty() bool {
	return atomic.LoadInt32(&tm.activeTasks) == 0 && len(tm.taskQueue) == 0
}

func (tm *TaskManager) ActiveCount() int32 {
	return atomic.LoadInt32(&tm.activeTasks)
}

func (tm *TaskManager) QueuedCount() int32 {
	return int32(len(tm.taskQueue))
}

// makePayload assigns an ID to a cube and wraps it in a TaskPayload.
// The timeout is fetched from TimeoutTracker so it always reflects the
// latest dynamic value.
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
		//TimeoutSec:  tm.timeoutTracker.CurrentTimeoutSec(),
	}
}

// SplitCube branches on the variable after the largest in the cube.
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

// More effective but simpler branching heuristic
// split on variables most appearing in the original formula, ignoring those already in the cube.
func SplitCubeSmart(cube []int32, clauses [][]int32) [][]int32 {
	var freq [1000000]int
	for _, clause := range clauses {
		for _, lit := range clause {
			var v int32
			if lit < 0 {
				v = -lit
			} else {
				v = lit
			}
			freq[v]++
		}
	}

	var maxVar int32
	for _, lit := range cube {
		if lit < 0 {
			lit = -lit
		}
		if lit > maxVar {
			maxVar = lit
		}
	}

	var bestVar int32
	var bestFreq int
	for v, f := range freq {
		if int32(v) > maxVar && f > bestFreq {
			bestVar = int32(v)
			bestFreq = f
		}
	}

	return [][]int32{
		append(append([]int32{}, cube...), bestVar),
		append(append([]int32{}, cube...), -bestVar),
	}
}
