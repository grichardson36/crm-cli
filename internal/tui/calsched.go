package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Scheduling fields of tasks and working hours, reachable from the Tasks and Calendar views and
// the palette.

// schedTaskID is the task the scheduling actions apply to: the task row under the cursor, in the
// Tasks, Home or Calendar view, or the task row of the detail form.
func (m *Model) schedTaskID() int64 {
	if m.focus == focusDetail && m.fcur < len(m.form) && m.form[m.fcur].kind == rkTask && !m.form[m.fcur].empty {
		return m.form[m.fcur].id
	}
	if r, ok := m.selected(); ok {
		return r.taskID
	}
	return 0
}

func whenSchedTask(m *Model) bool { return m.schedTaskID() != 0 && m.cmp == nil }

// startTaskSchedule opens "duration | start | deadline" for the selected task. Empty parts leave
// the value alone and "-" clears it.
func (m *Model) startTaskSchedule() tea.Cmd {
	id := m.schedTaskID()
	if id == 0 {
		return nil
	}
	t, err := m.tr.FindByID(m.ctx, id)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	dur, start, dl := "", "", ""
	if t.DurationMin != nil {
		dur = lengthText(minutesDuration(*t.DurationMin))
	}
	if t.ScheduledStart != nil {
		if s, err := tz.ParseInstant(*t.ScheduledStart); err == nil {
			start = s.In(m.loc).Format("2006-01-02 15:04")
		}
	}
	if t.DeadlineAt != nil {
		dl = *t.DeadlineAt
	}
	m.cal.pending = &calPending{eventID: id}
	m.startInput("task-schedule", "schedule "+cleanCell(t.Title)+" (duration | start | deadline, - clears): ", fmt.Sprintf("%s | %s | %s", dur, start, dl))
	return m.input.Focus()
}

func (m *Model) runTaskSchedule(value string) {
	p := m.cal.pending
	m.cal.pending = nil
	if p == nil {
		return
	}
	parts := strings.Split(value, "|")
	get := func(i int) string {
		if i < len(parts) {
			return strings.TrimSpace(parts[i])
		}
		return ""
	}
	var s model.TaskSchedule
	if v := get(0); v != "" {
		n := 0
		if v != "-" {
			d, err := when.ParseLength(v)
			if err != nil {
				m.setStatus(err.Error(), true)
				return
			}
			n = int(d.Minutes())
		}
		s.DurationMin = &n
	}
	if v := get(1); v != "" {
		out := ""
		if v != "-" {
			r, err := when.Parse(v, m.now(), m.loc)
			if err != nil || r.AllDay {
				m.setStatus("start needs a date and time, such as fri 14:00", true)
				return
			}
			out = tz.FormatInstant(r.Start)
		}
		s.ScheduledStart = &out
	}
	if v := get(2); v != "" {
		out := ""
		if v != "-" {
			r, err := when.Parse(v, m.now(), m.loc)
			if err != nil {
				m.setStatus(err.Error(), true)
				return
			}
			out = r.Start.Format(tz.DateLayout)
			if !r.AllDay {
				out = tz.FormatInstant(r.Start)
			}
		}
		s.DeadlineAt = &out
	}
	if _, err := m.tr.Update(m.ctx, p.eventID, model.UpdateTaskInput{Schedule: s}); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus("task schedule saved", false)
}

// toggleAutoSchedule flips auto_schedule on the selected task (the planner places it when on).
func (m *Model) toggleAutoSchedule() {
	id := m.schedTaskID()
	if id == 0 {
		return
	}
	t, err := m.tr.FindByID(m.ctx, id)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	on := !t.AutoSchedule
	if _, err := m.tr.Update(m.ctx, id, model.UpdateTaskInput{Schedule: model.TaskSchedule{AutoSchedule: &on}}); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	switch {
	case !on:
		m.setStatus("auto-schedule off", false)
	case repo.ScheduleIssue(&model.Task{AutoSchedule: true, DurationMin: t.DurationMin}) != "":
		m.setStatus("auto-schedule on, but the task has no duration yet (S sets one)", false)
	default:
		m.setStatus("auto-schedule on", false)
	}
}

// ---- working hours ----

func (m *Model) showHours() {
	ar := repo.NewAvailabilityRepo(m.db)
	h, err := ar.Get(m.ctx, "")
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	cfg, err := ar.Config(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "rule: %s   zone: %s", h.Name, h.TZ)
	if h.Default {
		b.WriteString("   (built-in hours, nothing stored yet)")
	}
	b.WriteString("\n\n")
	for _, d := range model.WeekdayKeys {
		fmt.Fprintf(&b, "%-4s %s\n", d, rangesLine(h.Weekly[d]))
	}
	if len(h.Overrides) > 0 {
		b.WriteString("\ndate overrides\n")
		for d, rs := range h.Overrides {
			fmt.Fprintf(&b, "%s %s\n", d, rangesLine(rs))
		}
	}
	fmt.Fprintf(&b, "\nbuffer %d min, step %d min, horizon %d days\n", cfg.BufferMin, cfg.StepMin, cfg.HorizonDays)
	b.WriteString("\nchange: palette \"Calendar: set working hours\" (days hours, e.g. mon-fri 09:00-17:00 or sat off)\nsame as crm cal hours show|set|override|config\n")
	m.openText("Working hours", b.String(), nil)
}

func minutesDuration(n int) time.Duration { return time.Duration(n) * time.Minute }

func rangesLine(rs []model.TimeRange) string {
	if len(rs) == 0 {
		return "off"
	}
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.Start + "-" + r.End
	}
	return strings.Join(parts, ", ")
}

func (m *Model) startSetHours() tea.Cmd {
	m.startInput("cal-hours", "working hours (days hours, e.g. mon-fri 09:00-17:00, sat off): ", "")
	return m.input.Focus()
}

func (m *Model) calSetHours(value string) {
	fields := strings.Fields(value)
	if len(fields) < 2 {
		m.setStatus("type days and hours, such as mon-fri 09:00-17:00", true)
		return
	}
	days, err := model.ParseDays(fields[0])
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	ranges, err := model.ParseRanges(strings.Join(fields[1:], ""))
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if _, err := repo.NewAvailabilityRepo(m.db).SetDays(m.ctx, "", "", days, ranges); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus("working hours saved", false)
}
