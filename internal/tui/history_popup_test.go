package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUndoHistoryPopupUndoesBackToTheChosenEntry(t *testing.T) {
	m, _ := newTestModel(t)
	var undone, redone []string
	for _, l := range []string{"one", "two", "three"} {
		l := l
		m.record(l, func() error { undone = append(undone, l); return nil }, func() error { redone = append(redone, l); return nil })
	}
	m.openHistory(false)
	require.NotNil(t, m.ov, "the history opens as a popup")
	assert.Equal(t, "Undo history", m.ov.title)
	assert.Equal(t, "three", m.ov.items[m.ov.shown[0]].label, "newest first")

	press(m, "down", "enter") // pick "two": undoes three and two
	assert.Equal(t, []string{"three", "two"}, undone)
	assert.Len(t, m.hist, 1)
	assert.Len(t, m.redoStack, 2)

	m.openHistory(true)
	assert.Equal(t, "Redo history", m.ov.title)
	assert.Equal(t, "two", m.ov.items[m.ov.shown[0]].label, "next redo first")
	press(m, "enter")
	assert.Equal(t, []string{"two"}, redone)
}

func TestHistoryPopupWithNothingSaysSo(t *testing.T) {
	m, _ := newTestModel(t)
	m.openHistory(false)
	assert.Nil(t, m.ov)
}

func TestUndoHistoryHasAKey(t *testing.T) {
	m, _ := newTestModel(t)
	a := m.actionByID("undo-history")
	require.NotNil(t, a)
	assert.True(t, a.hasKey("ctrl+shift+z"))
}
