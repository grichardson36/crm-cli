// Package schedule holds the deterministic calendar math: working hours expanded to instants and
// free time found around busy time. Nothing here reads the database or the clock.
package schedule

import (
	"sort"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Span is a half open interval of instants, [Start, End).
type Span struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Len is the length of the span.
func (s Span) Len() time.Duration { return s.End.Sub(s.Start) }

// Merge sorts spans and joins the ones that touch or overlap.
func Merge(in []Span) []Span {
	if len(in) == 0 {
		return nil
	}
	s := append([]Span(nil), in...)
	sort.Slice(s, func(i, j int) bool { return s[i].Start.Before(s[j].Start) })
	out := []Span{s[0]}
	for _, x := range s[1:] {
		last := &out[len(out)-1]
		if !x.Start.After(last.End) {
			if x.End.After(last.End) {
				last.End = x.End
			}
			continue
		}
		out = append(out, x)
	}
	return out
}

func minutes(d time.Duration) int { return int(d / time.Minute) }

// Intervals expands working hours to instants within [from, to). Hours are wall clock in the rule's
// zone, converted day by day, so they follow daylight saving time. A date override replaces the
// weekly hours of its date. A wall time that does not exist (the spring forward gap) or repeats (the
// fall back hour) follows RFC 5545 section 3.3.5.
func Intervals(h *model.WorkingHours, from, to time.Time) ([]Span, error) {
	loc, err := tz.Load(h.TZ)
	if err != nil {
		return nil, err
	}
	var out []Span
	first := tz.Wall(from, loc)
	first = time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	last := tz.Wall(to, loc)
	last = time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		ranges, ok := h.Overrides[d.Format(tz.DateLayout)]
		if !ok {
			ranges = h.Weekly[model.WeekdayKeys[(int(d.Weekday())+6)%7]]
		}
		for _, r := range ranges {
			a, err := model.ClockMinutes(r.Start)
			if err != nil {
				return nil, err
			}
			b, err := model.ClockMinutes(r.End)
			if err != nil {
				return nil, err
			}
			start := tz.Instant(d.Add(time.Duration(a)*time.Minute), loc)
			end := tz.Instant(d.Add(time.Duration(b)*time.Minute), loc)
			if b == 24*60 {
				end = tz.Instant(d.AddDate(0, 0, 1), loc)
			}
			if start.Before(from) {
				start = from
			}
			if end.After(to) {
				end = to
			}
			if end.After(start) {
				out = append(out, Span{start, end})
			}
		}
	}
	return Merge(out), nil
}
