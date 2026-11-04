package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boxOf finds the popup box on the screen: its top line index, its left column and its width.
func boxOf(t *testing.T, m *Model) (top, left, width int, lines []string) {
	t.Helper()
	lines = strings.Split(ansi.Strip(m.screen()), "\n")
	top = -1
	for i, l := range lines {
		if x := strings.Index(l, "┌"); x >= 0 && top < 0 {
			top, left = i, len([]rune(l[:x]))
			width = len([]rune(l[x:strings.LastIndex(l, "┐")+len("┐")]))
		}
	}
	require.GreaterOrEqual(t, top, 0, "a popup box is on the screen")
	return
}

func TestPopupIsCenteredWithHintsInsideTheBox(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	top, left, w, lines := boxOf(t, m)
	box := m.overlayBox(m.ov)
	assert.Equal(t, box.x, left)
	assert.Equal(t, box.y, top)
	assert.Equal(t, box.w, w)
	assert.InDelta(t, (m.width-w)/2, left, 1, "centered across")
	assert.InDelta(t, (m.height-box.h)/2, top, 1, "centered down")

	// The bottom row of the box holds the hints, between the borders.
	hint := -1
	for i, l := range lines {
		if strings.Contains(l, "esc cancel") {
			hint = i
		}
	}
	require.GreaterOrEqual(t, hint, top)
	assert.Equal(t, top+box.h-2, hint, "the hints are the last row inside the box")
	assert.True(t, strings.Contains(lines[hint], "│"), "between the border lines")
	assert.Contains(t, lines[hint], "enter run")
	assert.Contains(t, lines[top+box.h-1], "└")
}

func TestPopupFilterAndTitleVariants(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	top, _, _, lines := boxOf(t, m)
	assert.Contains(t, lines[top+1], "> ", "the palette has a filter line")
	assert.Contains(t, lines[top+1], "Select a command")

	m, _ = newTestModel(t)
	press(m, "v")
	top, _, _, lines = boxOf(t, m)
	assert.Contains(t, lines[top+1], "> Views", "the Views list has a filter line, its title is the placeholder")
	assert.Contains(t, strings.Join(lines, "\n"), "esc close")

	m, _ = newTestModel(t)
	press(m, "ctrl+o")
	top, _, _, lines = boxOf(t, m)
	assert.Contains(t, lines[top+1], "View options")
	assert.NotContains(t, lines[top+1], ">")
	assert.Contains(t, strings.Join(lines, "\n"), "←/→ change")
}

func TestPopupHintsFollowTheHighlightedRow(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "v")
	items := m.ov.items
	var saved bool
	for _, it := range items {
		saved = saved || strings.HasPrefix(it.value, savedPrefix)
	}
	hints := m.renderPopupHints(m.overlayHints(), 200)
	assert.Contains(t, ansi.Strip(hints), "shift+↑/↓ move")
	assert.NotContains(t, ansi.Strip(hints), "delete", "a built-in view cannot be deleted")
	_ = saved
}

func TestPopupFitsSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {36, 10}, {32, 10}} {
		m, _ := newTestModel(t)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(m, "ctrl+k")
		out := ansi.Strip(m.screen())
		lines := strings.Split(out, "\n")
		for _, l := range lines {
			assert.LessOrEqual(t, ansi.StringWidth(l), size[0], "no line is wider than the terminal %v", size)
		}
		assert.LessOrEqual(t, len(lines), size[1], "no more lines than the terminal %v", size)
		box := m.overlayBox(m.ov)
		assert.LessOrEqual(t, box.w, size[0]-0)
		assert.GreaterOrEqual(t, box.x, 0)
		assert.GreaterOrEqual(t, box.y, 0)
		assert.Contains(t, out, "esc cancel", "the hints stay in the box at %v", size)
	}
}

func TestPopupWidthIsLimitedOnWideTerminals(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	press(m, "ctrl+k")
	box := m.overlayBox(m.ov)
	assert.Equal(t, popupPaletteW+4, box.w)
	assert.Equal(t, popupMaxRows+popupChrome+1, box.h, "the palette adds the helper row")
}

func TestPopupListScrollsPastTheMaximum(t *testing.T) {
	m, _ := newTestModel(t)
	var items []overlayItem
	for i := 0; i < 40; i++ {
		items = append(items, overlayItem{id: string(rune('a' + i%26)), label: strings.Repeat("x", i+1)})
	}
	m.openPicker("Pick", items, func(*Model, overlayItem) {})
	require.True(t, m.ov.hasFilter(), "a long picker has a filter line")
	for i := 0; i < 30; i++ {
		press(m, "down")
	}
	assert.Greater(t, m.ov.offset, 0)
	out := ansi.Strip(m.screen())
	assert.Contains(t, out, strings.Repeat("x", 31))
	assert.NotContains(t, out, "\n│ "+strings.Repeat("x", 1)+" ")
}

func TestShortPickerHasATitleUntilYouType(t *testing.T) {
	m, _ := newTestModel(t)
	m.openPicker("Choose one", []overlayItem{{id: "a", label: "Alpha"}, {id: "b", label: "Beta"}}, func(*Model, overlayItem) {})
	top, _, _, lines := boxOf(t, m)
	assert.Contains(t, lines[top+1], "Choose one")
	assert.NotContains(t, lines[top+1], ">")
	press(m, "z")
	assert.Equal(t, "z", m.ov.filter.Value(), "typing filters, and the filter line replaces the title")
	_, _, _, lines = boxOf(t, m)
	assert.Contains(t, lines[top+1], "> z")
}

func TestPopupMouseClickPicksTheRowUnderThePointer(t *testing.T) {
	m, _ := newTestModel(t)
	picked := ""
	m.openPicker("Choose one", []overlayItem{{id: "a", label: "Alpha"}, {id: "b", label: "Beta"}}, func(m *Model, it overlayItem) { picked = it.id })
	box := m.overlayBox(m.ov)
	click(m, box.x+3, box.listY+1)
	assert.Equal(t, "b", picked)
	assert.Nil(t, m.ov)
}
