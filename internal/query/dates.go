package query

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

// Relative dates. A date value of a condition is an absolute date (2026-12-01) or a phrase that is
// read when the view runs, so "90 days ago" always means ninety days before today:
//
//	today, yesterday, tomorrow
//	90 days ago, 2 weeks ago, 3 months ago, 1 year ago
//	in 30 days, in 2 weeks
//	-90d, +30d, -2w, +3m, -1y
//
// Any other phrase goes to the same date parser as "by <date>" (next friday). The stored query keeps
// the phrase, never the date it resolved to.

var (
	agoRe   = regexp.MustCompile(`^(\d+)\s*(day|days|d|week|weeks|w|month|months|m|mon|mons|year|years|y)\s+ago$`)
	inRe    = regexp.MustCompile(`^in\s+(\d+)\s*(day|days|d|week|weeks|w|month|months|m|mon|mons|year|years|y)$`)
	shortRe = regexp.MustCompile(`^([+-])(\d+)(d|w|m|y)$`)
	isoRe   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	daysRe  = regexp.MustCompile(`^(\d+)\s*(d|day|days|w|week|weeks)?$`)
)

func addUnit(t time.Time, n int, unit string) time.Time {
	switch unit[0] {
	case 'w':
		return t.AddDate(0, 0, 7*n)
	case 'm':
		return t.AddDate(0, n, 0)
	case 'y':
		return t.AddDate(n, 0, 0)
	}
	return t.AddDate(0, 0, n)
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ParseDate resolves a date value against now. It returns the day as a UTC midnight.
func ParseDate(raw string, now time.Time) (time.Time, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	today := dayStart(now)
	switch s {
	case "today":
		return today, nil
	case "yesterday":
		return today.AddDate(0, 0, -1), nil
	case "tomorrow":
		return today.AddDate(0, 0, 1), nil
	}
	if isoRe.MatchString(s) {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("%q is not a calendar date", raw)
		}
		return t, nil
	}
	if m := agoRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return addUnit(today, -n, m[2]), nil
	}
	if m := inRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return addUnit(today, n, m[2]), nil
	}
	if m := shortRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[2])
		if m[1] == "-" {
			n = -n
		}
		return addUnit(today, n, m[3]), nil
	}
	res := shortsyntax.Parse("by "+s, shortsyntax.Options{Now: now})
	if res.Deadline != nil && res.Deadline.Date != "" && res.Title == "" {
		if t, err := time.Parse("2006-01-02", res.Deadline.Date); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q is not a date (use 2026-12-01, 90 days ago, in 30 days, -90d, today or next friday)", raw)
}

// ParseDays reads a count of days: 30, 30d or 4w.
func ParseDays(raw string) (int, error) {
	m := daysRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(raw)))
	if m == nil {
		return 0, fmt.Errorf("%q is not a number of days (use 30, 30d or 4w)", raw)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("%q is not a number of days", raw)
	}
	if strings.HasPrefix(m[2], "w") {
		n *= 7
	}
	if n < 0 || n > MaxDays {
		return 0, fmt.Errorf("%q is out of range, use 0 to %d days", raw, MaxDays)
	}
	return n, nil
}

func iso(t time.Time) string { return t.Format("2006-01-02") }

// monthDays lists month*100+day for each day from a to b inclusive, so a yearless birthday matches.
func monthDays(a, b time.Time) []int {
	var out []int
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		out = append(out, int(d.Month())*100+d.Day())
	}
	return out
}
