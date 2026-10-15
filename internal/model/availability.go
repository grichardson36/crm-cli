package model

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// WeekdayKeys are the day keys of weekly working hours, Monday first.
var WeekdayKeys = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// TimeRange is a span of the wall clock within one day, "09:00" to "17:00". End may be "24:00".
type TimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

var clockRE = regexp.MustCompile(`^([01]\d|2[0-4]):([0-5]\d)$`)

// ClockMinutes reads "HH:MM" as minutes since midnight.
func ClockMinutes(s string) (int, error) {
	m := clockRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("time %q must be HH:MM, such as 09:00: %w", s, ErrValidation)
	}
	var h, mi int
	_, _ = fmt.Sscanf(m[1], "%d", &h)
	_, _ = fmt.Sscanf(m[2], "%d", &mi)
	if h == 24 && mi != 0 {
		return 0, fmt.Errorf("time %q is after 24:00: %w", s, ErrValidation)
	}
	return h*60 + mi, nil
}

// CleanRanges validates ranges (start before end, no overlap) and returns them sorted.
func CleanRanges(in []TimeRange) ([]TimeRange, error) {
	out := make([]TimeRange, 0, len(in))
	for _, r := range in {
		a, err := ClockMinutes(r.Start)
		if err != nil {
			return nil, err
		}
		b, err := ClockMinutes(r.End)
		if err != nil {
			return nil, err
		}
		if a >= b {
			return nil, fmt.Errorf("range %s-%s must end after it starts: %w", r.Start, r.End, ErrValidation)
		}
		out = append(out, TimeRange{Start: fmt.Sprintf("%02d:%02d", a/60, a%60), End: fmt.Sprintf("%02d:%02d", b/60, b%60)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	for i := 1; i < len(out); i++ {
		if out[i].Start < out[i-1].End {
			return nil, fmt.Errorf("ranges %s-%s and %s-%s overlap: %w", out[i-1].Start, out[i-1].End, out[i].Start, out[i].End, ErrValidation)
		}
	}
	return out, nil
}

// WorkingHours is a named weekly schedule in a time zone, with date overrides.
type WorkingHours struct {
	Name string `json:"name"`
	UUID string `json:"uuid,omitempty"`
	TZ   string `json:"tz"`
	// Weekly maps mon..sun to ranges. A missing day has no hours.
	Weekly map[string][]TimeRange `json:"weekly"`
	// Overrides maps a date (2026-12-24) to its own ranges. An empty list is a day off.
	Overrides map[string][]TimeRange `json:"overrides,omitempty"`
	// Default is true when nothing is stored and these are the built-in hours.
	Default bool `json:"default,omitempty"`
}

// ScheduleConfig holds the scheduling defaults.
type ScheduleConfig struct {
	DefaultRule string `json:"default_rule"`
	BufferMin   int    `json:"buffer_min"`
	StepMin     int    `json:"step_min"`
	HorizonDays int    `json:"horizon_days"`
}

// DefaultScheduleConfig is Monday to Friday 09:00-17:00, 10 minute buffers, 15 minute steps and a
// 14 day horizon.
func DefaultScheduleConfig() ScheduleConfig {
	return ScheduleConfig{DefaultRule: "work", BufferMin: 10, StepMin: 15, HorizonDays: 14}
}

// ParseDays reads a day list such as "mon", "mon,wed", "mon-fri", "weekdays", "weekend" or "all".
func ParseDays(s string) ([]string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "all", "every", "daily":
		return append([]string{}, WeekdayKeys...), nil
	case "weekdays", "weekday":
		return append([]string{}, WeekdayKeys[:5]...), nil
	case "weekend", "weekends":
		return []string{"sat", "sun"}, nil
	}
	idx := func(d string) int {
		d = strings.TrimSpace(d)
		for i, k := range WeekdayKeys {
			if d == k || (len(d) > 3 && strings.HasPrefix(d, k)) {
				return i
			}
		}
		return -1
	}
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if a, b, ok := strings.Cut(part, "-"); ok {
			i, j := idx(a), idx(b)
			if i < 0 || j < 0 || i > j {
				return nil, fmt.Errorf("cannot read days %q (try mon-fri, weekdays, mon,wed): %w", s, ErrValidation)
			}
			for k := i; k <= j; k++ {
				if !seen[WeekdayKeys[k]] {
					seen[WeekdayKeys[k]] = true
					out = append(out, WeekdayKeys[k])
				}
			}
			continue
		}
		i := idx(part)
		if i < 0 {
			return nil, fmt.Errorf("cannot read days %q (try mon-fri, weekdays, mon,wed): %w", s, ErrValidation)
		}
		if !seen[WeekdayKeys[i]] {
			seen[WeekdayKeys[i]] = true
			out = append(out, WeekdayKeys[i])
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no days given: %w", ErrValidation)
	}
	return out, nil
}

// ParseRanges reads "09:00-12:00,13:00-17:00". "off" (or an empty string) means no hours.
func ParseRanges(s string) ([]TimeRange, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "off" || s == "none" || s == "closed" {
		return []TimeRange{}, nil
	}
	var out []TimeRange
	for _, part := range strings.Split(s, ",") {
		a, b, ok := strings.Cut(strings.TrimSpace(part), "-")
		if !ok {
			return nil, fmt.Errorf("range %q must look like 09:00-17:00: %w", part, ErrValidation)
		}
		out = append(out, TimeRange{Start: strings.TrimSpace(a), End: strings.TrimSpace(b)})
	}
	return CleanRanges(out)
}
