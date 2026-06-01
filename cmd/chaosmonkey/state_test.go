package main

import "testing"

func applyEvent(s *controlState, ev string) (changed bool) {
	switch ev {
	case "leaseValid":
		_, _, changed = s.onLeaseValid()
	case "leaseExpired":
		_, _, changed = s.onLeaseExpired()
	case "suspend":
		_, _, changed = s.manualSuspend()
	case "resume":
		_, _, changed = s.manualResume()
	default:
		panic("unknown event: " + ev)
	}
	return changed
}

func TestControlStateTransitions(t *testing.T) {
	tests := []struct {
		name       string
		autoResume bool
		initial    controlStateValue
		events     []string
		want       controlStateValue
	}{
		{
			name:    "cold start waits for lease",
			initial: stateWaitingForLease,
			want:    stateWaitingForLease,
		},
		{
			name:    "cold start enables on first valid lease (autoResume off)",
			initial: stateWaitingForLease,
			events:  []string{"leaseValid"},
			want:    stateRunning,
		},
		{
			name:    "missing lease keeps waiting, then enables when it appears",
			initial: stateWaitingForLease,
			events:  []string{"leaseExpired", "leaseExpired", "leaseValid"},
			want:    stateRunning,
		},
		{
			name:       "expiry with autoResume waits for lease",
			autoResume: true,
			initial:    stateRunning,
			events:     []string{"leaseExpired"},
			want:       stateWaitingForLease,
		},
		{
			name:    "expiry without autoResume requires manual resume",
			initial: stateRunning,
			events:  []string{"leaseExpired"},
			want:    stateManualResumeRequired,
		},
		{
			name:    "valid lease never clears manual resume required",
			initial: stateRunning,
			events:  []string{"leaseExpired", "leaseValid", "leaseValid"},
			want:    stateManualResumeRequired,
		},
		{
			name:       "valid lease never clears manual resume required (autoResume on)",
			autoResume: true,
			initial:    stateManualResumeRequired,
			events:     []string{"leaseValid"},
			want:       stateManualResumeRequired,
		},
		{
			name:       "manual suspend always wins over waiting",
			autoResume: true,
			initial:    stateWaitingForLease,
			events:     []string{"suspend"},
			want:       stateManualResumeRequired,
		},
		{
			name:    "manual resume clears manual suspend",
			initial: stateRunning,
			events:  []string{"suspend", "resume"},
			want:    stateRunning,
		},
		{
			name:    "lease wins: resume during outage re-suspends next poll (autoResume off)",
			initial: stateRunning,
			events:  []string{"leaseExpired", "resume", "leaseExpired"},
			want:    stateManualResumeRequired,
		},
		{
			name:       "lease wins: resume during outage re-suspends next poll (autoResume on)",
			autoResume: true,
			initial:    stateRunning,
			events:     []string{"leaseExpired", "resume", "leaseExpired"},
			want:       stateWaitingForLease,
		},
		{
			name:    "running re-suspends on a fresh expiry after recovery",
			initial: stateRunning,
			events:  []string{"leaseExpired", "resume", "leaseValid", "leaseExpired"},
			want:    stateManualResumeRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &controlState{}
			s.init(tt.initial, tt.autoResume)
			for _, ev := range tt.events {
				applyEvent(s, ev)
			}
			if got := s.current(); got != tt.want {
				t.Fatalf("after %v: got %s, want %s", tt.events, got, tt.want)
			}
		})
	}
}

// TestControlStateIdempotent verifies no-op observations report changed=false so
// callers don't emit spurious events/metrics on every poll.
func TestControlStateIdempotent(t *testing.T) {
	tests := []struct {
		name    string
		initial controlStateValue
		event   string
	}{
		{"running + valid lease", stateRunning, "leaseValid"},
		{"waiting + expired lease", stateWaitingForLease, "leaseExpired"},
		{"manual + expired lease", stateManualResumeRequired, "leaseExpired"},
		{"manual + valid lease", stateManualResumeRequired, "leaseValid"},
		{"manual + suspend again", stateManualResumeRequired, "suspend"},
		{"running + resume", stateRunning, "resume"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &controlState{}
			s.init(tt.initial, false)
			if changed := applyEvent(s, tt.event); changed {
				t.Fatalf("expected no-op transition for %s, but it reported changed", tt.event)
			}
			if got := s.current(); got != tt.initial {
				t.Fatalf("state moved from %s to %s on a no-op", tt.initial, got)
			}
		})
	}
}
