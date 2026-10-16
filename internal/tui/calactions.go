package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/cal/icsio"
	"github.com/jdanielnd/crm-cli/internal/cal/input"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

// Context predicates for the calendar actions.
func whenCalendar(m *Model) bool { return m.view == viewCalendar && m.cmp == nil }

func whenCalList(m *Model) bool { return whenCalendar(m) && m.focus == focusList }

func whenCalEvent(m *Model) bool {
	it := m.calSelected()
	return whenCalList(m) && it != nil && it.kind == "event"
}

func whenCalGrid(m *Model) bool { return whenCalendar(m) && m.calGrid() && m.focus == focusList }

func whenCalOpen(m *Model) bool {
	it := m.calSelected()
	return whenCalendar(m) && m.focus == focusList && it != nil && it.kind != "day"
}

func whenCalRestore(m *Model) bool { return whenCalendar(m) && len(m.deletedEvents) > 0 }

// ---- add and edit ----

// quickEvent is a parsed quick add or edit line: "Lunch with @Sam fri 12:30 for 1h at Cafe Rio every week #food".
type quickEvent struct {
	title      string
	start, end time.Time
	allDay     bool
	rule       *string
	hasWhen    bool
	location   string
	tags       []string
	people     []string // names after @, resolved when the event is saved
}

// parseQuick reads a quick add or edit line with the shared short syntax. See parseQuickSyntax.
func (m *Model) parseQuick(value string) (quickEvent, error) {
	return m.parseQuickSyntax(value)
}

// parseQuickSyntax reads a quick add line with the shared short syntax: the title with a date or
// time (fri 12:30), a length (for 1h), a place (at Cafe Rio), a repeat (every week), attendees
// (@Sam) and tags (#food). Without a date the event is all-day on the selected day, and a bare
// time means that time on the selected day.
func (m *Model) parseQuickSyntax(value string) (quickEvent, error) {
	day := m.calSelDate()
	base := tz.Instant(day, m.loc)
	if day.Equal(m.calToday()) {
		base = m.now()
	}
	env := m.quickEnv()
	res := shortsyntax.Parse(value, shortsyntax.Options{
		Now: base.In(m.loc), Disabled: m.noSyntax, Dates: true, Phrases: true, Names: env.Index.NameSet(),
		Skip: []shortsyntax.Kind{shortsyntax.KindPriority, shortsyntax.KindDeadline, shortsyntax.KindCadence},
	})
	q := quickEvent{title: strings.TrimSpace(res.Title), location: res.Location, tags: res.Tags, people: res.People}
	if q.title == "" {
		return q, fmt.Errorf("type a title first")
	}
	if res.Due == nil {
		q.start, q.allDay = day, true
	} else {
		q.hasWhen = true
		span := input.SpanText{Start: res.Due.String()}
		if res.DurationMin > 0 {
			span.Duration = shortsyntax.FormatMinutes(res.DurationMin)
		}
		start, end, allDay, err := input.Span(span, m.loc, base)
		if err != nil {
			return q, err
		}
		q.start, q.end, q.allDay = start, end, allDay
	}
	if res.Recurrence != "" {
		rule, err := input.Rule(input.RepeatText{Repeat: res.Recurrence, Changed: true}, "", q.allDay, m.loc, m.now())
		if err != nil {
			return q, err
		}
		q.rule = rule
	}
	return q, nil
}

// calAttendees resolves the @names of a quick add to people. A name that matches nobody or
// several people stops the save, so an event never links the wrong person.
func (m *Model) calAttendees(names []string) ([]model.EventAttendee, error) {
	var out []model.EventAttendee
	ix := m.quickEnv().Index
	for _, name := range names {
		matches, _ := ix.Resolve(name)
		if len(matches) == 0 {
			matches, _ = ix.ResolveLoose(name)
		}
		switch len(matches) {
		case 1:
			out = append(out, model.EventAttendee{PersonID: matches[0].ID})
		case 0:
			return nil, fmt.Errorf("no person matches @%s", name)
		default:
			return nil, fmt.Errorf("%d people match @%s: use the full name", len(matches), name)
		}
	}
	return out, nil
}

// calPromptHint previews what the quick add line will create, or says what is wrong.
func (m *Model) calPromptHint() (string, bool) {
	q, err := m.parseQuick(m.input.Value())
	if err != nil {
		return err.Error(), true
	}
	end := q.end
	if q.allDay {
		return q.start.Format("Mon 2 Jan") + ", all day" + ruleHint(q.rule), false
	}
	if end.IsZero() {
		end = q.start.Add(time.Hour)
	}
	ls, le := q.start.In(m.loc), end.In(m.loc)
	return ls.Format("Mon 2 Jan 15:04") + "-" + le.Format("15:04") + ruleHint(q.rule), false
}

func ruleHint(r *string) string {
	if r == nil || *r == "" {
		return ""
	}
	return ", repeats"
}

func (m *Model) startCalAdd() tea.Cmd {
	if m.view != viewCalendar {
		m.calGoto(m.calToday())
	}
	m.cal.pending = nil
	m.startInput("cal-add", "event on "+m.calSelDate().Format("Mon 2 Jan")+" (Lunch @Sam fri 12:30 for 1h at Cafe Rio every week): ", "")
	return m.input.Focus()
}

// calEventTarget returns the selected event instance.
func (m *Model) calEventTarget() *model.EventInstance {
	it := m.calSelected()
	if it == nil || it.kind != "event" {
		return nil
	}
	return it.inst
}

// calScopes asks which instances a change applies to when the event repeats. A single event goes
// straight to next.
func (m *Model) calScopes(verb string, in *model.EventInstance, next func(scope string)) {
	if !in.Recurring {
		next(model.ScopeAll)
		return
	}
	items := []overlayItem{
		{id: model.ScopeThis, label: "This event", desc: verb + " only the " + m.calOccurrenceLabel(in) + " instance", value: model.ScopeThis},
		{id: model.ScopeFollowing, label: "This and following events", desc: verb + " this instance and every later one", value: model.ScopeFollowing},
		{id: model.ScopeAll, label: "All events", desc: verb + " the whole series", value: model.ScopeAll},
	}
	m.openPicker(strings.ToUpper(verb[:1])+verb[1:]+" repeating event", items, func(m *Model, it overlayItem) { next(it.value) })
}

func (m *Model) calOccurrenceLabel(in *model.EventInstance) string {
	if in.AllDay {
		d, _ := tz.ParseDate(in.StartDate)
		return d.Format("2 Jan")
	}
	s, _ := tz.ParseInstant(in.Start)
	return s.In(m.loc).Format("2 Jan 15:04")
}

// calEdit opens the edit line prefilled with the event in the short syntax: "Lunch 2026-10-16 12:30 for 1h".
func (m *Model) calEdit() {
	in := m.calEventTarget()
	if in == nil {
		return
	}
	snapshot := *in
	m.pendingCmd = nil
	m.calScopes("edit", &snapshot, func(scope string) {
		// Always address the series: a scoped change finds the instance's own row by its start.
		anchor := ""
		if scope == model.ScopeThis || scope == model.ScopeFollowing {
			anchor = snapshot.Occurrence
		}
		m.cal.pending = &calPending{eventID: snapshot.SeriesID, scope: scope, anchor: anchor}
		text := cleanCell(snapshot.Title) + " "
		if snapshot.AllDay {
			s, _ := tz.ParseDate(snapshot.StartDate)
			e, _ := tz.ParseDate(snapshot.EndDate)
			text += s.Format(tz.DateLayout)
			if days := e.Sub(s); days > 24*time.Hour {
				text += " for " + lengthText(days)
			}
		} else {
			s, _ := tz.ParseInstant(snapshot.Start)
			e, _ := tz.ParseInstant(snapshot.End)
			text += s.In(m.loc).Format("2006-01-02 15:04") + " for " + lengthText(e.Sub(s))
		}
		m.startInput("cal-edit", "edit (title, date, for 1h, at place, every week): ", text)
		m.pendingCmd = m.input.Focus()
	})
}

// ---- prompt results ----

func (m *Model) runCalPrompt(kind, value string) {
	switch kind {
	case "cal-add":
		m.calCreate(value)
	case "cal-edit":
		m.calUpdate(value)
	case "cal-goto":
		m.calGotoText(value)
	case "cal-remind":
		m.calSetReminders(value)
	case "cal-tag":
		m.calTag(value)
	case "cal-zone":
		m.calSetZone(value)
	case "cal-import":
		m.calImportPath(value)
	case "cal-export":
		m.calExportPath(value)
	case "cal-hours":
		m.calSetHours(value)
	case "cal-free":
		m.calFreeRun(value)
	case "cal-invite":
		m.calInviteRun(value)
	}
}

func (m *Model) calCreate(value string) {
	q, err := m.parseQuick(value)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	in := model.CreateEventInput{Title: q.title, AllDay: q.allDay, Start: q.start, End: q.end, TZ: tz.Name(m.loc), Tags: q.tags}
	if q.location != "" {
		loc := q.location
		in.Location = &loc
	}
	if q.rule != nil {
		in.RRule = *q.rule
	}
	atts, err := m.calAttendees(q.people)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	in.Attendees = atts
	e, err := m.er.Create(m.ctx, in)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	day := q.start
	if !q.allDay {
		day = floatDate(q.start, m.loc)
	}
	id := e.ID
	m.record("add event "+truncateLabel(e.Title),
		func() error { return m.er.Archive(m.ctx, id, model.ScopeAll, "") },
		func() error { return m.er.Restore(m.ctx, id) })
	m.afterWriteCal(day, e.ID)
	m.setStatus("event added", false)
}

// afterWriteCal reloads after a calendar write and selects the event (or the day).
func (m *Model) afterWriteCal(day time.Time, eventID int64) {
	if day.Before(m.cal.from) || !day.Before(m.cal.to) {
		m.calReload(day)
	} else {
		m.report(m.reload())
	}
	for i, r := range m.rows {
		if r.cal != nil && r.cal.inst != nil && (r.cal.inst.EventID == eventID || r.cal.inst.SeriesID == eventID) && !r.cal.cont {
			m.cursor = i
			m.calTrack()
			m.clamp()
			m.refreshDetail(true)
			return
		}
	}
}

func (m *Model) calUpdate(value string) {
	p := m.cal.pending
	m.cal.pending = nil
	if p == nil {
		return
	}
	q, err := m.parseQuick(value)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	cur, err := m.er.FindByID(m.ctx, p.eventID)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if len(q.tags) > 0 || len(q.people) > 0 {
		m.setStatus("tags and attendees are not changed here: use Calendar: tag event or add attendee", true)
		return
	}
	var up model.UpdateEventInput
	if q.location != "" && q.location != str(cur.Location) {
		loc := q.location
		up.Location = &loc
	}
	if q.title != cur.Title {
		t := q.title
		up.Title = &t
	}
	if q.hasWhen {
		start, end := q.start, q.end
		if end.IsZero() {
			// No length given: keep the current one.
			if cs, ce, ok := eventSpanOf(cur); ok {
				end = start.Add(ce.Sub(cs))
			}
		}
		if cs, ce, ok := eventSpanOf(cur); !ok || !cs.Equal(start) || !ce.Equal(end) || cur.AllDay != q.allDay {
			up.Start, up.End = &start, &end
			if cur.AllDay != q.allDay {
				up.AllDay = &q.allDay
			}
		}
	}
	if q.rule != nil && p.scope == model.ScopeAll {
		up.RRule = q.rule
	}
	if up == (model.UpdateEventInput{}) {
		m.setStatus("nothing changed", false)
		return
	}
	e, err := m.er.Update(m.ctx, p.eventID, up, p.scope, p.anchor)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	day := m.calSelDate()
	if s, _, ok := eventSpanOf(e); ok {
		if e.AllDay {
			day = s
		} else {
			day = floatDate(s, m.loc)
		}
	}
	m.afterWriteCal(day, e.ID)
	m.setStatus("event updated", false)
}

func eventSpanOf(e *model.Event) (start, end time.Time, ok bool) {
	var err error
	if e.AllDay {
		if e.StartDate == nil || e.EndDate == nil {
			return
		}
		if start, err = tz.ParseDate(*e.StartDate); err != nil {
			return
		}
		end, err = tz.ParseDate(*e.EndDate)
		return start, end, err == nil
	}
	if e.StartUTC == nil || e.EndUTC == nil {
		return
	}
	if start, err = tz.ParseInstant(*e.StartUTC); err != nil {
		return
	}
	end, err = tz.ParseInstant(*e.EndUTC)
	return start, end, err == nil
}

// ---- delete, restore ----

func (m *Model) calDelete() {
	in := m.calEventTarget()
	if in == nil {
		return
	}
	snapshot := *in
	m.calScopes("delete", &snapshot, func(scope string) {
		target, anchor := snapshot.SeriesID, ""
		if scope == model.ScopeThis || scope == model.ScopeFollowing {
			anchor = snapshot.Occurrence
		}
		what := fmt.Sprintf("delete %q", cleanCell(snapshot.Title))
		if scope == model.ScopeThis {
			what += " on " + m.calOccurrenceLabel(&snapshot)
		} else if scope == model.ScopeFollowing {
			what += " from " + m.calOccurrenceLabel(&snapshot) + " on"
		}
		m.mode = modeConfirm
		m.confirm = &confirmState{prompt: what + "? y/n", yes: func() {
			if err := m.er.Archive(m.ctx, target, scope, anchor); err != nil {
				m.setStatus(err.Error(), true)
				return
			}
			if scope == model.ScopeAll {
				m.deletedEvents = append(m.deletedEvents, target)
				m.record("delete event "+truncateLabel(snapshot.Title),
					func() error { return m.er.Restore(m.ctx, target) },
					func() error { return m.er.Archive(m.ctx, target, model.ScopeAll, "") })
			}
			m.afterWriteCal(m.calSelDate(), 0)
			m.setStatus("event deleted (palette: Restore last deleted event)", false)
		}}
	})
}

func (m *Model) calRestore() {
	if len(m.deletedEvents) == 0 {
		m.setStatus("no deleted event to restore in this session", true)
		return
	}
	id := m.deletedEvents[len(m.deletedEvents)-1]
	if err := m.er.Restore(m.ctx, id); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.deletedEvents = m.deletedEvents[:len(m.deletedEvents)-1]
	m.afterWriteCal(m.calSelDate(), id)
	m.setStatus("event restored", false)
}

// ---- attendees, reminders, tags, status ----

func (m *Model) calAddAttendee() {
	in := m.calEventTarget()
	if in == nil {
		return
	}
	eventID := in.EventID
	items := make([]overlayItem, 0, len(m.people))
	for _, p := range m.people {
		desc := cleanCell(p.Company)
		if desc == "" {
			desc = cleanCell(p.Email)
		}
		items = append(items, overlayItem{id: strconv.FormatInt(p.ID, 10), label: cleanCell(p.Name), desc: desc, value: strconv.FormatInt(p.ID, 10)})
	}
	m.openPicker("Add attendee", items, func(m *Model, it overlayItem) {
		pid, _ := strconv.ParseInt(it.value, 10, 64)
		if err := m.er.SetAttendee(m.ctx, eventID, model.EventAttendee{PersonID: pid}); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWriteCal(m.calSelDate(), eventID)
		m.setStatus(it.label+" added to the event", false)
	})
}

func (m *Model) calRemoveAttendee() {
	in := m.calEventTarget()
	if in == nil {
		return
	}
	e, err := m.er.FindByID(m.ctx, in.EventID)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if len(e.Attendees) == 0 {
		m.setStatus("this event has no attendees", true)
		return
	}
	eventID := e.ID
	items := make([]overlayItem, 0, len(e.Attendees))
	for _, a := range e.Attendees {
		items = append(items, overlayItem{id: strconv.FormatInt(a.PersonID, 10), label: cleanCell(a.Name), desc: a.Role + ", " + a.RSVP, value: strconv.FormatInt(a.PersonID, 10)})
	}
	m.openPicker("Remove attendee", items, func(m *Model, it overlayItem) {
		pid, _ := strconv.ParseInt(it.value, 10, 64)
		if err := m.er.RemoveAttendee(m.ctx, eventID, pid); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWriteCal(m.calSelDate(), eventID)
		m.setStatus(it.label+" removed from the event", false)
	})
}

// calOpenAttendee jumps to one of the event's people.
func (m *Model) calOpenAttendee() {
	in := m.calEventTarget()
	if in == nil {
		return
	}
	e, err := m.er.FindByID(m.ctx, in.EventID)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if len(e.Attendees) == 0 {
		m.setStatus("this event has no attendees", true)
		return
	}
	items := make([]overlayItem, 0, len(e.Attendees))
	for _, a := range e.Attendees {
		items = append(items, overlayItem{id: strconv.FormatInt(a.PersonID, 10), label: cleanCell(a.Name), desc: a.Role, value: strconv.FormatInt(a.PersonID, 10)})
	}
	m.openPicker("Open attendee", items, func(m *Model, it overlayItem) {
		pid, _ := strconv.ParseInt(it.value, 10, 64)
		m.openPerson(pid)
	})
}

func (m *Model) calStartReminders() tea.Cmd {
	in := m.calEventTarget()
	if in == nil {
		return nil
	}
	e, err := m.er.FindByID(m.ctx, in.EventID)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	var cur []string
	for _, rm := range e.Reminders {
		s := lengthText(time.Duration(rm.MinutesBefore) * time.Minute)
		if rm.Method != "display" {
			s += ":" + rm.Method
		}
		cur = append(cur, s)
	}
	m.cal.pending = &calPending{eventID: e.ID}
	m.startInput("cal-remind", "reminders before the start (15m, 1d:email, empty clears): ", strings.Join(cur, ", "))
	return m.input.Focus()
}

func (m *Model) calSetReminders(value string) {
	p := m.cal.pending
	m.cal.pending = nil
	if p == nil {
		return
	}
	var list []string
	for _, s := range strings.Split(value, ",") {
		if s = strings.TrimSpace(s); s != "" {
			list = append(list, s)
		}
	}
	rems, err := input.Reminders(list)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if err := m.er.SetReminders(m.ctx, p.eventID, rems); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWriteCal(m.calSelDate(), p.eventID)
	m.setStatus("reminders set", false)
}

func (m *Model) calStartTag() tea.Cmd {
	in := m.calEventTarget()
	if in == nil {
		return nil
	}
	m.cal.pending = &calPending{eventID: in.SeriesID}
	m.startInput("cal-tag", "tag the event (-name removes): ", "")
	return m.input.Focus()
}

func (m *Model) calTag(value string) {
	p := m.cal.pending
	m.cal.pending = nil
	if p == nil {
		return
	}
	remove := strings.HasPrefix(value, "-")
	name := strings.TrimLeft(strings.TrimSpace(strings.TrimPrefix(value, "-")), "#")
	if name == "" {
		m.setStatus("enter a tag name (prefix with - to remove)", true)
		return
	}
	var err error
	if remove {
		err = m.tagr.Remove(m.ctx, "event", p.eventID, name)
	} else {
		err = m.tagr.Apply(m.ctx, "event", p.eventID, name)
	}
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWriteCal(m.calSelDate(), p.eventID)
	if remove {
		m.setStatus("tag removed", false)
	} else {
		m.setStatus("tag added", false)
	}
}

// calPickStatus sets confirmed, tentative or cancelled on the event.
func (m *Model) calPickStatus() {
	in := m.calEventTarget()
	if in == nil {
		return
	}
	id := in.EventID
	items := []overlayItem{
		{id: model.EventConfirmed, label: "confirmed", value: model.EventConfirmed},
		{id: model.EventTentative, label: "tentative", desc: "shown with a ?", value: model.EventTentative},
		{id: model.EventCancelled, label: "cancelled", desc: "hidden from the calendar", value: model.EventCancelled},
	}
	m.openPicker("Event status", items, func(m *Model, it overlayItem) {
		st := it.value
		if _, err := m.er.Update(m.ctx, id, model.UpdateEventInput{Status: &st}, model.ScopeAll, ""); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWriteCal(m.calSelDate(), id)
		m.setStatus("status "+st, false)
	})
}

func (m *Model) calToggleBusy() {
	in := m.calEventTarget()
	if in == nil {
		return
	}
	busy := !in.Busy
	if _, err := m.er.Update(m.ctx, in.EventID, model.UpdateEventInput{Busy: &busy}, model.ScopeAll, ""); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWriteCal(m.calSelDate(), in.EventID)
	if busy {
		m.setStatus("shows as busy", false)
	} else {
		m.setStatus("shows as free", false)
	}
}

// ---- navigation prompts ----

func (m *Model) calGotoText(value string) {
	start, _, allDay, err := input.Span(input.SpanText{Start: value}, m.loc, m.now())
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	day := start
	if !allDay {
		day = floatDate(start, m.loc)
	}
	m.calGoto(day)
	m.setStatus("went to "+day.Format("Mon 2 Jan 2006"), false)
}

func (m *Model) calSetZone(value string) {
	loc, err := tz.Load(strings.TrimSpace(value))
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.loc = loc
	if m.view == viewCalendar {
		m.calReload(m.calToday())
	}
	m.setStatus("showing times in "+tz.Name(loc), false)
}

// ---- ICS files ----

func (m *Model) calImportPath(path string) {
	data, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	opt := icsio.ImportOptions{Loc: m.loc}
	dry := opt
	dry.DryRun = true
	res, err := icsio.Import(m.ctx, m.db, data, dry)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	summary := fmt.Sprintf("%d new, %d changed events, %d new tasks", res.Events.Created+res.Overrides.Created, res.Events.Updated+res.Overrides.Updated, res.Todos.Created)
	if res.Events.Created+res.Events.Updated+res.Overrides.Created+res.Overrides.Updated+res.Todos.Created+res.Todos.Updated == 0 {
		m.setStatus("nothing to import: all "+fmt.Sprint(res.Events.Unchanged+res.Overrides.Unchanged+res.Todos.Unchanged)+" items are up to date", false)
		return
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{prompt: "import: " + summary + ". apply? y/n", yes: func() {
		done, err := icsio.Import(m.ctx, m.db, data, opt)
		if err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWriteCal(m.calSelDate(), 0)
		msg := fmt.Sprintf("imported %d new, %d changed", done.Events.Created+done.Overrides.Created+done.Todos.Created, done.Events.Updated+done.Overrides.Updated+done.Todos.Updated)
		m.setStatus(msg, false)
	}}
}

func (m *Model) calExportPath(path string) {
	path = strings.TrimSpace(path)
	res, err := icsio.Export(m.ctx, m.db, icsio.ExportOptions{Tasks: true, Name: "crm"})
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if err := os.WriteFile(path, res.Data, 0o600); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus(fmt.Sprintf("exported %d events and %d tasks to %s", res.Events+res.Overrides, res.Todos, path), false)
}

// calOpen handles Enter on a calendar row.
func (m *Model) calOpen() tea.Cmd {
	it := m.calSelected()
	if it == nil {
		return nil
	}
	if m.calEff() == calMonth {
		m.cal.mode = calDay
		m.calReload(it.date)
		return nil
	}
	switch it.kind {
	case "event":
		m.calEdit()
		return m.pendingCmd
	case "empty":
		return m.startCalAdd()
	case "birthday", "due", "task":
		if it.personID != 0 {
			m.openPerson(it.personID)
		}
	}
	return nil
}
