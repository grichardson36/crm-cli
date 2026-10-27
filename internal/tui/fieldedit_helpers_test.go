package tui

import (
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/require"
)

// setChoice edits the choice row with the given label on the open record page: enter, pick the
// option with up and down, enter.
func setChoice(t *testing.T, m *Model, label, option string) {
	t.Helper()
	if m.focus != focusDetail {
		press(m, "tab")
	}
	require.Equal(t, focusDetail, m.focus)
	focusLabel(t, m, label)
	press(m, "enter")
	require.NotNil(t, m.edit, "enter opens the choice editor")
	require.NotNil(t, m.edit.typed)
	for i := 0; i < len(m.edit.typed.spec.Choices)+1; i++ {
		c := m.edit.typed.spec.Choices[m.edit.typed.choice]
		if strings.EqualFold(c.Label, option) || strings.EqualFold(c.Value, option) {
			break
		}
		press(m, "down")
	}
	press(m, "enter")
}

// focusRow puts the detail cursor on the first row with the label, from anywhere on the page.
func focusLabel(t *testing.T, m *Model, label string) {
	t.Helper()
	for i, r := range m.form {
		if r.label == label {
			m.fcur = i
			m.syncDetail()
			return
		}
	}
	t.Fatalf("no row labelled %q", label)
}

// setCell edits the cell in column col (1 is the first cell) of the focused row: right moves across the
// cells, enter opens the editor of its type, a choice is picked with up and down, enter saves.
func setCell(t *testing.T, m *Model, col int, option string) {
	t.Helper()
	require.Equal(t, focusDetail, m.focus)
	for m.fcol < col {
		press(m, "right")
	}
	require.Equal(t, col, m.fcol)
	press(m, "enter")
	require.NotNil(t, m.edit, "enter opens the editor of the cell")
	require.NotNil(t, m.edit.typed)
	if m.edit.typed.spec.Type == model.FieldChoice {
		for i := 0; i < len(m.edit.typed.spec.Choices)+1; i++ {
			c := m.edit.typed.spec.Choices[m.edit.typed.choice]
			if strings.EqualFold(c.Label, option) || strings.EqualFold(c.Value, option) {
				break
			}
			press(m, "down")
		}
	} else {
		replaceText(m, option)
	}
	press(m, "enter")
}
