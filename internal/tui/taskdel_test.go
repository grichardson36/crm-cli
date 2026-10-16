package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func taskDelModel(t *testing.T) (*Model, int64) {
	t.Helper()
	m, _ := newTestModel(t)
	task, err := m.tr.Create(context.Background(), model.CreateTaskInput{Title: "send deck", Priority: "medium", PersonID: ip(2)})
	require.NoError(t, err)
	press(m, "4")
	require.Equal(t, viewTasks, m.view)
	return m, task.ID
}

func openTasks(t *testing.T, m *Model) []*model.Task {
	t.Helper()
	list, err := m.tr.FindAll(context.Background(), model.TaskFilters{})
	require.NoError(t, err)
	return list
}

func TestTaskDeleteKeyAsksThenUndoAndRedo(t *testing.T) {
	m, id := taskDelModel(t)
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "send deck")
	press(m, "n")
	assert.Len(t, openTasks(t, m), 1, "n keeps it")

	press(m, "d", "y")
	assert.Empty(t, openTasks(t, m))
	assert.Equal(t, []int64{id}, m.deleted["task"])
	assert.Contains(t, m.status, "task deleted")

	press(m, "ctrl+z")
	assert.Len(t, openTasks(t, m), 1)
	assert.Contains(t, m.toast.text, "Undone: delete task send deck")

	press(m, "ctrl+y")
	assert.Empty(t, openTasks(t, m))
	assert.Contains(t, m.toast.text, "Redone: delete task send deck")
}

func TestTaskDeleteFromPaletteAndRestore(t *testing.T) {
	m, id := taskDelModel(t)
	runPalette(m, "delete task")
	require.Equal(t, modeConfirm, m.mode)
	press(m, "y")
	assert.Empty(t, openTasks(t, m))

	runPalette(m, "restore task")
	assert.Equal(t, "task restored", m.status)
	list := openTasks(t, m)
	require.Len(t, list, 1)
	assert.Equal(t, id, list[0].ID)
	assert.Empty(t, m.deleted["task"])

	press(m, "ctrl+z")
	assert.Empty(t, openTasks(t, m), "restoring is undoable")
}

func TestTaskDeleteFromTaskRowInDetailPane(t *testing.T) {
	m, _ := newTestModel(t)
	_, err := m.tr.Create(context.Background(), model.CreateTaskInput{Title: "call back", Priority: "medium", PersonID: ip(2)})
	require.NoError(t, err)
	m.afterWrite()
	press(m, "j", "tab")
	require.Equal(t, focusDetail, m.focus)
	for i, r := range m.form {
		if r.kind == rkTask && !r.empty {
			m.fcur = i
		}
	}
	m.syncDetail()
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "call back")
	press(m, "y")
	assert.Empty(t, openTasks(t, m))
	assert.Len(t, m.deleted["task"], 1)
	press(m, "ctrl+z")
	assert.Len(t, openTasks(t, m), 1)
}

func TestTaskDeleteIsGreyedOutInReadOnlyAndWithoutATask(t *testing.T) {
	m, _ := taskDelModel(t)
	m.readOnly = true
	press(m, "d")
	assert.NotEqual(t, modeConfirm, m.mode)
	assert.Contains(t, m.status, "read-only")
	assert.Len(t, openTasks(t, m), 1)

	press(m, "ctrl+k")
	typeText(m, "delete task")
	var del, rest overlayItem
	for _, i := range m.ov.shown {
		switch m.ov.items[i].id {
		case "task-delete":
			del = m.ov.items[i]
		case "task-restore":
			rest = m.ov.items[i]
		}
	}
	assert.NotEmpty(t, del.disabled, "delete task is greyed out")
	assert.NotEmpty(t, rest.disabled, "restore task is greyed out")

	// People view: no task is selected, so d deletes the person and not a task.
	m2, _ := newTestModel(t)
	press(m2, "d")
	require.Equal(t, modeConfirm, m2.mode)
	assert.Contains(t, m2.confirm.prompt, "archive")
	assert.False(t, m2.actionByID("task-delete").enabled(m2))
}

func TestDeleteTagFromPaletteIsUndoable(t *testing.T) {
	m, _ := newTestModel(t) // Bob (2) carries #friend
	runPalette(m, "delete tag")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Delete tag", m.ov.title)
	press(m, "enter")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "#friend")
	assert.Contains(t, m.confirm.prompt, "1 records")
	press(m, "y")
	assert.False(t, m.hasTag(context.Background(), "person", 2, "friend"))

	press(m, "ctrl+z")
	assert.True(t, m.hasTag(context.Background(), "person", 2, "friend"))
	press(m, "ctrl+y")
	assert.False(t, m.hasTag(context.Background(), "person", 2, "friend"))
}

func TestDeleteTagIsGreyedOutInReadOnly(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	press(m, "ctrl+k")
	typeText(m, "delete tag")
	for _, i := range m.ov.shown {
		if m.ov.items[i].id == "tag-delete" {
			assert.NotEmpty(t, m.ov.items[i].disabled)
			return
		}
	}
	t.Fatal("Delete tag is not listed")
}

func ip(n int64) *int64 { return &n }
