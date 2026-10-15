package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/quick"
)

// Undo and redo for this session, like an office program: every data change made in the TUI is
// an entry on a stack. ctrl+z undoes the newest, ctrl+r (or ctrl+y) redoes it, and a new change
// clears what could be redone. The stack lives in memory only.
//
// Writes go through the h* methods below. They call the repository, then record how to reverse
// and repeat the change. Merging people and purging are not undoable and are not recorded.

type histEntry struct {
	label string
	undo  func() error
	redo  func() error
}

const maxHistory = 200

// record puts a change on the undo stack, or in the open group.
func (m *Model) record(label string, undo, redo func() error) {
	e := histEntry{label: label, undo: undo, redo: redo}
	if m.grpDepth > 0 {
		m.grp = append(m.grp, e)
		return
	}
	m.pushHist(e)
}

func (m *Model) pushHist(e histEntry) {
	m.hist = append(m.hist, e)
	if len(m.hist) > maxHistory {
		m.hist = m.hist[len(m.hist)-maxHistory:]
	}
	m.redoStack = nil
}

// beginGroup starts collecting entries so several writes undo as one.
func (m *Model) beginGroup() {
	if m.grpDepth == 0 {
		m.grp = nil
	}
	m.grpDepth++
}

// endGroup closes the group. One entry keeps its label. Several become one entry named label,
// or by the first entry and a count when label is empty.
func (m *Model) endGroup(label string) {
	if m.grpDepth == 0 {
		return
	}
	m.grpDepth--
	if m.grpDepth > 0 {
		return
	}
	g := m.grp
	m.grp = nil
	switch len(g) {
	case 0:
		return
	case 1:
		if label == "" {
			m.pushHist(g[0])
			return
		}
	}
	if label == "" {
		label = fmt.Sprintf("%s (+%d more)", g[0].label, len(g)-1)
	}
	m.pushHist(histEntry{
		label: label,
		undo: func() error {
			var first error
			for i := len(g) - 1; i >= 0; i-- {
				if err := g[i].undo(); err != nil && first == nil {
					first = err
				}
			}
			return first
		},
		redo: func() error {
			var first error
			for _, e := range g {
				if err := e.redo(); err != nil && first == nil {
					first = err
				}
			}
			return first
		},
	})
}

// recordQuick records the changes of a command-bar line or prompt.
func (m *Model) recordQuick(_ string, changes []quick.Change) {
	if len(changes) == 0 {
		return
	}
	r := m.qrepos()
	m.record(truncateLabel(quick.Summarize(changes)),
		func() error {
			if err := quick.CheckUndo(m.ctx, r, changes); err != nil {
				return staleError{err.Error()}
			}
			return quick.Undo(m.ctx, r, changes)
		},
		func() error {
			if err := quick.CheckRedo(m.ctx, r, changes); err != nil {
				return staleError{err.Error()}
			}
			return quick.Redo(m.ctx, r, changes)
		})
}

// staleError means the target changed after the entry was recorded, so undo or redo is refused.
type staleError struct{ msg string }

func (e staleError) Error() string { return e.msg }

func truncateLabel(s string) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > 70 {
		return string(r[:69]) + "…"
	}
	return string(r)
}

// undoLast reverses the newest entry and says what it did in a toast.
func (m *Model) undoLast() tea.Cmd {
	n := len(m.hist)
	if n == 0 {
		return m.showToast("Nothing to undo", true)
	}
	e := m.hist[n-1]
	m.hist = m.hist[:n-1]
	err := e.undo()
	m.afterWrite()
	var stale staleError
	if errors.As(err, &stale) {
		m.hist = append(m.hist, e) // still the newest change: the user can fix the cause and retry
		return m.showToast("Cannot undo "+e.label+": "+stale.msg, true)
	}
	if err != nil {
		return m.showToast("Could not undo "+e.label+": "+err.Error(), true)
	}
	m.redoStack = append(m.redoStack, e)
	return m.showToast("Undone: "+e.label+" · ctrl+r redo", false)
}

// redoLast repeats the entry that was undone last.
func (m *Model) redoLast() tea.Cmd {
	n := len(m.redoStack)
	if n == 0 {
		return m.showToast("Nothing to redo", true)
	}
	e := m.redoStack[n-1]
	m.redoStack = m.redoStack[:n-1]
	err := e.redo()
	m.afterWrite()
	var stale staleError
	if errors.As(err, &stale) {
		m.redoStack = append(m.redoStack, e)
		return m.showToast("Cannot redo "+e.label+": "+stale.msg, true)
	}
	if err != nil {
		return m.showToast("Could not redo "+e.label+": "+err.Error(), true)
	}
	m.hist = append(m.hist, e)
	return m.showToast("Redone: "+e.label+" · ctrl+z undo", false)
}

// personLabel is the name of a person for messages.
func (m *Model) personLabel(id int64) string {
	for _, p := range m.people {
		if p.ID == id {
			return p.Name
		}
	}
	if p, err := m.pr.FindByID(m.ctx, id); err == nil {
		return fullName(p)
	}
	return fmt.Sprintf("person %d", id)
}

func (m *Model) entityLabel(entity string, id int64) string {
	if entity == "person" {
		return m.personLabel(id)
	}
	return fmt.Sprintf("%s %d", entity, id)
}

// --- recorded writes ---

func (m *Model) hUpdatePerson(ctx context.Context, id int64, in model.UpdatePersonInput) (*model.Person, error) {
	before, _ := m.pr.FindByID(ctx, id)
	p, err := m.pr.Update(ctx, id, in)
	if err != nil || before == nil {
		return p, err
	}
	fields := quick.UpdateFields(in)
	if len(fields) == 0 {
		return p, nil
	}
	inv := quick.InverseUpdate(before, in)
	name := fullName(before)
	guard := func(expected model.UpdatePersonInput) error {
		cur, err := m.pr.FindByID(m.ctx, id)
		if err != nil {
			return staleError{name + " is gone or archived"}
		}
		if stale := quick.StaleFields(cur, expected); len(stale) > 0 {
			return staleError{strings.Join(stale, ", ") + " of " + name + " changed since"}
		}
		return nil
	}
	m.record("set "+strings.Join(fields, ", ")+" on "+name,
		func() error {
			if err := guard(in); err != nil {
				return err
			}
			_, err := m.pr.Update(m.ctx, id, inv)
			return err
		},
		func() error {
			if err := guard(inv); err != nil {
				return err
			}
			_, err := m.pr.Update(m.ctx, id, in)
			return err
		})
	return p, nil
}

func (m *Model) hCreatePerson(ctx context.Context, in model.CreatePersonInput) (*model.Person, error) {
	p, err := m.pr.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	id := p.ID
	m.record("create "+fullName(p),
		func() error { return m.pr.Archive(m.ctx, id) },
		func() error { return m.pr.Restore(m.ctx, id) })
	return p, nil
}

func (m *Model) hArchivePerson(ctx context.Context, id int64) error {
	name := m.personLabel(id)
	if err := m.pr.Archive(ctx, id); err != nil {
		return err
	}
	m.record("archive "+name,
		func() error { return m.pr.Restore(m.ctx, id) },
		func() error { return m.pr.Archive(m.ctx, id) })
	return nil
}

func (m *Model) hRestorePerson(ctx context.Context, id int64) error {
	name := m.personLabel(id)
	if err := m.pr.Restore(ctx, id); err != nil {
		return err
	}
	m.record("restore "+name,
		func() error { return m.pr.Archive(m.ctx, id) },
		func() error { return m.pr.Restore(m.ctx, id) })
	return nil
}

func (m *Model) hAddEmail(ctx context.Context, pid int64, v string, lbl *string, primary bool) error {
	if err := m.pr.AddEmail(ctx, pid, v, lbl, primary); err != nil {
		return err
	}
	m.record("add email "+v+" to "+m.personLabel(pid),
		func() error { return m.pr.RemoveEmail(m.ctx, pid, v) },
		func() error { return m.pr.AddEmail(m.ctx, pid, v, lbl, primary) })
	return nil
}

func (m *Model) hRemoveEmail(ctx context.Context, pid int64, v string) error {
	list, _ := m.pr.Emails(ctx, pid)
	if err := m.pr.RemoveEmail(ctx, pid, v); err != nil {
		return err
	}
	for _, e := range list {
		if e.Email == v {
			e := e
			m.record("remove email "+v+" from "+m.personLabel(pid),
				func() error { return m.pr.AddEmail(m.ctx, pid, e.Email, e.Label, e.IsPrimary) },
				func() error { return m.pr.RemoveEmail(m.ctx, pid, v) })
		}
	}
	return nil
}

func (m *Model) hAddPhone(ctx context.Context, pid int64, v string, lbl *string, primary bool) error {
	if err := m.pr.AddPhone(ctx, pid, v, lbl, primary); err != nil {
		return err
	}
	m.record("add phone "+v+" to "+m.personLabel(pid),
		func() error { return m.pr.RemovePhone(m.ctx, pid, v) },
		func() error { return m.pr.AddPhone(m.ctx, pid, v, lbl, primary) })
	return nil
}

func (m *Model) hRemovePhone(ctx context.Context, pid int64, v string) error {
	list, _ := m.pr.Phones(ctx, pid)
	if err := m.pr.RemovePhone(ctx, pid, v); err != nil {
		return err
	}
	for _, e := range list {
		if e.PhoneNumber == v {
			e := e
			m.record("remove phone "+v+" from "+m.personLabel(pid),
				func() error { return m.pr.AddPhone(m.ctx, pid, e.PhoneNumber, e.Label, e.IsPrimary) },
				func() error { return m.pr.RemovePhone(m.ctx, pid, v) })
		}
	}
	return nil
}

func (m *Model) handleOf(ctx context.Context, pid int64, kind string) string {
	list, _ := m.pr.Handles(ctx, pid)
	for _, h := range list {
		if h.Kind == kind {
			return h.Value
		}
	}
	return ""
}

func (m *Model) hSetHandle(ctx context.Context, pid int64, kind, v string) error {
	old := m.handleOf(ctx, pid, kind)
	if err := m.pr.SetHandle(ctx, pid, kind, v); err != nil {
		return err
	}
	m.record("set "+kind+" on "+m.personLabel(pid),
		func() error {
			if old == "" {
				return m.pr.RemoveHandle(m.ctx, pid, kind)
			}
			return m.pr.SetHandle(m.ctx, pid, kind, old)
		},
		func() error { return m.pr.SetHandle(m.ctx, pid, kind, v) })
	return nil
}

func (m *Model) hRemoveHandle(ctx context.Context, pid int64, kind string) error {
	old := m.handleOf(ctx, pid, kind)
	if err := m.pr.RemoveHandle(ctx, pid, kind); err != nil {
		return err
	}
	if old != "" {
		m.record("remove "+kind+" from "+m.personLabel(pid),
			func() error { return m.pr.SetHandle(m.ctx, pid, kind, old) },
			func() error { return m.pr.RemoveHandle(m.ctx, pid, kind) })
	}
	return nil
}

func (m *Model) hasTag(ctx context.Context, entity string, id int64, name string) bool {
	tags, err := m.tagr.GetForEntity(ctx, entity, id)
	if err != nil {
		return false
	}
	for _, t := range tags {
		if t.Name == name {
			return true
		}
	}
	return false
}

func (m *Model) hTagApply(ctx context.Context, entity string, id int64, name string) error {
	had := m.hasTag(ctx, entity, id, name)
	if err := m.tagr.Apply(ctx, entity, id, name); err != nil {
		return err
	}
	if !had {
		m.record("tag #"+name+" on "+m.entityLabel(entity, id),
			func() error { return m.tagr.Remove(m.ctx, entity, id, name) },
			func() error { return m.tagr.Apply(m.ctx, entity, id, name) })
	}
	return nil
}

func (m *Model) hTagRemove(ctx context.Context, entity string, id int64, name string) error {
	if err := m.tagr.Remove(ctx, entity, id, name); err != nil {
		return err
	}
	m.record("remove tag #"+name+" from "+m.entityLabel(entity, id),
		func() error { return m.tagr.Apply(m.ctx, entity, id, name) },
		func() error { return m.tagr.Remove(m.ctx, entity, id, name) })
	return nil
}

func (m *Model) hUpdateTask(ctx context.Context, id int64, in model.UpdateTaskInput) (*model.Task, error) {
	before, _ := m.tr.FindByID(ctx, id)
	t, err := m.tr.Update(ctx, id, in)
	if err != nil || before == nil {
		return t, err
	}
	var inv model.UpdateTaskInput
	if in.Title != nil {
		v := before.Title
		inv.Title = &v
	}
	if in.Description != nil {
		v := str(before.Description)
		inv.Description = &v
	}
	if in.DueAt != nil {
		v := str(before.DueAt)
		inv.DueAt = &v
	}
	if in.Priority != nil {
		v := before.Priority
		inv.Priority = &v
	}
	if in.Recurrence != nil {
		v := str(before.Recurrence)
		inv.Recurrence = &v
	}
	m.record("edit task "+truncateLabel(before.Title),
		func() error { _, err := m.tr.Update(m.ctx, id, inv); return err },
		func() error { _, err := m.tr.Update(m.ctx, id, in); return err })
	return t, nil
}

func (m *Model) hArchiveTask(ctx context.Context, id int64) error {
	title := fmt.Sprintf("task %d", id)
	if t, err := m.tr.FindByID(ctx, id); err == nil {
		title = "task " + truncateLabel(t.Title)
	}
	if err := m.tr.Archive(ctx, id); err != nil {
		return err
	}
	m.record("delete "+title,
		func() error { return m.tr.Restore(m.ctx, id) },
		func() error { return m.tr.Archive(m.ctx, id) })
	return nil
}

func (m *Model) hCompleteTask(ctx context.Context, id int64) (*model.Task, error) {
	t, err := m.tr.Complete(ctx, id)
	if err != nil {
		return nil, err
	}
	next := t.NextTaskID
	m.record("complete task "+truncateLabel(t.Title),
		func() error {
			if err := m.tr.Reopen(m.ctx, id); err != nil {
				return err
			}
			if next != nil {
				return m.tr.Archive(m.ctx, *next)
			}
			return nil
		},
		func() error {
			again, err := m.tr.Complete(m.ctx, id)
			if err == nil {
				next = again.NextTaskID
			}
			return err
		})
	return t, nil
}

func (m *Model) hUpdateNote(ctx context.Context, id int64, in model.UpdateInteractionInput) (*model.Interaction, error) {
	before, _ := m.ir.FindByID(ctx, id)
	it, err := m.ir.Update(ctx, id, in)
	if err != nil || before == nil {
		return it, err
	}
	var inv model.UpdateInteractionInput
	if in.Type != nil {
		v := before.Type
		inv.Type = &v
	}
	if in.Subject != nil {
		v := str(before.Subject)
		inv.Subject = &v
	}
	if in.Content != nil {
		v := str(before.Content)
		inv.Content = &v
	}
	if in.Direction != nil {
		v := str(before.Direction)
		inv.Direction = &v
	}
	if in.OccurredAt != nil {
		v := before.OccurredAt
		inv.OccurredAt = &v
	}
	what := "edit note"
	if in.Type != nil && in.Content == nil {
		what = "change note type"
	}
	m.record(what,
		func() error { _, err := m.ir.Update(m.ctx, id, inv); return err },
		func() error { _, err := m.ir.Update(m.ctx, id, in); return err })
	return it, nil
}

func (m *Model) hArchiveNote(ctx context.Context, id int64) error {
	if err := m.ir.Archive(ctx, id); err != nil {
		return err
	}
	m.record("delete note",
		func() error { return m.ir.Restore(m.ctx, id) },
		func() error { return m.ir.Archive(m.ctx, id) })
	return nil
}

func (m *Model) hRestoreNote(ctx context.Context, id int64) error {
	if err := m.ir.Restore(ctx, id); err != nil {
		return err
	}
	m.record("restore note",
		func() error { return m.ir.Archive(m.ctx, id) },
		func() error { return m.ir.Restore(m.ctx, id) })
	return nil
}

// taskTarget is the task the T key tags: the selected row in the Tasks view, or a saved task row
// in the detail pane. 0 when there is none.
func (m *Model) taskTarget() int64 {
	if m.focus == focusDetail && m.fcur < len(m.form) {
		if r := m.form[m.fcur]; r.kind == rkTask && !r.empty {
			return r.id
		}
		return 0
	}
	if m.view == viewTasks {
		if r, ok := m.selected(); ok {
			return r.taskID
		}
	}
	return 0
}

func whenTaskTarget(m *Model) bool { return m.taskTarget() != 0 }

// tagTask adds a tag to the target task, or removes it when the text starts with -.
func (m *Model) tagTask(value string) {
	id := m.taskTarget()
	if id == 0 {
		m.setStatus("select a task first", true)
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
		err = m.hTagRemove(m.ctx, "task", id, name)
	} else {
		err = m.hTagApply(m.ctx, "task", id, name)
	}
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if remove {
		m.setStatus("tag #"+name+" removed from the task", false)
	} else {
		m.setStatus("task tagged #"+name, false)
	}
	m.afterWrite()
}