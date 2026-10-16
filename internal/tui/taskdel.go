package tui

import (
	"context"
	"fmt"
)

// hRestoreTask restores a deleted task and records it. The Delete and Restore verbs themselves are
// generic (objects.go).
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
