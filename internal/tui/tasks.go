package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/quick"
)

const (
	taskPersonPrompt = "task (add a due date after a | e.g. call back | 2026-12-01): "
	taskNewPrompt    = "task (title | optional due date): "
)

// startTaskAdd is Add task. On a person (t) the task is for that person. In the Tasks view, or
// with no person, it is a task of its own that Link can attach to people and deals afterwards.
func (m *Model) startTaskAdd() tea.Cmd {
	if pid := m.curPerson(); pid != 0 && m.view != viewTasks {
		m.startPrompt("task", taskPersonPrompt, "")
		return m.input.Focus()
	}
	m.startInput("task-new", taskNewPrompt, "")
	return m.input.Focus()
}

// startTaskEdit opens the task as one line: title | due. The line saves with the Edit verb.
func (m *Model) startTaskEdit(id int64) tea.Cmd {
	t, err := m.tr.FindByID(m.ctx, id)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	m.lineID = id
	line := strings.TrimRight(strings.Join([]string{t.Title, str(t.DueAt)}, " | "), " |")
	m.startInput("task-edit", taskNewPrompt, line)
	return m.input.Focus()
}

func (m *Model) saveTaskEdit(id int64, value string) {
	title, due, hasDue := strings.Cut(value, "|")
	title, due = strings.TrimSpace(title), strings.TrimSpace(due)
	fail := func(err error) {
		m.startInput("task-edit", taskNewPrompt, value)
		m.pendingCmd = tea.Batch(m.input.Focus(), m.showToast(err.Error(), true))
		m.setStatus(err.Error(), true)
	}
	if title == "" {
		fail(fmt.Errorf("a task needs a title"))
		return
	}
	in := model.UpdateTaskInput{Title: &title}
	if hasDue && due != "" {
		d := m.normalizeDue(due)
		in.DueAt = &d
	}
	if _, err := m.hUpdateTask(m.ctx, id, in); err != nil {
		fail(err)
		return
	}
	m.afterWrite()
	m.setStatus("task saved (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast("task saved", false)
}

// runTaskLine adds a task from a prompt line through the same reader as the command bar. pid is the
// person the task is for, or 0 for a task of its own.
func (m *Model) runTaskLine(pid int64, value, kind string) {
	title, due, _ := strings.Cut(value, "|")
	env := m.quickEnvFor(pid)
	env.DueOverride = m.normalizeDue(due)
	prompt := taskNewPrompt
	if kind == "task" {
		prompt = taskPersonPrompt
	}
	_, err := m.runLineAsk("task "+strings.TrimSpace(title), env,
		func() {
			m.startInput(kind, prompt, value)
			_ = m.input.Focus()
		},
		func(plan *quick.Plan) tea.Cmd {
			m.setStatus("task added"+extras(plan), false)
			return m.showToast("task added"+extras(plan), false)
		})
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
}
