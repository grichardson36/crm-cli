package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The navigation standard (patterns/navigation-standard): every overlay and popup closes with esc and
// leaves the screen as it was, whatever else it does with the keys. One table, so a new overlay is
// added here and a deviation shows as a failing row.
func TestEveryOverlayClosesWithEsc(t *testing.T) {
	tests := []struct {
		name string
		open func(m *Model)
	}{
		{"palette", func(m *Model) { press(m, "ctrl+k") }},
		{"view options", func(m *Model) { press(m, "ctrl+o") }},
		{"help", func(m *Model) { press(m, "?") }},
		{"views screen", func(m *Model) { press(m, "v") }},
		{"add picker", func(m *Model) { press(m, "a") }},
		{"offer picker", func(m *Model) { m.env.DataDir = t.TempDir(); m.openOfferPicker(0) }},
		{"undo history", func(m *Model) {
			m.record("one", func() error { return nil }, func() error { return nil })
			m.openHistory(false)
		}},
		{"palette command line", func(m *Model) { press(m, "ctrl+k"); typeText(m, "task buy milk ") }},
		{"long picker with a filter", func(m *Model) {
			var items []overlayItem
			for i := 0; i < 20; i++ {
				items = append(items, overlayItem{id: string(rune('a' + i)), label: string(rune('A' + i))})
			}
			m.openPicker("Pick", items, func(*Model, overlayItem) {})
		}},
		{"redo history", func(m *Model) {
			m.record("one", func() error { return nil }, func() error { return nil })
			m.undoLast()
			m.openHistory(true)
		}},
		{"list editor",func(m *Model) { m.openListEditor("deal-stages") }},
		{"keys customize", func(m *Model) { m.openKeysCustomize() }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			view := m.view
			tc.open(m)
			assert.True(t, m.mode != modeNormal || m.help || m.ov != nil || m.view != view, "%s opens something", tc.name)
			press(m, "down", "esc")
			assert.Equal(t, modeNormal, m.mode, "esc leaves the %s", tc.name)
			assert.False(t, m.help, "esc closes help")
			assert.Nil(t, m.ov)
		})
	}
}

// A two-pane screen: right or tab enters the right pane, esc and left at the left edge go back to the
// list, and up and down move inside the pane that has focus.
func TestTwoPaneScreensFollowTheStandard(t *testing.T) {
	screens := []struct {
		name string
		open func(m *Model)
	}{
		{"settings", func(m *Model) { m.gotoSetting("deal-stages") }},
		{"tags", func(m *Model) { m.switchView(viewTags) }},
		{"people", func(m *Model) {}},
	}
	for _, sc := range screens {
		t.Run(sc.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			sc.open(m)
			if len(m.form) == 0 {
				t.Skip("nothing in the right pane of the test data")
			}
			press(m, "tab")
			assert.Equal(t, focusDetail, m.focus, "tab enters the right pane")
			press(m, "esc")
			assert.Equal(t, focusList, m.focus, "esc goes back to the list")
			press(m, "right")
			assert.Equal(t, focusDetail, m.focus, "right enters the right pane")
		})
	}
}
