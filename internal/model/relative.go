package model

import (
	"fmt"
	"strings"
	"time"
)

// RelativeDate says how far target is from today in calendar units, the way a person counts:
// "today", "in 6d", "in 1w 1d", "in 2w", "in 1mo", "in 1mo 1w 2d", "in 1y 2mo", and for the past
// "3d ago", "1w 2d ago". Days roll up into weeks at 7, and months are real calendar months from
// today (a month end is clamped: Jan 31 plus one month is Feb 28), so 31 days from Oct 20 is
// "in 1mo". The remainder after the months is shown as weeks, then days. Years are 12 months.
// Zero parts are left out and at most three parts show, the largest three. Only the date of each
// argument counts, the time of day is ignored.
func RelativeDate(today, target time.Time) string {
	today = dateOnly(today)
	target = dateOnly(target)
	if target.Equal(today) {
		return "today"
	}
	past := target.Before(today)
	from, to := today, target
	if past {
		from, to = target, today
	}
	// Months are counted from the earlier date, so "ago" mirrors "in" for the same distance. Today is
	// the fixed point: going forward from today, or going back from today.
	months := 0
	if !past {
		months = (to.Year()-from.Year())*12 + int(to.Month()-from.Month())
		for months > 0 && addMonths(from, months).After(to) {
			months--
		}
	} else {
		months = (today.Year()-target.Year())*12 + int(today.Month()-target.Month())
		for months > 0 && addMonths(today, -months).Before(target) {
			months--
		}
	}
	var rest int
	if !past {
		rest = int(to.Sub(addMonths(from, months)).Hours() / 24)
	} else {
		rest = int(addMonths(today, -months).Sub(target).Hours() / 24)
	}
	var parts []string
	add := func(n int, unit string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d%s", n, unit))
		}
	}
	add(months/12, "y")
	add(months%12, "mo")
	add(rest/7, "w")
	add(rest%7, "d")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	s := strings.Join(parts, " ")
	if past {
		return s + " ago"
	}
	return "in " + s
}

// RelativeDays is RelativeDate for a number of days from today (negative for the past).
func RelativeDays(today time.Time, days int) string {
	return RelativeDate(today, dateOnly(today).AddDate(0, 0, days))
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// addMonths adds n calendar months and clamps the day to the end of the month.
func addMonths(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, time.UTC)
}
