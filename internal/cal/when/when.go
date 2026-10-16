// Package when reads the date, time and length phrases people type for events: "fri 12:30 1h",
// "tomorrow 3pm", "2026-10-20 14:00", "oct 20". It is deterministic: the same text and the same
// "now" always give the same result. It is not the full short syntax parser (section K of the
// feature plan), it covers what the calendar commands and the quick add prompt need.
package when

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Result is a parsed phrase.
type Result struct {
	// Start is the UTC instant of a timed start, or midnight UTC of the date for an all-day start.
	Start time.Time
	// End is set when the text carried a length ("1h") or a range. Zero otherwise.
	End time.Time
	// AllDay is true when the text had a date and no time.
	AllDay bool
	// HasTime says the text carried a time of day.
	HasTime bool
	// Length is the parsed duration, zero when none.
	Length time.Duration
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday, "mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday, "wed": time.Wednesday, "weds": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday, "sat": time.Saturday, "saturday": time.Saturday,
}

var months = map[string]time.Month{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3, "apr": 4, "april": 4, "may": 5,
	"jun": 6, "june": 6, "jul": 7, "july": 7, "aug": 8, "august": 8, "sep": 9, "sept": 9, "september": 9,
	"oct": 10, "october": 10, "nov": 11, "november": 11, "dec": 12, "december": 12,
}

func bad(s, why string) error {
	return fmt.Errorf("cannot read %q as a date or time (%s): %w", s, why, model.ErrValidation)
}

// Parse reads a phrase relative to now in loc (the zone of the wall clock the phrase is about).
func Parse(text string, now time.Time, loc *time.Location) (Result, error) {
	if loc == nil {
		loc = time.UTC
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Result{}, bad(text, "empty")
	}
	// A full instant is taken as is.
	if t, err := time.Parse(time.RFC3339, text); err == nil {
		return Result{Start: t.UTC(), HasTime: true}, nil
	}

	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)

	var (
		date      time.Time
		haveDate  bool
		clock     time.Duration
		haveClock bool
		length    time.Duration
		next      bool
		byWeekday bool
	)
	toks := strings.Fields(strings.ToLower(strings.ReplaceAll(text, ",", " ")))
	for i := 0; i < len(toks); i++ {
		tk := toks[i]
		switch {
		case tk == "at" || tk == "on" || tk == "for" || tk == "the":
			continue
		case tk == "next":
			next = true
		case tk == "today":
			date, haveDate = today, true
		case tk == "tomorrow" || tk == "tmrw":
			date, haveDate = today.AddDate(0, 0, 1), true
		case tk == "yesterday":
			date, haveDate = today.AddDate(0, 0, -1), true
		default:
			if wd, ok := weekdays[tk]; ok {
				date, haveDate, byWeekday = nextWeekday(today, wd, next), true, true
				continue
			}
			if m, ok := months[tk]; ok {
				// "oct 20" or "20 oct"
				day := 0
				if i+1 < len(toks) {
					if d, err := strconv.Atoi(strings.TrimRight(toks[i+1], "stndrh")); err == nil {
						day = d
						i++
					}
				}
				if day == 0 {
					return Result{}, bad(tk, "a month needs a day, such as \"oct 20\"")
				}
				date, haveDate = monthDay(today, m, day), true
				continue
			}
			if d, err := time.Parse("2006-01-02", tk); err == nil {
				date, haveDate = d, true
				continue
			}
			if c, ok := parseClock(tk); ok {
				clock, haveClock = c, true
				continue
			}
			if d, ok := parseLength(tk); ok {
				length = d
				continue
			}
			// "20 oct"
			if n, err := strconv.Atoi(tk); err == nil && n >= 1 && n <= 31 && i+1 < len(toks) {
				if m, ok := months[toks[i+1]]; ok {
					date, haveDate = monthDay(today, m, n), true
					i++
					continue
				}
			}
			return Result{}, bad(tk, "try 2026-10-20, fri, tomorrow, 12:30, 3pm, 1h")
		}
	}

	if !haveDate && !haveClock {
		return Result{}, bad(text, "no date or time found")
	}
	if !haveDate {
		// A bare time means the next time the clock reads that.
		date = today
		if wallInstant(date, clock, loc).Before(now) {
			date = today.AddDate(0, 0, 1)
		}
	}
	if byWeekday && haveClock && date.Equal(today) && wallInstant(date, clock, loc).Before(now) {
		// "thu 6am" on a Thursday afternoon means next Thursday.
		date = date.AddDate(0, 0, 7)
	}
	if !haveClock {
		if length != 0 && length%(24*time.Hour) != 0 {
			return Result{}, bad(text, "a length in hours needs a time of day")
		}
		res := Result{Start: date, AllDay: true, Length: length}
		if length != 0 {
			res.End = date.Add(length)
		}
		return res, nil
	}
	res := Result{Start: wallInstant(date, clock, loc), HasTime: true, Length: length}
	if length != 0 {
		res.End = res.Start.Add(length)
	}
	return res, nil
}

func wallInstant(date time.Time, clock time.Duration, loc *time.Location) time.Time {
	return tz.Instant(date.Add(clock), loc)
}

func nextWeekday(today time.Time, wd time.Weekday, strictlyAfter bool) time.Time {
	ahead := (int(wd) - int(today.Weekday()) + 7) % 7
	if strictlyAfter && ahead == 0 {
		ahead = 7
	}
	return today.AddDate(0, 0, ahead)
}

// monthDay picks the next date with that month and day, this year or the next.
func monthDay(today time.Time, m time.Month, day int) time.Time {
	d := time.Date(today.Year(), m, day, 0, 0, 0, 0, time.UTC)
	if d.Before(today) {
		d = time.Date(today.Year()+1, m, day, 0, 0, 0, 0, time.UTC)
	}
	return d
}

// parseClock reads 12:30, 9:05, 3pm, 3:15pm, noon and midnight as a time since midnight.
func parseClock(s string) (time.Duration, bool) {
	switch s {
	case "noon":
		return 12 * time.Hour, true
	case "midnight":
		return 0, true
	}
	suffix := ""
	switch {
	case strings.HasSuffix(s, "am"):
		suffix, s = "am", strings.TrimSuffix(s, "am")
	case strings.HasSuffix(s, "pm"):
		suffix, s = "pm", strings.TrimSuffix(s, "pm")
	}
	hs, ms, hasMin := strings.Cut(s, ":")
	h, err := strconv.Atoi(hs)
	if err != nil || (!hasMin && suffix == "") {
		return 0, false
	}
	m := 0
	if hasMin {
		if len(ms) != 2 {
			return 0, false
		}
		if m, err = strconv.Atoi(ms); err != nil {
			return 0, false
		}
	}
	if m < 0 || m > 59 {
		return 0, false
	}
	switch suffix {
	case "am", "pm":
		if h < 1 || h > 12 {
			return 0, false
		}
		if h == 12 {
			h = 0
		}
		if suffix == "pm" {
			h += 12
		}
	default:
		if h < 0 || h > 23 {
			return 0, false
		}
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, true
}

// parseLength reads 30m, 1h, 1h30m, 90m, 2d, 1w as a duration.
func parseLength(s string) (time.Duration, bool) {
	if s == "" {
		return 0, false
	}
	var total time.Duration
	rest := s
	for rest != "" {
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		if i == 0 || i == len(rest) {
			return 0, false
		}
		n, err := strconv.Atoi(rest[:i])
		if err != nil {
			return 0, false
		}
		switch rest[i] {
		case 'h':
			total += time.Duration(n) * time.Hour
		case 'm':
			total += time.Duration(n) * time.Minute
		case 'd':
			total += time.Duration(n) * 24 * time.Hour
		case 'w':
			total += time.Duration(n) * 7 * 24 * time.Hour
		default:
			return 0, false
		}
		rest = rest[i+1:]
	}
	return total, total > 0
}

// ParseLength reads a duration such as 30m, 1h, 1h30m, 2d. A bare number is minutes.
func ParseLength(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return time.Duration(n) * time.Minute, nil
	}
	d, ok := parseLength(s)
	if !ok {
		return 0, bad(s, "a length like 30m, 1h, 1h30m or 2d")
	}
	return d, nil
}

// ParseReminder reads "15m", "1h", "1d" or a number of minutes as minutes before the start.
func ParseReminder(s string) (int, error) {
	d, err := ParseLength(s)
	if err != nil {
		return 0, err
	}
	return int(d / time.Minute), nil
}
