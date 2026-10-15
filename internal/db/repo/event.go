package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/cal/recur"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// EventRepo handles event database operations.
type EventRepo struct {
	db *sql.DB
}

// NewEventRepo creates a new EventRepo.
func NewEventRepo(db *sql.DB) *EventRepo {
	return &EventRepo{db: db}
}

const eventColumns = `id, uuid, title, notes, location, all_day, start_utc, end_utc, start_date, end_date,
	tz, rrule, recurrence_parent_id, recurrence_id, status, busy, source, floating, created_at, updated_at, archived`

func scanEvent(row interface{ Scan(...any) error }) (*model.Event, error) {
	var e model.Event
	err := row.Scan(&e.ID, &e.UUID, &e.Title, &e.Notes, &e.Location, &e.AllDay, &e.StartUTC, &e.EndUTC,
		&e.StartDate, &e.EndDate, &e.TZ, &e.RRule, &e.RecurrenceParentID, &e.RecurrenceID, &e.Status,
		&e.Busy, &e.Source, &e.Floating, &e.CreatedAt, &e.UpdatedAt, &e.Archived)
	return &e, err
}

func nilIfBlank(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func ptrStr(s string) *string { return &s }

// span returns the first start and end of the event: instants for a timed event, midnight UTC of
// the dates for an all-day event.
func eventSpan(e *model.Event) (start, end time.Time, err error) {
	if e.AllDay {
		if e.StartDate == nil || e.EndDate == nil {
			return start, end, fmt.Errorf("event %d has no dates: %w", e.ID, model.ErrValidation)
		}
		if start, err = tz.ParseDate(*e.StartDate); err != nil {
			return
		}
		end, err = tz.ParseDate(*e.EndDate)
		return
	}
	if e.StartUTC == nil || e.EndUTC == nil {
		return start, end, fmt.Errorf("event %d has no times: %w", e.ID, model.ErrValidation)
	}
	if start, err = tz.ParseInstant(*e.StartUTC); err != nil {
		return
	}
	end, err = tz.ParseInstant(*e.EndUTC)
	return
}

// eventLoc is the zone of the event's wall clock.
func eventLoc(e *model.Event) *time.Location {
	if e.AllDay || e.TZ == nil {
		return time.UTC
	}
	loc, err := tz.Load(*e.TZ)
	if err != nil {
		return time.UTC
	}
	return loc
}

// anchorString renders an instance anchor: an instant, or a date for all-day events.
func anchorString(allDay bool, t time.Time) string {
	if allDay {
		return t.UTC().Format(tz.DateLayout)
	}
	return tz.FormatInstant(t)
}

func parseAnchor(allDay bool, s string) (time.Time, error) {
	if allDay {
		return tz.ParseDate(s)
	}
	return tz.ParseInstant(s)
}

// spec builds the recurrence spec of a series row (exdates must be loaded).
func eventSpec(e *model.Event) (recur.Spec, error) {
	start, end, err := eventSpan(e)
	if err != nil {
		return recur.Spec{}, err
	}
	sp := recur.Spec{Start: start, End: end, AllDay: e.AllDay, TZ: eventLoc(e)}
	if e.RRule != nil {
		sp.RRule = *e.RRule
	}
	for _, x := range e.Exdates {
		t, err := parseAnchor(e.AllDay, x)
		if err != nil {
			return recur.Spec{}, err
		}
		sp.Exdates = append(sp.Exdates, t)
	}
	return sp, nil
}

func (r *EventRepo) loadChildren(ctx context.Context, e *model.Event) error {
	q := r.db
	rows, err := q.QueryContext(ctx, `SELECT exdate FROM event_exdates WHERE event_id = ? ORDER BY exdate`, e.ID)
	if err != nil {
		return fmt.Errorf("load exdates: %w", err)
	}
	e.Exdates = nil
	for rows.Next() {
		var x string
		if err := rows.Scan(&x); err != nil {
			rows.Close()
			return err
		}
		e.Exdates = append(e.Exdates, x)
	}
	rows.Close()

	rows, err = q.QueryContext(ctx,
		`SELECT ep.person_id, trim(p.first_name || ' ' || coalesce(p.last_name, '')), ep.role, ep.rsvp
		 FROM event_people ep JOIN people p ON p.id = ep.person_id
		 WHERE ep.event_id = ? ORDER BY CASE ep.role WHEN 'organizer' THEN 0 ELSE 1 END, ep.person_id`, e.ID)
	if err != nil {
		return fmt.Errorf("load attendees: %w", err)
	}
	e.Attendees = nil
	for rows.Next() {
		var a model.EventAttendee
		if err := rows.Scan(&a.PersonID, &a.Name, &a.Role, &a.RSVP); err != nil {
			rows.Close()
			return err
		}
		e.Attendees = append(e.Attendees, a)
	}
	rows.Close()

	rows, err = q.QueryContext(ctx, `SELECT minutes_before, method FROM event_reminders WHERE event_id = ? ORDER BY minutes_before DESC, method`, e.ID)
	if err != nil {
		return fmt.Errorf("load reminders: %w", err)
	}
	e.Reminders = nil
	for rows.Next() {
		var rm model.EventReminder
		if err := rows.Scan(&rm.MinutesBefore, &rm.Method); err != nil {
			rows.Close()
			return err
		}
		e.Reminders = append(e.Reminders, rm)
	}
	rows.Close()

	rows, err = q.QueryContext(ctx,
		`SELECT t.name FROM tags t JOIN taggings tg ON tg.tag_id = t.id
		 WHERE tg.entity_type = 'event' AND tg.entity_id = ? ORDER BY t.name`, e.ID)
	if err != nil {
		return fmt.Errorf("load event tags: %w", err)
	}
	e.Tags = nil
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		e.Tags = append(e.Tags, name)
	}
	rows.Close()
	return nil
}

// FindByID returns a live event by id.
func (r *EventRepo) FindByID(ctx context.Context, id int64) (*model.Event, error) {
	return r.find(ctx, "id = ? AND archived = 0", id, "event")
}

// FindByUUID returns a live event by uuid.
func (r *EventRepo) FindByUUID(ctx context.Context, id string) (*model.Event, error) {
	return r.find(ctx, "uuid = ? AND archived = 0", id, "event")
}

func (r *EventRepo) find(ctx context.Context, where string, arg any, what string) (*model.Event, error) {
	e, err := scanEvent(r.db.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM events WHERE `+where, arg))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%s %v: %w", what, arg, model.ErrNotFound)
		}
		return nil, fmt.Errorf("find %s %v: %w", what, arg, err)
	}
	if err := r.loadChildren(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func validateReminders(list []model.EventReminder) ([]model.EventReminder, error) {
	seen := map[model.EventReminder]bool{}
	var out []model.EventReminder
	for _, rm := range list {
		if rm.MinutesBefore < 0 {
			return nil, fmt.Errorf("reminder minutes cannot be negative: %w", model.ErrValidation)
		}
		if rm.Method == "" {
			rm.Method = "display"
		}
		if rm.Method != "display" && rm.Method != "email" {
			return nil, fmt.Errorf("invalid reminder method %q (display or email): %w", rm.Method, model.ErrValidation)
		}
		if !seen[rm] {
			seen[rm] = true
			out = append(out, rm)
		}
	}
	return out, nil
}

// newEventFromInput validates a create input and builds the row.
func newEventFromInput(in model.CreateEventInput) (*model.Event, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, fmt.Errorf("an event needs a title: %w", model.ErrValidation)
	}
	status := in.Status
	if status == "" {
		status = model.EventConfirmed
	}
	if !model.ValidEventStatus(status) {
		return nil, fmt.Errorf("invalid status %q (confirmed, tentative or cancelled): %w", status, model.ErrValidation)
	}
	if in.Start.IsZero() {
		return nil, fmt.Errorf("an event needs a start: %w", model.ErrValidation)
	}
	source := in.Source
	if source == "" {
		source = "local"
	}
	e := &model.Event{
		Title: title, Notes: nilIfBlank(in.Notes), Location: nilIfBlank(in.Location),
		AllDay: in.AllDay, Status: status, Busy: true, Source: source, Floating: in.Floating,
	}
	if in.Busy != nil {
		e.Busy = *in.Busy
	}
	if err := setSpan(e, in.Start, in.End, in.TZ, true); err != nil {
		return nil, err
	}
	if rule := strings.TrimSpace(in.RRule); rule != "" {
		norm, err := recur.Natural(rule)
		if err != nil {
			return nil, err
		}
		e.RRule = &norm
	}
	return e, nil
}

// setSpan validates and stores start, end and zone on the row. An all-day event keeps dates, a
// timed event keeps instants and the zone name. A zero end defaults to one day or one hour.
func setSpan(e *model.Event, start, end time.Time, tzName string, setTZ bool) error {
	if e.AllDay {
		day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		last := day.AddDate(0, 0, 1)
		if !end.IsZero() {
			last = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
		}
		if !last.After(day) {
			return fmt.Errorf("an all-day event must end after it starts: %w", model.ErrValidation)
		}
		e.StartDate, e.EndDate = ptrStr(day.Format(tz.DateLayout)), ptrStr(last.Format(tz.DateLayout))
		e.StartUTC, e.EndUTC, e.TZ = nil, nil, nil
		return nil
	}
	if end.IsZero() {
		end = start.Add(time.Hour)
	}
	if end.Before(start) {
		return fmt.Errorf("the event ends before it starts: %w", model.ErrValidation)
	}
	e.StartUTC, e.EndUTC = ptrStr(tz.FormatInstant(start)), ptrStr(tz.FormatInstant(end))
	e.StartDate, e.EndDate = nil, nil
	if setTZ || e.TZ == nil {
		loc, err := tz.Load(tzName)
		if err != nil {
			return fmt.Errorf("%v: %w", err, model.ErrValidation)
		}
		e.TZ = ptrStr(tz.Name(loc))
	}
	return nil
}

func insertEvent(ctx context.Context, q DBTX, e *model.Event) (int64, error) {
	e.UUID = uuid.New().String()
	res, err := q.ExecContext(ctx,
		`INSERT INTO events (uuid, title, notes, location, all_day, start_utc, end_utc, start_date, end_date, tz, rrule,
			recurrence_parent_id, recurrence_id, status, busy, source, floating)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UUID, e.Title, e.Notes, e.Location, e.AllDay, e.StartUTC, e.EndUTC, e.StartDate, e.EndDate, e.TZ, e.RRule,
		e.RecurrenceParentID, e.RecurrenceID, e.Status, e.Busy, e.Source, e.Floating)
	if err != nil {
		return 0, fmt.Errorf("create event: %w", err)
	}
	return res.LastInsertId()
}

func writeEvent(ctx context.Context, q DBTX, e *model.Event) error {
	before, berr := scanEvent(q.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM events WHERE id = ?`, e.ID))
	res, err := q.ExecContext(ctx,
		`UPDATE events SET title = ?, notes = ?, location = ?, all_day = ?, start_utc = ?, end_utc = ?, start_date = ?,
			end_date = ?, tz = ?, rrule = ?, status = ?, busy = ?, floating = ?, updated_at = datetime('now')
		 WHERE id = ? AND archived = 0`,
		e.Title, e.Notes, e.Location, e.AllDay, e.StartUTC, e.EndUTC, e.StartDate, e.EndDate, e.TZ, e.RRule,
		e.Status, e.Busy, e.Floating, e.ID)
	if err != nil {
		return fmt.Errorf("update event %d: %w", e.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("event %d: %w", e.ID, model.ErrNotFound)
	}
	if berr == nil {
		return recordFieldChanges(ctx, q, "event", e.ID, eventFieldValues(before), eventFieldValues(e))
	}
	return nil
}

func touchEvent(ctx context.Context, q DBTX, id int64) error {
	if _, err := q.ExecContext(ctx, `UPDATE events SET updated_at = datetime('now') WHERE id = ?`, id); err != nil {
		return fmt.Errorf("touch event %d: %w", id, err)
	}
	return nil
}

func (r *EventRepo) writeChildren(ctx context.Context, q DBTX, id int64, exdates []time.Time, allDay bool, attendees []model.EventAttendee, reminders []model.EventReminder) error {
	for _, x := range exdates {
		if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO event_exdates (event_id, exdate) VALUES (?, ?)`, id, anchorString(allDay, x)); err != nil {
			return fmt.Errorf("add exdate: %w", err)
		}
	}
	for _, a := range attendees {
		if a.Role == "" {
			a.Role = "attendee"
		}
		if a.RSVP == "" {
			a.RSVP = "needs-action"
		}
		if err := model.ValidateEventRole(a.Role); err != nil {
			return err
		}
		if err := model.ValidateEventRSVP(a.RSVP); err != nil {
			return err
		}
		var ok int
		if err := q.QueryRowContext(ctx, `SELECT 1 FROM people WHERE id = ? AND archived = 0`, a.PersonID).Scan(&ok); err != nil {
			return fmt.Errorf("person %d: %w", a.PersonID, model.ErrNotFound)
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO event_people (event_id, person_id, role, rsvp) VALUES (?, ?, ?, ?)
			 ON CONFLICT(event_id, person_id) DO UPDATE SET role = excluded.role, rsvp = excluded.rsvp`,
			id, a.PersonID, a.Role, a.RSVP); err != nil {
			return fmt.Errorf("add attendee: %w", err)
		}
	}
	for _, rm := range reminders {
		if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO event_reminders (event_id, minutes_before, method) VALUES (?, ?, ?)`,
			id, rm.MinutesBefore, rm.Method); err != nil {
			return fmt.Errorf("add reminder: %w", err)
		}
	}
	return nil
}

// Create inserts a new event with its attendees, reminders and tags.
func (r *EventRepo) Create(ctx context.Context, in model.CreateEventInput) (*model.Event, error) {
	e, err := newEventFromInput(in)
	if err != nil {
		return nil, err
	}
	reminders, err := validateReminders(in.Reminders)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	id, err := insertEvent(ctx, tx, e)
	if err != nil {
		return nil, err
	}
	if err := r.writeChildren(ctx, tx, id, in.Exdates, e.AllDay, in.Attendees, reminders); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit event: %w", err)
	}
	tagr := NewTagRepo(r.db)
	for _, name := range in.Tags {
		if name = strings.TrimSpace(strings.TrimPrefix(name, "#")); name == "" {
			continue
		}
		if err := tagr.Apply(ctx, "event", id, name); err != nil {
			return nil, err
		}
	}
	return r.FindByID(ctx, id)
}

// ---- listing ----

// defaultListCap bounds how many instances one listing returns.
const defaultListCap = 1000

type listed struct {
	inst model.EventInstance
	key  string
}

// List returns the event instances that overlap the window, in start order. Repeating events are
// expanded, with per-instance overrides and exceptions applied. truncated says the cap was hit.
func (r *EventRepo) List(ctx context.Context, f model.EventFilters) (out []model.EventInstance, truncated bool, err error) {
	if f.From.IsZero() || f.To.IsZero() || !f.To.After(f.From) {
		return nil, false, fmt.Errorf("a listing needs a window with from before to: %w", model.ErrValidation)
	}
	loc := f.Loc
	if loc == nil {
		loc = time.UTC
	}
	limit := f.Limit
	if limit <= 0 || limit > defaultListCap {
		limit = defaultListCap
	}
	fromDay := f.From.In(loc)
	fromDate := time.Date(fromDay.Year(), fromDay.Month(), fromDay.Day(), 0, 0, 0, 0, time.UTC)
	toDay := f.To.In(loc).Add(-time.Nanosecond)
	toDate := time.Date(toDay.Year(), toDay.Month(), toDay.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	fromS, toS := tz.FormatInstant(f.From), tz.FormatInstant(f.To)
	fromD, toD := fromDate.Format(tz.DateLayout), toDate.Format(tz.DateLayout)

	where := []string{`e.archived = 0`, `e.recurrence_parent_id IS NULL`, `(
		(e.rrule IS NULL AND e.all_day = 0 AND ((e.start_utc < ? AND e.end_utc > ?) OR (e.start_utc >= ? AND e.start_utc < ?)))
		OR (e.rrule IS NULL AND e.all_day = 1 AND e.start_date < ? AND e.end_date > ?)
		OR (e.rrule IS NOT NULL AND e.all_day = 0 AND e.start_utc < ?)
		OR (e.rrule IS NOT NULL AND e.all_day = 1 AND e.start_date < ?))`}
	args := []any{toS, fromS, fromS, toS, toD, fromD, toS, toD}
	if !f.IncludeCancelled {
		where = append(where, `e.status != 'cancelled'`)
	}
	if f.PersonID != nil {
		where = append(where, `e.id IN (SELECT event_id FROM event_people WHERE person_id = ?)`)
		args = append(args, *f.PersonID)
	}
	if f.Tag != "" {
		where = append(where, `e.id IN (SELECT tg.entity_id FROM taggings tg JOIN tags t ON t.id = tg.tag_id WHERE tg.entity_type = 'event' AND t.name = ?)`)
		args = append(args, strings.TrimPrefix(f.Tag, "#"))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, `e.id IN (SELECT rowid FROM events_fts WHERE events_fts MATCH ?)`)
		args = append(args, q)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+prefixed("e", eventColumns)+`, (SELECT COUNT(*) FROM event_people ep WHERE ep.event_id = e.id)
		 FROM events e WHERE `+strings.Join(where, " AND ")+` ORDER BY e.id`, args...)
	if err != nil {
		return nil, false, fmt.Errorf("list events: %w", err)
	}
	type base struct {
		ev        *model.Event
		attendees int
	}
	var bases []base
	for rows.Next() {
		var e model.Event
		var n int
		if err := rows.Scan(&e.ID, &e.UUID, &e.Title, &e.Notes, &e.Location, &e.AllDay, &e.StartUTC, &e.EndUTC,
			&e.StartDate, &e.EndDate, &e.TZ, &e.RRule, &e.RecurrenceParentID, &e.RecurrenceID, &e.Status,
			&e.Busy, &e.Source, &e.Floating, &e.CreatedAt, &e.UpdatedAt, &e.Archived, &n); err != nil {
			rows.Close()
			return nil, false, fmt.Errorf("scan event: %w", err)
		}
		bases = append(bases, base{&e, n})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, false, err
	}
	rows.Close()

	var recurringIDs []int64
	for _, b := range bases {
		if b.ev.RRule != nil {
			recurringIDs = append(recurringIDs, b.ev.ID)
		}
	}
	exdates, err := r.exdatesFor(ctx, recurringIDs)
	if err != nil {
		return nil, false, err
	}
	overrides, err := r.overridesFor(ctx, recurringIDs)
	if err != nil {
		return nil, false, err
	}

	var all []listed
	for _, b := range bases {
		e := b.ev
		start, end, err := eventSpan(e)
		if err != nil {
			continue // a damaged row must not hide the rest of the calendar
		}
		mk := func(src *model.Event, s, en time.Time, recurrenceID time.Time, recurring bool) listed {
			inst := model.EventInstance{
				EventID: src.ID, UUID: src.UUID, SeriesID: e.ID, Title: src.Title, Status: src.Status, Busy: src.Busy,
				AllDay: e.AllDay, Recurring: recurring, Attendees: b.attendees,
			}
			if src.Location != nil {
				inst.Location = *src.Location
			}
			if e.AllDay {
				inst.StartDate, inst.EndDate = s.Format(tz.DateLayout), en.Format(tz.DateLayout)
			} else {
				inst.Start, inst.End = tz.FormatInstant(s), tz.FormatInstant(en)
				if e.TZ != nil {
					inst.TZ = *e.TZ
				}
			}
			if recurring {
				inst.Occurrence = anchorString(e.AllDay, recurrenceID)
			}
			key := tz.FormatInstant(s)
			return listed{inst: inst, key: key}
		}
		if e.RRule == nil {
			all = append(all, mk(e, start, end, start, false))
			continue
		}
		e.Exdates = exdates[e.ID]
		sp, err := eventSpec(e)
		if err != nil {
			continue
		}
		winFrom, winTo := f.From, f.To
		if e.AllDay {
			winFrom, winTo = fromDate, toDate
		}
		occs, trunc, err := recur.Expand(sp, winFrom, winTo)
		if err != nil {
			continue
		}
		if trunc {
			truncated = true
		}
		var ovs []recur.Override
		ovRows := overrides[e.ID]
		for _, o := range ovRows {
			os, oe, err := eventSpan(o)
			if err != nil {
				continue
			}
			rid, err := parseAnchor(e.AllDay, deref(o.RecurrenceID))
			if err != nil {
				continue
			}
			ovs = append(ovs, recur.Override{RecurrenceID: rid, Start: os, End: oe, AllDay: e.AllDay, Cancelled: o.Status == model.EventCancelled})
		}
		// Overrides were matched to rows by position, so keep them parallel.
		occs = recur.ApplyOverrides(occs, ovs, winFrom, winTo)
		for _, o := range occs {
			src := e
			if o.Override > 0 {
				src = ovRows[overrideRowIndex(ovRows, ovs[o.Override-1], e.AllDay)]
			}
			all = append(all, mk(src, o.Start, o.End, o.RecurrenceID, true))
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].key != all[j].key {
			return all[i].key < all[j].key
		}
		return all[i].inst.EventID < all[j].inst.EventID
	})
	for _, l := range all {
		if len(out) >= limit {
			truncated = true
			break
		}
		out = append(out, l.inst)
	}
	return out, truncated, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// overrideRowIndex finds the row an override came from.
func overrideRowIndex(rows []*model.Event, ov recur.Override, allDay bool) int {
	for i, o := range rows {
		if deref(o.RecurrenceID) == anchorString(allDay, ov.RecurrenceID) {
			return i
		}
	}
	return 0
}

// prefixed puts a table alias in front of each column in a comma separated list.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = alias + "." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

func inClause(ids []int64) (string, []any) {
	ph := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		ph[i], args[i] = "?", id
	}
	return strings.Join(ph, ","), args
}

func (r *EventRepo) exdatesFor(ctx context.Context, ids []int64) (map[int64][]string, error) {
	out := map[int64][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	ph, args := inClause(ids)
	rows, err := r.db.QueryContext(ctx, `SELECT event_id, exdate FROM event_exdates WHERE event_id IN (`+ph+`) ORDER BY exdate`, args...)
	if err != nil {
		return nil, fmt.Errorf("load exdates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var x string
		if err := rows.Scan(&id, &x); err != nil {
			return nil, err
		}
		out[id] = append(out[id], x)
	}
	return out, rows.Err()
}

func (r *EventRepo) overridesFor(ctx context.Context, ids []int64) (map[int64][]*model.Event, error) {
	out := map[int64][]*model.Event{}
	if len(ids) == 0 {
		return out, nil
	}
	ph, args := inClause(ids)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE recurrence_parent_id IN (`+ph+`) AND archived = 0 ORDER BY recurrence_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("load overrides: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out[*e.RecurrenceParentID] = append(out[*e.RecurrenceParentID], e)
	}
	return out, rows.Err()
}

// Search finds events matching a full text query. Only series rows and single events are returned.
func (r *EventRepo) Search(ctx context.Context, query string, limit int) ([]*model.Event, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE archived = 0 AND recurrence_parent_id IS NULL
		 AND id IN (SELECT rowid FROM events_fts WHERE events_fts MATCH ?)
		 ORDER BY coalesce(start_utc, start_date) DESC LIMIT ?`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search events: %w", err)
	}
	defer rows.Close()
	var out []*model.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- update ----

// applyUpdate returns a copy of cur with the changes applied, and whether the schedule of the
// event (start, zone or all-day flag) changed.
func applyUpdate(cur *model.Event, in model.UpdateEventInput) (*model.Event, bool, error) {
	n := *cur
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			return nil, false, fmt.Errorf("an event needs a title: %w", model.ErrValidation)
		}
		n.Title = t
	}
	if in.Notes != nil {
		n.Notes = nilIfBlank(in.Notes)
	}
	if in.Location != nil {
		n.Location = nilIfBlank(in.Location)
	}
	if in.Status != nil {
		if !model.ValidEventStatus(*in.Status) {
			return nil, false, fmt.Errorf("invalid status %q (confirmed, tentative or cancelled): %w", *in.Status, model.ErrValidation)
		}
		n.Status = *in.Status
	}
	if in.Busy != nil {
		n.Busy = *in.Busy
	}
	if in.RRule != nil {
		if strings.TrimSpace(*in.RRule) == "" {
			n.RRule = nil
		} else {
			norm, err := recur.Natural(*in.RRule)
			if err != nil {
				return nil, false, err
			}
			n.RRule = &norm
		}
	}

	allDay := cur.AllDay
	if in.AllDay != nil {
		allDay = *in.AllDay
	}
	if allDay != cur.AllDay && in.Start == nil {
		return nil, false, fmt.Errorf("changing between timed and all-day needs a new start: %w", model.ErrValidation)
	}
	curStart, curEnd, err := eventSpan(cur)
	if err != nil {
		return nil, false, err
	}
	start, end := curStart, curEnd
	changed := allDay != cur.AllDay
	if in.Start != nil {
		start = *in.Start
		if in.End == nil {
			if allDay != cur.AllDay {
				end = time.Time{} // defaults to one hour or one day
			} else {
				end = start.Add(curEnd.Sub(curStart))
			}
		}
	}
	if in.End != nil {
		end = *in.End
	}
	n.AllDay = allDay
	tzName := ""
	if cur.TZ != nil {
		tzName = *cur.TZ
	}
	if in.TZ != nil {
		tzName = *in.TZ
	}
	if err := setSpan(&n, start, end, tzName, in.TZ != nil || allDay != cur.AllDay); err != nil {
		return nil, false, err
	}
	if in.TZ != nil && !allDay && deref(cur.TZ) != deref(n.TZ) {
		changed = true
	}
	ns, _, _ := eventSpan(&n)
	if !ns.Equal(curStart) {
		changed = true
	}
	return &n, changed, nil
}

// removeOverrides deletes override rows of a series (and their tombstones) matching an extra
// condition on recurrence_id. Overrides are internal rows, so they are removed, not archived.
func removeOverrides(ctx context.Context, tx DBTX, parentID int64, notBefore string) error {
	cond, args := `recurrence_parent_id = ?`, []any{parentID}
	if notBefore != "" {
		cond += ` AND recurrence_id >= ?`
		args = append(args, notBefore)
	}
	device, err := DeviceID(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO tombstones (table_name, uuid, device_id) SELECT 'events', uuid, ? FROM events WHERE `+cond,
		append([]any{device}, args...)...); err != nil {
		return fmt.Errorf("tombstone overrides: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM external_ids WHERE entity = 'event' AND local_id IN (SELECT id FROM events WHERE `+cond+`)`, args...); err != nil {
		return fmt.Errorf("delete override links: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM taggings WHERE entity_type = 'event' AND entity_id IN (SELECT id FROM events WHERE `+cond+`)`, args...); err != nil {
		return fmt.Errorf("delete override tags: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE `+cond, args...); err != nil {
		return fmt.Errorf("delete overrides: %w", err)
	}
	return nil
}

// occurrenceOf checks that an instance of the series starts at the anchor and returns its span.
func occurrenceOf(series *model.Event, at string) (start, end time.Time, err error) {
	if series.RRule == nil {
		return start, end, fmt.Errorf("event %d does not repeat: %w", series.ID, model.ErrValidation)
	}
	if strings.TrimSpace(at) == "" {
		return start, end, fmt.Errorf("pass the start of the instance (--at) for scope this or following: %w", model.ErrValidation)
	}
	atT, err := parseAnchor(series.AllDay, at)
	if err != nil {
		return start, end, fmt.Errorf("%v: %w", err, model.ErrValidation)
	}
	sp, err := eventSpec(series)
	if err != nil {
		return start, end, err
	}
	occs, _, err := recur.Expand(sp, atT, atT.Add(time.Second))
	if err != nil {
		return start, end, err
	}
	for _, o := range occs {
		if o.RecurrenceID.Equal(atT) {
			return o.Start, o.End, nil
		}
	}
	return start, end, fmt.Errorf("event %d has no instance starting at %s: %w", series.ID, at, model.ErrNotFound)
}

func (r *EventRepo) copyChildren(ctx context.Context, tx DBTX, from, to int64, withTags bool) error {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO event_people (event_id, person_id, role, rsvp) SELECT ?, person_id, role, rsvp FROM event_people WHERE event_id = ?`, to, from); err != nil {
		return fmt.Errorf("copy attendees: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO event_reminders (event_id, minutes_before, method) SELECT ?, minutes_before, method FROM event_reminders WHERE event_id = ?`, to, from); err != nil {
		return fmt.Errorf("copy reminders: %w", err)
	}
	if withTags {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO taggings (tag_id, entity_type, entity_id) SELECT tag_id, 'event', ? FROM taggings WHERE entity_type = 'event' AND entity_id = ?`, to, from); err != nil {
			return fmt.Errorf("copy tags: %w", err)
		}
	}
	return nil
}

// Update changes an event. For a repeating event, scope picks what changes: "all" the series,
// "this" one instance (at is its original start, an RFC 3339 instant or a date for all-day
// events), or "following" that instance and every later one. It returns the event that was edited:
// the series, the override row for one instance, or the new series after a split.
func (r *EventRepo) Update(ctx context.Context, id int64, in model.UpdateEventInput, scope, at string) (*model.Event, error) {
	if !model.ValidScope(scope) {
		return nil, fmt.Errorf("invalid scope %q (all, this or following): %w", scope, model.ErrValidation)
	}
	cur, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.RecurrenceParentID != nil || cur.RRule == nil {
		scope = model.ScopeAll // a single event or an override row has only itself
	}
	if scope == "" {
		scope = model.ScopeAll
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	resultID := id
	switch scope {
	case model.ScopeAll:
		n, changed, err := applyUpdate(cur, in)
		if err != nil {
			return nil, err
		}
		if err := writeEvent(ctx, tx, n); err != nil {
			return nil, err
		}
		if changed && cur.RRule != nil {
			// Exceptions and overrides are keyed by the old schedule.
			if _, err := tx.ExecContext(ctx, `DELETE FROM event_exdates WHERE event_id = ?`, id); err != nil {
				return nil, fmt.Errorf("reset exdates: %w", err)
			}
			if err := removeOverrides(ctx, tx, id, ""); err != nil {
				return nil, err
			}
		}

	case model.ScopeThis:
		if in.RRule != nil {
			return nil, fmt.Errorf("the repeat rule applies to a whole series, not one instance: %w", model.ErrValidation)
		}
		occStart, occEnd, err := occurrenceOf(cur, at)
		if err != nil {
			return nil, err
		}
		existing, err := scanEvent(tx.QueryRowContext(ctx,
			`SELECT `+eventColumns+` FROM events WHERE recurrence_parent_id = ? AND recurrence_id = ? AND archived = 0`, id, anchorString(cur.AllDay, occStart)))
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("find override: %w", err)
		}
		if err == nil {
			// The override row exists: edit it.
			n, _, err := applyUpdate(existing, in)
			if err != nil {
				return nil, err
			}
			if err := writeEvent(ctx, tx, n); err != nil {
				return nil, err
			}
			resultID = existing.ID
			break
		}
		base := *cur
		base.RRule = nil
		base.RecurrenceParentID, base.RecurrenceID = &id, ptrStr(anchorString(cur.AllDay, occStart))
		if cur.AllDay {
			base.StartDate, base.EndDate = ptrStr(occStart.Format(tz.DateLayout)), ptrStr(occEnd.Format(tz.DateLayout))
		} else {
			base.StartUTC, base.EndUTC = ptrStr(tz.FormatInstant(occStart)), ptrStr(tz.FormatInstant(occEnd))
		}
		n, _, err := applyUpdate(&base, in)
		if err != nil {
			return nil, err
		}
		newID, err := insertEvent(ctx, tx, n)
		if err != nil {
			return nil, err
		}
		if err := r.copyChildren(ctx, tx, id, newID, false); err != nil {
			return nil, err
		}
		if err := touchEvent(ctx, tx, id); err != nil {
			return nil, err
		}
		resultID = newID

	case model.ScopeFollowing:
		occStart, occEnd, err := occurrenceOf(cur, at)
		if err != nil {
			return nil, err
		}
		sp, err := eventSpec(cur)
		if err != nil {
			return nil, err
		}
		oldRule, newRule, err := recur.Split(sp, occStart)
		if err != nil {
			return nil, err
		}
		if oldRule == "" {
			// Nothing precedes the instance, so this is an edit of the whole series.
			n, changed, err := applyUpdate(cur, in)
			if err != nil {
				return nil, err
			}
			if err := writeEvent(ctx, tx, n); err != nil {
				return nil, err
			}
			if changed {
				if _, err := tx.ExecContext(ctx, `DELETE FROM event_exdates WHERE event_id = ?`, id); err != nil {
					return nil, fmt.Errorf("reset exdates: %w", err)
				}
				if err := removeOverrides(ctx, tx, id, ""); err != nil {
					return nil, err
				}
			}
			break
		}
		base := *cur
		base.RRule = &newRule
		if cur.AllDay {
			base.StartDate, base.EndDate = ptrStr(occStart.Format(tz.DateLayout)), ptrStr(occEnd.Format(tz.DateLayout))
		} else {
			base.StartUTC, base.EndUTC = ptrStr(tz.FormatInstant(occStart)), ptrStr(tz.FormatInstant(occEnd))
		}
		n, _, err := applyUpdate(&base, in)
		if err != nil {
			return nil, err
		}
		anchor := anchorString(cur.AllDay, occStart)
		// Old series ends before the instance.
		if _, err := tx.ExecContext(ctx, `UPDATE events SET rrule = ?, updated_at = datetime('now') WHERE id = ?`, oldRule, id); err != nil {
			return nil, fmt.Errorf("end old series: %w", err)
		}
		n.RecurrenceParentID, n.RecurrenceID = nil, nil
		newID, err := insertEvent(ctx, tx, n)
		if err != nil {
			return nil, err
		}
		if err := r.copyChildren(ctx, tx, id, newID, true); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE event_exdates SET event_id = ? WHERE event_id = ? AND exdate >= ?`, newID, id, anchor); err != nil {
			return nil, fmt.Errorf("move exdates: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE events SET recurrence_parent_id = ?, updated_at = datetime('now') WHERE recurrence_parent_id = ? AND recurrence_id >= ?`, newID, id, anchor); err != nil {
			return nil, fmt.Errorf("move overrides: %w", err)
		}
		resultID = newID
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit event: %w", err)
	}
	return r.FindByID(ctx, resultID)
}

// ---- delete, restore, purge ----

// Archive soft-deletes an event. For a repeating event, scope "this" skips one instance and
// "following" ends the series before the instance, "all" archives the whole series.
func (r *EventRepo) Archive(ctx context.Context, id int64, scope, at string) error {
	if !model.ValidScope(scope) {
		return fmt.Errorf("invalid scope %q (all, this or following): %w", scope, model.ErrValidation)
	}
	cur, err := r.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if cur.RecurrenceParentID != nil || cur.RRule == nil {
		scope = model.ScopeAll
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	archiveAll := func() error {
		_, err := tx.ExecContext(ctx,
			`UPDATE events SET archived = 1, archived_at = datetime('now'), updated_at = datetime('now')
			 WHERE (id = ? OR recurrence_parent_id = ?) AND archived = 0`, id, id)
		if err != nil {
			return fmt.Errorf("archive event %d: %w", id, err)
		}
		return nil
	}
	switch scope {
	case "", model.ScopeAll:
		if err := archiveAll(); err != nil {
			return err
		}
	case model.ScopeThis:
		occStart, _, err := occurrenceOf(cur, at)
		if err != nil {
			return err
		}
		anchor := anchorString(cur.AllDay, occStart)
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO event_exdates (event_id, exdate) VALUES (?, ?)`, id, anchor); err != nil {
			return fmt.Errorf("skip instance: %w", err)
		}
		if err := removeOverridesAt(ctx, tx, id, anchor); err != nil {
			return err
		}
		if err := touchEvent(ctx, tx, id); err != nil {
			return err
		}
	case model.ScopeFollowing:
		occStart, _, err := occurrenceOf(cur, at)
		if err != nil {
			return err
		}
		sp, err := eventSpec(cur)
		if err != nil {
			return err
		}
		oldRule, _, err := recur.Split(sp, occStart)
		if err != nil {
			return err
		}
		if oldRule == "" {
			if err := archiveAll(); err != nil {
				return err
			}
			break
		}
		anchor := anchorString(cur.AllDay, occStart)
		if _, err := tx.ExecContext(ctx, `UPDATE events SET rrule = ?, updated_at = datetime('now') WHERE id = ?`, oldRule, id); err != nil {
			return fmt.Errorf("end series: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM event_exdates WHERE event_id = ? AND exdate >= ?`, id, anchor); err != nil {
			return fmt.Errorf("trim exdates: %w", err)
		}
		if err := removeOverrides(ctx, tx, id, anchor); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// removeOverridesAt removes the override of one instance, if there is one.
func removeOverridesAt(ctx context.Context, tx DBTX, parentID int64, anchor string) error {
	var overrideID int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM events WHERE recurrence_parent_id = ? AND recurrence_id = ?`, parentID, anchor).Scan(&overrideID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find override: %w", err)
	}
	device, err := DeviceID(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO tombstones (table_name, uuid, device_id) SELECT 'events', uuid, ? FROM events WHERE id = ?`, device, overrideID); err != nil {
		return fmt.Errorf("tombstone override: %w", err)
	}
	for _, q := range []string{
		`DELETE FROM external_ids WHERE entity = 'event' AND local_id = ?`,
		`DELETE FROM taggings WHERE entity_type = 'event' AND entity_id = ?`,
		`DELETE FROM events WHERE id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, overrideID); err != nil {
			return fmt.Errorf("delete override: %w", err)
		}
	}
	return nil
}

// Restore brings an archived event (and its overrides) back.
func (r *EventRepo) Restore(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE events SET archived = 0, archived_at = NULL, updated_at = datetime('now')
		 WHERE (id = ? OR recurrence_parent_id = ?) AND archived = 1`, id, id)
	if err != nil {
		return fmt.Errorf("restore event %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("archived event %d: %w", id, model.ErrNotFound)
	}
	return nil
}

// Purge permanently deletes an archived event, its overrides and links, and records tombstones.
func (r *EventRepo) Purge(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var archived int
	err = tx.QueryRowContext(ctx, `SELECT archived FROM events WHERE id = ?`, id).Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("event %d: %w", id, model.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("find event %d: %w", id, err)
	}
	if archived == 0 {
		return fmt.Errorf("event %d is not archived, delete it first: %w", id, model.ErrConflict)
	}
	device, err := DeviceID(ctx, tx)
	if err != nil {
		return err
	}
	where := `(id = ? OR recurrence_parent_id = ?)`
	steps := []struct {
		q    string
		args []any
	}{
		{`INSERT OR REPLACE INTO tombstones (table_name, uuid, device_id) SELECT 'events', uuid, ? FROM events WHERE ` + where, []any{device, id, id}},
		{`DELETE FROM external_ids WHERE entity = 'event' AND local_id IN (SELECT id FROM events WHERE ` + where + `)`, []any{id, id}},
		{`DELETE FROM taggings WHERE entity_type = 'event' AND entity_id IN (SELECT id FROM events WHERE ` + where + `)`, []any{id, id}},
		{`DELETE FROM events WHERE ` + where, []any{id, id}},
		{`DELETE FROM field_versions WHERE entity = 'event' AND local_id NOT IN (SELECT id FROM events)`, nil},
	}
	for _, s := range steps {
		if _, err := tx.ExecContext(ctx, s.q, s.args...); err != nil {
			return fmt.Errorf("purge event %d: %w", id, err)
		}
	}
	return tx.Commit()
}

// ---- attendees, reminders ----

// SetAttendee links a person to an event, or updates the role and RSVP of an existing link.
func (r *EventRepo) SetAttendee(ctx context.Context, eventID int64, a model.EventAttendee) error {
	if _, err := r.FindByID(ctx, eventID); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.writeChildren(ctx, tx, eventID, nil, false, []model.EventAttendee{a}, nil); err != nil {
		return err
	}
	if err := touchEvent(ctx, tx, eventID); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveAttendee unlinks a person from an event.
func (r *EventRepo) RemoveAttendee(ctx context.Context, eventID, personID int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM event_people WHERE event_id = ? AND person_id = ?`, eventID, personID)
	if err != nil {
		return fmt.Errorf("remove attendee: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("person %d is not an attendee of event %d: %w", personID, eventID, model.ErrNotFound)
	}
	return touchEvent(ctx, r.db, eventID)
}

// SetReminders replaces the reminders of an event.
func (r *EventRepo) SetReminders(ctx context.Context, eventID int64, reminders []model.EventReminder) error {
	if _, err := r.FindByID(ctx, eventID); err != nil {
		return err
	}
	list, err := validateReminders(reminders)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM event_reminders WHERE event_id = ?`, eventID); err != nil {
		return fmt.Errorf("clear reminders: %w", err)
	}
	if err := r.writeChildren(ctx, tx, eventID, nil, false, nil, list); err != nil {
		return err
	}
	if err := touchEvent(ctx, tx, eventID); err != nil {
		return err
	}
	return tx.Commit()
}
