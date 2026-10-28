package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bulk purge in the Archived view: palette only, typed count, same writes as a single purge.
func TestBulkPurgeNeedsTheTypedCount(t *testing.T) {
	setup := func(t *testing.T) (*Model, func() int) {
		m, pr := newTestModel(t)
		press(m, "d", "y", "d", "y", "d", "y") // archive all three people
		press(m, "9")
		require.Len(t, m.rows, 3)
		return m, func() int { return len(archivedIDs(t, pr)) }
	}
	run := func(m *Model, id string) {
		a := m.actionByID(id)
		require.NotNil(t, a, id)
		require.True(t, a.enabled(m), id)
		m.pendingCmd = m.runAction(a)
	}

	tests := []struct {
		name     string
		prepare  func(m *Model)
		action   string
		typed    string
		wantLeft int
		wantErr  bool
		wantMsg  string
	}{
		{"selected rows, right count", func(m *Model) { press(m, " ", "down", " ") }, "purge-selected", "purge 2", 1, false, "deleted 2 people forever"},
		{"selected rows, wrong count", func(m *Model) { press(m, " ", "down", " ") }, "purge-selected", "purge 3", 3, true, "did not match"},
		{"all remaining", func(m *Model) {}, "purge-all", "PURGE  3", 0, false, "deleted 3 people forever"},
		{"all remaining, bare word", func(m *Model) {}, "purge-all", "purge", 3, true, "did not match"},
		{"all remaining, a name is not enough", func(m *Model) {}, "purge-all", "Zed Adams", 3, true, "did not match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, left := setup(t)
			tt.prepare(m)
			run(m, tt.action)
			require.Equal(t, modePrompt, m.mode)
			assert.Contains(t, m.screen(), "purge ")
			typeText(m, tt.typed)
			press(m, "enter")
			assert.Equal(t, tt.wantErr, m.statusIsErr, m.status)
			assert.Contains(t, m.status, tt.wantMsg)
			assert.Equal(t, tt.wantLeft, left())
		})
	}

	t.Run("no bare key and d keeps its meaning", func(t *testing.T) {
		for _, id := range []string{"purge-selected", "purge-all"} {
			assert.Empty(t, newTestModelAction(t, id).keys, id)
		}
		m, left := setup(t)
		press(m, " ", "d")
		require.Equal(t, modePrompt, m.mode)
		assert.Contains(t, m.screen(), `to delete forever`, "d still asks for the name of the one row")
		assert.NotContains(t, m.screen(), "people forever")
		press(m, "esc")
		assert.Equal(t, 3, left())
	})

	t.Run("the type filter limits what purge all takes", func(t *testing.T) {
		m, _ := setup(t)
		m.archKind = "task"
		require.NoError(t, m.rebuild(false))
		assert.False(t, m.actionByID("purge-all").enabled(m), "no archived tasks, nothing to purge")
		m.archKind = "person"
		require.NoError(t, m.rebuild(false))
		assert.True(t, m.actionByID("purge-all").enabled(m))
	})

	t.Run("esc cancels", func(t *testing.T) {
		m, left := setup(t)
		run(m, "purge-all")
		press(m, "esc")
		assert.Equal(t, 3, left())
	})
}

func newTestModelAction(t *testing.T, id string) *action {
	t.Helper()
	m, _ := newTestModel(t)
	a := m.actionByID(id)
	require.NotNil(t, a)
	return a
}
