package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewSwitcherListsEveryViewWithItsKey(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "v")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Go to view", m.ov.title)
	require.Len(t, m.ov.items, 10, "every module is on by default")
	for i, v := range m.visibleViews() {
		assert.Equal(t, viewNames[v], m.ov.items[i].label)
		assert.Equal(t, modules.Key(i+1), m.ov.items[i].hint)
	}
	assert.Equal(t, "current view", m.ov.items[1].desc)

	// A number key picks at once.
	press(m, "8")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, viewMerge, m.view)

	// Typing filters, enter picks.
	press(m, "v")
	typeText(m, "archiv")
	press(m, "enter")
	assert.Equal(t, viewArchived, m.view)

	// Esc closes without moving.
	press(m, "v", "esc")
	assert.Equal(t, viewArchived, m.view)
	assert.Equal(t, modeNormal, m.mode)
}

func TestGoToViewInPalette(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "go to view")
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Go to view", m.ov.title)
	press(m, "0")
	assert.Equal(t, viewSync, m.view)
}

func TestHomeIsTheFirstViewAndOneJumpsToIt(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "1")
	assert.Equal(t, viewHome, m.view)
	assert.Equal(t, 0, int(viewHome))
	press(m, "2")
	assert.Equal(t, viewPeople, m.view)
}

func TestViewSwitcherReachableByMouseOnMore(t *testing.T) {
	m, _ := newTestModel(t)
	segs := m.tabLayout()
	more := segs[len(segs)-1]
	require.True(t, more.more)
	click(m, more.x0+1, 0)
	require.Equal(t, modeOverlay, m.mode)
	// Click the Birthdays line (view 5 is the fifth item under the two header lines).
	click(m, 3, overlayTop+m.viewPosition(viewBirthdays)-1)
	assert.Equal(t, viewBirthdays, m.view)
	assert.Contains(t, strings.Split(m.View(), "\n")[0], "7 Birthdays")
}

func TestHelpAndFooterMentionNewKeys(t *testing.T) {
	m, _ := newTestModel(t)
	help := m.helpText()
	for _, want := range []string{"list the views that are on", "open the highlighted thing further", "mark the highlighted thing", "an object: a picker of kinds", "merge the space-selected rows"} {
		assert.Contains(t, help, want)
	}
	assert.Contains(t, m.hintsLine(), "help")

	// The Sync view lists its own key, the inbox record's delete. The sync run and dry run are palette entries.
	m.switchView(viewSync)
	m.settle()
	syncHelp := m.helpText()
	assert.Contains(t, syncHelp, "dismiss the selected inbox")
	assert.NotContains(t, syncHelp, "run the selected source")

	// The Archived view shows restore and the delete that purges.
	m.switchView(viewArchived)
	m.settle()
	archivedHelp := m.helpText()
	assert.Contains(t, archivedHelp, "restore the archived object")
	assert.Contains(t, archivedHelp, "permanently delete the archived object")
}

func TestTabBarNeverExceeds80Columns(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for i := range m.visibleViews() {
		press(m, modules.Key(i+1))
		bar := strings.Split(m.View(), "\n")[0]
		assert.LessOrEqual(t, lipgloss.Width(bar), 80, "view %d", i+1)
		assert.Contains(t, bar, "1 Home")
		assert.Contains(t, bar, "2 People")
		assert.Contains(t, bar, "3 Follow-up")
		assert.Contains(t, bar, "4 Tasks")
	}
}
