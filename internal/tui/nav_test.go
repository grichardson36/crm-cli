package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewSwitcherListsEveryViewWithItsKey(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "v")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "Go to view", m.ov.title)
	require.Len(t, m.ov.items, int(numViews))
	for i, it := range m.ov.items {
		assert.Equal(t, viewNames[i], it.label)
		assert.Equal(t, string(rune('1'+i)), it.hint)
	}
	assert.Equal(t, "current view", m.ov.items[viewPeople].desc)

	// A number key picks at once.
	press(m, "6")
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
	press(m, "8")
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
	click(m, 3, overlayTop+int(viewBirthdays))
	assert.Equal(t, viewBirthdays, m.view)
	assert.Contains(t, strings.Split(m.View(), "\n")[0], "v Birthdays")
}

func TestHelpAndFooterMentionNewKeys(t *testing.T) {
	m, _ := newTestModel(t)
	help := m.helpText()
	for _, want := range []string{"list every view", "run the selected source", "would change, writes nothing", "create a person from the selected inbox", "attach the selected inbox", "dismiss the selected inbox", "first pending inbox record", "new type for the note", "bring back the note deleted", "open the person (People view"} {
		assert.Contains(t, help, want)
	}
	assert.Contains(t, m.hintsLine(), "help")
}

func TestTabBarNeverExceeds80Columns(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for v := 1; v <= int(numViews); v++ {
		press(m, string(rune('0'+v)))
		bar := strings.Split(m.View(), "\n")[0]
		assert.LessOrEqual(t, lipgloss.Width(bar), 80, "view %d", v)
		assert.Contains(t, bar, "1 Home")
		assert.Contains(t, bar, "2 People")
		assert.Contains(t, bar, "3 Follow-up")
		assert.Contains(t, bar, "4 Tasks")
	}
}
