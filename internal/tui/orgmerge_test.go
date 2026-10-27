package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaletteHasOneEntryPerAdvancedScreen(t *testing.T) {
	count := map[string]int{}
	for _, a := range buildActions() {
		lower := strings.ToLower(a.title)
		switch {
		case lower == "organizations view" || lower == "organizations":
			count["orgs"]++
		case lower == "deals view" || lower == "deals":
			count["deals"]++
		}
	}
	assert.Equal(t, map[string]int{"orgs": 1, "deals": 1}, count)
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

func TestOrgEditLineCarriesNativeFieldsAndOldLineStillWorks(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	logo := "a.png"
	_, err = m.orr.Update(ctx, o.ID, model.UpdateOrgInput{OrgNativeFields: model.OrgNativeFields{LogoPath: &logo}})
	require.NoError(t, err)
	runPalette(m, "organizations view")

	press(m, "a")
	submit(m, "Globex | globex.com | Energy | 51-200 | Austin, TX, US | https://globex.com | https://linkedin.com/company/globex | Makes things | notes here")
	var g *model.Organization
	for _, x := range openOrgs(t, m) {
		if x.Name == "Globex" {
			g = x
		}
	}
	require.NotNil(t, g)
	assert.Equal(t, "51-200", *g.Size)
	assert.Equal(t, "Austin, TX, US", g.Location())
	assert.Equal(t, "https://globex.com", *g.Website)
	assert.Equal(t, "Makes things", *g.Description)
	assert.Equal(t, "notes here", *g.Notes)

	// The edit line is filled with every field, the old four field form leaves the native ones alone.
	line := orgLine(g)
	assert.Equal(t, 8, strings.Count(line, "|"))
	_, err = m.saveOrgEdit(g.ID, "Globex | globex.com | Oil")
	require.NoError(t, err)
	got, _ := m.orr.FindByID(ctx, g.ID)
	assert.Equal(t, "Oil", *got.Industry)
	assert.Equal(t, "51-200", *got.Size)
	assert.Equal(t, "Austin, TX, US", got.Location())

	// A full line clears a field that is empty in it, and the logo path is kept.
	_, err = m.saveOrgEdit(o.ID, "Acme |  |  |  | Paris |  |  |  | ")
	require.NoError(t, err)
	got, _ = m.orr.FindByID(ctx, o.ID)
	assert.Equal(t, "Paris", *got.City)
	assert.Equal(t, "a.png", *got.LogoPath)

	m.form = m.buildOrgForm(g.ID)
	var shown []string
	for _, r := range m.form {
		shown = append(shown, r.label+"="+r.value)
	}
	joined := strings.Join(shown, "\n")
	assert.Contains(t, joined, "location=Austin, TX, US")
	assert.Contains(t, joined, "size=51-200")
	assert.Contains(t, joined, "website=https://globex.com")
}

// goDeals turns the Deals module on (it is off by default) and opens the view from the palette.
func goDeals(m *Model) {
	m.mods, _ = m.mods.With("deals", true)
	m.patchModuleActions()
	runPalette(m, "deals view")
}

func mustWith(t *testing.T, a modules.Arrangement, name string, on bool) modules.Arrangement {
	t.Helper()
	n, err := a.With(name, on)
	require.NoError(t, err)
	return n
}

func modelOrg(name string) model.CreateOrgInput { return model.CreateOrgInput{Name: name} }
