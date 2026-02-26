package timespans

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidClockTimeRange is returned if the passed in string is in the wrong format
var ErrInvalidClockTimeRange = errors.New("invalid time range: must be in format hh:mm-hh:mm")

// ClockTimeRange describes a range between two times during a day.
// If From is before To, it describe the time between.
// If From is after To, it describes the time outside the range, i.e. wrapping around midnight.
// From and To can not be equal.
type ClockTimeRange struct {
	from ClockTime
	to   ClockTime
}

// TimeSpanOnDate returns the concrete TimeSpan for this range on the given day.
// For wrapping ranges (e.g. 17:00-08:00), the span starts on day and ends the next day.
func (r ClockTimeRange) TimeSpanOnDate(day time.Time) TimeSpan {
	from := r.from.OnDate(day)
	to := r.to.OnDate(day)
	if r.from.After(r.to) {
		to = r.to.OnDate(day.AddDate(0, 0, 1))
	}
	return TimeSpan{from: from, to: to}
}

// Contains reports whether the given time falls within this clock time range.
func (r ClockTimeRange) Contains(t time.Time) bool {
	ct := ClockTime{hour: t.Hour(), minute: t.Minute()}
	if r.from.Before(r.to) {
		// Normal range: from <= ct < to
		return !ct.Before(r.from) && ct.Before(r.to)
	}
	// Wrapping range (e.g. 17:00-08:00): ct >= from OR ct < to
	return !ct.Before(r.from) || ct.Before(r.to)
}

// String returns the range in "HH:MM-HH:MM" format.
func (r ClockTimeRange) String() string {
	return fmt.Sprintf("%02d:%02d-%02d:%02d", r.from.hour, r.from.minute, r.to.hour, r.to.minute)
}

func ParseClockTimeRange(s string) (ClockTimeRange, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return ClockTimeRange{}, ErrInvalidClockTimeRange
	}

	from, err := ParseClockTime(parts[0])
	if err != nil {
		return ClockTimeRange{}, fmt.Errorf("invalid range: %w", err)
	}

	to, err := ParseClockTime(parts[1])
	if err != nil {
		return ClockTimeRange{}, fmt.Errorf("invalid range: %w", err)
	}

	if from == to {
		return ClockTimeRange{}, ErrInvalidClockTimeRange
	}

	return ClockTimeRange{
		from,
		to,
	}, nil
}
