package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/postfinance/chaosmonkey/pkg/timespans"
)

func TestParseTimeRanges(t *testing.T) {
	ranges, err := parseTimeRanges([]string{"16:00-08:00", "12:00-13:00"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 2 {
		t.Fatalf("got %d ranges, want 2", len(ranges))
	}

	want0, _ := timespans.ParseClockTimeRange("16:00-08:00")
	want1, _ := timespans.ParseClockTimeRange("12:00-13:00")
	if ranges[0] != want0 {
		t.Errorf("range[0] = %v, want 16:00-08:00", ranges[0])
	}
	if ranges[1] != want1 {
		t.Errorf("range[1] = %v, want 12:00-13:00", ranges[1])
	}

	// Empty
	ranges, err = parseTimeRanges(nil)
	if err != nil {
		t.Fatal(err)
	}
	if ranges != nil {
		t.Errorf("expected nil for empty input, got %v", ranges)
	}

	// Invalid
	_, err = parseTimeRanges([]string{"invalid"})
	if err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestParseDates(t *testing.T) {
	dates, err := parseDates([]string{"2026-01-01", "2026-12-25"})
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 2 {
		t.Fatalf("got %d dates, want 2", len(dates))
	}
	if dates[0].Month() != time.January || dates[0].Day() != 1 {
		t.Errorf("date[0] = %v, want Jan 1", dates[0])
	}

	_, err = parseDates([]string{"not-a-date"})
	if err == nil {
		t.Error("expected error for invalid date")
	}
}

func TestParseDays(t *testing.T) {
	days, err := parseDays([]string{"sat", "sun"})
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("got %d days, want 2", len(days))
	}
	if days[0] != time.Saturday || days[1] != time.Sunday {
		t.Errorf("got %v, want [Saturday Sunday]", days)
	}

	_, err = parseDays([]string{"xxx"})
	if err == nil {
		t.Error("expected error for invalid day")
	}
}

func TestDurationParsing(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"1h", time.Hour},
		{"10d", 10 * 24 * time.Hour},
		{"1y", 8760 * time.Hour},
		{"30m", 30 * time.Minute},
		{"2d", 48 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if err != nil {
				t.Fatalf("parseDuration(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}

	// Invalid
	_, err := parseDuration("")
	if err == nil {
		t.Error("expected error for empty string")
	}
	_, err = parseDuration("abc")
	if err == nil {
		t.Error("expected error for invalid duration")
	}
}

func TestLoad(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		yaml := `
default:
  maxAge: 240h
aggressive:
  minAge: 1h
  maxAge: 10d
  excludedTimes:
    - "16:00-08:00"
    - "12:00-13:00"
  excludedDates:
    - "2026-01-01"
    - "2026-12-25"
  excludedDays:
    - sat
    - sun
`
		path := writeTempFile(t, yaml)
		profiles, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(profiles) != 2 {
			t.Fatalf("got %d profiles, want 2", len(profiles))
		}

		def := profiles["default"]
		if def.MaxAge != 240*time.Hour {
			t.Errorf("default.MaxAge = %v, want 240h", def.MaxAge)
		}
		if def.MinAge != 0 {
			t.Errorf("default.MinAge = %v, want 0", def.MinAge)
		}

		agg := profiles["aggressive"]
		if agg.MinAge != time.Hour {
			t.Errorf("aggressive.MinAge = %v, want 1h", agg.MinAge)
		}
		if agg.MaxAge != 10*24*time.Hour {
			t.Errorf("aggressive.MaxAge = %v, want 240h", agg.MaxAge)
		}
		if len(agg.ExcludedTimes) != 2 {
			t.Errorf("aggressive.ExcludedTimes = %d, want 2", len(agg.ExcludedTimes))
		}
		if len(agg.ExcludedDates) != 2 {
			t.Errorf("aggressive.ExcludedDates = %d, want 2", len(agg.ExcludedDates))
		}
		if len(agg.ExcludedDays) != 2 {
			t.Errorf("aggressive.ExcludedDays = %d, want 2", len(agg.ExcludedDays))
		}
	})

	t.Run("empty file", func(t *testing.T) {
		path := writeTempFile(t, "{}")
		_, err := Load(path)
		if err == nil {
			t.Error("expected error for empty profiles")
		}
	})

	t.Run("missing maxAge", func(t *testing.T) {
		path := writeTempFile(t, "bad:\n  minAge: 1h\n")
		_, err := Load(path)
		if err == nil {
			t.Error("expected error for missing maxAge")
		}
	})

	t.Run("maxAge < minAge", func(t *testing.T) {
		path := writeTempFile(t, "bad:\n  minAge: 10h\n  maxAge: 1h\n")
		_, err := Load(path)
		if err == nil {
			t.Error("expected error for maxAge < minAge")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := Load("/nonexistent/path.yaml")
		if err == nil {
			t.Error("expected error for missing file")
		}
	})
}

func mustParseRange(s string) timespans.ClockTimeRange {
	r, err := timespans.ParseClockTimeRange(s)
	if err != nil {
		panic(err)
	}
	return r
}

func TestKillTimeOfficeHoursProfile(t *testing.T) {
	// Matches samples/profiles.yaml "default" profile.
	p := &KillProfile{
		MinAge: time.Hour,
		MaxAge: 14 * 24 * time.Hour,
		ExcludedTimes: []timespans.ClockTimeRange{
			mustParseRange("17:00-08:00"),
		},
		ExcludedDays: []time.Weekday{time.Saturday, time.Sunday},
	}

	t.Run("kill time never outside 08:00-17:00", func(t *testing.T) {
		// Use a Monday so the full window has eligible days.
		creation := time.Date(2026, 2, 23, 10, 0, 0, 0, time.UTC) // Monday
		for i := range 100 {
			uid := fmt.Sprintf("test-uid-%04d", i)
			kt, err := p.KillTime(uid, creation, time.UTC)
			if err != nil {
				t.Fatalf("uid %s: unexpected error: %v", uid, err)
			}
			if kt.IsZero() {
				t.Fatalf("uid %s: kill time should not be zero", uid)
			}
			h, m := kt.Hour(), kt.Minute()
			if h < 8 || h > 16 || (h == 17 && m > 0) {
				t.Errorf("uid %s: kill time %v is outside 08:00-17:00 (got %02d:%02d)", uid, kt, h, m)
			}
			// Stricter: must be before 17:00 (exclusive)
			if h >= 17 {
				t.Errorf("uid %s: kill time %v is at or after 17:00", uid, kt)
			}
		}
	})

	t.Run("kill time never on saturday or sunday", func(t *testing.T) {
		creation := time.Date(2026, 2, 23, 10, 0, 0, 0, time.UTC) // Monday
		for i := range 100 {
			uid := fmt.Sprintf("test-uid-%04d", i)
			kt, err := p.KillTime(uid, creation, time.UTC)
			if err != nil {
				t.Fatalf("uid %s: unexpected error: %v", uid, err)
			}
			if kt.IsZero() {
				t.Fatalf("uid %s: kill time should not be zero", uid)
			}
			wd := kt.Weekday()
			if wd == time.Saturday || wd == time.Sunday {
				t.Errorf("uid %s: kill time %v is on %v", uid, kt, wd)
			}
		}
	})
}

func TestKillTime(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		p := &KillProfile{
			MaxAge: 240 * time.Hour,
		}
		creation := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)
		uid := "test-pod-uid-12345"

		kt1, err := p.KillTime(uid, creation, time.UTC)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		kt2, err := p.KillTime(uid, creation, time.UTC)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if kt1.IsZero() {
			t.Fatal("kill time should not be zero")
		}
		if !kt1.Equal(kt2) {
			t.Errorf("kill time not deterministic: %v != %v", kt1, kt2)
		}
	})

	t.Run("different UIDs get different times", func(t *testing.T) {
		p := &KillProfile{
			MaxAge: 240 * time.Hour,
		}
		creation := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)

		kt1, err := p.KillTime("uid-a", creation, time.UTC)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		kt2, err := p.KillTime("uid-b", creation, time.UTC)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if kt1.Equal(kt2) {
			t.Error("different UIDs should (usually) produce different kill times")
		}
	})

	t.Run("within window", func(t *testing.T) {
		p := &KillProfile{
			MinAge: time.Hour,
			MaxAge: 48 * time.Hour,
		}
		creation := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)
		windowStart := creation.Add(time.Hour)
		windowEnd := creation.Add(48 * time.Hour)

		for i := range 100 {
			uid := fmt.Sprintf("pod-%d", i)
			kt, err := p.KillTime(uid, creation, time.UTC)
			if err != nil {
				t.Fatalf("uid %s: unexpected error: %v", uid, err)
			}
			if kt.IsZero() {
				t.Fatalf("uid %s: kill time should not be zero", uid)
			}
			if kt.Before(windowStart) || !kt.Before(windowEnd) {
				t.Errorf("uid %s: kill time %v outside window [%v, %v)", uid, kt, windowStart, windowEnd)
			}
		}
	})

	t.Run("respects excluded days", func(t *testing.T) {
		// Create a profile that excludes all days except Wednesday.
		p := &KillProfile{
			MaxAge:       7 * 24 * time.Hour,
			ExcludedDays: []time.Weekday{time.Monday, time.Tuesday, time.Thursday, time.Friday, time.Saturday, time.Sunday},
		}
		// Wednesday 2026-02-25
		creation := time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC)

		for i := range 50 {
			uid := fmt.Sprintf("pod-%d", i)
			kt, err := p.KillTime(uid, creation, time.UTC)
			if err != nil {
				t.Fatalf("uid %s: unexpected error: %v", uid, err)
			}
			if kt.IsZero() {
				t.Fatalf("uid %s: kill time should not be zero", uid)
			}
			if kt.Weekday() != time.Wednesday {
				t.Errorf("uid %s: kill time on %v, want Wednesday", uid, kt.Weekday())
			}
		}
	})

	t.Run("respects excluded times", func(t *testing.T) {
		// Only allow 08:00-12:00 by excluding 12:00-08:00 (crossing midnight).
		p := &KillProfile{
			MaxAge: 7 * 24 * time.Hour,
			ExcludedTimes: []timespans.ClockTimeRange{
				mustParseRange("12:00-08:00"),
			},
		}
		creation := time.Date(2026, 2, 25, 0, 0, 0, 0, time.UTC)

		for i := range 50 {
			uid := fmt.Sprintf("pod-%d", i)
			kt, err := p.KillTime(uid, creation, time.UTC)
			if err != nil {
				t.Fatalf("uid %s: unexpected error: %v", uid, err)
			}
			if kt.IsZero() {
				t.Fatalf("uid %s: kill time should not be zero", uid)
			}
			h := kt.Hour()
			if h < 8 || h >= 12 {
				t.Errorf("uid %s: kill time at %02d:%02d, want between 08:00-12:00", uid, kt.Hour(), kt.Minute())
			}
		}
	})

	t.Run("no eligible window returns zero", func(t *testing.T) {
		// Exclude all days.
		p := &KillProfile{
			MaxAge:       48 * time.Hour,
			ExcludedDays: []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday},
		}
		creation := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)
		kt, err := p.KillTime("uid", creation, time.UTC)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !kt.IsZero() {
			t.Errorf("expected zero time, got %v", kt)
		}
	})

	t.Run("invalid window returns error", func(t *testing.T) {
		p := &KillProfile{
			MinAge: 2 * time.Hour,
			MaxAge: time.Hour,
		}
		creation := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)

		kt, err := p.KillTime("uid", creation, time.UTC)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !kt.IsZero() {
			t.Fatalf("expected zero time on error, got %v", kt)
		}
	})
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
