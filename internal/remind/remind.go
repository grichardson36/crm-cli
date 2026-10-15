// Package remind finds event reminders that are due. Nothing here sends anything: the owner or an
// agent reads the list and delivers it. A reminder is handed out once.
package remind

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Due is one reminder that should be shown now.
type Due struct {
	EventID       int64    `json:"event_id"`
	Title         string   `json:"title"`
	Start         string   `json:"start"`
	LocalStart    string   `json:"local_start"`
	FireAt        string   `json:"fire_at"`
	MinutesBefore int      `json:"minutes_before"`
	Method        string   `json:"method"`
	Location      string   `json:"location,omitempty"`
	People        []string `json:"people,omitempty"`
	Guests        []string `json:"guests,omitempty"`
	StartsIn      string   `json:"starts_in"`
}

// List returns the reminders whose time has come (fire time at or before now plus within) for
// instances that have not started, minus the ones already handed out. Earliest start first.
// loc places all-day events: their reminder counts from midnight in loc.
func List(ctx context.Context, db *sql.DB, now time.Time, within time.Duration, loc *time.Location) ([]Due, error) {
	var maxMin sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(minutes_before) FROM event_reminders`).Scan(&maxMin); err != nil {
		return nil, fmt.Errorf("read reminders: %w", err)
	}
	if !maxMin.Valid {
		return []Due{}, nil
	}
	horizon := now.Add(within + time.Duration(maxMin.Int64)*time.Minute)
	er := repo.NewEventRepo(db)
	insts, _, err := er.List(ctx, model.EventFilters{From: now.Add(-time.Minute), To: horizon.Add(time.Minute), Loc: loc, Limit: 5000})
	if err != nil {
		return nil, err
	}
	series := map[int64]*model.Event{}
	sent, err := sentSet(ctx, db)
	if err != nil {
		return nil, err
	}
	out := []Due{}
	for _, in := range insts {
		start, ok := startOf(in, loc)
		if !ok || !start.After(now) {
			continue
		}
		ev, ok := series[in.SeriesID]
		if !ok {
			if ev, err = er.FindByID(ctx, in.SeriesID); err != nil {
				return nil, err
			}
			series[in.SeriesID] = ev
		}
		for _, rm := range ev.Reminders {
			fire := start.Add(-time.Duration(rm.MinutesBefore) * time.Minute)
			if fire.After(now.Add(within)) {
				continue
			}
			key := sentKey{in.SeriesID, tz.FormatInstant(start), rm.MinutesBefore, rm.Method}
			if sent[key] {
				continue
			}
			d := Due{
				EventID: in.SeriesID, Title: in.Title, Start: tz.FormatInstant(start), LocalStart: start.In(loc).Format("Mon 2 Jan 15:04"),
				FireAt: tz.FormatInstant(fire), MinutesBefore: rm.MinutesBefore, Method: rm.Method, Location: in.Location,
				StartsIn: Humanize(start.Sub(now)),
			}
			for _, a := range ev.Attendees {
				d.People = append(d.People, a.Name)
			}
			for _, g := range ev.Guests {
				d.Guests = append(d.Guests, g.Email)
			}
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].MinutesBefore > out[j].MinutesBefore
	})
	return out, nil
}

// Mark records reminders as handed out.
func Mark(ctx context.Context, db *sql.DB, list []Due) error {
	for _, d := range list {
		if _, err := db.ExecContext(ctx,
			`INSERT OR IGNORE INTO reminders_sent (event_id, instance_start, minutes_before, method) VALUES (?, ?, ?, ?)`,
			d.EventID, d.Start, d.MinutesBefore, d.Method); err != nil {
			return fmt.Errorf("mark reminder: %w", err)
		}
	}
	return nil
}

type sentKey struct {
	event   int64
	start   string
	minutes int
	method  string
}

func sentSet(ctx context.Context, db *sql.DB) (map[sentKey]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT event_id, instance_start, minutes_before, method FROM reminders_sent`)
	if err != nil {
		return nil, fmt.Errorf("read sent reminders: %w", err)
	}
	defer rows.Close()
	out := map[sentKey]bool{}
	for rows.Next() {
		var k sentKey
		if err := rows.Scan(&k.event, &k.start, &k.minutes, &k.method); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

func startOf(in model.EventInstance, loc *time.Location) (time.Time, bool) {
	if in.AllDay {
		d, err := tz.ParseDate(in.StartDate)
		if err != nil {
			return time.Time{}, false
		}
		return tz.Instant(d, loc), true
	}
	t, err := tz.ParseInstant(in.Start)
	return t, err == nil
}

// Humanize says a span in the two largest useful units: "in 10m", "in 2h 5m", "in 3d".
func Humanize(d time.Duration) string {
	if d < time.Minute {
		return "now"
	}
	m := int(d / time.Minute)
	switch {
	case m < 60:
		return fmt.Sprintf("in %dm", m)
	case m < 24*60:
		if m%60 == 0 {
			return fmt.Sprintf("in %dh", m/60)
		}
		return fmt.Sprintf("in %dh %dm", m/60, m%60)
	}
	return fmt.Sprintf("in %dd", m/(24*60))
}
