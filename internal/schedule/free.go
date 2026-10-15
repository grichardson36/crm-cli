package schedule

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Subtract removes busy spans from free spans. Both lists may be unsorted. The result is sorted.
func Subtract(free, busy []Span) []Span {
	free, busy = Merge(free), Merge(busy)
	var out []Span
	for _, f := range free {
		cur := f.Start
		for _, b := range busy {
			if !b.End.After(cur) {
				continue
			}
			if !b.Start.Before(f.End) {
				break
			}
			if b.Start.After(cur) {
				out = append(out, Span{cur, b.Start})
			}
			if b.End.After(cur) {
				cur = b.End
			}
		}
		if cur.Before(f.End) {
			out = append(out, Span{cur, f.End})
		}
	}
	return out
}

// Pad grows every span by buffer on both sides.
func Pad(in []Span, buffer time.Duration) []Span {
	out := make([]Span, len(in))
	for i, s := range in {
		out[i] = Span{s.Start.Add(-buffer), s.End.Add(buffer)}
	}
	return out
}

// Slots cuts free spans into slots of length that start on a step grid. The grid is the wall clock
// of loc, so a 30 minute step offers 09:00, 09:30, ... whatever the daylight saving offset. At most
// limit slots are returned (0 means no limit).
func Slots(free []Span, length, step time.Duration, loc *time.Location, limit int) []Span {
	if length <= 0 || step <= 0 {
		return nil
	}
	var out []Span
	for _, f := range Merge(free) {
		s := alignUp(f.Start, step, loc)
		for ; !s.Add(length).After(f.End); s = alignUp(s.Add(step), step, loc) {
			out = append(out, Span{s, s.Add(length)})
			if limit > 0 && len(out) >= limit {
				return out
			}
		}
	}
	return out
}

// alignUp returns the first instant at or after t whose local clock is a whole multiple of step
// since local midnight.
func alignUp(t time.Time, step time.Duration, loc *time.Location) time.Time {
	w := tz.Wall(t, loc)
	midnight := time.Date(w.Year(), w.Month(), w.Day(), 0, 0, 0, 0, time.UTC)
	since := w.Sub(midnight)
	if rem := since % step; rem != 0 || w.Nanosecond() != 0 {
		since = since - since%step + step
	}
	return tz.Instant(midnight.Add(since), loc)
}

// FindParams asks for time to meet.
type FindParams struct {
	// Length of the meeting. Required.
	Length time.Duration
	// From and To bound the search. Zero From means Now. Zero To means From plus the horizon.
	From, To time.Time
	Now      time.Time
	// Rule is the working hours rule. Empty means the default rule.
	Rule string
	// Optional overrides of the stored scheduling defaults.
	Buffer, Step *time.Duration
	MinNotice    time.Duration
	Limit        int
	// BusyTasks also counts tasks that are placed but not locked as busy. Locked tasks always are.
	BusyTasks bool
}

// Slot is one offered time.
type Slot struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// FindResult is the answer of FindTime.
type FindResult struct {
	Rule      string `json:"rule"`
	TZ        string `json:"tz"`
	From      string `json:"from"`
	To        string `json:"to"`
	LengthMin int    `json:"length_min"`
	BufferMin int    `json:"buffer_min"`
	StepMin   int    `json:"step_min"`
	Slots     []Slot `json:"slots"`
}

// Busy returns the busy time in [from, to): events that block time (repeats expanded, cancelled
// and transparent events left out, all-day events block their whole days in loc) and placed tasks.
func Busy(ctx context.Context, db *sql.DB, from, to time.Time, loc *time.Location, busyTasks bool) ([]Span, error) {
	var out []Span
	list, _, err := repo.NewEventRepo(db).List(ctx, model.EventFilters{From: from, To: to, Loc: loc})
	if err != nil {
		return nil, err
	}
	for _, in := range list {
		if !in.Busy || in.Status == model.EventCancelled {
			continue
		}
		if in.AllDay {
			s, err1 := tz.ParseDate(in.StartDate)
			e, err2 := tz.ParseDate(in.EndDate)
			if err1 != nil || err2 != nil {
				continue
			}
			out = append(out, Span{tz.Instant(s, loc), tz.Instant(e, loc)})
			continue
		}
		s, err1 := tz.ParseInstant(in.Start)
		e, err2 := tz.ParseInstant(in.End)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, Span{s, e})
	}
	tasks, err := repo.NewTaskRepo(db).FindAll(ctx, model.TaskFilters{})
	if err != nil {
		return nil, err
	}
	chunks, err := repo.NewPlanRepo(db).Chunks(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.ScheduledStart == nil || t.DurationMin == nil || (!t.ScheduledLocked && !busyTasks) {
			continue
		}
		if cs := chunks[t.ID]; len(cs) > 0 {
			for _, c := range cs {
				s, err1 := tz.ParseInstant(c.Start)
				e, err2 := tz.ParseInstant(c.End)
				if err1 == nil && err2 == nil {
					out = append(out, Span{s, e})
				}
			}
			continue
		}
		s, err := tz.ParseInstant(*t.ScheduledStart)
		if err != nil {
			continue
		}
		out = append(out, Span{s, s.Add(time.Duration(*t.DurationMin) * time.Minute)})
	}
	return Merge(out), nil
}

// FindTime offers meeting slots inside working hours that clear busy time and its buffers.
func FindTime(ctx context.Context, db *sql.DB, p FindParams) (*FindResult, error) {
	if p.Length <= 0 {
		return nil, fmt.Errorf("a meeting needs a length: %w", model.ErrValidation)
	}
	ar := repo.NewAvailabilityRepo(db)
	cfg, err := ar.Config(ctx)
	if err != nil {
		return nil, err
	}
	hours, err := ar.Get(ctx, p.Rule)
	if err != nil {
		return nil, err
	}
	loc, err := tz.Load(hours.TZ)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, model.ErrValidation)
	}
	buffer := time.Duration(cfg.BufferMin) * time.Minute
	if p.Buffer != nil {
		buffer = *p.Buffer
	}
	step := time.Duration(cfg.StepMin) * time.Minute
	if p.Step != nil {
		step = *p.Step
	}
	if buffer < 0 || step <= 0 {
		return nil, fmt.Errorf("buffer cannot be negative and step must be positive: %w", model.ErrValidation)
	}

	from := p.From
	earliest := p.Now.Add(p.MinNotice)
	if from.IsZero() || from.Before(earliest) {
		from = earliest
	}
	to := p.To
	if to.IsZero() {
		to = from.AddDate(0, 0, cfg.HorizonDays)
	}
	res := &FindResult{
		Rule: hours.Name, TZ: hours.TZ, From: tz.FormatInstant(from), To: tz.FormatInstant(to),
		LengthMin: int(p.Length / time.Minute), BufferMin: int(buffer / time.Minute), StepMin: int(step / time.Minute), Slots: []Slot{},
	}
	if !to.After(from) {
		return res, nil
	}
	avail, err := Intervals(hours, from, to)
	if err != nil {
		return nil, err
	}
	busy, err := Busy(ctx, db, from.Add(-buffer), to.Add(buffer), loc, p.BusyTasks)
	if err != nil {
		return nil, err
	}
	free := Subtract(avail, Pad(busy, buffer))
	for _, s := range Slots(free, p.Length, step, loc, p.Limit) {
		res.Slots = append(res.Slots, Slot{Start: tz.FormatInstant(s.Start), End: tz.FormatInstant(s.End)})
	}
	return res, nil
}
