package shortsyntax

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// partial is a date, a time, or both, before the clock fills the gaps. A time with no date is
// resolved when the parse ends, so "@3pm @fri" can still become Friday at 3pm.
type partial struct {
	hasDate, hasTime bool
	rel              bool // computed from the clock (+2h, now): cannot be merged
	y, m, d          int
	hh, mm           int
}

type dateVal struct{ y, m, d int }
type clockVal struct{ h, m int }

var (
	isoRe     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	mdRe      = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})$`)
	mdyRe     = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})/(\d{4})$`)
	offsetRe  = regexp.MustCompile(`^\+?(\d{1,3})(d|w|m|y)$`)
	relTimeRe = regexp.MustCompile(`^\+?(\d{1,4})(h|hr|hrs|min|mins)$`)
	dayNumRe  = regexp.MustCompile(`^(\d{1,2})(?:st|nd|rd|th)?$`)
	clock12Re = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?(am|pm)$`)
	clock24Re = regexp.MustCompile(`^(\d{1,2}):(\d{2})$`)
	hourOnly  = regexp.MustCompile(`^(\d{1,2})(?::(\d{2}))?$`)
	yearRe    = regexp.MustCompile(`^\d{4}$`)
)

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday,
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "weds": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

var months = map[string]int{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3, "apr": 4, "april": 4,
	"may": 5, "jun": 6, "june": 6, "jul": 7, "july": 7, "aug": 8, "august": 8,
	"sep": 9, "sept": 9, "september": 9, "oct": 10, "october": 10, "nov": 11, "november": 11, "dec": 12, "december": 12,
}

// unitWord maps a unit word to d, w, m, y (date offsets) or h, n (hours, minutes).
func unitWord(s string) (byte, bool) {
	switch s {
	case "d", "day", "days":
		return 'd', true
	case "w", "wk", "wks", "week", "weeks":
		return 'w', true
	case "m", "mo", "mos", "mon", "mons", "month", "months":
		return 'm', true
	case "y", "yr", "yrs", "year", "years":
		return 'y', true
	case "h", "hr", "hrs", "hour", "hours":
		return 'h', true
	case "min", "mins", "minute", "minutes":
		return 'n', true
	}
	return 0, false
}

func dateOf(t time.Time) dateVal { return dateVal{t.Year(), int(t.Month()), t.Day()} }

func (d dateVal) time(loc *time.Location) time.Time {
	return time.Date(d.y, time.Month(d.m), d.d, 0, 0, 0, 0, loc)
}

func (d dateVal) valid() bool {
	if d.y < 1 || d.m < 1 || d.m > 12 || d.d < 1 {
		return false
	}
	t := time.Date(d.y, time.Month(d.m), d.d, 0, 0, 0, 0, time.UTC)
	return t.Day() == d.d && int(t.Month()) == d.m
}

func (d dateVal) addDays(n int, loc *time.Location) dateVal {
	return dateOf(d.time(loc).AddDate(0, 0, n))
}

// addMonths adds months and clamps the day to the end of the target month.
func (d dateVal) addMonths(n int) dateVal {
	total := d.y*12 + (d.m - 1) + n
	y, m := total/12, total%12+1
	day := d.d
	if last := time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day(); day > last {
		day = last
	}
	return dateVal{y, m, day}
}

func (d dateVal) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.y, d.m, d.d) }

func datePartial(d dateVal) partial {
	return partial{hasDate: true, y: d.y, m: d.m, d: d.d}
}

// parseWhen reads a date, a time, or a date with a time out of lower-case words. strict is for
// plain words in a sentence: it leaves out the forms that are common in ordinary text (now, 1/2,
// a bare 2w, the short names tod and tom).
func parseWhen(w []string, now time.Time, strict bool) (partial, bool) {
	if len(w) == 0 {
		return partial{}, false
	}
	if p, ok := parseRelativeTime(w, now, strict); ok {
		return p, true
	}
	if d, ok := matchDate(w, now, strict); ok {
		return datePartial(d), true
	}
	if c, ok := matchClock(w); ok {
		return partial{hasTime: true, hh: c.h, mm: c.m}, true
	}
	for k := 1; k < len(w); k++ {
		a, b := w[:k], w[k:]
		if b[0] == "at" && len(b) > 1 {
			b = b[1:]
		}
		if d, ok := matchDate(a, now, strict); ok {
			if c, ok := matchClock(b); ok {
				p := datePartial(d)
				p.hasTime, p.hh, p.mm = true, c.h, c.m
				return p, true
			}
		}
		if c, ok := matchClock(a); ok {
			if d, ok := matchDate(b, now, strict); ok {
				p := datePartial(d)
				p.hasTime, p.hh, p.mm = true, c.h, c.m
				return p, true
			}
		}
	}
	return partial{}, false
}

// parseRelativeTime reads now, +2h, in 2h, in 30 min, in 2 hours.
func parseRelativeTime(w []string, now time.Time, strict bool) (partial, bool) {
	var d time.Duration
	switch {
	case len(w) == 1 && w[0] == "now":
		if strict {
			return partial{}, false
		}
	case len(w) == 1:
		if strict && !strings.HasPrefix(w[0], "+") {
			return partial{}, false
		}
		m := relTimeRe.FindStringSubmatch(w[0])
		if m == nil {
			return partial{}, false
		}
		n, _ := strconv.Atoi(m[1])
		d = relDuration(n, m[2])
	case len(w) == 2 && w[0] == "in":
		m := relTimeRe.FindStringSubmatch(w[1])
		if m == nil {
			return partial{}, false
		}
		n, _ := strconv.Atoi(m[1])
		d = relDuration(n, m[2])
	case len(w) == 3 && w[0] == "in" && allDigits(w[1]) && len(w[1]) <= 4:
		u, ok := unitWord(w[2])
		if !ok || (u != 'h' && u != 'n') {
			return partial{}, false
		}
		n, _ := strconv.Atoi(w[1])
		if u == 'h' {
			d = time.Duration(n) * time.Hour
		} else {
			d = time.Duration(n) * time.Minute
		}
	default:
		return partial{}, false
	}
	t := now.Add(d).Truncate(time.Minute)
	return partial{hasDate: true, hasTime: true, rel: true, y: t.Year(), m: int(t.Month()), d: t.Day(), hh: t.Hour(), mm: t.Minute()}, true
}

func relDuration(n int, unit string) time.Duration {
	if unit == "h" || unit == "hr" || unit == "hrs" {
		return time.Duration(n) * time.Hour
	}
	return time.Duration(n) * time.Minute
}

func matchDate(w []string, now time.Time, strict bool) (dateVal, bool) {
	loc := now.Location()
	today := dateOf(now)
	switch len(w) {
	case 1:
		return matchDate1(w[0], now, strict)
	case 2:
		a, b := w[0], w[1]
		switch a {
		case "next", "nxt", "this", "last":
			return relativeNamed(a, b, today, loc)
		case "in":
			if c, ok := parseCompactUnit(b); ok && c.n > 0 && c.unit != 'h' {
				return offsetDate(today, c.n, c.unit, loc), true
			}
			return dateVal{}, false
		}
		if mo, ok := months[a]; ok {
			if d, ok := dayOf(b); ok {
				if dv := yearlessDate(mo, d, today); dv.y != 0 {
					return dv, true
				}
			}
		}
		if d, ok := dayOf(a); ok {
			if mo, ok := months[b]; ok {
				if dv := yearlessDate(mo, d, today); dv.y != 0 {
					return dv, true
				}
			}
		}
	case 3:
		a, b, c := w[0], w[1], w[2]
		if a == "in" && allDigits(b) && len(b) <= 3 {
			if u, ok := unitWord(c); ok && u != 'h' && u != 'n' {
				n, _ := strconv.Atoi(b)
				if n > 0 {
					return offsetDate(today, n, u, loc), true
				}
			}
			return dateVal{}, false
		}
		if mo, ok := months[a]; ok && yearRe.MatchString(c) {
			if d, ok := dayOf(b); ok {
				y, _ := strconv.Atoi(c)
				dv := dateVal{y, mo, d}
				return dv, dv.valid()
			}
		}
		if mo, ok := months[b]; ok && yearRe.MatchString(c) {
			if d, ok := dayOf(a); ok {
				y, _ := strconv.Atoi(c)
				dv := dateVal{y, mo, d}
				return dv, dv.valid()
			}
		}
	}
	return dateVal{}, false
}

func dayOf(s string) (int, bool) {
	m := dayNumRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	d, _ := strconv.Atoi(m[1])
	return d, d >= 1 && d <= 31
}

// yearlessDate is the next month/day on or after today. It skips years where the day does not exist.
func yearlessDate(mo, d int, today dateVal) dateVal {
	for y := today.y; y <= today.y+8; y++ {
		dv := dateVal{y, mo, d}
		if dv.valid() && !dateBefore(dv, today) {
			return dv
		}
	}
	return dateVal{}
}

func dateBefore(a, b dateVal) bool {
	if a.y != b.y {
		return a.y < b.y
	}
	if a.m != b.m {
		return a.m < b.m
	}
	return a.d < b.d
}

func offsetDate(today dateVal, n int, unit byte, loc *time.Location) dateVal {
	switch unit {
	case 'd':
		return today.addDays(n, loc)
	case 'w':
		return today.addDays(7*n, loc)
	case 'm':
		return today.addMonths(n)
	case 'y':
		return today.addMonths(12 * n)
	}
	return dateVal{}
}

func matchDate1(s string, now time.Time, strict bool) (dateVal, bool) {
	loc := now.Location()
	today := dateOf(now)
	if strict && (s == "tod" || s == "tom" || s == "tomorow") {
		return dateVal{}, false
	}
	switch s {
	case "today", "tod":
		return today, true
	case "tomorrow", "tom", "tmr", "tmrw", "tomorow":
		return today.addDays(1, loc), true
	case "yesterday", "yday":
		return today.addDays(-1, loc), true
	}
	if wd, ok := weekdays[s]; ok {
		ahead := (int(wd) - int(now.Weekday()) + 7) % 7
		if ahead == 0 {
			ahead = 7
		}
		return today.addDays(ahead, loc), true
	}
	if m := isoRe.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		dv := dateVal{y, mo, d}
		return dv, dv.valid()
	}
	if m := mdyRe.FindStringSubmatch(s); m != nil {
		mo, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		dv := dateVal{y, mo, d}
		return dv, dv.valid()
	}
	if m := mdRe.FindStringSubmatch(s); m != nil && !strict {
		mo, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		if mo < 1 || mo > 12 || d < 1 || d > 31 {
			return dateVal{}, false
		}
		dv := yearlessDate(mo, d, today)
		return dv, dv.y != 0
	}
	if m := offsetRe.FindStringSubmatch(s); m != nil && (!strict || strings.HasPrefix(s, "+")) {
		n, _ := strconv.Atoi(m[1])
		if n == 0 {
			return dateVal{}, false
		}
		return offsetDate(today, n, m[2][0], loc), true
	}
	return dateVal{}, false
}

// relativeNamed reads next/this/last followed by a weekday, week, month or year.
func relativeNamed(prefix, name string, today dateVal, loc *time.Location) (dateVal, bool) {
	t := today.time(loc)
	if wd, ok := weekdays[name]; ok {
		switch prefix {
		case "this":
			return today.addDays((int(wd)-int(t.Weekday())+7)%7, loc), true
		case "last":
			back := (int(t.Weekday()) - int(wd) + 7) % 7
			if back == 0 {
				back = 7
			}
			return today.addDays(-back, loc), true
		default: // next: that weekday in next week (weeks start on Monday)
			off := (int(t.Weekday()) + 6) % 7
			nextMon := today.addDays(7-off, loc)
			return nextMon.addDays((int(wd)+6)%7, loc), true
		}
	}
	if prefix != "next" && prefix != "nxt" {
		return dateVal{}, false
	}
	switch name {
	case "week":
		off := (int(t.Weekday()) + 6) % 7
		return today.addDays(7-off, loc), true
	case "month":
		first := dateVal{today.y, today.m, 1}
		return first.addMonths(1), true
	case "year":
		return dateVal{today.y + 1, 1, 1}, true
	}
	return dateVal{}, false
}

func matchClock(w []string) (clockVal, bool) {
	switch len(w) {
	case 1:
		s := w[0]
		switch s {
		case "noon":
			return clockVal{12, 0}, true
		case "midnight":
			return clockVal{0, 0}, true
		}
		if m := clock12Re.FindStringSubmatch(s); m != nil {
			return clock12(m[1], m[2], m[3])
		}
		if m := clock24Re.FindStringSubmatch(s); m != nil {
			h, _ := strconv.Atoi(m[1])
			mi, _ := strconv.Atoi(m[2])
			return clockVal{h, mi}, h <= 23 && mi <= 59
		}
	case 2:
		if w[1] == "am" || w[1] == "pm" {
			if m := hourOnly.FindStringSubmatch(w[0]); m != nil {
				return clock12(m[1], m[2], w[1])
			}
		}
	}
	return clockVal{}, false
}

func clock12(hs, ms, ap string) (clockVal, bool) {
	h, _ := strconv.Atoi(hs)
	mi := 0
	if ms != "" {
		mi, _ = strconv.Atoi(ms)
	}
	if h < 1 || h > 12 || mi > 59 {
		return clockVal{}, false
	}
	if ap == "am" && h == 12 {
		h = 0
	}
	if ap == "pm" && h != 12 {
		h += 12
	}
	return clockVal{h, mi}, true
}

// mergePartial adds a second @ or ! token to the first when they hold different parts.
func mergePartial(cur *partial, add partial) (partial, bool) {
	if cur == nil {
		return add, true
	}
	if cur.rel || add.rel {
		return partial{}, false
	}
	out := *cur
	switch {
	case add.hasDate && !add.hasTime && !cur.hasDate:
		out.hasDate, out.y, out.m, out.d = true, add.y, add.m, add.d
	case add.hasTime && !add.hasDate && !cur.hasTime:
		out.hasTime, out.hh, out.mm = true, add.hh, add.mm
	default:
		return partial{}, false
	}
	return out, true
}

// finalize turns a partial into a When. A time with no date means today, or tomorrow when that
// time has already passed.
func (p partial) finalize(now time.Time) When {
	var w When
	if p.hasTime {
		w.Time = fmt.Sprintf("%02d:%02d", p.hh, p.mm)
	}
	if p.hasDate {
		w.Date = dateVal{p.y, p.m, p.d}.String()
		return w
	}
	today := dateOf(now)
	at := time.Date(today.y, time.Month(today.m), today.d, p.hh, p.mm, 0, 0, now.Location())
	if !at.After(now) {
		today = today.addDays(1, now.Location())
	}
	w.Date = today.String()
	return w
}
