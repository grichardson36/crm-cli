package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-019, TUI door: the person page has a "middle name" detail field after the last name. It edits
// and clears like the other text fields. The list row and the page title keep "First Last".
func TestMiddleNameDetailFieldEditsAndClears(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "tab")
	goTo(t, m, "last name")
	assert.Equal(t, "middle name", m.form[m.fcur+1].label, "the field sits right after the last name")

	goTo(t, m, "middle name")
	press(m, "enter")
	require.Equal(t, modeEdit, m.mode)
	replaceText(m, "Quill")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	got := personByID(t, pr, 1)
	require.NotNil(t, got.MiddleName)
	assert.Equal(t, "Quill", *got.MiddleName)
	assert.Equal(t, "middle name", m.form[m.fcur].label, "the cursor stays on the row")
	assert.Contains(t, m.screen(), "Quill")
	assert.Equal(t, "Zed Adams", m.formTitle[:len("Zed Adams")], "the title keeps First Last")

	// d asks first, y clears it.
	press(m, "d")
	assert.Equal(t, modeConfirm, m.mode)
	press(m, "y")
	assert.Equal(t, modeNormal, m.mode)
	cleared := personByID(t, pr, 1).MiddleName
	assert.True(t, cleared == nil || *cleared == "")
}

// The compare screen of a merge lists the middle name as a field, so a merge can choose it.
func TestMiddleNameIsAMergeCompareField(t *testing.T) {
	m, _ := dupeModel(t)
	_, err := m.db.Exec(`UPDATE people SET middle_name = 'Quill' WHERE id = 4`)
	require.NoError(t, err)
	press(m, "8", "down", "enter")
	require.NotNil(t, m.cmp)
	i := rowIndex(m.cmp, "middle name")
	require.GreaterOrEqual(t, i, 0)
	assert.Equal(t, "Quill", m.cmp.finalText(i), "a value beats an empty one")
}
