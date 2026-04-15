package main

import (
	"math"
	"sync"
)

// TimeoutTracker maintains the running average of completed task durations
// and derives the current timeout from it.
//
// Rules:
//   - Before any task completes: use InitialTimeoutSec.
//     If InitialTimeoutSec == 0 the returned timeout is math.MaxInt32
//     (i.e. effectively no timeout) until we have real data.
//   - After at least one task completes: timeout = avgDuration * Multiplier,
//     rounded up to the nearest second, minimum 1 s.
type TimeoutTracker struct {
	mu          sync.RWMutex
	cfg         *HeuristicConfig
	totalSec    float64
	sampleCount int64
}

func NewTimeoutTracker(cfg *HeuristicConfig) *TimeoutTracker {
	return &TimeoutTracker{cfg: cfg}
}

// RecordCompletion registers the wall-clock duration of one finished task.
// Only call this for tasks that completed normally (not timeouts).
func (tt *TimeoutTracker) RecordCompletion(durationSec float64) {
	if durationSec <= 0 {
		return
	}
	tt.mu.Lock()
	tt.totalSec += durationSec
	tt.sampleCount++
	tt.mu.Unlock()
}

// CurrentTimeoutSec returns the timeout (in seconds as int32) to embed in
// the next TaskPayload.
//
//   - dynamic disabled  → static InitialTimeoutSec (0 ⇒ no timeout sentinel)
//   - dynamic enabled, no samples yet → InitialTimeoutSec (0 ⇒ no timeout)
//   - dynamic enabled, ≥1 sample      → ceil(avg * multiplier), min 1
func (tt *TimeoutTracker) CurrentTimeoutSec() int32 {
	if !tt.cfg.DynamicTimeout {
		return staticTimeout(tt.cfg.InitialTimeoutSec)
	}

	tt.mu.RLock()
	count := tt.sampleCount
	total := tt.totalSec
	tt.mu.RUnlock()

	if count == 0 {
		return staticTimeout(tt.cfg.InitialTimeoutSec)
	}

	avg := total / float64(count)
	t := math.Ceil(avg * tt.cfg.TimeoutMultiplier)
	if t < 1 {
		t = 1
	}
	return int32(t)
}

// AvgTaskSec returns the running average task duration in seconds (0 if none).
func (tt *TimeoutTracker) AvgTaskSec() float64 {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	if tt.sampleCount == 0 {
		return 0
	}
	return tt.totalSec / float64(tt.sampleCount)
}

// SampleCount returns how many completed (non-timeout) tasks have been recorded.
func (tt *TimeoutTracker) SampleCount() int64 {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	return tt.sampleCount
}

// staticTimeout converts the float seconds from config to int32.
// 0 → math.MaxInt32 (no-timeout sentinel understood by the worker).
func staticTimeout(sec int32) int32 {
	if sec <= 0 {
		return math.MaxInt32
	}
	return sec
}
