package timespans

import (
	"time"
)

// CutTimeSpan returns a CutFn that removes a fixed time span.
func CutTimeSpan(cut TimeSpan) CutFn {
	return func(span TimeSpan) []TimeSpan {
		if !cut.Overlap(span) {
			return []TimeSpan{span}
		}

		var result []TimeSpan
		if cut.from.After(span.from) {
			result = append(result, TimeSpan{from: span.from, to: cut.from})
		}
		if cut.to.Before(span.to) {
			result = append(result, TimeSpan{from: cut.to, to: span.to})
		}
		return result
	}
}

// CutClockTimeRange returns a CutFn that removes a daily recurring time window.
// It finds the first daily occurrence that overlaps the span and cuts it.
// The outer Cut loop handles repetition across subsequent days.
func CutClockTimeRange(r ClockTimeRange) CutFn {
	return func(span TimeSpan) []TimeSpan {
		// Start one day before to catch wrapping ranges (e.g. 17:00-08:00)
		// that started the previous evening.
		day := startOfDay(span.from).AddDate(0, 0, -1)

		for {
			cut := r.TimeSpanOnDate(day)

			if !cut.from.Before(span.to) {
				return []TimeSpan{span}
			}
			if cut.Overlap(span) {
				return CutTimeSpan(cut)(span)
			}

			day = day.AddDate(0, 0, 1)
		}
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// CutDate returns a CutFn that removes a specific date (midnight to midnight).
func CutDate(year int, month time.Month, day int, loc *time.Location) CutFn {
	from := time.Date(year, month, day, 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 1)
	return CutTimeSpan(TimeSpan{from: from, to: to})
}

// CutWeekday returns a CutFn that removes all occurrences of a given weekday
// (midnight to midnight).
func CutWeekday(day time.Weekday) CutFn {
	return func(span TimeSpan) []TimeSpan {
		d := span.from
		offset := (int(day-d.Weekday()) + 7) % 7
		dayStart := time.Date(d.Year(), d.Month(), d.Day()+offset, 0, 0, 0, 0, d.Location())
		dayEnd := dayStart.AddDate(0, 0, 1)
		return CutTimeSpan(TimeSpan{from: dayStart, to: dayEnd})(span)
	}
}
