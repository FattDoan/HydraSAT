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
	// Build the set of variable indices already committed in this cube.
	inCube := make(map[int32]struct{}, len(cube))
	for _, lit := range cube {
		v := lit
		if v < 0 {
			v = -v
		}
		inCube[v] = struct{}{}
	}
 
	// Tally how often each variable appears across all clauses.
	freq := make(map[int32]int)
	for _, clause := range clauses {
		for _, lit := range clause {
			v := lit
			if v < 0 {
				v = -v
			}
			if v == 0 {
				continue // skip any stray terminator
			}
			freq[v]++
		}
	}
 
	// Pick the most frequent variable that is NOT already in the cube.
	var bestVar int32
	var bestFreq int
	for v, f := range freq {
		if _, alreadyIn := inCube[v]; alreadyIn {
			continue
		}
		// Break ties by preferring the higher-numbered variable so the
		// choice is deterministic regardless of map iteration order.
		if f > bestFreq || (f == bestFreq && v > bestVar) {
			bestVar = v
			bestFreq = f
		}
	}
 
	// Fallback: all formula variables are already in the cube (shouldn't
	// happen in practice, but guard against it so we never append 0).
	if bestVar == 0 {
		var maxVar int32
		for _, lit := range cube {
			if lit < 0 {
				lit = -lit
			}
			if lit > maxVar {
				maxVar = lit
			}
		}
		bestVar = maxVar + 1
	}
 
	return [][]int32{
		append(append([]int32{}, cube...), bestVar),
		append(append([]int32{}, cube...), -bestVar),
	}
}
 
