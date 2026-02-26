package timespans

import (
	"testing"
	"time"
)

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestCutDate(t *testing.T) {
	cutMarch8 := CutDate(2026, 3, 8, time.UTC)

	tests := []struct {
		name string
		span TimeSpan
		want []TimeSpan
	}{
		{
			name: "no overlap, span is before the date",
			span: TimeSpan{from: date(2026, 3, 5), to: date(2026, 3, 7)},
			want: []TimeSpan{
				{from: date(2026, 3, 5), to: date(2026, 3, 7)},
			},
		},
		{
			name: "no overlap, span is after the date",
			span: TimeSpan{from: date(2026, 3, 9), to: date(2026, 3, 11)},
			want: []TimeSpan{
				{from: date(2026, 3, 9), to: date(2026, 3, 11)},
			},
		},
		{
			name: "complete overlap, span is exactly the date",
			span: TimeSpan{from: date(2026, 3, 8), to: date(2026, 3, 9)},
			want: []TimeSpan{},
		},
		{
			name: "cuts beginning",
			span: TimeSpan{from: date(2026, 3, 8), to: date(2026, 3, 10)},
			want: []TimeSpan{
				{from: date(2026, 3, 9), to: date(2026, 3, 10)},
			},
		},
		{
			name: "cuts end",
			span: TimeSpan{from: date(2026, 3, 6), to: date(2026, 3, 9)},
			want: []TimeSpan{
				{from: date(2026, 3, 6), to: date(2026, 3, 8)},
			},
		},
		{
			name: "split, date is in the middle",
			span: TimeSpan{from: date(2026, 3, 7), to: date(2026, 3, 10)},
			want: []TimeSpan{
				{from: date(2026, 3, 7), to: date(2026, 3, 8)},
				{from: date(2026, 3, 9), to: date(2026, 3, 10)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cutMarch8(tt.span)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d spans, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if !got[i].from.Equal(tt.want[i].from) || !got[i].to.Equal(tt.want[i].to) {
					t.Errorf("span[%d] = %v-%v, want %v-%v", i, got[i].from, got[i].to, tt.want[i].from, tt.want[i].to)
				}
			}
		})
	}
}

func ct(hour, minute int) ClockTime {
	return ClockTime{hour: hour, minute: minute}
}

func dt(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

func TestCutClockTimeRange(t *testing.T) {
	tests := []struct {
		name string
		r    ClockTimeRange
		span TimeSpan
		want []TimeSpan
	}{
		{
			name: "no overlap, span is before the range",
			r:    ClockTimeRange{from: ct(12, 0), to: ct(13, 0)},
			span: TimeSpan{from: dt(2026, 3, 8, 8, 0), to: dt(2026, 3, 8, 10, 0)},
			want: []TimeSpan{
				{from: dt(2026, 3, 8, 8, 0), to: dt(2026, 3, 8, 10, 0)},
			},
		},
		{
			name: "no overlap, span is after the range",
			r:    ClockTimeRange{from: ct(12, 0), to: ct(13, 0)},
			span: TimeSpan{from: dt(2026, 3, 8, 14, 0), to: dt(2026, 3, 8, 16, 0)},
			want: []TimeSpan{
				{from: dt(2026, 3, 8, 14, 0), to: dt(2026, 3, 8, 16, 0)},
			},
		},
		{
			name: "complete overlap",
			r:    ClockTimeRange{from: ct(12, 0), to: ct(13, 0)},
			span: TimeSpan{from: dt(2026, 3, 8, 12, 0), to: dt(2026, 3, 8, 13, 0)},
			want: []TimeSpan{},
		},
		{
			name: "cuts beginning",
			r:    ClockTimeRange{from: ct(8, 0), to: ct(10, 0)},
			span: TimeSpan{from: dt(2026, 3, 8, 9, 0), to: dt(2026, 3, 8, 14, 0)},
			want: []TimeSpan{
				{from: dt(2026, 3, 8, 10, 0), to: dt(2026, 3, 8, 14, 0)},
			},
		},
		{
			name: "cuts end",
			r:    ClockTimeRange{from: ct(14, 0), to: ct(18, 0)},
			span: TimeSpan{from: dt(2026, 3, 8, 10, 0), to: dt(2026, 3, 8, 16, 0)},
			want: []TimeSpan{
				{from: dt(2026, 3, 8, 10, 0), to: dt(2026, 3, 8, 14, 0)},
			},
		},
		{
			name: "split, range is in the middle",
			r:    ClockTimeRange{from: ct(12, 0), to: ct(13, 0)},
			span: TimeSpan{from: dt(2026, 3, 8, 8, 0), to: dt(2026, 3, 8, 18, 0)},
			want: []TimeSpan{
				{from: dt(2026, 3, 8, 8, 0), to: dt(2026, 3, 8, 12, 0)},
				{from: dt(2026, 3, 8, 13, 0), to: dt(2026, 3, 8, 18, 0)},
			},
		},
		{
			name: "wrapping range cuts morning",
			r:    ClockTimeRange{from: ct(22, 0), to: ct(6, 0)},
			span: TimeSpan{from: dt(2026, 3, 8, 3, 0), to: dt(2026, 3, 8, 10, 0)},
			want: []TimeSpan{
				{from: dt(2026, 3, 8, 6, 0), to: dt(2026, 3, 8, 10, 0)},
			},
		},
		{
			name: "wrapping range cuts evening",
			r:    ClockTimeRange{from: ct(22, 0), to: ct(6, 0)},
			span: TimeSpan{from: dt(2026, 3, 8, 18, 0), to: dt(2026, 3, 9, 3, 0)},
			want: []TimeSpan{
				{from: dt(2026, 3, 8, 18, 0), to: dt(2026, 3, 8, 22, 0)},
			},
		},
		{
			name: "wrapping range splits overnight span",
			r:    ClockTimeRange{from: ct(22, 0), to: ct(6, 0)},
			span: TimeSpan{from: dt(2026, 3, 8, 18, 0), to: dt(2026, 3, 9, 10, 0)},
			want: []TimeSpan{
				{from: dt(2026, 3, 8, 18, 0), to: dt(2026, 3, 8, 22, 0)},
				{from: dt(2026, 3, 9, 6, 0), to: dt(2026, 3, 9, 10, 0)},
			},
		},
		{
			name: "multi-day span finds first occurrence",
			r:    ClockTimeRange{from: ct(12, 0), to: ct(13, 0)},
			span: TimeSpan{from: dt(2026, 3, 7, 8, 0), to: dt(2026, 3, 9, 18, 0)},
			want: []TimeSpan{
				{from: dt(2026, 3, 7, 8, 0), to: dt(2026, 3, 7, 12, 0)},
				{from: dt(2026, 3, 7, 13, 0), to: dt(2026, 3, 9, 18, 0)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cut := CutClockTimeRange(tt.r)
			got := cut(tt.span)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d spans, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if !got[i].from.Equal(tt.want[i].from) || !got[i].to.Equal(tt.want[i].to) {
					t.Errorf("span[%d] = %v-%v, want %v-%v", i, got[i].from, got[i].to, tt.want[i].from, tt.want[i].to)
				}
			}
		})
	}
}

func TestCutWeekday(t *testing.T) {
	cutSunday := CutWeekday(time.Sunday)

	tests := []struct {
		name string
		span TimeSpan
		want []TimeSpan
	}{
		{
			name: "no overlap, span is entirely on weekdays",
			// Mon 2026-03-02 to Wed 2026-03-04
			span: TimeSpan{from: date(2026, 3, 2), to: date(2026, 3, 4)},
			want: []TimeSpan{
				{from: date(2026, 3, 2), to: date(2026, 3, 4)},
			},
		},
		{
			name: "complete overlap, span is exactly Sunday",
			// Sun 2026-03-08
			span: TimeSpan{from: date(2026, 3, 8), to: date(2026, 3, 9)},
			want: []TimeSpan{},
		},
		{
			name: "cuts beginning, span starts on Sunday",
			// Sun 2026-03-08 to Tue 2026-03-10
			span: TimeSpan{from: date(2026, 3, 8), to: date(2026, 3, 10)},
			want: []TimeSpan{
				{from: date(2026, 3, 9), to: date(2026, 3, 10)},
			},
		},
		{
			name: "cuts end, span ends on Sunday",
			// Fri 2026-03-06 to Sun 2026-03-08 end of day
			span: TimeSpan{from: date(2026, 3, 6), to: date(2026, 3, 9)},
			want: []TimeSpan{
				{from: date(2026, 3, 6), to: date(2026, 3, 8)},
			},
		},
		{
			name: "split, Sunday is in the middle",
			// Sat 2026-03-07 to Tue 2026-03-10
			span: TimeSpan{from: date(2026, 3, 7), to: date(2026, 3, 10)},
			want: []TimeSpan{
				{from: date(2026, 3, 7), to: date(2026, 3, 8)},
				{from: date(2026, 3, 9), to: date(2026, 3, 10)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cutSunday(tt.span)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d spans, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if !got[i].from.Equal(tt.want[i].from) || !got[i].to.Equal(tt.want[i].to) {
					t.Errorf("span[%d] = %v-%v, want %v-%v", i, got[i].from, got[i].to, tt.want[i].from, tt.want[i].to)
				}
			}
		})
	}
}

func TestParseClockTimeRange(t *testing.T) {
	r, err := ParseClockTimeRange("08:00-17:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.String() != "08:00-17:00" {
		t.Errorf("got %s, want 08:00-17:00", r.String())
	}

	_, err = ParseClockTimeRange("12:00-12:00")
	if err == nil {
		t.Fatal("expected error for from == to, got nil")
	}

	_, err = ParseClockTimeRange("invalid")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}
