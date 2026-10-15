package icsio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/ics"
	"github.com/jdanielnd/crm-cli/internal/cal/recur"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// ImportOptions controls an import.
type ImportOptions struct {
	// Source is the external id source UIDs are remembered under. Default "ics".
	Source string
	// DryRun reports what would change and writes nothing.
	DryRun bool
	// Loc is the zone given to floating times. Default: this device.
	Loc *time.Location
	// SkipTasks leaves VTODO components out.
	SkipTasks bool
}

// Counts are the outcomes for one kind of record.
type Counts struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
}

// ImportResult says what an import did.
type ImportResult struct {
	Source    string `json:"source"`
	DryRun    bool   `json:"dry_run"`
	Calendar  string `json:"calendar,omitempty"`
	Events    Counts `json:"events"`
	Overrides Counts `json:"overrides"`
	Todos     Counts `json:"todos"`
	// Ignored counts components that are not events or tasks, by name.
	Ignored map[string]int `json:"ignored,omitempty"`
	// Warnings name components that were left out because they cannot be placed (no UID, no start).
	Warnings []string `json:"warnings,omitempty"`
}

func (c *Counts) add(outcome string) {
	switch outcome {
	case repo.OutcomeCreated:
		c.Created++
	case repo.OutcomeUpdated:
		c.Updated++
	default:
		c.Unchanged++
	}
}

type importer struct {
	ctx   context.Context
	db    *sql.DB
	opt   ImportOptions
	er    *repo.EventRepo
	tr    *repo.TaskRepo
	ext   *repo.ExternalIDRepo
	res   *ImportResult
	cache map[string]int64 // email -> person id, 0 when none or ambiguous
}

func (im *importer) warn(format string, a ...any) {
	im.res.Warnings = append(im.res.Warnings, fmt.Sprintf(format, a...))
}

// Import reads an iCalendar file into the database.
func Import(ctx context.Context, db *sql.DB, data []byte, opt ImportOptions) (*ImportResult, error) {
	if opt.Source == "" {
		opt.Source = DefaultSource
	}
	if opt.Loc == nil {
		opt.Loc = tz.Device()
	}
	cal, err := ics.Decode(data, opt.Loc)
	if err != nil {
		return nil, fmt.Errorf("%v: %w", err, model.ErrValidation)
	}
	im := &importer{
		ctx: ctx, db: db, opt: opt, er: repo.NewEventRepo(db), tr: repo.NewTaskRepo(db), ext: repo.NewExternalIDRepo(db),
		res:   &ImportResult{Source: opt.Source, DryRun: opt.DryRun, Calendar: cal.Name, Ignored: cal.Skipped},
		cache: map[string]int64{},
	}
	if len(cal.Skipped) == 0 {
		im.res.Ignored = nil
	}
	im.res.Warnings = append(im.res.Warnings, cal.Warnings...)

	// Series and single events first, so overrides find their series.
	ids := map[string]int64{}   // uid -> local event id
	dryNew := map[string]bool{} // uids that a dry run would create
	for _, ev := range cal.Events {
		if ev.RecurrenceID != nil {
			continue
		}
		if err := im.event(ev, ids, dryNew); err != nil {
			return nil, err
		}
	}
	for _, ev := range cal.Events {
		if ev.RecurrenceID == nil {
			continue
		}
		if err := im.override(ev, ids, dryNew); err != nil {
			return nil, err
		}
	}
	if !opt.SkipTasks {
		for _, td := range cal.Todos {
			if err := im.todo(td); err != nil {
				return nil, err
			}
		}
	}
	return im.res, nil
}

// lookup finds the local row for a UID: a mapping from this source, a mapping from any other
// source (the same UID arrives the same way whichever file or feed brought it), or a row whose own
// uuid is the UID (a file exported from this database). A deleted row counts as no match, so the
// import inserts the UID again. 0 means no match.
func (im *importer) lookup(entity, uid string) (int64, error) {
	var id int64
	l, err := im.ext.Lookup(im.ctx, im.opt.Source, entity, uid)
	if errors.Is(err, model.ErrNotFound) {
		l, err = im.ext.LookupAny(im.ctx, entity, uid)
	}
	switch {
	case err == nil:
		id = l.LocalID
	case !errors.Is(err, model.ErrNotFound):
		return 0, err
	}
	if id == 0 {
		switch entity {
		case "event":
			if e, err := im.er.FindByUUID(im.ctx, uid); err == nil {
				id = e.ID
			}
		case "task":
			if t, err := im.tr.FindByUUID(im.ctx, uid); err == nil {
				id = t.ID
			}
		}
	}
	if id == 0 {
		return 0, nil
	}
	exists, archived, err := repo.EntityState(im.ctx, im.db, entity, id)
	if err != nil || !exists || archived {
		return 0, err
	}
	return id, nil
}

func (im *importer) link(entity, uid string, localID int64) error {
	if im.opt.DryRun {
		return nil
	}
	_, err := im.ext.Upsert(im.ctx, repo.ExternalIDInput{Entity: entity, LocalID: localID, Source: im.opt.Source, ExternalID: uid})
	return err
}

func statusOf(s string) string {
	switch strings.ToUpper(s) {
	case "TENTATIVE":
		return model.EventTentative
	case "CANCELLED":
		return model.EventCancelled
	}
	return model.EventConfirmed
}

func rsvpOf(s string) string {
	switch strings.ToUpper(s) {
	case "ACCEPTED":
		return "accepted"
	case "DECLINED":
		return "declined"
	case "TENTATIVE":
		return "tentative"
	}
	return "needs-action"
}

// person matches an address to the one contact that has it. No match, or more than one, gives 0
// and the address is kept on the event as a guest. No contact is created.
func (im *importer) person(p ics.Person) int64 {
	if p.Email == "" {
		return 0
	}
	if id, ok := im.cache[p.Email]; ok {
		return id
	}
	var id int64
	if ids, err := im.er.MatchPersonByEmail(im.ctx, p.Email); err == nil && len(ids) == 1 {
		id = ids[0]
	}
	im.cache[p.Email] = id
	return id
}

func (im *importer) input(ev *ics.Event) model.CreateEventInput {
	in := model.CreateEventInput{
		Title: ev.Summary, AllDay: ev.AllDay, Start: ev.Start, End: ev.End, TZ: ev.TZ, Floating: ev.Floating,
		Status: statusOf(ev.Status), Source: im.opt.Source, Tags: ev.Categories,
	}
	busy := !ev.Transparent
	in.Busy = &busy
	if ev.Description != "" {
		d := ev.Description
		in.Notes = &d
	}
	if ev.Location != "" {
		l := ev.Location
		in.Location = &l
	}
	if ev.AllDay {
		in.TZ = ""
	}
	if ev.RRule != "" {
		norm, err := recur.Natural(ev.RRule)
		if err != nil {
			im.warn("event %q: repeat rule %q is not supported, imported as a single event", ev.UID, ev.RRule)
		} else {
			in.RRule = norm
		}
	}
	for _, x := range ev.Exdates {
		in.Exdates = append(in.Exdates, x)
	}
	byPerson := map[int64]*model.EventAttendee{}
	var order []int64
	add := func(p ics.Person, organizer bool) {
		id := im.person(p)
		role := "attendee"
		if organizer || p.Role == "organizer" {
			role = "organizer"
		}
		if id == 0 {
			if p.Email == "" {
				return
			}
			for i := range in.Guests {
				if in.Guests[i].Email == p.Email {
					if role == "organizer" {
						in.Guests[i].Role = role
					}
					return
				}
			}
			in.Guests = append(in.Guests, model.EventGuest{Email: p.Email, Name: p.Name, Role: role, RSVP: rsvpOf(p.PartStat)})
			return
		}
		if cur, ok := byPerson[id]; ok {
			if role == "organizer" {
				cur.Role = role
			}
			return
		}
		byPerson[id] = &model.EventAttendee{PersonID: id, Role: role, RSVP: rsvpOf(p.PartStat)}
		order = append(order, id)
	}
	// Attendees first: they carry the answer (PARTSTAT). The organizer line only adds the role.
	for _, a := range ev.Attendees {
		add(a, false)
	}
	if ev.Organizer != nil {
		add(*ev.Organizer, true)
	}
	for _, id := range order {
		in.Attendees = append(in.Attendees, *byPerson[id])
	}
	for _, al := range ev.Alarms {
		method := "display"
		if strings.EqualFold(al.Action, "EMAIL") {
			method = "email"
		}
		in.Reminders = append(in.Reminders, model.EventReminder{MinutesBefore: al.MinutesBefore, Method: method})
	}
	return in
}

func (im *importer) event(ev *ics.Event, ids map[string]int64, dryNew map[string]bool) error {
	id, err := im.lookup("event", ev.UID)
	if err != nil {
		return err
	}
	in := im.input(ev)
	got, outcome, err := im.er.Upsert(im.ctx, id, in, im.opt.DryRun)
	if err != nil {
		if errors.Is(err, model.ErrValidation) {
			im.res.Events.Skipped++
			im.warn("event %q skipped: %v", ev.UID, err)
			return nil
		}
		return err
	}
	im.res.Events.add(outcome)
	if got.ID != 0 {
		ids[ev.UID] = got.ID
		if outcome == repo.OutcomeCreated || id == 0 {
			if err := im.link("event", ev.UID, got.ID); err != nil {
				return err
			}
		}
	} else if outcome == repo.OutcomeCreated {
		dryNew[ev.UID] = true
	}
	return nil
}

func (im *importer) override(ev *ics.Event, ids map[string]int64, dryNew map[string]bool) error {
	parent := ids[ev.UID]
	if parent == 0 {
		if dryNew[ev.UID] {
			im.res.Overrides.Created++
			return nil
		}
		// The series may have been imported earlier.
		id, err := im.lookup("event", ev.UID)
		if err != nil {
			return err
		}
		if id == 0 {
			im.res.Overrides.Skipped++
			im.warn("override of %q has no series in the file or the database, skipped", ev.UID)
			return nil
		}
		parent = id
	}
	anchor := tz.FormatInstant(*ev.RecurrenceID)
	if ev.AllDay {
		anchor = ev.RecurrenceID.Format(tz.DateLayout)
	}
	in := im.input(ev)
	in.RRule, in.Exdates = "", nil
	_, outcome, err := im.er.UpsertOverride(im.ctx, parent, anchor, in, im.opt.DryRun)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) || errors.Is(err, model.ErrValidation) {
			im.res.Overrides.Skipped++
			im.warn("override of %q at %s skipped: %v", ev.UID, anchor, err)
			return nil
		}
		return err
	}
	im.res.Overrides.add(outcome)
	return nil
}

func priorityOf(p int) string {
	switch {
	case p >= 1 && p <= 4:
		return "high"
	case p >= 6 && p <= 9:
		return "low"
	}
	return "medium"
}

func (im *importer) todo(td *ics.Todo) error {
	id, err := im.lookup("task", td.UID)
	if err != nil {
		return err
	}
	if td.RRule != "" {
		im.warn("task %q: the repeat rule is not kept, tasks repeat by their own recurrence", td.UID)
	}
	var due *string
	if td.Due != nil {
		s := td.Due.Format(tz.DateLayout)
		if !td.DueIsDate {
			s = tz.FormatInstant(*td.Due)
		}
		due = &s
	}
	var desc *string
	if td.Description != "" {
		d := td.Description
		desc = &d
	}
	prio := priorityOf(td.Priority)
	completed := td.Status == "COMPLETED" || td.Completed != nil

	if id == 0 {
		im.res.Todos.add(repo.OutcomeCreated)
		if im.opt.DryRun {
			return nil
		}
		t, err := im.tr.Create(im.ctx, model.CreateTaskInput{Title: td.Summary, Description: desc, DueAt: due, Priority: prio})
		if err != nil {
			return err
		}
		if completed {
			if _, err := im.tr.Complete(im.ctx, t.ID); err != nil {
				return err
			}
		}
		return im.link("task", td.UID, t.ID)
	}
	cur, err := im.tr.FindByID(im.ctx, id)
	if err != nil {
		return err
	}
	changed := cur.Title != td.Summary || cur.Priority != prio || (desc != nil && (cur.Description == nil || *cur.Description != *desc)) ||
		(due != nil && (cur.DueAt == nil || *cur.DueAt != *due)) || completed != cur.Completed
	if !changed {
		im.res.Todos.add(repo.OutcomeUnchanged)
		return im.link("task", td.UID, id)
	}
	im.res.Todos.add(repo.OutcomeUpdated)
	if im.opt.DryRun {
		return nil
	}
	in := model.UpdateTaskInput{Title: &td.Summary, Priority: &prio, Description: desc, DueAt: due}
	if _, err := im.tr.Update(im.ctx, id, in); err != nil {
		return err
	}
	if completed && !cur.Completed {
		if _, err := im.tr.Complete(im.ctx, id); err != nil {
			return err
		}
	} else if !completed && cur.Completed {
		if _, err := im.tr.Reopen(im.ctx, id); err != nil {
			return err
		}
	}
	return im.link("task", td.UID, id)
}
