package timespans

import (
	"math/rand/v2"
	"testing"
	"time"
)

func TestNewTimeSpans(t *testing.T) {
	tests := []struct {
		name    string
		spans   []TimeSpan
		wantErr bool
		wantLen int
	}{
		{
			name:    "empty",
			spans:   nil,
			wantLen: 0,
		},
		{
			name: "single span",
			spans: []TimeSpan{
				{from: date(2026, 3, 1), to: date(2026, 3, 2)},
			},
			wantLen: 1,
		},
		{
			name: "sorted non-overlapping",
			spans: []TimeSpan{
				{from: date(2026, 3, 1), to: date(2026, 3, 2)},
				{from: date(2026, 3, 3), to: date(2026, 3, 4)},
			},
			wantLen: 2,
		},
		{
			name: "unsorted non-overlapping gets sorted",
			spans: []TimeSpan{
				{from: date(2026, 3, 5), to: date(2026, 3, 6)},
				{from: date(2026, 3, 1), to: date(2026, 3, 2)},
			},
			wantLen: 2,
		},
		{
			name: "adjacent spans are ok",
			spans: []TimeSpan{
				{from: date(2026, 3, 1), to: date(2026, 3, 2)},
				{from: date(2026, 3, 2), to: date(2026, 3, 3)},
			},
			wantLen: 2,
		},
		{
			name: "overlapping spans",
			spans: []TimeSpan{
				{from: date(2026, 3, 1), to: date(2026, 3, 3)},
				{from: date(2026, 3, 2), to: date(2026, 3, 4)},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTimeSpans(tt.spans...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("got %d spans, want %d", len(got), tt.wantLen)
			}
			// verify sorted
			for i := 1; i < len(got); i++ {
				if got[i].from.Before(got[i-1].from) {
					t.Errorf("spans not sorted: [%d].from=%v before [%d].from=%v",
						i, got[i].from, i-1, got[i-1].from)
				}
			}
		})
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name string
		ts   TimeSpans
		want time.Duration
	}{
		{
			name: "empty",
			ts:   nil,
			want: 0,
		},
		{
			name: "single span one day",
			ts: TimeSpans{
				{from: date(2026, 3, 1), to: date(2026, 3, 2)},
			},
			want: 24 * time.Hour,
		},
		{
			name: "multiple spans",
			ts: TimeSpans{
				{from: dt(2026, 3, 1, 8, 0), to: dt(2026, 3, 1, 12, 0)},
				{from: dt(2026, 3, 1, 13, 0), to: dt(2026, 3, 1, 17, 0)},
			},
			want: 8 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ts.Duration()
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	spans := TimeSpans{
		{from: dt(2026, 3, 1, 8, 0), to: dt(2026, 3, 1, 12, 0)},  // 4h
		{from: dt(2026, 3, 1, 13, 0), to: dt(2026, 3, 1, 17, 0)}, // 4h
		{from: dt(2026, 3, 2, 8, 0), to: dt(2026, 3, 2, 12, 0)},  // 4h
	}

	tests := []struct {
		name string
		d    time.Duration
		want time.Time
	}{
		{
			name: "zero duration",
			d:    0,
			want: dt(2026, 3, 1, 8, 0),
		},
		{
			name: "within first span",
			d:    2 * time.Hour,
			want: dt(2026, 3, 1, 10, 0),
		},
		{
			name: "start of second span",
			d:    4 * time.Hour,
			want: dt(2026, 3, 1, 13, 0),
		},
		{
			name: "within second span",
			d:    6 * time.Hour,
			want: dt(2026, 3, 1, 15, 0),
		},
		{
			name: "skips overnight gap to third span",
			d:    9 * time.Hour,
			want: dt(2026, 3, 2, 9, 0),
		},
		{
			name: "exceeds total duration",
			d:    13 * time.Hour,
			want: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spans.Add(tt.d)
			if !got.Equal(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddRandomNeverLandsOnCutTime(t *testing.T) {
	loc := time.UTC
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, loc)
	maxDur := 365 * 24 * time.Hour

	goodFriday := TimeSpan{
		from: time.Date(2026, time.April, 3, 0, 0, 0, 0, loc),
		to:   time.Date(2026, time.April, 4, 0, 0, 0, 0, loc),
	}
	easterMonday := TimeSpan{
		from: time.Date(2026, time.April, 6, 0, 0, 0, 0, loc),
		to:   time.Date(2026, time.April, 7, 0, 0, 0, 0, loc),
	}

	for i := range 1000 {
		// 1. Create a random span between 1 second and 1 year
		spanStart := base.Add(time.Duration(rand.Int64N(int64(maxDur))))
		spanLen := time.Second + time.Duration(rand.Int64N(int64(maxDur)-int64(time.Second)))
		span := TimeSpan{from: spanStart, to: spanStart.Add(spanLen)}

		// 2. Apply the same cuts as main
		spans := TimeSpans{span}.
			Cut(CutTimeSpan(goodFriday)).
			Cut(CutTimeSpan(easterMonday)).
			Cut(CutWeekday(time.Saturday)).
			Cut(CutWeekday(time.Sunday)).
			Cut(CutClockTimeRange(ClockTimeRange{
				from: ClockTime{hour: 17, minute: 0},
				to:   ClockTime{hour: 8, minute: 0},
			})).
			Cut(CutClockTimeRange(ClockTimeRange{
				from: ClockTime{hour: 12, minute: 0},
				to:   ClockTime{hour: 13, minute: 0},
			}))

		// 3. Get total duration
		dur := spans.Duration()
		if dur == 0 {
			continue
		}

		// 4. Pick a random point within that duration
		randDur := time.Duration(rand.Int64N(int64(dur)))

		// 5. Add it
		got := spans.Add(randDur)
		if got.IsZero() {
			t.Fatalf("iteration %d: Add(%v) returned zero time for spans with duration %v", i, randDur, dur)
		}

		// 6. Verify the result doesn't conflict with any cut
		wd := got.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			t.Errorf("iteration %d: %v lands on %s", i, got, wd)
		}

		clock := ClockTime{hour: got.Hour(), minute: got.Minute()}
		if clock.Before(ClockTime{hour: 8, minute: 0}) || !clock.Before(ClockTime{hour: 17, minute: 0}) {
			t.Errorf("iteration %d: %v is outside working hours (%02d:%02d)", i, got, clock.hour, clock.minute)
		}
		if !clock.Before(ClockTime{hour: 12, minute: 0}) && clock.Before(ClockTime{hour: 13, minute: 0}) {
			t.Errorf("iteration %d: %v is during lunch break", i, got)
		}

		if goodFriday.from.Before(got) && got.Before(goodFriday.to) || got.Equal(goodFriday.from) {
			t.Errorf("iteration %d: %v lands on Good Friday", i, got)
		}
		if easterMonday.from.Before(got) && got.Before(easterMonday.to) || got.Equal(easterMonday.from) {
			t.Errorf("iteration %d: %v lands on Easter Monday", i, got)
		}
	}
}
