package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Home view: what needs attention today. A section only appears when it has content.

const (
	homeDueMax      = 5
	homeTaskMax     = 8
	homeNoteMax     = 8
	homeBirthday    = 14 // days ahead
	homeBirthdayMax = 5
)

func whenHomeRow(m *Model) bool {
	if m.view != viewHome || m.focus != focusList {
		return false
	}
	r, ok := m.selected()
	return ok && !r.skip
}

// dayOf parses the date part of a stored due date.
func dayOf(s string) (time.Time, bool) {
	if len(s) < 10 {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s[:10])
	return t, err == nil
}

// ago renders a stored UTC timestamp ("2006-01-02 15:04:05") relative to now.
func ago(ts string, now time.Time) string {
	t, err := time.Parse("2006-01-02 15:04:05", ts)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, ts); err != nil {
			return ts
		}
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return cleanCell(s)
}

// homeRows builds the Home list. Heading rows are skipped by the cursor.
func (m *Model) homeRows() ([]row, error) {
	var rows []row
	head := func(title, meta string) { rows = append(rows, row{title: title, meta: meta, skip: true}) }
	now := m.now()
	today, _ := time.Parse("2006-01-02", now.Format("2006-01-02"))

	tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{})
	if err != nil {
		return nil, err
	}
	overdueTasks := 0
	var dueTasks []*model.Task // overdue or due within a week, in due order
	for _, t := range tasks {
		if t.DueAt == nil {
			continue
		}
		d, ok := dayOf(*t.DueAt)
		if !ok {
			continue
		}
		if d.Before(today) {
			overdueTasks++
		}
		if !d.After(today.AddDate(0, 0, 7)) {
			dueTasks = append(dueTasks, t)
		}
	}
	rows = append(rows, row{title: fmt.Sprintf("%d people, %d open tasks, %d overdue", len(m.people), len(tasks), overdueTasks), skip: true})

	due, err := m.pr.Due(m.ctx, 0, 1000)
	if err != nil {
		return nil, err
	}
	if len(due) > 0 {
		head("Due to reach out", fmt.Sprint(len(due)))
		for i, d := range due {
			if i == homeDueMax {
				rows = append(rows, row{title: fmt.Sprintf("all %d in Due", len(due)), jump: int(viewDue) + 1})
				break
			}
			rows = append(rows, row{personID: d.PersonID, title: cleanCell(d.Name), sub: d.KeepInTouch, meta: fmt.Sprintf("%dd overdue", d.DaysOverdue)})
		}
	}

	bdays, err := m.pr.Birthdays(m.ctx, homeBirthday, now)
	if err != nil {
		return nil, err
	}
	if len(bdays) > 0 {
		head("Birthdays next 14 days", fmt.Sprint(len(bdays)))
		for i, b := range bdays {
			if i == homeBirthdayMax {
				rows = append(rows, row{title: fmt.Sprintf("all %d in Birthdays", len(bdays)), jump: int(viewBirthdays) + 1})
				break
			}
			meta := fmt.Sprintf("in %dd", b.DaysUntil)
			if b.DaysUntil == 0 {
				meta = "today"
			}
			sub := b.Date
			if b.Turning != nil {
				sub += fmt.Sprintf(" (turns %d)", *b.Turning)
			}
			rows = append(rows, row{personID: b.PersonID, title: cleanCell(b.Name), sub: sub, meta: meta})
		}
	}

	if len(dueTasks) > 0 {
		head("Tasks overdue / this week", fmt.Sprint(len(dueTasks)))
		for i, t := range dueTasks {
			if i == homeTaskMax {
				rows = append(rows, row{title: "all open tasks in Tasks", jump: int(viewTasks) + 1})
				break
			}
			r := row{taskID: t.ID, title: cleanCell(t.Title), meta: dateOnly(*t.DueAt)}
			if len(t.PersonIDs) > 0 {
				r.personID = t.PersonIDs[0]
			}
			if r.personID != 0 {
				r.sub = m.nameOf(r.personID)
			}
			if d, ok := dayOf(*t.DueAt); ok && d.Before(today) {
				r.sub += ", overdue"
			}
			rows = append(rows, r)
		}
	}

	notes, err := m.ir.FindAll(m.ctx, model.InteractionFilters{Limit: homeNoteMax})
	if err != nil {
		return nil, err
	}
	if len(notes) > 0 {
		head("Recent notes", "")
		for _, n := range notes {
			r := row{title: "(no person)", meta: dateOnly(n.OccurredAt)}
			if len(n.PersonIDs) > 0 {
				r.personID = n.PersonIDs[0]
				r.title = cleanCell(m.nameOf(r.personID))
			}
			r.sub = n.Type
			if c := firstLine(str(n.Content)); c != "" {
				r.sub += ": " + c
			} else if s := cleanCell(str(n.Subject)); s != "" {
				r.sub += ": " + s
			}
			rows = append(rows, r)
		}
	}

	ov, err := m.runner.Overview(m.ctx)
	if err != nil {
		return nil, err
	}
	if len(ov.Sources) > 0 || ov.InboxPending > 0 {
		head("Sync", "")
		for _, s := range ov.Sources {
			r := row{title: s.Source, sub: sourceLine(s, now), jump: int(viewSync) + 1}
			if s.LastRun != nil && s.LastRun.Errors > 0 {
				r.meta = fmt.Sprintf("%d errors", s.LastRun.Errors)
			}
			rows = append(rows, r)
		}
		if ov.InboxPending > 0 {
			rows = append(rows, row{title: fmt.Sprintf("inbox: %d pending", ov.InboxPending), jump: int(viewSync) + 1})
		}
	}
	return rows, nil
}

// openHomeRow handles Enter on a Home row: open the person, or the view the row stands for.
func (m *Model) openHomeRow() tea.Cmd {
	r, ok := m.selected()
	if !ok || r.skip {
		return nil
	}
	if r.jump > 0 {
		m.switchView(viewKind(r.jump - 1))
		return nil
	}
	if r.personID != 0 {
		m.openPerson(r.personID)
	}
	return nil
}

// openPerson shows a person in the People view with the detail pane focused.
func (m *Model) openPerson(id int64) {
	found := false
	for _, p := range m.people {
		if p.ID == id {
			found = true
		}
	}
	if !found {
		m.setStatus("that person is archived, restore them from the Archived view", true)
		return
	}
	m.query = ""
	if m.view == viewPeople {
		m.report(m.rebuild(false))
	} else {
		m.switchView(viewPeople)
	}
	for i, r := range m.rows {
		if r.personID == id {
			m.cursor = i
			break
		}
	}
	m.clamp()
	m.refreshDetail(true)
	if len(m.form) > 0 {
		m.focus = focusDetail
		m.fcur = m.firstSelectable()
		m.syncDetail()
	}
}
