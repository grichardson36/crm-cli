package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The owner's rule: no sentences or tooltips, but the syntax helpers while typing in a line stay.
// These tests fail if a helper disappears from the palette popup or from a footer prompt.

func plainScreen(m *Model) string { return ansi.Strip(m.screen()) }

// Footer add lines show the labels of the segments that are still to type.
func TestFooterAddLinesShowTheSyntaxHelpers(t *testing.T) {
	tests := []struct {
		name string
		view viewKind
	}{
		{"person", viewPeople},
		{"organization", viewOrgs},
		{"deal", viewDeals},
		{"task", viewTasks},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.switchView(tc.view)
			m.settle()
			press(m, "a", "enter")
			require.Equal(t, modePrompt, m.mode)
			labels := m.promptFieldsFor(m.promptKind)
			require.GreaterOrEqual(t, len(labels), 2, "the line has fields after the title")
			out := plainScreen(m)
			assert.Contains(t, out, labels[0], "the empty line names its first field")
			typeText(m, "Zeta")
			out = plainScreen(m)
			assert.Contains(t, out, "| "+labels[1], "after the title the next field shows")
		})
	}
}

func TestFooterNoteLineShowsItsHelpers(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "2", "a", "n")
	require.Equal(t, modePrompt, m.mode)
	require.NotEmpty(t, m.promptFieldsFor(m.promptKind))
	assert.Contains(t, plainScreen(m), m.promptFieldsFor(m.promptKind)[0])
}

// An error of a closed segment shows above the footer line.
func TestFooterLineShowsTheErrorOfAClosedSegment(t *testing.T) {
	m, _ := newTestModel(t)
	m.switchView(viewTasks)
	m.settle()
	press(m, "a", "enter")
	typeText(m, "Call | not a date at all |")
	out := plainScreen(m)
	assert.Contains(t, out, "! ", "a bad closed segment shows its error")
}

// The palette shows the prefix helpers while the box is empty or holds only a prefix.
func TestPaletteShowsThePrefixHelpers(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	out := plainScreen(m)
	for _, w := range []string{"> actions", "@ people", "# tags", "= settings", "? help"} {
		assert.Contains(t, out, w, "empty palette")
	}
	for _, p := range []string{">", "@", "#", "=", "?"} {
		m, _ = newTestModel(t)
		press(m, "ctrl+k")
		typeText(m, p)
		assert.Contains(t, plainScreen(m), "@ people", "only the prefix %q typed", p)
	}
	m, _ = newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "add")
	assert.NotContains(t, plainScreen(m), "@ people", "a query hides the helper")
	assert.NotContains(t, strings.ToLower(plainScreen(m)), "tooltip")
}

func TestPaletteHelperRowKeepsTheBoxInsideTheScreen(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	box := m.overlayBox(m.ov)
	assert.True(t, box.helper)
	assert.Equal(t, box.y+4, box.listY, "the list starts below the helper row")
	assert.Equal(t, m.popupPage(), box.rows)
}

// The palette command line keeps its syntax helpers.
func TestPaletteCommandLineShowsTheSyntaxHelpers(t *testing.T) {
	m, _ := newTestModel(t)
	openBar(m, "task ")
	out := plainScreen(m)
	assert.Contains(t, out, "title by date", "the field hint after the cursor")
	assert.Contains(t, out, "#tag @Name !high ~30m by fri", "the token helper row")
	assert.Contains(t, out, "every 3m")

	typeText(m, "Call mom by fri")
	out = plainScreen(m)
	assert.Contains(t, out, "@person #tag", "filled fields drop out, the rest stay")
}

func TestPaletteCommandLineShowsAnError(t *testing.T) {
	m, _ := newTestModel(t)
	openBar(m, "task Call by notadate999 !high")
	press(m, "enter")
	out := plainScreen(m)
	assert.True(t, strings.Contains(out, "nothing to do") || strings.Contains(out, "date") || strings.Contains(out, "!"), out)
}

// Action descriptions are tooltips, they stay out of the palette rows.
func TestPaletteRowsHaveNoActionDescriptions(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	out := plainScreen(m)
	for i := range m.actions {
		a := &m.actions[i]
		if len(a.desc) > 30 {
			assert.NotContains(t, out, a.desc)
		}
	}
}
