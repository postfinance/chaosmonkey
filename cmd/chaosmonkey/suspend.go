package main

import (
	"sync"
	"time"
)

// controlStateValue is the high-level run state of the chaos monkey.
type controlStateValue int

const (
	// stateRunning: evictions happen.
	stateRunning controlStateValue = iota
	// stateWaitingForLease: DMS-owned suspend. A valid lease auto-resumes to
	// running. This is also the cold-start state when the DMS is enabled.
	stateWaitingForLease
	// stateManualResumeRequired: only a human /resume resumes. A healthy lease
	// must never clear this.
	stateManualResumeRequired
)

func (s controlStateValue) String() string {
	switch s {
	case stateRunning:
		return "RUNNING"
	case stateWaitingForLease:
		return "WAITING_FOR_LEASE"
	case stateManualResumeRequired:
		return "MANUAL_RESUME_REQUIRED"
	default:
		return "UNKNOWN"
	}
}

// Label returns a human-readable form of the state for the dashboard.
func (s controlStateValue) Label() string {
	switch s {
	case stateRunning:
		return "Running"
	case stateWaitingForLease:
		return "Waiting for Lease"
	case stateManualResumeRequired:
		return "Manual Resume Required"
	default:
		return "Unknown"
	}
}

// controlState is the suspend/resume state machine. The dead man's switch feeds
// lease observations (onLeaseValid/onLeaseExpired) and the HTTP handlers feed
// manual intent (manualSuspend/manualResume). Lease observations are
// level-triggered and idempotent: a no-op transition reports changed=false so
// callers emit nothing.
type controlState struct {
	mu         sync.Mutex
	state      controlStateValue
	autoResume bool
	since      time.Time
}

// transition moves to target, recording the time. Must hold the lock.
func (s *controlState) transition(target controlStateValue) (from, to controlStateValue, changed bool) {
	from = s.state
	if from == target {
		return from, from, false
	}
	s.state = target
	s.since = time.Now()
	return from, target, true
}

// onLeaseValid records a healthy lease observation.
func (s *controlState) onLeaseValid() (from, to controlStateValue, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Only WAITING_FOR_LEASE auto-recovers; MANUAL_RESUME_REQUIRED is never
	// cleared by the lease, and RUNNING stays put.
	if s.state == stateWaitingForLease {
		return s.transition(stateRunning)
	}
	return s.state, s.state, false
}

// onLeaseExpired records an expired/missing lease observation.
func (s *controlState) onLeaseExpired() (from, to controlStateValue, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Only a running monkey reacts to expiry. autoResume picks whether recovery
	// is automatic (WAITING) or requires a human (MANUAL).
	if s.state == stateRunning {
		if s.autoResume {
			return s.transition(stateWaitingForLease)
		}
		return s.transition(stateManualResumeRequired)
	}
	return s.state, s.state, false
}

// manualSuspend handles a /suspend request.
func (s *controlState) manualSuspend() (from, to controlStateValue, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transition(stateManualResumeRequired)
}

// manualResume handles a /resume request.
func (s *controlState) manualResume() (from, to controlStateValue, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transition(stateRunning)
}

// init sets the starting state and auto-resume mode (no event emitted).
func (s *controlState) init(state controlStateValue, autoResume bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.autoResume = autoResume
	s.since = time.Now()
}

func (s *controlState) current() controlStateValue {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *controlState) isSuspended() bool {
	return s.current() != stateRunning
}

func (s *controlState) status() (state controlStateValue, since time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.since
}
