package timespans

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrInvalidTimeSpan is returned when trying to construct an invalid time range
var ErrInvalidTimeSpan = errors.New("invalid time span: to must be after from")

// TimeSpan defines a non-empty time range
type TimeSpan struct {
	from time.Time
	to   time.Time
}

func NewTimeSpan(from time.Time, to time.Time) (TimeSpan, error) {
	if !to.After(from) {
		return TimeSpan{}, ErrInvalidTimeSpan
	}

	return TimeSpan{
		from,
		to,
	}, nil
}

func (tr TimeSpan) Equal(o TimeSpan) bool {
	return tr.from.Equal(o.from) && tr.to.Equal(o.to)
}

// Overlap returns true if the TimeRange overlaps with another
func (tr TimeSpan) Overlap(o TimeSpan) bool {
	return o.from.Before(tr.to) && o.to.After(tr.from)
}

// TimeSpans is an ordered list of non-overlapping TimeSpan values.
type TimeSpans []TimeSpan

// NewTimeSpans creates a TimeSpans from the given spans, sorted by start time.
// Returns an error if any spans overlap.
func NewTimeSpans(spans ...TimeSpan) (TimeSpans, error) {
	ts := make(TimeSpans, len(spans))
	copy(ts, spans)

	sort.Slice(ts, func(i, j int) bool {
		return ts[i].from.Before(ts[j].from)
	})

	for i := 1; i < len(ts); i++ {
		if ts[i].Overlap(ts[i-1]) {
			return nil, fmt.Errorf("overlapping spans: %v-%v and %v-%v",
				ts[i-1].from, ts[i-1].to, ts[i].from, ts[i].to)
		}
	}

	return ts, nil
}

// Duration returns the total duration across all spans.
func (ts TimeSpans) Duration() time.Duration {
	var d time.Duration
	for _, s := range ts {
		d += s.to.Sub(s.from)
	}
	return d
}

// Add adds a duration to the start of the first span, skipping gaps between spans.
// Returns the zero time if the duration exceeds the total span duration.
func (ts TimeSpans) Add(d time.Duration) time.Time {
	for _, s := range ts {
		spanDur := s.to.Sub(s.from)
		if d < spanDur {
			return s.from.Add(d)
		}
		d -= spanDur
	}
	return time.Time{}
}

// CutFn takes a TimeSpan and returns the remaining TimeSpans after cutting.
//
// It must find the FIRST matching cut point within the span. This guarantees
// that in the split case, the first returned element is clean and does not
// need to be re-evaluated by the same CutFn.
//
// Return values:
//   - []  — the cut fully covers the span, nothing remains
//   - [s] — the span was trimmed (from start/end) or left unchanged
//   - [a, b] — the span was split; a is final, b may need further cutting
type CutFn func(TimeSpan) []TimeSpan

// Cut applies a CutFn repeatedly to all spans until each is fully resolved.
func (ts TimeSpans) Cut(cut CutFn) TimeSpans {
	var result TimeSpans
	for _, cur := range ts {
		result = append(result, cutOne(cur, cut)...)
	}
	return result
}

// cutOne recursively applies a CutFn to a single span until fully resolved.
// A CutFn only handles the first matching cut point, so repeated application
// is needed to process all cut points within a span.
//
//   - Split (2 parts): the first part is final, recurse on the second.
//   - Trim (1 part, changed): the span was narrowed, recurse to find more cuts.
//   - Unchanged or empty: no more cuts apply, return as-is.
func cutOne(cur TimeSpan, cut CutFn) TimeSpans {
	parts := cut(cur)
	switch {
	case len(parts) == 2:
		return append(TimeSpans{parts[0]}, cutOne(parts[1], cut)...)
	case len(parts) == 1 && !parts[0].Equal(cur):
		return cutOne(parts[0], cut)
	default:
		return parts
	}
}
