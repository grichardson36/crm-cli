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
	homeDueMax  = 5
	homeTaskMax = 8
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
	var dueTasks []*model.Task // overdue or due today, linked to a person
	for _, t := range tasks {
		if t.DueAt == nil || (len(t.PersonIDs) == 0 && t.PersonID == nil) {
			continue
		}
		d, ok := dayOf(*t.DueAt)
		if !ok {
			continue
		}
		if !d.After(today) {
			dueTasks = append(dueTasks, t)
		}
	}

	due, err := m.pr.Due(m.ctx, 0, 1000)
	if err != nil {
		return nil, err
	}
	if len(due) > 0 {
		head("Due to reach out", fmt.Sprint(len(due)))
		for i, d := range due {
			if i == homeDueMax {
				rows = append(rows, row{title: fmt.Sprintf("all %d in Follow-up", len(due)), jump: int(viewDue) + 1})
				break
			}
			rows = append(rows, row{personID: d.PersonID, title: cleanCell(d.Name), sub: d.KeepInTouch, meta: fmt.Sprintf("%dd overdue", d.DaysOverdue)})
		}
	}

	if len(dueTasks) > 0 {
		head("Tasks due today", fmt.Sprint(len(dueTasks)))
		for i, t := range dueTasks {
			if i == homeTaskMax {
				rows = append(rows, row{title: "all due tasks in Tasks", jump: int(viewTasks) + 1})
				break
			}
			r := row{taskID: t.ID, title: cleanCell(t.Title), meta: dateOnly(*t.DueAt)}
			if len(t.PersonIDs) > 0 {
				r.personID = t.PersonIDs[0]
			} else if t.PersonID != nil {
				r.personID = *t.PersonID
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
