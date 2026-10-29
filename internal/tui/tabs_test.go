package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An arrangement with every module on (11 tabs) must still fit the usual widths and keep the current name.
func TestTabBarFitsWithEveryModuleOn(t *testing.T) {
	m, _ := newTestModel(t)
	require.Len(t, m.visibleViews(), 11)
	for _, w := range []int{80, 120} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		for _, v := range m.visibleViews() {
			m.switchView(v)
			line := strings.Split(m.screen(), "\n")[0]
			assert.LessOrEqual(t, lipgloss.Width(line), w, "tab bar at width %d view %d", w, v)
			assert.Contains(t, line, viewNames[v], "the current tab keeps its name at width %d", w)
		}
	}
}

// The default arrangement has nine tabs (Archived and Sync are hidden) and they fit the usual widths.
func TestTabBarHandlesTheDefaultViews(t *testing.T) {
	m := newDefaultModel(t)
	require.Len(t, m.visibleViews(), 9)
	for _, w := range []int{80, 120} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		for i, v := range m.visibleViews() {
			press(m, modules.Key(i+1))
			require.Equal(t, v, m.view, "key %s at width %d", modules.Key(i+1), w)
			line := strings.Split(m.screen(), "\n")[0]
			assert.LessOrEqual(t, lipgloss.Width(line), w, "tab bar at width %d view %d", w, i+1)
			assert.Contains(t, line, viewNames[v], "the current tab keeps its name at width %d", w)
			assert.Contains(t, line, "v More", "nothing is dropped at width %d", w)
			for n := 1; n <= 9; n++ {
				assert.Contains(t, line, modules.Key(n), "every tab keeps its key at width %d", w)
			}
		}
	}

	// 120 columns show every name, 80 columns the first four and the keys of the rest.
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	press(m, "1")
	line := strings.Split(m.screen(), "\n")[0]
	for _, v := range m.visibleViews() {
		assert.Contains(t, line, viewNames[v])
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	line = strings.Split(m.screen(), "\n")[0]
	for _, name := range []string{"Home", "People", "Follow-up", "Tasks"} {
		assert.Contains(t, line, name)
	}
	assert.NotContains(t, line, "Birthdays")
}

// A narrow terminal drops tabs from the end into a "v +N" entry that opens the view list.
func TestTabBarOverflowCountsWhatDidNotFit(t *testing.T) {
	m := newDefaultModel(t)
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 30})
	press(m, "2")
	segs := m.tabLayout()
	last := segs[len(segs)-1]
	require.True(t, last.more)
	assert.LessOrEqual(t, last.x1, 30)
	shown := 0
	for _, s := range segs {
		if !s.more {
			shown++
		}
	}
	assert.Equal(t, fmt.Sprintf("v +%d", len(m.visibleViews())-shown), last.label, "the overflow entry counts the tabs that did not fit")
	assert.Contains(t, segs[1].label, "People", "the current tab stays")
	click(m, last.x0+1, 0)
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Views", m.ov.title)
	assert.Len(t, m.ov.items, 13, "the Views screen has every module and the divider of the hidden group")
}
