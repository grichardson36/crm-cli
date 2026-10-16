// Package book offers meeting times from a booking link and turns an accepted time into a
// calendar event. Tier 0: nothing here talks to a network. The host sends the text, the page or
// the invitation by hand and confirms the reply.
package book

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/schedule"
)

// Reasons a link offers nothing.
const (
	ReasonPaused   = "paused"
	ReasonExpired  = "expired"
	ReasonUsedUp   = "used_up"
	ReasonNoLength = "length_not_offered"
)

// Slot is one offered time.
type Slot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Result is what a link offers for one length.
type Result struct {
	Link      *model.BookingLink `json:"link"`
	TZ        string             `json:"tz"`
	LengthMin int                `json:"length_min"`
	From      time.Time          `json:"from"`
	To        time.Time          `json:"to"`
	Slots     []Slot             `json:"slots"`
	// Reason says why the list is empty when the whole link is closed.
	Reason string `json:"reason,omitempty"`
}

// PickLength returns the requested length, or the first length of the link when want is 0. It
// rejects a length the link does not offer.
func PickLength(l *model.BookingLink, want int) (int, error) {
	if want == 0 {
		if len(l.Durations) == 0 {
			return 30, nil
		}
		return l.Durations[0], nil
	}
	for _, d := range l.Durations {
		if d == want {
			return want, nil
		}
	}
	return 0, fmt.Errorf("link %q offers %v min, not %d: %w", l.Slug, l.Durations, want, model.ErrValidation)
}

// Slots lists the times a link offers for a meeting of lengthMin minutes. A time is offered when
// it is inside the link's working hours, starts after the notice and the start date, ends inside
// the horizon, keeps the buffers clear of events, placed tasks (when the link counts them) and
// other bookings, lies on the step grid, and its day, week and month are under the caps.
func Slots(ctx context.Context, db *sql.DB, l *model.BookingLink, lengthMin int, now time.Time) (*Result, error) {
	length, err := PickLength(l, lengthMin)
	if err != nil {
		return nil, err
	}
	hours, err := repo.NewAvailabilityRepo(db).Get(ctx, l.Schedule)
	if err != nil {
		return nil, err
	}
	loc, err := tz.Load(hours.TZ)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, model.ErrValidation)
	}
	res := &Result{Link: l, TZ: hours.TZ, LengthMin: length, Slots: []Slot{}}

	from := now.Add(time.Duration(l.MinNoticeMin) * time.Minute)
	if l.StartAfter != nil {
		if d, err := tz.ParseDate(*l.StartAfter); err == nil {
			if s := tz.Instant(d, loc); s.After(from) {
				from = s
			}
		}
	}
	to := now.AddDate(0, 0, l.HorizonDays)
	res.From, res.To = from, to

	switch {
	case !l.Active:
		res.Reason = ReasonPaused
		return res, nil
	case l.ExpiresAt != nil:
		if exp, err := tz.ParseInstant(*l.ExpiresAt); err == nil && !now.Before(exp) {
			res.Reason = ReasonExpired
			return res, nil
		}
	}
	bk := repo.NewBookingRepo(db)
	if l.MaxUses != nil {
		if n, err := bk.Uses(ctx, l.ID); err != nil {
			return nil, err
		} else if n >= *l.MaxUses {
			res.Reason = ReasonUsedUp
			return res, nil
		}
	}
	if !to.After(from) {
		return res, nil
	}

	before := time.Duration(l.BufferBeforeMin) * time.Minute
	after := time.Duration(l.BufferAfterMin) * time.Minute
	avail, err := schedule.Intervals(hours, from, to)
	if err != nil {
		return nil, err
	}
	busy, err := schedule.Busy(ctx, db, from.Add(-before-time.Hour), to.Add(after+time.Hour), loc, l.BusyTasks)
	if err != nil {
		return nil, err
	}
	held, err := bk.ListBookings(ctx, repo.BookingFilters{
		From: tz.FormatInstant(from.Add(-24 * time.Hour)), To: tz.FormatInstant(to.Add(24 * time.Hour)),
	})
	if err != nil {
		return nil, err
	}
	for _, b := range held {
		s, err1 := tz.ParseInstant(b.Start)
		e, err2 := tz.ParseInstant(b.End)
		if err1 == nil && err2 == nil {
			busy = append(busy, schedule.Span{Start: s, End: e})
		}
	}
	// A new meeting [S, E] needs S - before >= busy end and E + after <= busy start, so each busy
	// span grows by "after" on its start side and by "before" on its end side.
	grown := make([]schedule.Span, len(busy))
	for i, b := range busy {
		grown[i] = schedule.Span{Start: b.Start.Add(-after), End: b.End.Add(before)}
	}
	free := schedule.Subtract(avail, grown)
	step := time.Duration(l.SlotStepMin) * time.Minute
	spans := schedule.Slots(free, time.Duration(length)*time.Minute, step, loc, 0)

	counts := newCounter(loc)
	for _, b := range held {
		if b.LinkID == l.ID {
			if s, err := tz.ParseInstant(b.Start); err == nil {
				counts.add(s)
			}
		}
	}
	for _, s := range spans {
		if counts.full(l, s.Start) {
			continue
		}
		res.Slots = append(res.Slots, Slot{Start: s.Start, End: s.End})
	}
	return res, nil
}

// counter counts bookings per local day, week (Monday first) and month.
type counter struct {
	loc              *time.Location
	day, week, month map[string]int
}

func newCounter(loc *time.Location) *counter {
	return &counter{loc: loc, day: map[string]int{}, week: map[string]int{}, month: map[string]int{}}
}

func (c *counter) keys(t time.Time) (d, w, m string) {
	wall := tz.Wall(t, c.loc)
	y, wk := wall.ISOWeek()
	return wall.Format("2006-01-02"), fmt.Sprintf("%d-W%02d", y, wk), wall.Format("2006-01")
}

func (c *counter) add(t time.Time) {
	d, w, m := c.keys(t)
	c.day[d]++
	c.week[w]++
	c.month[m]++
}

func (c *counter) full(l *model.BookingLink, t time.Time) bool {
	d, w, m := c.keys(t)
	return (l.MaxPerDay != nil && c.day[d] >= *l.MaxPerDay) ||
		(l.MaxPerWeek != nil && c.week[w] >= *l.MaxPerWeek) ||
		(l.MaxPerMonth != nil && c.month[m] >= *l.MaxPerMonth)
}

// Offers reports whether start is one of the offered times.
func (r *Result) Offers(start time.Time) bool {
	i := sort.Search(len(r.Slots), func(i int) bool { return !r.Slots[i].Start.Before(start) })
	return i < len(r.Slots) && r.Slots[i].Start.Equal(start)
}

// Limit keeps the first n slots (0 keeps all).
func (r *Result) Limit(n int) {
	if n > 0 && len(r.Slots) > n {
		r.Slots = r.Slots[:n]
	}
}
