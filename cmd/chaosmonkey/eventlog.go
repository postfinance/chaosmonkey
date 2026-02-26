package main

import (
	"sync"
	"time"
)

type suspendEvent struct {
	Time   time.Time
	Action string // "suspended" or "resumed"
	Reason string
}

type suspendEventLog struct {
	mu     sync.Mutex
	events []suspendEvent
	cap    int
}

func newSuspendEventLog(cap int) *suspendEventLog {
	return &suspendEventLog{cap: cap}
}

func (l *suspendEventLog) add(action, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, suspendEvent{
		Time:   time.Now(),
		Action: action,
		Reason: reason,
	})
	if len(l.events) > l.cap {
		l.events = l.events[len(l.events)-l.cap:]
	}
}

// snapshot returns a copy of events, newest first.
func (l *suspendEventLog) snapshot() []suspendEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]suspendEvent, len(l.events))
	for i, e := range l.events {
		out[len(l.events)-1-i] = e
	}
	return out
}
