package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/recur"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/mattn/go-runewidth"
)

// Calendar view: events, birthdays, keep-in-touch dates and tasks by day. Agenda and Day are
// lists with a detail pane. Week and Month draw a grid and fall back to the Agenda list on a
// narrow terminal. The list rows are the single source of selection in every mode.

type calMode int

const (
	calAgenda calMode = iota
	calDay
	calWeek
	calMonth
)

var calModeNames = [...]string{"Agenda", "Day", "Week", "Month"}

const (
	calAgendaDays = 14
	// calGridMinWidth is the narrowest terminal that draws the Week and Month grids.
	calGridMinWidth = 70
)

// calItem is one thing on a day.
type calItem struct {
	kind     string // event, birthday, due, task, empty, day
	date     time.Time
	inst     *model.EventInstance
	personID int64
	taskID   int64
	rank     int
	clock    string // "12:30" for a timed event, sort key within the day
	title    string
	sub      string
	cont     bool // a later day of a multi-day event
}

// Display order within a day.
const (
	rankAllDay = iota
	rankBirthday
	rankTimed
	rankTask
	rankDue
)

type calState struct {
	mode      calMode
	day       time.Time // the anchor: the selected day, or the first day of the agenda
	birthdays bool
	due       bool
	tasks     bool

	byDay    map[string][]*calItem
	from, to time.Time // loaded dates, to exclusive

	// pending carries the target of a scoped edit or delete across a picker and a prompt.
	pending *calPending

	// cells are the day rectangles of the last grid drawn, for mouse clicks.
	cells []calCell
}

type calPending struct {
	eventID int64
	scope   string
	anchor  string
}

func newCalState() calState {
	return calState{mode: calAgenda, birthdays: true, tasks: true}
}

func dayKey(d time.Time) string { return d.Format(tz.DateLayout) }

func floatDate(t time.Time, loc *time.Location) time.Time {
	w := tz.Wall(t, loc)
	return time.Date(w.Year(), w.Month(), w.Day(), 0, 0, 0, 0, time.UTC)
}

func mondayOf(d time.Time) time.Time {
	back := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -back)
}

func (m *Model) calToday() time.Time { return floatDate(m.now(), m.loc) }

// eff is the mode actually drawn: Week and Month need room for a grid.
func (m *Model) calEff() calMode {
	if (m.cal.mode == calWeek || m.cal.mode == calMonth) && m.width < calGridMinWidth {
		return calAgenda
	}
	return m.cal.mode
}

func (m *Model) calGrid() bool {
	e := m.calEff()
	return m.view == viewCalendar && (e == calWeek || e == calMonth)
}

// calWindow is the range of dates the current mode shows, to exclusive.
func (m *Model) calWindow() (time.Time, time.Time) {
	day := m.cal.day
	switch m.calEff() {
	case calDay:
		return day, day.AddDate(0, 0, 1)
	case calWeek:
		f := mondayOf(day)
		return f, f.AddDate(0, 0, 7)
	case calMonth:
		f := mondayOf(time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC))
		return f, f.AddDate(0, 0, 42)
	}
	return day, day.AddDate(0, 0, calAgendaDays)
}

// calLoad reads events and the overlays for the window into cal.byDay.
func (m *Model) calLoad() error {
	if m.cal.day.IsZero() {
		m.cal.day = m.calToday()
	}
	from, to := m.calWindow()
	m.cal.from, m.cal.to = from, to
	byDay := map[string][]*calItem{}
	add := func(it *calItem) { byDay[dayKey(it.date)] = append(byDay[dayKey(it.date)], it) }

	list, _, err := m.er.List(m.ctx, model.EventFilters{From: tz.Instant(from, m.loc), To: tz.Instant(to, m.loc), Loc: m.loc})
	if err != nil {
		return err
	}
	for i := range list {
		in := &list[i]
		var first, last time.Time
		var clock, span string
		if in.AllDay {
			first, _ = tz.ParseDate(in.StartDate)
			end, _ := tz.ParseDate(in.EndDate)
			last = end.AddDate(0, 0, -1)
		} else {
			s, _ := tz.ParseInstant(in.Start)
			e, _ := tz.ParseInstant(in.End)
			ls, le := s.In(m.loc), e.In(m.loc)
			first, last = floatDate(s, m.loc), floatDate(e, m.loc)
			if e.After(s) && le.Hour() == 0 && le.Minute() == 0 && le.Second() == 0 {
				last = last.AddDate(0, 0, -1) // ends at midnight: the last day is the one before
			}
			if last.Before(first) {
				last = first
			}
			clock = ls.Format("15:04")
			span = ls.Format("15:04") + "-" + le.Format("15:04")
		}
		for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
			if d.Before(from) || !d.Before(to) {
				continue
			}
			it := &calItem{kind: "event", date: d, inst: in, clock: clock, rank: rankAllDay, title: cleanCell(in.Title), cont: d.After(first)}
			if !in.AllDay {
				it.rank = rankTimed
				if it.cont {
					it.clock = ""
					it.rank = rankAllDay
					it.sub = "continues"
				} else {
					it.sub = span
				}
			}
			if in.Location != "" {
				it.sub = strings.TrimSpace(it.sub + " " + cleanCell(in.Location))
			}
			add(it)
		}
	}

	today := m.calToday()
	days := int(to.Sub(from).Hours() / 24)
	if m.cal.birthdays {
		bds, err := m.pr.Birthdays(m.ctx, days, from)
		if err != nil {
			return err
		}
		for _, b := range bds {
			d, err := tz.ParseDate(b.Date)
			if err != nil || d.Before(from) || !d.Before(to) {
				continue
			}
			sub := "birthday"
			if b.Turning != nil {
				sub = fmt.Sprintf("birthday, turns %d", *b.Turning)
			}
			add(&calItem{kind: "birthday", date: d, personID: b.PersonID, rank: rankBirthday, title: cleanCell(b.Name), sub: sub})
		}
	}
	if m.cal.due {
		due, err := m.pr.Due(m.ctx, 400, 1000)
		if err != nil {
			return err
		}
		for _, c := range due {
			d, ok := dayOf(c.DueAt)
			if !ok {
				continue
			}
			sub := "reach out"
			if d.Before(today) {
				d, sub = today, "reach out, overdue"
			}
			if d.Before(from) || !d.Before(to) {
				continue
			}
			add(&calItem{kind: "due", date: d, personID: c.PersonID, rank: rankDue, title: cleanCell(c.Name), sub: sub})
		}
	}
	if m.cal.tasks {
		tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{})
		if err != nil {
			return err
		}
		for _, t := range tasks {
			if t.ScheduledStart != nil {
				if s, err := tz.ParseInstant(*t.ScheduledStart); err == nil {
					d := floatDate(s, m.loc)
					if !d.Before(from) && d.Before(to) {
						sub := "scheduled"
						if t.DurationMin != nil {
							sub += ", " + lengthText(minutesDuration(*t.DurationMin))
						}
						it := &calItem{kind: "task", date: d, taskID: t.ID, rank: rankTimed, clock: s.In(m.loc).Format("15:04"), title: cleanCell(t.Title), sub: sub}
						if len(t.PersonIDs) > 0 {
							it.personID = t.PersonIDs[0]
						}
						add(it)
						continue
					}
				}
			}
			if t.DueAt == nil {
				continue
			}
			d, ok := dayOf(*t.DueAt)
			if !ok {
				continue
			}
			sub := "due"
			if d.Before(today) {
				d, sub = today, "due, overdue"
			}
			if d.Before(from) || !d.Before(to) {
				continue
			}
			it := &calItem{kind: "task", date: d, taskID: t.ID, rank: rankTask, title: cleanCell(t.Title), sub: sub}
			if len(t.PersonIDs) > 0 {
				it.personID = t.PersonIDs[0]
			} else if t.PersonID != nil {
				it.personID = *t.PersonID
			}
			add(it)
		}
	}
	for k := range byDay {
		items := byDay[k]
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].rank != items[j].rank {
				return items[i].rank < items[j].rank
			}
			if items[i].clock != items[j].clock {
				return items[i].clock < items[j].clock
			}
			return items[i].title < items[j].title
		})
	}
	m.cal.byDay = byDay
	return nil
}

// calRows builds the list rows of the current window.
func (m *Model) calRows() []row {
	mode := m.calEff()
	today := m.calToday()
	var rows []row
	for d := m.cal.from; d.Before(m.cal.to); d = d.AddDate(0, 0, 1) {
		items := m.cal.byDay[dayKey(d)]
		if mode == calAgenda && len(items) == 0 && !d.Equal(m.cal.day) && !d.Equal(today) {
			continue
		}
		head := row{title: d.Format("Mon 2 Jan"), skip: true, cal: &calItem{kind: "day", date: d}}
		switch {
		case d.Equal(today):
			head.meta = "today"
		case d.Equal(today.AddDate(0, 0, 1)):
			head.meta = "tomorrow"
		}
		if mode == calMonth && d.Month() != m.cal.day.Month() {
			head.meta = strings.TrimSpace(head.meta + " " + d.Format("Jan"))
		}
		rows = append(rows, head)
		if len(items) == 0 {
			rows = append(rows, row{title: "nothing planned", cal: &calItem{kind: "empty", date: d}})
			continue
		}
		for _, it := range items {
			r := row{title: it.title, sub: it.sub, cal: it}
			switch it.kind {
			case "event":
				if it.clock != "" {
					r.title = it.clock + " " + it.title
					r.sub = strings.TrimPrefix(it.sub, it.clock)
					r.sub = strings.TrimSpace(r.sub)
				}
				if it.inst.Recurring {
					r.meta = "repeats"
				}
				if it.inst.Status == model.EventTentative {
					r.meta = strings.TrimSpace(r.meta + " ?")
				}
			case "birthday":
				r.title = "* " + it.title
			case "due":
				r.title = "~ " + it.title
			case "task":
				r.title = "[ ] " + it.title
				if it.clock != "" {
					r.title = it.clock + " [ ] " + it.title
				}
			}
			r.personID = it.personID
			r.taskID = it.taskID
			rows = append(rows, r)
		}
	}
	return rows
}

// calRowKey identifies a row across rebuilds.
func calRowKey(r row) string {
	if r.cal == nil {
		return ""
	}
	id := int64(0)
	occ := ""
	if r.cal.inst != nil {
		id, occ = r.cal.inst.EventID, r.cal.inst.Occurrence
	}
	return fmt.Sprintf("%s|%s|%d|%s|%d|%d", r.cal.kind, dayKey(r.cal.date), id, occ, r.personID, r.taskID)
}

// calCursor picks the cursor row after a rebuild: the row with the same key, else the first
// selectable row on the anchor day or later, else the last selectable row.
func (m *Model) calCursor(key string) int {
	if key != "" {
		for i, r := range m.rows {
			if !r.skip && calRowKey(r) == key {
				return i
			}
		}
	}
	return m.calFirstOnOrAfter(m.cal.day)
}

func (m *Model) calFirstOnOrAfter(day time.Time) int {
	last := -1
	for i, r := range m.rows {
		if r.skip || r.cal == nil {
			continue
		}
		last = i
		if !r.cal.date.Before(day) {
			return i
		}
	}
	if last < 0 {
		return 0
	}
	return last
}

// calSelected is the calendar item under the cursor, or nil.
func (m *Model) calSelected() *calItem {
	if m.view != viewCalendar || m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].cal
}

// calSelDate is the day the selection is on.
func (m *Model) calSelDate() time.Time {
	if it := m.calSelected(); it != nil {
		return it.date
	}
	return m.cal.day
}

// calTrack keeps the anchor on the selected day (Agenda keeps its own start).
func (m *Model) calTrack() {
	if m.view != viewCalendar || m.calEff() == calAgenda {
		return
	}
	if it := m.calSelected(); it != nil {
		m.cal.day = it.date
	}
}

// calReload loads the window and rebuilds the rows, then places the cursor on the day.
func (m *Model) calReload(day time.Time) {
	m.cal.day = day
	m.report(m.rebuild(false))
	m.cursor = m.calFirstOnOrAfter(day)
	m.settle()
	m.clamp()
	m.refreshDetail(true)
}

// calGoto moves the anchor to a date and selects it.
func (m *Model) calGoto(day time.Time) {
	m.focus = focusList
	if m.view != viewCalendar {
		m.view = viewCalendar
		m.marked = map[int64]bool{}
		m.setStatus("", false)
	}
	m.calReload(day)
}

// calMoveDay moves the selection by whole days, loading a new window when it leaves the old one.
func (m *Model) calMoveDay(delta int) {
	target := m.calSelDate().AddDate(0, 0, delta)
	if target.Before(m.cal.from) || !target.Before(m.cal.to) {
		m.calReload(target)
		return
	}
	m.cal.day = target
	m.cursor = m.calFirstOnOrAfter(target)
	m.settle()
	m.clamp()
	m.refreshDetail(false)
}

// calStep moves the window by one period: a week for the Agenda and Week, a day for Day, a month
// for Month.
func (m *Model) calStep(dir int) {
	d := m.calSelDate()
	if m.calEff() == calAgenda {
		d = m.cal.day
	}
	switch m.calEff() {
	case calAgenda, calWeek:
		d = d.AddDate(0, 0, 7*dir)
	case calDay:
		d = d.AddDate(0, 0, dir)
	case calMonth:
		first := time.Date(d.Year(), d.Month()+time.Month(dir), 1, 0, 0, 0, 0, time.UTC)
		last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		d = time.Date(first.Year(), first.Month(), min(d.Day(), last), 0, 0, 0, 0, time.UTC)
	}
	m.calReload(d)
}

func (m *Model) calSetMode(mode calMode) {
	m.cal.mode = mode
	m.calReload(m.calSelDate())
	m.setStatus(calModeNames[mode]+" view", false)
}

func (m *Model) calToggle(which *bool, name string) {
	*which = !*which
	state := "hidden"
	if *which {
		state = "shown"
	}
	m.report(m.rebuild(true))
	m.setStatus(name+" "+state, false)
}

// calCountText is the right side of the tab bar.
func (m *Model) calCountText() string {
	switch m.calEff() {
	case calMonth:
		return m.cal.day.Format("January 2006")
	case calWeek:
		f := mondayOf(m.cal.day)
		return f.Format("2 Jan") + " - " + f.AddDate(0, 0, 6).Format("2 Jan 2006")
	case calDay:
		return m.cal.day.Format("Mon 2 Jan 2006")
	}
	return "Agenda from " + m.cal.day.Format("2 Jan")
}

// calDetail is the detail pane text of the selected item.
func (m *Model) calDetail() string {
	w := max(1, m.detail.Width)
	line := func(s string) string { return runewidth.Truncate(s, w, "…") }
	kv := func(k, v string) string { return dimStyle.Render(fit(k, labelWidth)) + line(v) + "\n" }
	it := m.calSelected()
	if it == nil {
		return dimStyle.Render("Nothing selected.")
	}
	var b strings.Builder
	switch it.kind {
	case "event":
		e, err := m.er.FindByID(m.ctx, it.inst.EventID)
		if err != nil {
			return errStyle.Render(line(err.Error()))
		}
		b.WriteString(nameStyle.Render(line(cleanCell(e.Title))) + dimStyle.Render(fmt.Sprintf("  #%d", e.ID)) + "\n\n")
		b.WriteString(kv("when", m.calWhen(it.inst)))
		if e.RRule != nil {
			b.WriteString(kv("repeats", recur.Describe(*e.RRule)))
		}
		if e.Location != nil {
			b.WriteString(kv("where", cleanCell(*e.Location)))
		}
		if e.Status != model.EventConfirmed {
			b.WriteString(kv("status", e.Status))
		}
		if !e.Busy {
			b.WriteString(kv("shows as", "free"))
		}
		if len(e.Attendees) == 0 && it.inst.Attendees > 0 {
			b.WriteString(kv("people", fmt.Sprint(it.inst.Attendees)))
		}
		for i, a := range e.Attendees {
			label := ""
			if i == 0 {
				label = "people"
			}
			b.WriteString(kv(label, fmt.Sprintf("%s (%s)", cleanCell(a.Name), a.RSVP)))
		}
		for i, rm := range e.Reminders {
			label := ""
			if i == 0 {
				label = "reminders"
			}
			b.WriteString(kv(label, fmt.Sprintf("%s before (%s)", lengthText(time.Duration(rm.MinutesBefore)*time.Minute), rm.Method)))
		}
		if len(e.Tags) > 0 {
			b.WriteString(kv("tags", strings.Join(e.Tags, ", ")))
		}
		if e.Notes != nil && strings.TrimSpace(*e.Notes) != "" {
			b.WriteString("\n")
			for _, l := range strings.Split(strings.TrimSpace(*e.Notes), "\n") {
				b.WriteString(line(cleanCell(l)) + "\n")
			}
		}
		b.WriteString("\n" + dimStyle.Render(line("enter edits, D deletes, palette: attendees, reminders, tags")))
	case "empty":
		b.WriteString(nameStyle.Render(it.date.Format("Monday 2 January")) + "\n\n")
		b.WriteString(dimStyle.Render(line("Nothing planned. + adds an event.")))
	default:
		b.WriteString(nameStyle.Render(line(it.title)) + "\n\n" + dimStyle.Render(line(it.sub)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// calWhen renders an instance's time in the display zone, plus the event's own zone if it differs.
func (m *Model) calWhen(in *model.EventInstance) string {
	if in.AllDay {
		s, _ := tz.ParseDate(in.StartDate)
		e, _ := tz.ParseDate(in.EndDate)
		last := e.AddDate(0, 0, -1)
		if last.After(s) {
			return s.Format("Mon 2 Jan") + " - " + last.Format("Mon 2 Jan") + ", all day"
		}
		return s.Format("Mon 2 Jan") + ", all day"
	}
	s, _ := tz.ParseInstant(in.Start)
	e, _ := tz.ParseInstant(in.End)
	ls, le := s.In(m.loc), e.In(m.loc)
	out := ls.Format("Mon 2 Jan 15:04") + "-" + le.Format("15:04")
	if le.YearDay() != ls.YearDay() || le.Year() != ls.Year() {
		out = ls.Format("Mon 2 Jan 15:04") + " - " + le.Format("Mon 2 Jan 15:04")
	}
	if in.TZ != "" && in.TZ != tz.Name(m.loc) {
		if loc, err := tz.Load(in.TZ); err == nil {
			out += "  (" + s.In(loc).Format("15:04") + " " + in.TZ + ")"
		}
	}
	return out
}

// lengthText renders a duration as 1h30m, 45m or 2d.
func lengthText(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	h, mi := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", mi)
	case mi == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, mi)
}
