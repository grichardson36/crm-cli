package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaletteHasOneEntryPerAdvancedScreen(t *testing.T) {
	count := map[string]int{}
	for _, a := range buildActions() {
		lower := strings.ToLower(a.title)
		switch {
		case lower == "company review" || lower == "company review view":
			count["company"]++
			assert.Equal(t, "Company review", a.title)
		case lower == "organizations view" || lower == "organizations":
			count["orgs"]++
		case lower == "deals view" || lower == "deals":
			count["deals"]++
		}
	}
	assert.Equal(t, map[string]int{"company": 1, "orgs": 1, "deals": 1}, count)
}

func TestNewOrganizationThatExistsAsksUseCreateOrCancel(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	_, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Blue Origin"})
	require.NoError(t, err)
	runPalette(m, "organizations view")

	press(m, "a")
	submit(m, "BLUE ORIGIN |  |  | ")
	require.NotNil(t, m.ov, "a prompt asks what to do")
	assert.Contains(t, m.ov.title, "already exists")
	labels := []string{}
	for _, it := range m.ov.items {
		labels = append(labels, it.label)
	}
	assert.Equal(t, []string{"Use Blue Origin", "Create anyway", "Cancel"}, labels)
	press(m, "esc")
	assert.Len(t, openOrgs(t, m), 1, "cancel creates nothing")

	press(m, "a")
	submit(m, "blue origin, Inc. |  |  | ")
	press(m, "enter") // use the existing one
	assert.Len(t, openOrgs(t, m), 1, "using the existing one creates nothing")

	press(m, "a")
	submit(m, "Blue Origin |  |  | ")
	press(m, "down", "enter") // create anyway
	assert.Len(t, openOrgs(t, m), 2)
	press(m, "ctrl+z")
	assert.Len(t, openOrgs(t, m), 1, "creating anyway is one undo step")
}

func TestMergeMarkedOrganizationsIsOneUndoStep(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	dom := "blueorigin.com"
	for _, in := range []model.CreateOrgInput{
		{Name: "BLUE ORIGIN", AllowDuplicate: true},
		{Name: "Blue Origin", Domain: &dom, AllowDuplicate: true},
		{Name: "Other", AllowDuplicate: true},
	} {
		_, err := m.orr.Create(ctx, in)
		require.NoError(t, err)
	}
	runPalette(m, "organizations view")
	require.Len(t, m.rows, 3)

	press(m, "M")
	assert.Nil(t, m.ov, "nothing to merge without marks")
	assert.Contains(t, m.status, "mark two")

	press(m, " ", " ") // marks the first two rows, the cursor moves down
	assert.Len(t, m.markedOrgIDs(), 2)
	assert.Contains(t, m.View(), "✓")
	press(m, "M")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "Keep which")
	press(m, "enter")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "merge")
	press(m, "y")

	assert.Len(t, openOrgs(t, m), 2)
	assert.Empty(t, m.marked)
	press(m, "ctrl+z")
	assert.Len(t, openOrgs(t, m), 3, "one undo step brings the duplicate back")
	press(m, "ctrl+y")
	assert.Len(t, openOrgs(t, m), 2)
}
