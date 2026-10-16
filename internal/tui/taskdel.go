package tui

import (
	"context"
	"fmt"
)

// Delete and restore tasks. A deleted task is archived (the same soft delete as crm task delete),
// so ctrl+z brings it back and "Restore deleted task" restores the newest one of this session.

func whenDeletedTask(m *Model) bool { return len(m.deletedTasks) > 0 }

// askDeleteTask asks y/n, then deletes the task id.
func (m *Model) askDeleteTask(id int64) {
	if id == 0 {
		m.setStatus("select a task in the Tasks view (4) or a task row", true)
		return
	}
	title := fmt.Sprintf("task %d", id)
	if t, err := m.tr.FindByID(m.ctx, id); err == nil {
		title = truncateLabel(t.Title)
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("delete task %q? y/n", title),
		yes: func() {
			if err := m.hArchiveTask(m.ctx, id); err != nil {
				m.setStatus(err.Error(), true)
				return
			}
			m.deletedTasks = append(m.deletedTasks, id)
			m.afterWrite()
			m.setStatus("task deleted (ctrl+z undoes)", false)
		},
	}
}

// restoreLastTask brings back the task deleted most recently in this session. Tasks that came
// back through ctrl+z are skipped.
func (m *Model) restoreLastTask() {
	for len(m.deletedTasks) > 0 {
		id := m.deletedTasks[len(m.deletedTasks)-1]
		m.deletedTasks = m.deletedTasks[:len(m.deletedTasks)-1]
		if err := m.hRestoreTask(m.ctx, id); err != nil {
			continue
		}
		m.afterWrite()
		if m.view == viewTasks {
			m.cursorOn(func(r row) bool { return r.taskID == id })
		}
		m.setStatus("task restored", false)
		return
	}
	m.setStatus("no deleted task to restore in this session", true)
}

func (m *Model) hRestoreTask(ctx context.Context, id int64) error {
	if err := m.tr.Restore(ctx, id); err != nil {
		return err
	}
	title := fmt.Sprintf("task %d", id)
	if t, err := m.tr.FindByID(ctx, id); err == nil {
		title = "task " + truncateLabel(t.Title)
	}
	m.record("restore "+title,
		func() error { return m.tr.Archive(m.ctx, id) },
		func() error { return m.tr.Restore(m.ctx, id) })
	return nil
}
