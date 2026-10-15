package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Outcomes of a sync write.
const (
	OutcomeCreated   = "created"
	OutcomeUpdated   = "updated"
	OutcomeUnchanged = "unchanged"
)

// ExportRows returns every live event, overrides included, with attendees, reminders, exceptions
// and tags, in id order.
func (r *EventRepo) ExportRows(ctx context.Context) ([]*model.Event, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+eventColumns+` FROM events WHERE archived = 0 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	var out []*model.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, e := range out {
		if err := r.loadChildren(ctx, e); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// FindOverride returns the override row of one instance of a series, or ErrNotFound.
func (r *EventRepo) FindOverride(ctx context.Context, parentID int64, anchor string) (*model.Event, error) {
	e, err := scanEvent(r.db.QueryRowContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE recurrence_parent_id = ? AND recurrence_id = ? AND archived = 0`, parentID, anchor))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("override of event %d at %s: %w", parentID, anchor, model.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find override: %w", err)
	}
	if err := r.loadChildren(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// PersonEmails returns the primary email of each person that has one.
func (r *EventRepo) PersonEmails(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	ph, args := inClause(ids)
	rows, err := r.db.QueryContext(ctx,
		`SELECT p.id, coalesce(nullif(p.email, ''), (SELECT pe.email FROM person_emails pe WHERE pe.person_id = p.id ORDER BY pe.is_primary DESC, pe.id LIMIT 1), '')
		 FROM people p WHERE p.id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load emails: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, err
		}
		if email != "" {
			out[id] = email
		}
	}
	return out, rows.Err()
}

// MatchPersonByEmail finds the one live person who has the email (any of their addresses, case
// insensitive). ids holds every match so a caller can tell none from several.
func (r *EventRepo) MatchPersonByEmail(ctx context.Context, email string) (ids []int64, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM people WHERE archived = 0 AND (lower(email) = ? OR id IN (SELECT person_id FROM person_emails WHERE lower(email) = ?)) ORDER BY id`,
		email, email)
	if err != nil {
		return nil, fmt.Errorf("match email: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type attKey struct{ role, rsvp string }

func attendeeSet(list []model.EventAttendee) map[int64]attKey {
	m := map[int64]attKey{}
	for _, a := range list {
		if a.Role == "" {
			a.Role = "attendee"
		}
		if a.RSVP == "" {
			a.RSVP = "needs-action"
		}
		m[a.PersonID] = attKey{a.Role, a.RSVP}
	}
	return m
}

func sameAttendees(a, b []model.EventAttendee) bool {
	x, y := attendeeSet(a), attendeeSet(b)
	if len(x) != len(y) {
		return false
	}
	for k, v := range x {
		if y[k] != v {
			return false
		}
	}
	return true
}

func cleanTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func reminderStrings(list []model.EventReminder) []string {
	out := make([]string, len(list))
	for i, rm := range list {
		m := rm.Method
		if m == "" {
			m = "display"
		}
		out[i] = fmt.Sprintf("%d/%s", rm.MinutesBefore, m)
	}
	return out
}

// sameEventFields compares what a sync writes: the row fields only.
func sameEventFields(a, b *model.Event) bool {
	return a.Title == b.Title && deref(a.Notes) == deref(b.Notes) && deref(a.Location) == deref(b.Location) &&
		a.AllDay == b.AllDay && deref(a.StartUTC) == deref(b.StartUTC) && deref(a.EndUTC) == deref(b.EndUTC) &&
		deref(a.StartDate) == deref(b.StartDate) && deref(a.EndDate) == deref(b.EndDate) &&
		deref(a.TZ) == deref(b.TZ) && deref(a.RRule) == deref(b.RRule) &&
		a.Status == b.Status && a.Busy == b.Busy && a.Floating == b.Floating
}

// Upsert makes the event match the input: it creates the event when existingID is 0, updates it
// when anything differs and leaves it (and updated_at) alone when nothing does. With dryRun it
// reports the outcome and writes nothing. Exceptions, attendees, reminders and tags are replaced
// with the input's.
func (r *EventRepo) Upsert(ctx context.Context, existingID int64, in model.CreateEventInput, dryRun bool) (*model.Event, string, error) {
	want, err := newEventFromInput(in)
	if err != nil {
		return nil, "", err
	}
	reminders, err := validateReminders(in.Reminders)
	if err != nil {
		return nil, "", err
	}
	if existingID == 0 {
		if dryRun {
			return want, OutcomeCreated, nil
		}
		e, err := r.Create(ctx, in)
		return e, OutcomeCreated, err
	}
	cur, err := r.FindByID(ctx, existingID)
	if err != nil {
		return nil, "", err
	}
	if cur.RecurrenceParentID != nil {
		return nil, "", fmt.Errorf("event %d is an override of a series: %w", existingID, model.ErrValidation)
	}
	wantEx := make([]string, len(in.Exdates))
	for i, x := range in.Exdates {
		wantEx[i] = anchorString(want.AllDay, x)
	}
	if sameEventFields(cur, want) && sameStrings(cur.Exdates, wantEx) && sameAttendees(cur.Attendees, in.Attendees) &&
		sameStrings(reminderStrings(cur.Reminders), reminderStrings(reminders)) && sameStrings(cur.Tags, cleanTags(in.Tags)) {
		return cur, OutcomeUnchanged, nil
	}
	if dryRun {
		want.ID = cur.ID
		return want, OutcomeUpdated, nil
	}

	want.ID = cur.ID
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := writeEvent(ctx, tx, want); err != nil {
		return nil, "", err
	}
	for _, q := range []string{`DELETE FROM event_exdates WHERE event_id = ?`, `DELETE FROM event_people WHERE event_id = ?`, `DELETE FROM event_reminders WHERE event_id = ?`} {
		if _, err := tx.ExecContext(ctx, q, cur.ID); err != nil {
			return nil, "", fmt.Errorf("replace children: %w", err)
		}
	}
	if err := r.writeChildren(ctx, tx, cur.ID, in.Exdates, want.AllDay, in.Attendees, reminders); err != nil {
		return nil, "", err
	}
	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("commit event: %w", err)
	}
	tagr := NewTagRepo(r.db)
	wantTags := cleanTags(in.Tags)
	have := map[string]bool{}
	for _, t := range cur.Tags {
		have[t] = true
	}
	for _, t := range wantTags {
		if !have[t] {
			if err := tagr.Apply(ctx, "event", cur.ID, t); err != nil {
				return nil, "", err
			}
		}
		delete(have, t)
	}
	for t := range have {
		if err := tagr.Remove(ctx, "event", cur.ID, t); err != nil {
			return nil, "", err
		}
	}
	e, err := r.FindByID(ctx, cur.ID)
	return e, OutcomeUpdated, err
}

// UpsertOverride makes one instance of a series match the input. It returns ErrNotFound when the
// series has no instance at the anchor. An instance that already matches the series (and has no
// override row) is left alone.
func (r *EventRepo) UpsertOverride(ctx context.Context, parentID int64, anchor string, in model.CreateEventInput, dryRun bool) (*model.Event, string, error) {
	want, err := newEventFromInput(in)
	if err != nil {
		return nil, "", err
	}
	parent, err := r.FindByID(ctx, parentID)
	if err != nil {
		return nil, "", err
	}
	occStart, occEnd, err := occurrenceOf(parent, anchor)
	if err != nil {
		return nil, "", err
	}
	anchorS := anchorString(parent.AllDay, occStart)

	existing, err := r.FindOverride(ctx, parentID, anchorS)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		return nil, "", err
	}
	base := existing
	if base == nil {
		b := *parent
		b.RRule = nil
		if parent.AllDay {
			b.StartDate, b.EndDate = ptrStr(occStart.Format(tz.DateLayout)), ptrStr(occEnd.Format(tz.DateLayout))
		} else {
			b.StartUTC, b.EndUTC = ptrStr(tz.FormatInstant(occStart)), ptrStr(tz.FormatInstant(occEnd))
		}
		base = &b
	}
	cmp := *want
	cmp.TZ, cmp.RRule, cmp.Floating, cmp.Source = base.TZ, nil, base.Floating, base.Source
	if want.AllDay != base.AllDay {
		return nil, "", fmt.Errorf("an instance cannot change between timed and all-day: %w", model.ErrValidation)
	}
	if sameEventFields(base, &cmp) {
		if existing != nil {
			return existing, OutcomeUnchanged, nil
		}
		return parent, OutcomeUnchanged, nil
	}
	outcome := OutcomeUpdated
	if existing == nil {
		outcome = OutcomeCreated
	}
	if dryRun {
		return want, outcome, nil
	}
	start, end, err := eventSpan(want)
	if err != nil {
		return nil, "", err
	}
	title, notes, loc := want.Title, deref(want.Notes), deref(want.Location)
	status, busy := want.Status, want.Busy
	e, err := r.Update(ctx, parentID, model.UpdateEventInput{
		Title: &title, Notes: &notes, Location: &loc, Status: &status, Busy: &busy, Start: &start, End: &end,
	}, model.ScopeThis, anchorS)
	return e, outcome, err
}
