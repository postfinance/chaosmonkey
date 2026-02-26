package main

import (
	"log/slog"
	"sync"
	"time"
)

// suspendState tracks whether evictions are suspended.
type suspendState struct {
	mu        sync.Mutex
	suspended bool
	reason    string
	since     time.Time
}

func (s *suspendState) suspend(reason string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.suspended {
		return false
	}
	s.suspended = true
	s.reason = reason
	s.since = time.Now()
	slog.Warn("evictions suspended", "reason", reason)
	return true
}

func (s *suspendState) resume() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.suspended {
		return false
	}
	s.suspended = false
	slog.Info("evictions resumed")
	return true
}

func (s *suspendState) isSuspended() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.suspended
}

func (s *suspendState) status() (suspended bool, reason string, since time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.suspended, s.reason, s.since
}
