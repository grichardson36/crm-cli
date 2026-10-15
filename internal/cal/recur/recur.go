// Package recur expands RFC 5545 recurrence rules. It stores no rows: a series is one rule, an
// anchor start, a zone and a list of exceptions, and instances are computed for a window when
// they are asked for.
//
// Recurrence runs on the wall clock of the event's zone and each instance is then converted to
// UTC (see package tz), so a 09:00 weekly meeting stays at 09:00 local across daylight saving
// changes. The rule engine is github.com/teambition/rrule-go. It is fed floating wall times
// only, so the zone handling here is the single source of truth for daylight saving rules.
package recur

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/teambition/rrule-go"
)

// MaxInstances caps how many instances one query returns.
const MaxInstances = 1000

// maxSteps stops a runaway rule that never reaches the window.
const maxSteps = 200000

// Spec describes one series.
type Spec struct {
	// Start is the first occurrence: a UTC instant, or for an all-day event midnight UTC of its date.
	Start time.Time
	// End is the end of the first occurrence (exclusive). For an all-day event it is midnight UTC of
	// the day after the last day.
	End    time.Time
	AllDay bool
	// TZ is the zone of the author's wall clock. Nil means UTC. All-day events ignore it.
	TZ *time.Location
	// RRule is the rule body, such as "FREQ=WEEKLY;BYDAY=MO,WE". Empty means a single event.
	RRule string
	// Exdates are skipped instances, identified by their original start.
	Exdates []time.Time
	// Rdates are extra instances.
	Rdates []time.Time
}

// Occurrence is one instance of a series.
type Occurrence struct {
	Start  time.Time
	End    time.Time
	AllDay bool
	// RecurrenceID is the original start of the instance in the series. It stays the same when an
	// override moves the instance.
	RecurrenceID time.Time
	// Override is 1 + the index of the Override that replaced the instance, or 0 for none.
	Override int
}

func (s Spec) loc() *time.Location {
	if s.AllDay || s.TZ == nil {
		return time.UTC
	}
	return s.TZ
}

func (s Spec) duration() time.Duration {
	if d := s.End.Sub(s.Start); d > 0 {
		return d
	}
	return 0
}

type kv struct{ k, v string }

// parseKV splits a rule body into ordered KEY=VALUE pairs and checks the parts this package relies on.
func parseKV(body string) ([]kv, error) {
	body = strings.ToUpper(strings.TrimSpace(body))
	body = strings.TrimPrefix(body, "RRULE:")
	if body == "" {
		return nil, fmt.Errorf("empty recurrence rule: %w", model.ErrValidation)
	}
	var out []kv
	seen := map[string]bool{}
	for _, part := range strings.Split(body, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		k = strings.ToUpper(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("bad recurrence part %q: %w", part, model.ErrValidation)
		}
		if seen[k] {
			return nil, fmt.Errorf("recurrence part %s appears twice: %w", k, model.ErrValidation)
		}
		seen[k] = true
		out = append(out, kv{k, strings.ToUpper(v)})
	}
	if !seen["FREQ"] {
		return nil, fmt.Errorf("recurrence rule needs FREQ: %w", model.ErrValidation)
	}
	if seen["COUNT"] && seen["UNTIL"] {
		return nil, fmt.Errorf("recurrence rule cannot have both COUNT and UNTIL: %w", model.ErrValidation)
	}
	for _, p := range out {
		switch p.k {
		case "FREQ":
			switch p.v {
			case "DAILY", "WEEKLY", "MONTHLY", "YEARLY":
			default:
				return nil, fmt.Errorf("FREQ=%s is not supported (use DAILY, WEEKLY, MONTHLY or YEARLY): %w", p.v, model.ErrValidation)
			}
		case "COUNT", "INTERVAL":
			n, err := strconv.Atoi(p.v)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("%s must be a positive number: %w", p.k, model.ErrValidation)
			}
		}
	}
	return out, nil
}

func joinKV(parts []kv) string {
	s := make([]string, len(parts))
	for i, p := range parts {
		s[i] = p.k + "=" + p.v
	}
	return strings.Join(s, ";")
}

// Validate checks a rule body. It returns a validation error naming the problem.
func Validate(body string) error {
	parts, err := parseKV(body)
	if err != nil {
		return err
	}
	_, err = build(parts, Spec{Start: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)})
	return err
}

// Normalize returns the rule body in a canonical form: upper case, no RRULE: prefix.
func Normalize(body string) (string, error) {
	parts, err := parseKV(body)
	if err != nil {
		return "", err
	}
	if _, err := build(parts, Spec{Start: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)}); err != nil {
		return "", err
	}
	return joinKV(parts), nil
}

// build creates the rule engine for a series anchored at the spec start.
func build(parts []kv, spec Spec) (*rrule.RRule, error) {
	loc := spec.loc()
	var keep []kv
	var untilRaw string
	for _, p := range parts {
		if p.k == "UNTIL" {
			untilRaw = p.v
			continue
		}
		keep = append(keep, p)
	}
	opt, err := rrule.StrToROption(joinKV(keep))
	if err != nil {
		return nil, fmt.Errorf("invalid recurrence rule: %v: %w", err, model.ErrValidation)
	}
	opt.Dtstart = tz.Wall(spec.Start, loc)
	if spec.AllDay {
		opt.Dtstart = spec.Start
	}
	if untilRaw != "" {
		u, err := untilWall(untilRaw, spec)
		if err != nil {
			return nil, err
		}
		opt.Until = u
	}
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return nil, fmt.Errorf("invalid recurrence rule: %v: %w", err, model.ErrValidation)
	}
	return r, nil
}

// untilWall turns an UNTIL value into a floating wall time comparable with the rule's wall times.
//
//   - a date (20261231) ends at the end of that day
//   - a UTC time (20261231T150000Z) is converted to the zone's wall clock. RFC 5545 requires UTC
//     here when DTSTART carries a zone
//   - a floating time is taken as is
func untilWall(v string, spec Spec) (time.Time, error) {
	switch {
	case len(v) == 8:
		d, err := time.ParseInLocation("20060102", v, time.UTC)
		if err != nil {
			return time.Time{}, fmt.Errorf("bad UNTIL %q: %w", v, model.ErrValidation)
		}
		return d.Add(24*time.Hour - time.Second), nil
	case strings.HasSuffix(v, "Z"):
		t, err := time.ParseInLocation("20060102T150405Z", v, time.UTC)
		if err != nil {
			return time.Time{}, fmt.Errorf("bad UNTIL %q: %w", v, model.ErrValidation)
		}
		if spec.AllDay {
			return t, nil
		}
		return tz.Wall(t, spec.loc()), nil
	default:
		t, err := time.ParseInLocation("20060102T150405", v, time.UTC)
		if err != nil {
			return time.Time{}, fmt.Errorf("bad UNTIL %q: %w", v, model.ErrValidation)
		}
		return t, nil
	}
}

// each walks the rule's instances in order (before exceptions). fn gets the original start and
// returns false to stop. A spec without a rule has one instance.
func each(spec Spec, fn func(start time.Time) bool) error {
	if strings.TrimSpace(spec.RRule) == "" {
		fn(spec.Start)
		return nil
	}
	parts, err := parseKV(spec.RRule)
	if err != nil {
		return err
	}
	r, err := build(parts, spec)
	if err != nil {
		return err
	}
	loc := spec.loc()
	next := r.Iterator()
	for i := 0; i < maxSteps; i++ {
		w, ok := next()
		if !ok {
			return nil
		}
		start := w
		if !spec.AllDay {
			start = tz.Instant(w, loc)
		}
		if !fn(start) {
			return nil
		}
	}
	return fmt.Errorf("recurrence rule produced more than %d instances before the window: %w", maxSteps, model.ErrValidation)
}

func contains(list []time.Time, t time.Time) bool {
	for _, x := range list {
		if x.Equal(t) {
			return true
		}
	}
	return false
}

// Expand returns the instances of a series that overlap [from, to), in start order. At most
// MaxInstances are returned and truncated says when more existed.
func Expand(spec Spec, from, to time.Time) (occs []Occurrence, truncated bool, err error) {
	dur := spec.duration()
	overlaps := func(start, end time.Time) bool {
		return start.Before(to) && (end.After(from) || !start.Before(from))
	}
	add := func(start time.Time) bool {
		if contains(spec.Exdates, start) {
			return true
		}
		end := start.Add(dur)
		if !overlaps(start, end) {
			return true
		}
		if len(occs) >= MaxInstances {
			truncated = true
			return false
		}
		occs = append(occs, Occurrence{Start: start, End: end, AllDay: spec.AllDay, RecurrenceID: start})
		return true
	}
	err = each(spec, func(start time.Time) bool {
		if !start.Before(to) {
			return false
		}
		return add(start)
	})
	if err != nil {
		return nil, false, err
	}
	if !truncated {
		for _, rd := range spec.Rdates {
			already := false
			for _, o := range occs {
				if o.Start.Equal(rd) {
					already = true
				}
			}
			if !already && !add(rd) {
				break
			}
		}
	}
	sort.SliceStable(occs, func(i, j int) bool { return occs[i].Start.Before(occs[j].Start) })
	return occs, truncated, nil
}

// Override replaces or cancels one instance of a series.
type Override struct {
	// RecurrenceID is the original start of the instance it replaces.
	RecurrenceID time.Time
	Start, End   time.Time
	AllDay       bool
	Cancelled    bool
}

// ApplyOverrides applies per-instance overrides to expanded instances. A replaced instance keeps
// its RecurrenceID and takes the override's times. An override whose original instance is outside
// the window still shows when its new time falls inside. The result is in start order and
// limited to [from, to).
func ApplyOverrides(occs []Occurrence, ovs []Override, from, to time.Time) []Occurrence {
	if len(ovs) == 0 {
		return occs
	}
	overlaps := func(start, end time.Time) bool {
		return start.Before(to) && (end.After(from) || !start.Before(from))
	}
	used := make([]bool, len(ovs))
	var out []Occurrence
	for _, o := range occs {
		hit := -1
		for i, ov := range ovs {
			if ov.RecurrenceID.Equal(o.RecurrenceID) {
				hit = i
				break
			}
		}
		if hit < 0 {
			out = append(out, o)
			continue
		}
		used[hit] = true
		ov := ovs[hit]
		if ov.Cancelled || !overlaps(ov.Start, ov.End) {
			continue
		}
		o.Start, o.End, o.Override = ov.Start, ov.End, hit+1
		out = append(out, o)
	}
	for i, ov := range ovs {
		if used[i] || ov.Cancelled || !overlaps(ov.Start, ov.End) {
			continue
		}
		out = append(out, Occurrence{Start: ov.Start, End: ov.End, AllDay: ov.AllDay, RecurrenceID: ov.RecurrenceID, Override: i + 1})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

// Count returns how many instances of the rule start before at, ignoring exceptions (COUNT
// counts excluded instances too).
func Count(spec Spec, before time.Time) (int, error) {
	n := 0
	err := each(spec, func(start time.Time) bool {
		if !start.Before(before) {
			return false
		}
		n++
		return true
	})
	return n, err
}

// Split cuts a series at the instance that starts at "at" for a "this and following" edit. It
// returns the rule that ends the old series just before "at" and the rule for the new series that
// starts at "at". When no instance comes before "at", oldRule is empty and the whole series moves
// to the new rule, so the caller edits the series in place.
//
// A COUNT rule is divided: the old series keeps the instances before "at" and the new series the
// rest. Other rules end the old series with UNTIL in UTC, as RFC 5545 section 3.3.10 requires when
// DTSTART carries a zone.
func Split(spec Spec, at time.Time) (oldRule, newRule string, err error) {
	if strings.TrimSpace(spec.RRule) == "" {
		return "", "", fmt.Errorf("the event does not repeat: %w", model.ErrValidation)
	}
	parts, err := parseKV(spec.RRule)
	if err != nil {
		return "", "", err
	}
	before, err := Count(spec, at)
	if err != nil {
		return "", "", err
	}
	if before == 0 {
		return "", joinKV(parts), nil
	}

	var oldParts, newParts []kv
	hadCount := false
	for _, p := range parts {
		switch p.k {
		case "COUNT":
			hadCount = true
			total, _ := strconv.Atoi(p.v)
			if total-before < 1 {
				return "", "", fmt.Errorf("no instances left at or after %s: %w", tz.FormatInstant(at), model.ErrValidation)
			}
			oldParts = append(oldParts, kv{"COUNT", strconv.Itoa(before)})
			newParts = append(newParts, kv{"COUNT", strconv.Itoa(total - before)})
		case "UNTIL":
			newParts = append(newParts, p)
		default:
			oldParts = append(oldParts, p)
			newParts = append(newParts, p)
		}
	}
	if !hadCount {
		var until string
		if spec.AllDay {
			until = at.AddDate(0, 0, -1).Format("20060102")
		} else {
			until = at.Add(-time.Second).UTC().Format("20060102T150405Z")
		}
		oldParts = append(oldParts, kv{"UNTIL", until})
	}
	return joinKV(oldParts), joinKV(newParts), nil
}
