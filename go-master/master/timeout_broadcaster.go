package main

import (
	"sync"

	pb "HydraSAT/proto"
)

// TimeoutBroadcaster keeps a set of open subscriber channels and fans out
// a new TimeoutUpdate whenever the master's dynamic timeout changes.
type TimeoutBroadcaster struct {
	mu          sync.Mutex
	subscribers map[string]chan *pb.TimeoutUpdate // workerID -> channel
}

func NewTimeoutBroadcaster() *TimeoutBroadcaster {
	return &TimeoutBroadcaster{
		subscribers: make(map[string]chan *pb.TimeoutUpdate),
	}
}

// Subscribe registers a worker and returns a channel it should read from.
// The channel is buffered so a slow worker doesn't block a broadcast.
// Change buffer size to 1
func (tb *TimeoutBroadcaster) Subscribe(workerID string) chan *pb.TimeoutUpdate {
	ch := make(chan *pb.TimeoutUpdate, 1) // Was 8, now 1
	tb.mu.Lock()
	tb.subscribers[workerID] = ch
	tb.mu.Unlock()
	return ch
}

// Unsubscribe removes a worker and closes its channel.
func (tb *TimeoutBroadcaster) Unsubscribe(workerID string) {
	tb.mu.Lock()
	if ch, ok := tb.subscribers[workerID]; ok {
		delete(tb.subscribers, workerID)
		close(ch)
	}
	tb.mu.Unlock()
}

// Broadcast pushes a new timeout to every subscribed worker.
// Non-blocking: workers that aren't reading fast enough get the latest value
// overwritten in their buffer (old update dropped, new one enqueued).
func (tb *TimeoutBroadcaster) Broadcast(timeoutSec int32) {
	update := &pb.TimeoutUpdate{TimeoutSec: timeoutSec}
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for _, ch := range tb.subscribers {
		// 1. Flush the channel using a Label
	FlushLoop:
		for {
			select {
			case <-ch:
				// Effectively discarding the old value
			default:
				// No more messages to read, exit the NAMED loop
				break FlushLoop
			}
		}

		// 2. Push the single freshest update
		// Now this code IS reachable
		select {
		case ch <- update:
		default:
			// If the channel is still full (rare with 1 slot),
			// we skip to ensure we don't block the Master
		}
	}
}
