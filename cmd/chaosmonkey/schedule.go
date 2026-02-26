package main

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/postfinance/chaosmonkey/pkg/profile"
)

const (
	maxUpcoming    = 100
	maxRecentKills = 20
)

// podEntry represents a pod tracked by the schedule.
// A single entry per UID exists at any time — either pending (scheduled for kill)
// or killed (recent history for display).
type podEntry struct {
	Namespace     string
	Name          string
	UID           string
	Profile       string
	KillMode      profile.KillMode
	NodeName      string
	CreationTime  time.Time
	KillTime      time.Time
	Deterministic bool
	Killed        bool
	Result        string
	KilledAt      time.Time
}

// schedule tracks pods using a single map keyed by UID.
// This guarantees no duplicates across pending and killed states.
type schedule struct {
	mu      sync.Mutex
	entries map[string]*podEntry
}

func newSchedule() *schedule {
	return &schedule{entries: make(map[string]*podEntry)}
}

// knownUIDs returns UIDs of all tracked entries (pending and killed).
func (s *schedule) knownUIDs() map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := make(map[string]struct{}, len(s.entries))
	for uid := range s.entries {
		m[uid] = struct{}{}
	}
	return m
}

// snapshot returns pending entries sorted by KillTime and killed entries
// sorted by KilledAt (newest first) for display.
func (s *schedule) snapshot() (pending []*podEntry, killed []*podEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.Killed {
			killed = append(killed, e)
		} else {
			pending = append(pending, e)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].KillTime.Before(pending[j].KillTime)
	})
	sort.Slice(killed, func(i, j int) bool {
		return killed[i].KilledAt.After(killed[j].KilledAt)
	})
	return
}

// upcomingCount returns the number of pending entries with KillTime before t.
func (s *schedule) upcomingCount(t time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	for _, e := range s.entries {
		if !e.Killed && e.KillTime.Before(t) {
			n++
		}
	}
	return n
}

// overdue returns pending entries whose KillTime <= now.
func (s *schedule) overdue(now time.Time) []*podEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*podEntry
	for _, e := range s.entries {
		if !e.Killed && !e.KillTime.After(now) {
			result = append(result, e)
		}
	}
	return result
}

// markKilled marks an entry as killed with the given result.
func (s *schedule) markKilled(uid, result string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[uid]; ok {
		e.Killed = true
		e.Result = result
		e.KilledAt = time.Now()
	}
}

// remove deletes an entry entirely (e.g. pod 404 during kill).
func (s *schedule) remove(uid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, uid)
}

// reconcile syncs the schedule with the current cluster state.
//
// It performs these steps atomically:
//  1. Remove pending entries for pods no longer in the cluster.
//  2. Add new entries. If a killed entry exists for the same UID (pod
//     survived, e.g. dry-run), it is replaced (re-armed as pending).
//  3. During suspend: reschedule overdue pending entries into the future
//     using now as creation time, so the window starts at now+minAge.
//  4. Trim killed entries to maxRecentKills (keep newest).
//  5. Trim pending entries to maxUpcoming (keep soonest).
func (s *schedule) reconcile(liveUIDs map[string]struct{}, newEntries []*podEntry, suspended bool, profiles map[string]*profile.KillProfile, loc *time.Location) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Prune pending entries for pods that disappeared.
	for uid, e := range s.entries {
		if !e.Killed {
			if _, alive := liveUIDs[uid]; !alive {
				delete(s.entries, uid)
			}
		}
	}

	// 2. Add new entries (only for UIDs not already tracked).
	for _, e := range newEntries {
		if _, exists := s.entries[e.UID]; !exists {
			s.entries[e.UID] = e
		}
	}

	// 3. During suspend, reschedule overdue pending entries.
	//    Use now as fake creation time so window starts at now+minAge,
	//    guaranteeing the new kill time is well into the future.
	if suspended {
		now := time.Now()
		for _, e := range s.entries {
			if !e.Killed && !e.KillTime.After(now) {
				if p := profiles[e.Profile]; p != nil {
					t, err := p.KillTime(e.UID, now, loc)
					if err != nil {
						slog.Error("failed to reschedule kill time for suspended pod",
							"pod", e.Namespace+"/"+e.Name,
							"profile", e.Profile,
							"error", err,
						)
						continue
					}
					if !t.IsZero() {
						e.KillTime = t
						e.Deterministic = false
					}
				}
			}
		}
	}

	// 4. Trim killed entries to cap.
	var killedEntries []*podEntry
	for _, e := range s.entries {
		if e.Killed {
			killedEntries = append(killedEntries, e)
		}
	}
	if len(killedEntries) > maxRecentKills {
		sort.Slice(killedEntries, func(i, j int) bool {
			return killedEntries[i].KilledAt.Before(killedEntries[j].KilledAt)
		})
		for i := range len(killedEntries) - maxRecentKills {
			delete(s.entries, killedEntries[i].UID)
		}
	}

	// 5. Trim pending entries to cap (keep soonest kill times).
	var pendingEntries []*podEntry
	for _, e := range s.entries {
		if !e.Killed {
			pendingEntries = append(pendingEntries, e)
		}
	}
	if len(pendingEntries) > maxUpcoming {
		sort.Slice(pendingEntries, func(i, j int) bool {
			return pendingEntries[i].KillTime.Before(pendingEntries[j].KillTime)
		})
		for i := maxUpcoming; i < len(pendingEntries); i++ {
			delete(s.entries, pendingEntries[i].UID)
		}
	}
}
