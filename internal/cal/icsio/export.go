// Package icsio moves events and tasks between the database and iCalendar files. The file format
// itself lives in package ics. Files are matched to rows by UID through external_ids, so a second
// import updates instead of duplicating, and an export followed by an import is a no-op.
package icsio

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/ics"
	"github.com/jdanielnd/crm-cli/internal/cal/recur"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// DefaultSource is the external id source for ICS files.
const DefaultSource = "ics"

// ExportOptions selects what to export.
type ExportOptions struct {
	// From and To bound the export to events that overlap [From, To). Zero means no bound.
	From, To time.Time
	// Tasks adds open and completed tasks as VTODO.
	Tasks bool
	// Name is written as the calendar name.
	Name string
	// Source is the external id source whose UIDs are reused. Default "ics".
	Source string
	// EventIDs limits the export to these events (and their overrides). Empty means all.
	EventIDs []int64
	// Method makes the file an invitation (REQUEST, CANCEL). Organizer and Invitees are then added
	// to every event: the file is meant to be mailed, nothing is stored.
	Method    string
	Organizer *ics.Person
	Invitees  []ics.Person
}

// ExportResult is the file and what went into it.
type ExportResult struct {
	Data      []byte `json:"-"`
	Events    int    `json:"events"`
	Overrides int    `json:"overrides"`
	Todos     int    `json:"todos"`
	// AttendeesWithoutEmail counts attendees left out because iCalendar identifies people by address.
	AttendeesWithoutEmail int `json:"attendees_without_email"`
}

func derefS(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func eventSpan(e *model.Event) (start, end time.Time, err error) {
	if e.AllDay {
		if start, err = tz.ParseDate(derefS(e.StartDate)); err != nil {
			return
		}
		end, err = tz.ParseDate(derefS(e.EndDate))
		return
	}
	if start, err = tz.ParseInstant(derefS(e.StartUTC)); err != nil {
		return
	}
	end, err = tz.ParseInstant(derefS(e.EndUTC))
	return
}

func parseStored(allDay bool, s string) (time.Time, error) {
	if allDay {
		return tz.ParseDate(s)
	}
	return tz.ParseInstant(s)
}

func containsID(list []int64, id int64) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

func upper(s string) string { return strings.ToUpper(s) }

// Export writes the calendar as an iCalendar file.
func Export(ctx context.Context, db *sql.DB, opt ExportOptions) (*ExportResult, error) {
	source := opt.Source
	if source == "" {
		source = DefaultSource
	}
	er := repo.NewEventRepo(db)
	rows, err := er.ExportRows(ctx)
	if err != nil {
		return nil, err
	}
	links, err := repo.NewExternalIDRepo(db).ListBySource(ctx, source, "event")
	if err != nil {
		return nil, err
	}
	uidOf := map[int64]string{}
	for _, l := range links {
		uidOf[l.LocalID] = l.ExternalID
	}

	var personIDs []int64
	seen := map[int64]bool{}
	for _, e := range rows {
		for _, a := range e.Attendees {
			if !seen[a.PersonID] {
				seen[a.PersonID] = true
				personIDs = append(personIDs, a.PersonID)
			}
		}
	}
	emails, err := er.PersonEmails(ctx, personIDs)
	if err != nil {
		return nil, err
	}

	overrides := map[int64][]*model.Event{}
	for _, e := range rows {
		if e.RecurrenceParentID != nil {
			overrides[*e.RecurrenceParentID] = append(overrides[*e.RecurrenceParentID], e)
		}
	}

	res := &ExportResult{}
	var events []*ics.Event
	for _, e := range rows {
		if e.RecurrenceParentID != nil {
			continue
		}
		if len(opt.EventIDs) > 0 && !containsID(opt.EventIDs, e.ID) {
			continue
		}
		start, end, err := eventSpan(e)
		if err != nil {
			continue
		}
		if !opt.From.IsZero() && !opt.To.IsZero() {
			in := start.Before(opt.To) && (end.After(opt.From) || !start.Before(opt.From))
			if e.RRule != nil {
				in = start.Before(opt.To)
			}
			if !in {
				continue
			}
		}
		uid := uidOf[e.ID]
		if uid == "" {
			uid = e.UUID
		}
		ev, skipped, err := toICSEvent(e, uid, emails)
		if err != nil {
			return nil, err
		}
		res.AttendeesWithoutEmail += skipped
		if opt.Method != "" {
			invite(ev, opt)
		}
		events = append(events, ev)
		res.Events++

		for _, o := range overrides[e.ID] {
			oev, skipped, err := toICSEvent(o, uid, emails)
			if err != nil {
				return nil, err
			}
			res.AttendeesWithoutEmail += skipped
			rid, err := parseStored(e.AllDay, derefS(o.RecurrenceID))
			if err != nil {
				continue
			}
			oev.RecurrenceID = &rid
			oev.RRule, oev.Exdates = "", nil
			events = append(events, oev)
			res.Overrides++
		}
	}

	var todos []*ics.Todo
	if opt.Tasks {
		tl, err := exportTodos(ctx, db, source)
		if err != nil {
			return nil, err
		}
		todos = tl
		res.Todos = len(todos)
	}
	res.Data = ics.EncodeMethod(opt.Name, opt.Method, events, todos)
	return res, nil
}

// invite turns an event into an invitation: the organizer is the host and the invitees are
// asked to reply.
func invite(ev *ics.Event, opt ExportOptions) {
	if opt.Organizer != nil {
		org := *opt.Organizer
		org.Role = "organizer"
		ev.Organizer = &org
	}
	have := map[string]bool{}
	for _, a := range ev.Attendees {
		have[strings.ToLower(a.Email)] = true
	}
	for _, p := range opt.Invitees {
		if p.Email == "" || have[strings.ToLower(p.Email)] {
			continue
		}
		if p.PartStat == "" {
			p.PartStat = "NEEDS-ACTION"
		}
		ev.Attendees = append(ev.Attendees, p)
	}
	if opt.Method == "CANCEL" {
		ev.Status = "CANCELLED"
	}
}

func toICSEvent(e *model.Event, uid string, emails map[int64]string) (*ics.Event, int, error) {
	start, end, err := eventSpan(e)
	if err != nil {
		return nil, 0, err
	}
	ev := &ics.Event{
		UID: uid, Summary: e.Title, Description: derefS(e.Notes), Location: derefS(e.Location),
		Status: upper(e.Status), Transparent: !e.Busy, AllDay: e.AllDay, Start: start, End: end,
		TZ: derefS(e.TZ), Floating: e.Floating, Categories: e.Tags,
	}
	if e.AllDay {
		ev.TZ = ""
	}
	ev.Created, _ = tz.ParseInstant(e.CreatedAt)
	ev.Modified, _ = tz.ParseInstant(e.UpdatedAt)
	if e.RRule != nil {
		loc := time.UTC
		if !e.AllDay && e.TZ != nil {
			if l, err := tz.Load(*e.TZ); err == nil {
				loc = l
			}
		}
		rule, err := recur.ICSRule(recur.Spec{Start: start, End: end, AllDay: e.AllDay, TZ: loc, RRule: *e.RRule})
		if err != nil {
			return nil, 0, fmt.Errorf("event %d: %w", e.ID, err)
		}
		ev.RRule = rule
	}
	for _, x := range e.Exdates {
		t, err := parseStored(e.AllDay, x)
		if err == nil {
			ev.Exdates = append(ev.Exdates, t)
		}
	}
	skipped := 0
	for _, a := range e.Attendees {
		email := emails[a.PersonID]
		if email == "" {
			skipped++
			continue
		}
		p := ics.Person{Email: email, Name: a.Name, Role: a.Role, PartStat: upper(a.RSVP)}
		ev.Attendees = append(ev.Attendees, p)
		if a.Role == "organizer" && ev.Organizer == nil {
			org := p
			ev.Organizer = &org
		}
	}
	for _, g := range e.Guests {
		p := ics.Person{Email: g.Email, Name: g.Name, Role: g.Role, PartStat: upper(g.RSVP)}
		ev.Attendees = append(ev.Attendees, p)
		if g.Role == "organizer" && ev.Organizer == nil {
			org := p
			ev.Organizer = &org
		}
	}
	for _, rm := range e.Reminders {
		ev.Alarms = append(ev.Alarms, ics.Alarm{MinutesBefore: rm.MinutesBefore, Action: upper(rm.Method)})
	}
	return ev, skipped, nil
}

func exportTodos(ctx context.Context, db *sql.DB, source string) ([]*ics.Todo, error) {
	tasks, err := repo.NewTaskRepo(db).FindAll(ctx, model.TaskFilters{IncludeCompleted: true})
	if err != nil {
		return nil, err
	}
	links, err := repo.NewExternalIDRepo(db).ListBySource(ctx, source, "task")
	if err != nil {
		return nil, err
	}
	uidOf := map[int64]string{}
	for _, l := range links {
		uidOf[l.LocalID] = l.ExternalID
	}
	var out []*ics.Todo
	for _, t := range tasks {
		uid := uidOf[t.ID]
		if uid == "" {
			uid = t.UUID
		}
		td := &ics.Todo{UID: uid, Summary: t.Title, Status: "NEEDS-ACTION", Priority: map[string]int{"high": 1, "medium": 5, "low": 9}[t.Priority]}
		if t.Description != nil {
			td.Description = *t.Description
		}
		td.Created, _ = tz.ParseInstant(t.CreatedAt)
		td.Modified, _ = tz.ParseInstant(t.UpdatedAt)
		if t.DueAt != nil {
			if d, date, ok := parseDue(*t.DueAt); ok {
				td.Due, td.DueIsDate = &d, date
			}
		}
		if t.Completed {
			td.Status = "COMPLETED"
			if t.CompletedAt != nil {
				if c, err := tz.ParseInstant(*t.CompletedAt); err == nil {
					td.Completed = &c
				}
			}
		}
		if t.Recurrence != nil {
			if rule, err := recur.FromTaskRecurrence(*t.Recurrence); err == nil {
				td.RRule = rule
			}
		}
		out = append(out, td)
	}
	return out, nil
}

// parseDue reads a stored task due date: a date or a timestamp.
func parseDue(s string) (t time.Time, isDate, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) == 10 {
		if d, err := tz.ParseDate(s); err == nil {
			return d, true, true
		}
		return time.Time{}, false, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04"} {
		if v, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return v.UTC(), false, true
		}
	}
	return time.Time{}, false, false
}
