package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The add standard (patterns/add-standard): every kind adds from one footer line, esc abandons it,
// and the cursor lands on the new object in its list. A new kind is added to this table.
func TestEveryKindAddsFromAFooterLineAndLandsOnTheObject(t *testing.T) {
	tests := []struct {
		name string
		view viewKind
		line string
		want string
	}{
		{"person", viewPeople, "Dana Dunn", "Dana Dunn"},
		{"organization", viewOrgs, "Zeta Works", "Zeta Works"},
		{"deal", viewDeals, "Zeta rollout | 5000", "Zeta rollout"},
		{"task", viewTasks, "Zeta follow up", "Zeta follow up"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.switchView(tc.view)
			m.settle()
			press(m, "a", "enter")
			require.Equal(t, modePrompt, m.mode, "a then enter opens the footer line")
			press(m, "esc")
			assert.Equal(t, modeNormal, m.mode, "esc abandons it")

			press(m, "a", "enter")
			typeText(m, tc.line)
			press(m, "enter")
			assert.Equal(t, modeNormal, m.mode)
			require.Less(t, m.cursor, len(m.rows))
			assert.Equal(t, tc.want, m.rows[m.cursor].title, "the cursor lands on the new %s", tc.name)
			assert.Equal(t, focusList, m.focus)
		})
	}
}
