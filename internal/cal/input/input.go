// Package input turns the text a person or an agent supplies for an event (start phrases,
// lengths, repeats, reminders, windows) into the typed values the repository takes. The CLI, the
// MCP tools and the TUI share it, so the same text means the same thing on every surface.
package input

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/recur"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/model"
)

func invalid(format string, a ...any) error {
	return fmt.Errorf(format+": %w", append(a, model.ErrValidation)...)
}

func prefixed(flag string, err error) error {
	return fmt.Errorf("%s: %w", flag, err)
}

// EventLocation is the zone of an event's own wall clock.
func EventLocation(e *model.Event) *time.Location {
	if e.AllDay || e.TZ == nil {
		return time.UTC
	}
	if loc, err := tz.Load(*e.TZ); err == nil {
		return loc
	}
	return time.UTC
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// dateOf returns midnight UTC of the local date of an instant in loc.
func dateOf(t time.Time, loc *time.Location) time.Time {
	w := tz.Wall(t, loc)
	return time.Date(w.Year(), w.Month(), w.Day(), 0, 0, 0, 0, time.UTC)
}

// SpanText is the timing text of an event.
type SpanText struct {
	Start, End, Duration string
	AllDay               bool
}

// Span resolves start, end and duration text in the zone loc. Without an end or a length the
// returned end is zero and the repository applies its default. For an all-day event the text end
// is the last day, inclusive, and the returned end is the day after.
func Span(s SpanText, loc *time.Location, now time.Time) (start, end time.Time, allDay bool, err error) {
	r, err := when.Parse(s.Start, now, loc)
	if err != nil {
		return start, end, false, prefixed("start", err)
	}
	allDay = s.AllDay || r.AllDay
	start = r.Start
	if allDay && r.HasTime {
		start = dateOf(r.Start, loc)
	}
	switch {
	case strings.TrimSpace(s.End) != "":
		e, perr := when.Parse(s.End, start, loc)
		if perr != nil {
			return start, end, allDay, prefixed("end", perr)
		}
		end = e.Start
		if allDay {
			if e.HasTime {
				end = dateOf(e.Start, loc)
			}
			end = end.AddDate(0, 0, 1)
		}
	case strings.TrimSpace(s.Duration) != "":
		d, perr := when.ParseLength(s.Duration)
		if perr != nil {
			return start, end, allDay, prefixed("duration", perr)
		}
		end = start.Add(d)
	case r.Length > 0:
		end = start.Add(r.Length)
	}
	return start, end, allDay, nil
}

// Bound replaces the COUNT or UNTIL of a rule. UNTIL text is a date (inclusive). It is stored in
// UTC for timed events (RFC 5545 section 3.3.10) and as a date for all-day events.
func Bound(rule, until string, count int, allDay bool, loc *time.Location, now time.Time) (string, error) {
	if until == "" && count == 0 {
		return rule, nil
	}
	if until != "" && count != 0 {
		return "", invalid("use until or count, not both")
	}
	var parts []string
	for _, p := range strings.Split(rule, ";") {
		if strings.HasPrefix(p, "COUNT=") || strings.HasPrefix(p, "UNTIL=") {
			continue
		}
		parts = append(parts, p)
	}
	if count != 0 {
		if count < 0 {
			return "", invalid("count must be positive")
		}
		return strings.Join(append(parts, "COUNT="+strconv.Itoa(count)), ";"), nil
	}
	r, err := when.Parse(until, now, loc)
	if err != nil {
		return "", prefixed("until", err)
	}
	day := r.Start
	if r.HasTime {
		day = dateOf(r.Start, loc)
	}
	if allDay {
		return strings.Join(append(parts, "UNTIL="+day.Format("20060102")), ";"), nil
	}
	endOfDay := tz.Instant(day.Add(24*time.Hour-time.Second), loc)
	return strings.Join(append(parts, "UNTIL="+endOfDay.UTC().Format("20060102T150405Z")), ";"), nil
}

// RepeatText is the repeat text of an event.
type RepeatText struct {
	Repeat  string
	Changed bool // Repeat was supplied (an empty Repeat then clears the rule)
	Until   string
	Count   int
}

// Rule compiles repeat text. base is the current rule (for an edit). A nil result means no
// change. A result pointing at "" clears the rule.
func Rule(t RepeatText, base string, allDay bool, loc *time.Location, now time.Time) (*string, error) {
	rule := base
	if t.Changed {
		if strings.TrimSpace(t.Repeat) == "" {
			if t.Until != "" || t.Count != 0 {
				return nil, invalid("until and count need a repeat")
			}
			empty := ""
			return &empty, nil
		}
		r, err := recur.Natural(t.Repeat)
		if err != nil {
			return nil, err
		}
		rule = r
	}
	if rule == "" {
		if t.Until != "" || t.Count != 0 {
			return nil, invalid("until and count need a repeat")
		}
		return nil, nil
	}
	rule, err := Bound(rule, t.Until, t.Count, allDay, loc, now)
	if err != nil {
		return nil, err
	}
	if rule == base && !t.Changed {
		return nil, nil
	}
	return &rule, nil
}

// Reminders reads reminder text such as "15m", "1h" or "1d:email" (a number of minutes before the
// start, then an optional method).
func Reminders(list []string) ([]model.EventReminder, error) {
	var out []model.EventReminder
	for _, s := range list {
		method := "display"
		if v, m, ok := strings.Cut(s, ":"); ok {
			s, method = v, m
		}
		mins, err := when.ParseReminder(s)
		if err != nil {
			return nil, prefixed("remind", err)
		}
		out = append(out, model.EventReminder{MinutesBefore: mins, Method: method})
	}
	return out, nil
}

// Anchor resolves "at" text for a scoped edit or delete into the stored anchor of an instance: an
// RFC 3339 instant for timed events, a date for all-day events. Empty text gives "".
func Anchor(e *model.Event, at string, now time.Time) (string, error) {
	if strings.TrimSpace(at) == "" {
		return "", nil
	}
	loc := EventLocation(e)
	r, err := when.Parse(at, now, loc)
	if err != nil {
		return "", prefixed("at", err)
	}
	if e.AllDay {
		d := r.Start
		if r.HasTime {
			d = dateOf(r.Start, loc)
		}
		return d.Format(tz.DateLayout), nil
	}
	if !r.HasTime {
		return "", invalid("at needs the date and time of the instance, such as the occurrence value from the event list")
	}
	return tz.FormatInstant(r.Start), nil
}

// Window turns from, to and days text into a window of instants. Day boundaries are local
// midnight in disp. Empty from means today. Empty to means from plus days.
func Window(from, to string, days int, disp *time.Location, now time.Time) (time.Time, time.Time, error) {
	localDay := func(s string) (time.Time, error) {
		r, err := when.Parse(s, now, disp)
		if err != nil {
			return time.Time{}, err
		}
		if r.AllDay {
			return r.Start, nil
		}
		return dateOf(r.Start, disp), nil
	}
	day := dateOf(now, disp)
	if strings.TrimSpace(from) != "" {
		d, err := localDay(from)
		if err != nil {
			return time.Time{}, time.Time{}, prefixed("from", err)
		}
		day = d
	}
	if days <= 0 && strings.TrimSpace(to) == "" {
		return time.Time{}, time.Time{}, invalid("days must be positive")
	}
	endDay := day.AddDate(0, 0, days)
	if strings.TrimSpace(to) != "" {
		d, err := localDay(to)
		if err != nil {
			return time.Time{}, time.Time{}, prefixed("to", err)
		}
		endDay = d
	}
	if !endDay.After(day) {
		return time.Time{}, time.Time{}, invalid("to must be after from")
	}
	return tz.Instant(day, disp), tz.Instant(endDay, disp), nil
}

// EditText is the timing text of an edit. Each Has flag says the text was supplied.
type EditText struct {
	Start, End, Duration          string
	HasStart, HasEnd, HasDuration bool
	AllDay                        bool // the all-day state after the edit
}

// Edit resolves the timing text of an edit into the update. A date alone on a timed event moves
// the day and keeps the time of day, and the length is kept unless an end or length is given.
func Edit(t EditText, cur *model.Event, loc *time.Location, now time.Time, in *model.UpdateEventInput) error {
	var curStart time.Time
	if cur.StartUTC != nil {
		curStart, _ = tz.ParseInstant(*cur.StartUTC)
	}
	if t.HasStart {
		r, err := when.Parse(t.Start, now, loc)
		if err != nil {
			return prefixed("start", err)
		}
		start := r.Start
		switch {
		case t.AllDay:
			if r.HasTime {
				start = dateOf(r.Start, loc)
			}
		case r.AllDay && !cur.AllDay:
			w := tz.Wall(curStart, loc)
			d := r.Start
			start = tz.Instant(time.Date(d.Year(), d.Month(), d.Day(), w.Hour(), w.Minute(), w.Second(), 0, time.UTC), loc)
		case r.AllDay:
			return invalid("start needs a time of day when the event becomes timed")
		}
		in.Start = &start
		if r.Length > 0 && !t.HasEnd && !t.HasDuration {
			end := start.Add(r.Length)
			in.End = &end
		}
	}
	if t.HasEnd || t.HasDuration {
		base := curStart
		switch {
		case in.Start != nil:
			base = *in.Start
		case cur.AllDay:
			base, _ = tz.ParseDate(deref(cur.StartDate))
		}
		var end time.Time
		if t.HasEnd {
			e, err := when.Parse(t.End, base, loc)
			if err != nil {
				return prefixed("end", err)
			}
			end = e.Start
			if t.AllDay {
				if e.HasTime {
					end = dateOf(e.Start, loc)
				}
				end = end.AddDate(0, 0, 1)
			}
		} else {
			d, err := when.ParseLength(t.Duration)
			if err != nil {
				return prefixed("duration", err)
			}
			end = base.Add(d)
		}
		in.End = &end
	}
	return nil
}
