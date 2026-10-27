package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openOrgs(t *testing.T, m *Model) []*model.Organization {
	t.Helper()
	list, err := m.orr.FindAll(context.Background(), 0)
	require.NoError(t, err)
	return list
}

func openDeals(t *testing.T, m *Model) []*model.Deal {
	t.Helper()
	list, err := m.dlr.FindAll(context.Background(), model.DealFilters{})
	require.NoError(t, err)
	return list
}

// submit types a prompt line and presses enter.
func submit(m *Model, line string) {
	m.input.SetValue(line)
	press(m, "enter")
}

func TestOrgsAndDealsAreOnByDefaultAndFollowTheArrangement(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "v")
	require.NotNil(t, m.ov)
	var labels []string
	for _, it := range m.ov.items {
		labels = append(labels, it.label)
	}
	assert.Contains(t, labels, "Organizations")
	assert.Contains(t, labels, "Deals", "the default shows every module")
	press(m, "esc")
	assert.Equal(t, 5, m.viewPosition(viewOrgs))
	assert.Equal(t, 6, m.viewPosition(viewDeals))
	press(m, "5")
	assert.Equal(t, viewOrgs, m.view)
	press(m, "]")
	assert.Equal(t, viewDeals, m.view)
	press(m, "]")
	assert.Equal(t, viewBirthdays, m.view)

	// Turned off, Deals leaves the cycle but a link still opens it. ] goes to the first view and [ to the last.
	require.NoError(t, m.setModules(mustWith(t, m.mods, "deals", false)))
	m.switchView(viewDeals)
	assert.Contains(t, m.View(), "v Deals")
	press(m, "]")
	assert.Equal(t, viewHome, m.view, "] leaves a view that is off")
	m.switchView(viewDeals)
	press(m, "[")
	assert.Equal(t, viewSync, m.view)
}

func TestOrgAddEditDeleteRestoreWithUndo(t *testing.T) {
	m, _ := newTestModel(t)
	runPalette(m, "organizations view")
	require.Equal(t, viewOrgs, m.view)
	assert.Contains(t, m.View(), "No organizations")

	press(m, "a", "enter")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Initech | initech.com | Software | Makes TPS reports")
	list := openOrgs(t, m)
	require.Len(t, list, 1)
	assert.Equal(t, "Initech", list[0].Name)
	assert.Equal(t, "Software", *list[0].Industry)
	require.NotNil(t, m.toast)
	assert.Contains(t, m.toast.text, "organization added: Initech")
	assert.Equal(t, list[0].ID, m.rows[m.cursor].orgID)
	assert.Contains(t, m.View(), "initech.com")

	// Undo and redo of the add.
	press(m, "ctrl+z")
	assert.Empty(t, openOrgs(t, m))
	assert.Contains(t, m.toast.text, "Undone: add organization Initech")
	press(m, "ctrl+y")
	require.Len(t, openOrgs(t, m), 1)

	// Edit: enter opens the line with the current values, emptied fields are cleared.
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "Initech | initech.com | Software |  |  |  |  |  | Makes TPS reports", m.input.Value())
	submit(m, "Initrode | | Software")
	got := openOrgs(t, m)[0]
	assert.Equal(t, "Initrode", got.Name)
	assert.Nil(t, got.Domain)
	assert.Nil(t, got.Notes)
	press(m, "ctrl+z")
	got = openOrgs(t, m)[0]
	assert.Equal(t, "Initech", got.Name)
	assert.Equal(t, "initech.com", *got.Domain)
	assert.Equal(t, "Makes TPS reports", *got.Notes)
	assert.Contains(t, m.toast.text, "Undone: edit organization Initech")
	press(m, "ctrl+y")
	assert.Equal(t, "Initrode", openOrgs(t, m)[0].Name)

	// An empty name is refused and the prompt stays open with the text.
	press(m, "e")
	submit(m, " | x.com")
	assert.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.status, "needs a name")
	assert.Equal(t, "| x.com", m.input.Value())
	press(m, "esc")

	// Delete asks, n keeps it, y deletes, the palette restores.
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "Initrode")
	press(m, "n")
	assert.Len(t, openOrgs(t, m), 1)
	press(m, "d", "y")
	assert.Empty(t, openOrgs(t, m))
	assert.Equal(t, []int64{got.ID}, m.deleted["organization"])
	assert.Contains(t, m.toast.text, "organization deleted")
	runPalette(m, "restore organization")
	require.Len(t, openOrgs(t, m), 1)
	assert.Contains(t, m.toast.text, "organization restored")
	assert.Empty(t, m.deleted["organization"])
	press(m, "ctrl+z")
	assert.Empty(t, openOrgs(t, m), "restoring is undoable")
	press(m, "ctrl+z")
	require.Len(t, openOrgs(t, m), 1, "ctrl+z undoes the delete")
	assert.Contains(t, m.toast.text, "Undone: delete organization Initrode")
}

func TestOrgAddFromPaletteInAnyView(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "2")
	runPalette(m, "add organization")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Globex")
	assert.Equal(t, viewPeople, m.view)
	require.Len(t, openOrgs(t, m), 1)
	assert.Contains(t, m.toast.text, "organization added: Globex")
}

func TestDealAddEditStageDeleteRestoreWithUndo(t *testing.T) {
	m, _ := newTestModel(t)
	org, err := m.orr.Create(context.Background(), model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	goDeals(m)
	require.Equal(t, viewDeals, m.view)

	press(m, "a", "enter")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Site redesign | $12,500 | proposal | globex | Bob Baker | phase one")
	list := openDeals(t, m)
	require.Len(t, list, 1)
	d := list[0]
	assert.Equal(t, "Site redesign", d.Title)
	assert.Equal(t, 12500.0, *d.Value)
	assert.Equal(t, "proposal", d.Stage)
	assert.Equal(t, org.ID, *d.OrgID)
	assert.Equal(t, []int64{2}, d.PersonIDs)
	assert.Equal(t, "phase one", *d.Notes)
	body := m.View()
	assert.Contains(t, body, "Site redesign")
	assert.Contains(t, body, "12,500")
	assert.Contains(t, body, "Bob Baker")

	press(m, "ctrl+z")
	assert.Empty(t, openDeals(t, m))
	press(m, "ctrl+y")
	require.Len(t, openDeals(t, m), 1)

	// Stage picker.
	runID(m, "deal-stage")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Deal stage", m.ov.title)
	press(m, "5") // won
	assert.Equal(t, "won", openDeals(t, m)[0].Stage)
	assert.Contains(t, m.toast.text, "deal moved to won")
	press(m, "ctrl+z")
	assert.Equal(t, "proposal", openDeals(t, m)[0].Stage)
	assert.Contains(t, m.toast.text, "Undone: move deal Site redesign to won")

	// Edit keeps the linked names and clears what is emptied. Undo brings every field back.
	press(m, "enter")
	assert.Equal(t, "Site redesign | 12500 | proposal | Globex | Bob Baker | phase one", m.input.Value())
	submit(m, "Site v2 | | proposal | Globex")
	got := openDeals(t, m)[0]
	assert.Equal(t, "Site v2", got.Title)
	assert.Nil(t, got.Value)
	assert.Empty(t, got.PersonIDs)
	assert.Nil(t, got.Notes)
	assert.Equal(t, org.ID, *got.OrgID)
	press(m, "ctrl+z")
	got = openDeals(t, m)[0]
	assert.Equal(t, "Site redesign", got.Title)
	assert.Equal(t, 12500.0, *got.Value)
	assert.Equal(t, []int64{2}, got.PersonIDs)

	// Bad input keeps the prompt open.
	press(m, "e")
	submit(m, "Site | abc")
	assert.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.status, "not a number")
	press(m, "esc")
	press(m, "e")
	submit(m, "Site | | nope")
	assert.Contains(t, m.status, "stage must be one of")
	press(m, "esc")
	press(m, "e")
	submit(m, "Site | | lead | nobody")
	assert.Contains(t, m.status, "no organization matches")
	press(m, "esc")

	// Delete and restore.
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "Site redesign")
	press(m, "y")
	assert.Empty(t, openDeals(t, m))
	runPalette(m, "restore deal")
	require.Len(t, openDeals(t, m), 1)
	assert.Empty(t, m.deleted["deal"])
	press(m, "ctrl+z")
	assert.Empty(t, openDeals(t, m))
}

func TestOrgDetailListsPeopleAndDeals(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	org, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Globex", Domain: sp("globex.com")})
	require.NoError(t, err)
	_, err = m.pr.Update(ctx, 2, model.UpdatePersonInput{OrgID: &org.ID})
	require.NoError(t, err)
	_, err = m.dlr.Create(ctx, model.CreateDealInput{Title: "Renewal", Stage: "lead", OrgID: &org.ID})
	require.NoError(t, err)
	runPalette(m, "organizations view")
	detail := m.detail.View()
	assert.Contains(t, detail, "globex.com")
	assert.Contains(t, detail, "Bob Baker")
	assert.Contains(t, detail, "Renewal")
}

func TestOrgDealActionsAreReadOnlyAndViewScoped(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	runPalette(m, "organizations view")
	require.Equal(t, viewOrgs, m.view, "viewing is allowed")
	press(m, "a", "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.status, "read-only")
	assert.Empty(t, openOrgs(t, m))

	// Keys do nothing in other views.
	m.readOnly = false
	press(m, "2")
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode, "d deletes the person in the People view")
	assert.Contains(t, m.confirm.prompt, "archive")
	press(m, "n")
	assert.Empty(t, openOrgs(t, m))
	assert.True(t, m.actionByID("org-add").write)
	assert.False(t, m.actionByID("org-edit").enabled(m), "needs a selected organization")
}
