package profile

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/postfinance/chaosmonkey/pkg/timespans"
	"go.yaml.in/yaml/v4"
)

// KillMode defines how a pod is terminated.
type KillMode string

const (
	KillModeEvict       KillMode = "evict"
	KillModeDelete      KillMode = "delete"
	KillModeForceDelete KillMode = "force-delete"
)

// KillProfile controls when and which pods can be killed.
type KillProfile struct {
	MinAge        time.Duration
	MaxAge        time.Duration
	KillMode      KillMode
	ExcludedTimes []timespans.ClockTimeRange
	ExcludedDates []time.Time
	ExcludedDays  []time.Weekday
}

// duration is a custom type for YAML unmarshaling that supports d and y suffixes.
type duration time.Duration

func (d *duration) UnmarshalYAML(node *yaml.Node) error {
	parsed, err := parseDuration(node.Value)
	if err != nil {
		return err
	}
	*d = duration(parsed)
	return nil
}

// parseDuration parses a duration string with support for d (day=24h) and y (year=8760h) suffixes
// on top of Go's time.ParseDuration.
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	// Check for our custom suffixes
	last := s[len(s)-1]
	switch last {
	case 'd':
		s = s[:len(s)-1] + "h"
		val, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return val * 24, nil
	case 'y':
		s = s[:len(s)-1] + "h"
		val, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return val * 8760, nil
	default:
		return time.ParseDuration(s)
	}
}

// yamlProfile is the internal representation for YAML unmarshaling.
type yamlProfile struct {
	MinAge        duration `yaml:"minAge"`
	MaxAge        duration `yaml:"maxAge"`
	KillMode      string   `yaml:"killMode"`
	ExcludedTimes []string `yaml:"excludedTimes"`
	ExcludedDates []string `yaml:"excludedDates"`
	ExcludedDays  []string `yaml:"excludedDays"`
}

// Load reads a YAML file and returns a map of profile name to KillProfile.
func Load(path string) (map[string]*KillProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profiles: %w", err)
	}

	var raw map[string]yamlProfile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing profiles YAML: %w", err)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("profiles file contains no profiles")
	}

	profiles := make(map[string]*KillProfile, len(raw))
	for name, yp := range raw {
		p, err := convertProfile(name, yp)
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", name, err)
		}
		profiles[name] = p
	}

	return profiles, nil
}

func convertProfile(name string, yp yamlProfile) (*KillProfile, error) {
	maxAge := time.Duration(yp.MaxAge)
	if maxAge <= 0 {
		return nil, fmt.Errorf("maxAge is required and must be positive")
	}

	minAge := time.Duration(yp.MinAge)
	if minAge < 0 {
		return nil, fmt.Errorf("minAge must not be negative")
	}

	if maxAge < minAge {
		return nil, fmt.Errorf("maxAge (%s) must be >= minAge (%s)", maxAge, minAge)
	}

	times, err := parseTimeRanges(yp.ExcludedTimes)
	if err != nil {
		return nil, err
	}
	dates, err := parseDates(yp.ExcludedDates)
	if err != nil {
		return nil, err
	}
	days, err := parseDays(yp.ExcludedDays)
	if err != nil {
		return nil, err
	}

	km, err := parseKillMode(yp.KillMode)
	if err != nil {
		return nil, err
	}

	return &KillProfile{
		MinAge:        minAge,
		MaxAge:        maxAge,
		KillMode:      km,
		ExcludedTimes: times,
		ExcludedDates: dates,
		ExcludedDays:  days,
	}, nil
}

func parseTimeRanges(ss []string) ([]timespans.ClockTimeRange, error) {
	if len(ss) == 0 {
		return nil, nil
	}
	ranges := make([]timespans.ClockTimeRange, 0, len(ss))
	for _, s := range ss {
		r, err := timespans.ParseClockTimeRange(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, r)
	}
	return ranges, nil
}

func parseKillMode(s string) (KillMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "evict":
		return KillModeEvict, nil
	case "delete":
		return KillModeDelete, nil
	case "force-delete":
		return KillModeForceDelete, nil
	default:
		return "", fmt.Errorf("invalid killMode %q: expected evict, delete, or force-delete", s)
	}
}

func parseDates(ss []string) ([]time.Time, error) {
	if len(ss) == 0 {
		return nil, nil
	}
	dates := make([]time.Time, 0, len(ss))
	for _, s := range ss {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("invalid date %q: %w", s, err)
		}
		dates = append(dates, t)
	}
	return dates, nil
}

var dayMap = map[string]time.Weekday{
	"mon": time.Monday,
	"tue": time.Tuesday,
	"wed": time.Wednesday,
	"thu": time.Thursday,
	"fri": time.Friday,
	"sat": time.Saturday,
	"sun": time.Sunday,
}

func parseDays(ss []string) ([]time.Weekday, error) {
	if len(ss) == 0 {
		return nil, nil
	}
	days := make([]time.Weekday, 0, len(ss))
	for _, s := range ss {
		day, ok := dayMap[strings.TrimSpace(strings.ToLower(s))]
		if !ok {
			return nil, fmt.Errorf("invalid day abbreviation %q: expected mon,tue,wed,thu,fri,sat,sun", s)
		}
		days = append(days, day)
	}
	return days, nil
}

// applyExclusions cuts all configured exclusions from the given TimeSpans.
func (p *KillProfile) applyExclusions(ts timespans.TimeSpans, loc *time.Location) timespans.TimeSpans {
	for _, d := range p.ExcludedDays {
		ts = ts.Cut(timespans.CutWeekday(d))
	}
	for _, d := range p.ExcludedDates {
		y, m, dd := d.Date()
		ts = ts.Cut(timespans.CutDate(y, m, dd, loc))
	}
	for _, tr := range p.ExcludedTimes {
		ts = ts.Cut(timespans.CutClockTimeRange(tr))
	}
	return ts
}

// KillTime computes a deterministic kill time for a pod based on its UID and creation time.
// The location is used for evaluating time-based exclusions (excluded times, days, dates).
// A zero time with nil error means no eligible kill time remains after exclusions.
func (p *KillProfile) KillTime(podUID string, creationTime time.Time, loc *time.Location) (time.Time, error) {
	windowStart := creationTime.In(loc).Add(p.MinAge)
	windowEnd := creationTime.In(loc).Add(p.MaxAge)

	span, err := timespans.NewTimeSpan(windowStart, windowEnd)
	if err != nil {
		return time.Time{}, fmt.Errorf("building kill time window: %w", err)
	}

	eligible, err := timespans.NewTimeSpans(span)
	if err != nil {
		return time.Time{}, fmt.Errorf("building eligible kill spans: %w", err)
	}

	eligible = p.applyExclusions(eligible, loc)

	totalEligible := eligible.Duration()
	if totalEligible <= 0 {
		return time.Time{}, nil
	}

	h := fnv.New64a()
	h.Write([]byte(podUID))
	seed := int64(h.Sum64())

	rng := rand.New(rand.NewSource(seed))
	offset := time.Duration(rng.Int63n(int64(totalEligible)))

	return eligible.Add(offset), nil
}
