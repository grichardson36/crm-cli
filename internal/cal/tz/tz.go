// Package tz holds the time zone rules the calendar relies on. Instants are stored in UTC and
// events keep the IANA zone of their author, because recurrence and display need the author's
// wall clock (a 09:00 weekly meeting stays at 09:00 local across daylight saving changes).
package tz

import (
	"fmt"
	"strings"
	"time"
	// Ship the IANA database in the binary so zones resolve on machines without system tzdata,
	// which is the normal case on Windows.
	_ "time/tzdata"
)

// Load resolves an IANA zone name. An empty name or "Local" is the device zone, "UTC" and "Z" are UTC.
func Load(name string) (*time.Location, error) {
	switch strings.TrimSpace(name) {
	case "", "Local", "local":
		return time.Local, nil
	case "UTC", "utc", "Z", "z":
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(strings.TrimSpace(name))
	if err != nil {
		return nil, fmt.Errorf("unknown time zone %q (use an IANA name such as America/Chicago): %w", name, err)
	}
	return loc, nil
}

// Name is the IANA name to store for a location. The device zone has no stable name, so it is
// stored as the name the runtime reports, and "UTC" when that is unavailable.
func Name(loc *time.Location) string {
	if loc == nil {
		return "UTC"
	}
	n := loc.String()
	if n == "" || n == "Local" {
		return "UTC"
	}
	return n
}

// Wall returns the wall clock of an instant in loc as a floating time: a UTC time.Time whose
// fields (year, month, day, hour, ...) read the local clock. It carries no zone of its own.
func Wall(instant time.Time, loc *time.Location) time.Time {
	l := instant.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), l.Second(), l.Nanosecond(), time.UTC)
}

// Instant converts a floating wall clock (see Wall) in loc to a UTC instant, following RFC 5545
// section 3.3.5:
//
//   - a local time that does not exist (the gap when clocks spring forward) uses the UTC offset
//     in effect before the gap, so 02:30 becomes 03:30 after the change
//   - a local time that occurs twice (when clocks fall back) is the first occurrence
func Instant(wall time.Time, loc *time.Location) time.Time {
	w := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), time.UTC)
	// Offsets around the wall time. A transition within this window changes the offset.
	_, before := w.Add(-36 * time.Hour).In(loc).Zone()
	_, after := w.Add(36 * time.Hour).In(loc).Zone()
	if before == after {
		return w.Add(-time.Duration(before) * time.Second)
	}
	valid := func(off int) (time.Time, bool) {
		inst := w.Add(-time.Duration(off) * time.Second)
		_, got := inst.In(loc).Zone()
		return inst, got == off
	}
	a, okA := valid(before)
	b, okB := valid(after)
	switch {
	case okA && okB:
		if b.Before(a) {
			return b
		}
		return a
	case okA:
		return a
	case okB:
		return b
	}
	// Gap: the offset before the transition.
	return w.Add(-time.Duration(before) * time.Second)
}

// Storage layouts. Instants are stored as RFC 3339 UTC, dates as plain dates.
const (
	InstantLayout = "2006-01-02T15:04:05Z"
	DateLayout    = "2006-01-02"
)

// FormatInstant renders an instant in the storage layout.
func FormatInstant(t time.Time) string { return t.UTC().Format(InstantLayout) }

// ParseInstant reads a stored instant. It also accepts the datetime('now') layout.
func ParseInstant(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{InstantLayout, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse instant %q", s)
}

// ParseDate reads a stored date as midnight UTC.
func ParseDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation(DateLayout, strings.TrimSpace(s), time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot parse date %q", s)
	}
	return t, nil
}
