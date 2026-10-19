package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func personRef(id int64) repo.Ref { return repo.Ref{Kind: "person", ID: id} }

func linkedOrgs(t *testing.T, m *Model, pid int64) []model.LinkedObject {
	t.Helper()
	items, err := m.links.Linked(context.Background(), personRef(pid), "organization")
	require.NoError(t, err)
	return items
}

// --- the Link form ---

func TestLinkFormAsksRoleDatesAndPrimaryAndEditChangesThemInPlace(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	a, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)

	runPalette(m, "link organization")
	typeText(m, "acme")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.input.Prompt, "role | from | to | primary | notes")
	submit(m, "CTO | 2019-03 | | yes | joined via Pat")
	items := linkedOrgs(t, m, 1)
	require.Len(t, items, 1)
	assert.Equal(t, a.ID, items[0].ID)
	assert.Equal(t, "CTO", *items[0].Role)
	assert.Equal(t, "2019-03", *items[0].StartedOn)
	assert.True(t, items[0].Primary)
	assert.Equal(t, "joined via Pat", *items[0].Notes)

	// The person page shows it with role and dates.
	press(m, "tab")
	goTo(t, m, "organization")
	row := m.form[m.fcur]
	assert.Equal(t, "Acme", row.value)
	assert.Contains(t, row.suffix, "CTO")
	assert.Contains(t, row.suffix, "since 2019-03")
	assert.Contains(t, row.suffix, "primary")
	assert.False(t, row.dim)

	// E edits the same link: the prompt is filled, a field you empty is cleared.
	press(m, "E")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "CTO | 2019-03 |  | yes | joined via Pat", m.input.Value())
	submit(m, "Chief Technology Officer | 2019-03 | 2021-06")
	items = linkedOrgs(t, m, 1)
	require.Len(t, items, 1, "edited in place, not a second link")
	assert.Equal(t, "Chief Technology Officer", *items[0].Role)
	assert.Equal(t, "2021-06", *items[0].EndedOn)
	assert.False(t, items[0].Current)
	assert.Nil(t, items[0].Notes, "the emptied notes are cleared")
	assert.Contains(t, m.toast.text, "saved the link")

	// The ended membership is dim on the page.
	goTo(t, m, "organization")
	assert.True(t, m.form[m.fcur].dim)
	assert.Contains(t, m.form[m.fcur].suffix, "past")

	// Undo and redo of the attribute edit.
	press(m, "ctrl+z")
	items = linkedOrgs(t, m, 1)
	assert.Equal(t, "CTO", *items[0].Role)
	assert.Nil(t, items[0].EndedOn)
	assert.True(t, items[0].Primary)
	press(m, "ctrl+y")
	items = linkedOrgs(t, m, 1)
	assert.Equal(t, "Chief Technology Officer", *items[0].Role)
}

func TestLinkingAPickedOrganizationAgainEditsOrAddsAnotherStay(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	_, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)

	runPalette(m, "link organization")
	typeText(m, "acme")
	press(m, "enter")
	submit(m, "Engineer | 2015 | 2018")

	// One link exists and a membership may repeat: the form asks which.
	runPalette(m, "link organization")
	typeText(m, "acme")
	press(m, "enter")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "already linked")
	var labels []string
	for _, i := range m.ov.shown {
		labels = append(labels, m.ov.items[i].label)
	}
	require.Len(t, labels, 2)
	assert.Contains(t, labels[0], "Edit:")
	assert.Equal(t, "Add another link", labels[1])
	typeText(m, "another")
	press(m, "enter")
	submit(m, "Director | 2022")
	items := linkedOrgs(t, m, 1)
	require.Len(t, items, 2, "a second stay at the same organization")
	assert.Equal(t, "Director", *items[0].Role, "the current stay is listed first")
	assert.Equal(t, "Engineer", *items[1].Role)
	press(m, "ctrl+z")
	assert.Len(t, linkedOrgs(t, m, 1), 1)
}

func TestRoleTabCompletesFromRolesInUse(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	role := "Founder"
	_, err = m.links.Link(ctx, personRef(2), repo.Ref{Kind: "organization", ID: o.ID}, model.LinkAttrs{Role: &role})
	require.NoError(t, err)
	m.afterWrite()

	runPalette(m, "link organization")
	typeText(m, "acme")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.status, "Founder")
	m.input.SetValue("fou")
	press(m, "tab")
	assert.Equal(t, "Founder", m.input.Value())
}

func TestPersonToPersonLinkAsksKindAndShowsTheInverseLabel(t *testing.T) {
	m, _ := newTestModel(t)
	runPalette(m, "link person")
	typeText(m, "bob")
	press(m, "enter")
	require.NotNil(t, m.ov, "a kind picker follows the person picker")
	assert.Contains(t, m.ov.title, "Kind of link")
	var parent *overlayItem
	for i := range m.ov.items {
		if m.ov.items[i].value == "parent" {
			parent = &m.ov.items[i]
		}
	}
	require.NotNil(t, parent)
	assert.Contains(t, parent.desc, "Zed Adams is parent", "the picker shows both labels")
	assert.Contains(t, parent.desc, "Bob Baker is child")
	typeText(m, "parent")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode, "a person link needs no more attributes")

	sees := func(pid int64) []model.LinkedObject {
		items, err := m.links.Linked(context.Background(), personRef(pid), "person")
		require.NoError(t, err)
		return items
	}
	require.Len(t, sees(1), 1)
	assert.Equal(t, "child", sees(1)[0].Label)
	assert.Equal(t, "parent", sees(2)[0].Label)

	// Bob's page shows Zed as his parent, and enter opens Zed.
	m.selectPerson(2)
	press(m, "tab")
	goTo(t, m, "parent")
	assert.Equal(t, "Zed Adams", m.form[m.fcur].value)
	press(m, "enter")
	assert.Equal(t, int64(1), m.curPerson())

	// Undo removes the link, redo brings it back.
	press(m, "esc", "ctrl+z")
	assert.Empty(t, sees(1))
	press(m, "ctrl+y")
	assert.Len(t, sees(1), 1)
}

func TestUnlinkRowAndUndo(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	_, err = m.links.Link(ctx, personRef(1), repo.Ref{Kind: "organization", ID: o.ID}, model.LinkAttrs{})
	require.NoError(t, err)
	m.afterWrite()

	press(m, "tab")
	goTo(t, m, "organization")
	press(m, "d", "y")
	assert.Empty(t, linkedOrgs(t, m, 1))
	press(m, "ctrl+z")
	items := linkedOrgs(t, m, 1)
	require.Len(t, items, 1)
	assert.True(t, items[0].Primary, "the primary flag comes back with the link")
	press(m, "ctrl+y")
	assert.Empty(t, linkedOrgs(t, m, 1))
}

func TestMovingThePrimaryOrganizationIsOneUndoStep(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	a, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	g, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	yes := true
	_, err := m.links.Link(ctx, personRef(1), repo.Ref{Kind: "organization", ID: a.ID}, model.LinkAttrs{})
	require.NoError(t, err)
	_, err = m.links.Link(ctx, personRef(1), repo.Ref{Kind: "organization", ID: g.ID}, model.LinkAttrs{})
	require.NoError(t, err)
	m.afterWrite()

	runPalette(m, "link organization")
	typeText(m, "globex")
	press(m, "enter")
	press(m, "enter") // edit the existing link
	m.input.SetValue("| | | yes")
	press(m, "enter")
	_ = yes
	assert.Equal(t, g.ID, *personOrg(t, m, 1), "Globex is primary now")
	press(m, "ctrl+z")
	assert.Equal(t, a.ID, *personOrg(t, m, 1), "undo gives the flag back to Acme")
	press(m, "ctrl+y")
	assert.Equal(t, g.ID, *personOrg(t, m, 1))
}

// --- organization and deal pages ---

func TestOrganizationPageHasMembersParentsDealsAndOpensLinkedRows(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	acme, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme", Industry: sp("Rockets")})
	hold, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme Holdings"})
	role := "CEO"
	_, err := m.links.Link(ctx, personRef(1), repo.Ref{Kind: "organization", ID: acme.ID}, model.LinkAttrs{Role: &role, StartedOn: sp("2020")})
	require.NoError(t, err)
	kind := "parent"
	_, err = m.links.Link(ctx, repo.Ref{Kind: "organization", ID: hold.ID}, repo.Ref{Kind: "organization", ID: acme.ID}, model.LinkAttrs{Kind: &kind})
	require.NoError(t, err)
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Launch", Stage: "lead", OrgID: &acme.ID})
	require.NoError(t, err)
	_ = d

	runPalette(m, "organizations view")
	m.cursorOn(func(r row) bool { return r.orgID == acme.ID })
	view := m.detail.View()
	for _, want := range []string{"Acme", "Rockets", "Members", "Zed Adams", "CEO", "since 2020", "Parent organizations", "Acme Holdings", "Deals", "Launch"} {
		assert.Contains(t, view, want)
	}

	// Enter on a member row opens that person.
	press(m, "tab")
	goTo(t, m, "member")
	assert.Equal(t, "Zed Adams", m.form[m.fcur].value)
	press(m, "enter")
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, int64(1), m.curPerson())
	press(m, "esc")

	// The parent row opens the parent organization.
	runPalette(m, "organizations view")
	m.cursorOn(func(r row) bool { return r.orgID == acme.ID })
	press(m, "tab")
	goTo(t, m, "parent")
	press(m, "enter")
	assert.Equal(t, viewOrgs, m.view)
	assert.Equal(t, hold.ID, m.rows[m.cursor].orgID)
	// And from the holding company the same link reads child.
	if m.focus == focusList {
		press(m, "tab")
	}
	goTo(t, m, "child")
	assert.Equal(t, "Acme", m.form[m.fcur].value)
}

func TestOrgLinkRejectsACycleWithAMessage(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	a, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	b, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Beta"})
	kind := "parent"
	_, err := m.links.Link(ctx, repo.Ref{Kind: "organization", ID: a.ID}, repo.Ref{Kind: "organization", ID: b.ID}, model.LinkAttrs{Kind: &kind})
	require.NoError(t, err)
	m.afterWrite()

	runPalette(m, "organizations view")
	m.cursorOn(func(r row) bool { return r.orgID == b.ID })
	runPalette(m, "link organization")
	typeText(m, "acme")
	press(m, "enter")
	typeText(m, "parent")
	press(m, "enter")
	assert.Contains(t, m.status, "cycle")
	items, err := m.links.Linked(ctx, repo.Ref{Kind: "organization", ID: b.ID}, "organization")
	require.NoError(t, err)
	assert.Len(t, items, 1, "nothing new was linked")
}

func TestDealPageAndNoteOnAnOrganization(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	org, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "proposal", OrgID: &org.ID})
	require.NoError(t, err)
	role := "champion"
	_, err = m.links.Link(ctx, repo.Ref{Kind: "deal", ID: d.ID}, personRef(2), model.LinkAttrs{Role: &role})
	require.NoError(t, err)

	runPalette(m, "deals view")
	view := m.detail.View()
	for _, want := range []string{"Site", "proposal", "Organizations", "Globex", "primary", "People", "Bob Baker", "champion"} {
		assert.Contains(t, view, want)
	}

	// n adds a note to the deal, ctrl+z undoes it, the deal page lists it.
	press(m, "n")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "sent the proposal")
	list, err := m.ir.FindAll(ctx, model.InteractionFilters{DealID: &d.ID})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "sent the proposal", *list[0].Content)
	assert.Contains(t, m.detail.View(), "Recent")
	press(m, "ctrl+z")
	list, _ = m.ir.FindAll(ctx, model.InteractionFilters{DealID: &d.ID})
	assert.Empty(t, list)

	// Same on an organization.
	runPalette(m, "organizations view")
	press(m, "n")
	submit(m, "met the team")
	olist, err := m.ir.FindAll(ctx, model.InteractionFilters{OrgID: &org.ID})
	require.NoError(t, err)
	assert.Len(t, olist, 1)
}

func TestDealStageSetsAndClearsClosedAtInTheTUI(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, err)
	runPalette(m, "deals view")
	press(m, "s", "5") // won
	got, err := m.dlr.FindByID(ctx, d.ID)
	require.NoError(t, err)
	require.NotNil(t, got.ClosedAt)
	press(m, "s", "3") // proposal: reopened
	got, _ = m.dlr.FindByID(ctx, d.ID)
	assert.Nil(t, got.ClosedAt)
	press(m, "ctrl+z")
	got, _ = m.dlr.FindByID(ctx, d.ID)
	assert.Equal(t, "won", got.Stage)
	assert.NotNil(t, got.ClosedAt, "undo brings the close date back")
}

// --- the Archived view ---

func TestArchivedViewListsEveryKindFiltersByTypeAndRestores(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	org, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	deal, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	task, _ := m.tr.Create(ctx, model.CreateTaskInput{Title: "Send deck", Priority: "medium"})
	role := "CTO"
	_, err := m.links.Link(ctx, personRef(1), repo.Ref{Kind: "organization", ID: org.ID}, model.LinkAttrs{Role: &role})
	require.NoError(t, err)
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "organization", org.ID))
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "deal", deal.ID))
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "task", task.ID))
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "person", 3))

	press(m, "7") // Archived
	require.Equal(t, viewArchived, m.view)
	kinds := map[string]bool{}
	for _, r := range m.rows {
		kinds[r.akind] = true
	}
	assert.Equal(t, map[string]bool{"person": true, "organization": true, "deal": true, "task": true}, kinds, "every object kind is listed")
	assert.Contains(t, m.View(), "4 archived")

	// The type filter keeps one kind.
	press(m, "T")
	require.NotNil(t, m.ov)
	typeText(m, "deals")
	press(m, "enter")
	require.Len(t, m.rows, 1)
	assert.Equal(t, "deal", m.rows[0].akind)
	assert.Contains(t, m.View(), "1 archived deals")
	assert.Contains(t, m.detail.View(), "Site")

	// u restores the deal, with ctrl+z to undo.
	press(m, "u")
	live, err := m.dlr.FindByID(ctx, deal.ID)
	require.NoError(t, err)
	assert.Equal(t, "Site", live.Title)
	assert.Empty(t, m.rows)
	press(m, "ctrl+z")
	_, err = m.dlr.FindByID(ctx, deal.ID)
	assert.ErrorIs(t, err, model.ErrNotFound)
	require.Len(t, m.rows, 1, "the undone restore puts the deal back in the Archived list")

	// Back to all types: restore the organization and its membership comes back with it.
	press(m, "T")
	typeText(m, "all")
	press(m, "enter")
	m.cursorOn(func(r row) bool { return r.akind == "organization" })
	press(m, "u")
	items := linkedOrgs(t, m, 1)
	require.Len(t, items, 1, "restoring the organization brings its links back")
	assert.Equal(t, "CTO", *items[0].Role)

	// A person restores from the same view.
	m.cursorOn(func(r row) bool { return r.akind == "person" })
	press(m, "u")
	_, err = m.pr.FindByID(ctx, 3)
	assert.NoError(t, err)
	// And the task.
	m.cursorOn(func(r row) bool { return r.akind == "task" })
	press(m, "u")
	_, err = m.tr.FindByID(ctx, task.ID)
	assert.NoError(t, err)
	require.Len(t, m.rows, 1, "only the deal is still archived")
	assert.Equal(t, "deal", m.rows[0].akind)
}

func TestPurgeAppliesToArchivedPeopleOnly(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	org, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "organization", org.ID))
	press(m, "7")
	require.Len(t, m.rows, 1)
	assert.False(t, m.actionByID("purge").enabled(m), "an archived organization cannot be purged here")
}

// --- registry ---

func TestLinkActionsAndKeysExistForPersonToPerson(t *testing.T) {
	m, _ := newTestModel(t)
	a := m.actionByID("link-person")
	require.NotNil(t, a)
	assert.True(t, a.enabled(m), "a person links to a person")
	assert.True(t, strings.Contains(a.desc, "link"))
	assert.NotNil(t, m.actionByID("link-edit"))
	assert.NotNil(t, m.actionByID("archived-type"))
	assert.NotNil(t, m.actionByID("note-object"))
}

func TestReferredByRendersFromTheViewingSideInTheTUI(t *testing.T) {
	m, _ := newTestModel(t)
	kind := "referred-by"
	_, err := m.links.Link(context.Background(), personRef(1), personRef(2), model.LinkAttrs{Kind: &kind})
	require.NoError(t, err)
	m.afterWrite()
	press(m, "tab")
	goTo(t, m, "referred-by")
	assert.Equal(t, "Bob Baker", m.form[m.fcur].value, "Zed was referred by Bob")
	m.selectPerson(2)
	m.focus = focusDetail
	m.syncDetail()
	goTo(t, m, "referred")
	assert.Equal(t, "Zed Adams", m.form[m.fcur].value, "Bob referred Zed")
}

func TestDealMovedBetweenClosedStagesIsRestamped(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, err)
	runPalette(m, "deals view")
	press(m, "s", "5") // won
	_, err = m.db.Exec(`UPDATE deals SET closed_at = '2000-01-01 00:00:00' WHERE id = ?`, d.ID)
	require.NoError(t, err)
	m.afterWrite()
	press(m, "s", "6") // lost
	got, err := m.dlr.FindByID(ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, "lost", got.Stage)
	require.NotNil(t, got.ClosedAt)
	assert.Equal(t, "2026-10-13 12:00:00", *got.ClosedAt, "re-stamped with the model clock")
}
