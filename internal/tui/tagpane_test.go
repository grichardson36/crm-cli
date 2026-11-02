package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// objectRows are the tagged-object rows of the right pane.
func objectRows(m *Model) []int {
	var idx []int
	for i, r := range m.form {
		if r.kind == rkLinked && r.li != nil && r.li.Relation == tagKind {
			idx = append(idx, i)
		}
	}
	return idx
}

func TestTagPaneListsTheTaggedObjects(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "client")
	m.refreshDetail(true)
	rows := objectRows(m)
	require.Len(t, rows, 2, "client is on one person and one organization")
	assert.Equal(t, "Objects", m.form[rows[0]].head)
	var names []string
	for _, i := range rows {
		names = append(names, m.form[i].value)
	}
	assert.ElementsMatch(t, []string{"Cara Cole", "Acme"}, names)
}

func TestTagPaneEnterOpensAndDTakesTheTagOff(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "client")
	m.refreshDetail(true)
	rows := objectRows(m)
	require.NotEmpty(t, rows)
	press(m, "tab") // into the pane
	require.Equal(t, focusDetail, m.focus)
	m.fcur = rows[0]
	for _, i := range rows {
		if m.form[i].value == "Acme" {
			m.fcur = i
		}
	}
	m.syncDetail()
	orgID := m.form[m.fcur].id
	require.True(t, m.hasTag(m.ctx, "organization", orgID, "client"))
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode, "d asks before it takes the tag off")
	press(m, "y")
	assert.False(t, m.hasTag(m.ctx, "organization", orgID, "client"))
}
