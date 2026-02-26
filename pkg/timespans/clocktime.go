package timespans

import (
	"errors"
	"time"
)

// ErrInvalidClockTime is returned when the string given is in the wrong format
var ErrInvalidClockTime = errors.New("invalid time: must be in 'hh:mm' format (24h)")

// ClockTime describe a time during the day
type ClockTime struct {
	hour   int
	minute int
}

func (c ClockTime) After(o ClockTime) bool {
	return c.hour > o.hour || (c.hour == o.hour && c.minute > o.minute)
}

func (c ClockTime) Before(o ClockTime) bool {
	return c.hour < o.hour || (c.hour == o.hour && c.minute < o.minute)
}

func (c ClockTime) OnDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), c.hour, c.minute, 0, 0, t.Location())
}

func ParseClockTime(s string) (ClockTime, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return ClockTime{}, ErrInvalidClockTime
	}

	return ClockTime{
		hour:   t.Hour(),
		minute: t.Minute(),
	}, nil
}
