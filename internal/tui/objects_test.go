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

// --- the registry ---

func TestActionsAreGeneratedFromTheObjectRegistry(t *testing.T) {
	m, _ := newTestModel(t)
	for _, o := range objectDefs() {
		for _, v := range []verb{vAdd, vEdit, vDelete, vRestore} {
			if !o.has(v) {
				continue
			}
			a := m.actionByID(o.actionID(v))
			require.NotNil(t, a, "%s %s", v, o.label)
			assert.Equal(t, strings.ToUpper(string(v)[:1])+string(v)[1:]+" "+o.label, a.title)
			assert.Equal(t, o.group, a.group)
		}
	}
	// One verb set: no per-object wording such as "Restore deleted task" or "New contact".
	for _, a := range m.actions {
		assert.False(t, strings.HasPrefix(a.title, "Restore deleted"), a.title)
		assert.NotEqual(t, "New contact", a.title)
	}
	// Link and Unlink exist for every kind an object relates to, and for tags.
	for _, other := range []string{"person", "organization", "deal", "task", "tag"} {
		assert.NotNil(t, m.actionByID("link-"+cliNoun(other)))
		assert.NotNil(t, m.actionByID("unlink-"+cliNoun(other)))
	}
}

func TestRelationsInTheRegistryMatchTheModel(t *testing.T) {
	for _, o := range objectDefs() {
		if o.kind == tagKind {
			continue // a tag is the pseudo kind of the tag relation, it relates to nothing itself
		}
		assert.Equal(t, model.RelatedKinds(o.kind), relationsOf(o.kind)[:len(relationsOf(o.kind))-1], o.kind)
		assert.Equal(t, tagKind, relationsOf(o.kind)[len(relationsOf(o.kind))-1], "every object can take a tag")
	}
}

// --- standard keys ---

func TestAddAndDeleteKeysAreTheSameInEveryObjectView(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()

	// Organizations: a then enter adds, d deletes, ctrl+z brings it back.
	runPalette(m, "organizations view")
	press(m, "a", "enter")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Globex")
	press(m, "a", "enter")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Initech")
	require.Len(t, openOrgs(t, m), 2)
	press(m, "d", "y")
	require.Len(t, openOrgs(t, m), 1)
	press(m, "ctrl+z")
	assert.Len(t, openOrgs(t, m), 2, "ctrl+z restores in the Organizations view")

	// Deals.
	goDeals(m)
	press(m, "a", "enter")
	require.Equal(t, modePrompt, m.mode)
	submit(m, "Site redesign")
	require.Len(t, openDeals(t, m), 1)
	press(m, "d", "y")
	assert.Empty(t, openDeals(t, m))
	press(m, "ctrl+z")
	assert.Len(t, openDeals(t, m), 1, "ctrl+z restores in the Deals view")

	// Tasks.
	press(m, "4")
	press(m, "a", "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "task-new", m.promptKind)
	submit(m, "Call back")
	tasks, err := m.tr.FindAll(ctx, model.TaskFilters{})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Empty(t, tasks[0].PersonIDs, "a task added in the Tasks view has no person until it is linked")
	press(m, "d", "y")
	assert.Empty(t, openTasks(t, m))
	press(m, "ctrl+z")
	assert.Len(t, openTasks(t, m), 1, "ctrl+z restores in the Tasks view")

	// People: a then enter adds, d deletes (archives), r restores in the Archived view.
	press(m, "2")
	before := len(m.people)
	press(m, "a", "enter")
	require.Equal(t, focusDetail, m.focus, "a then enter opens the blank contact form")
	typeText(m, "Dana Fox")
	press(m, "enter")
	assert.Len(t, m.people, before+1)
	press(m, "esc")
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	press(m, "y")
	assert.Len(t, m.people, before)
	press(m, "9") // Archived
	press(m, "r")
	assert.Len(t, m.people, before+1, "r restores in the Archived view")
}

func TestRestoreKeyDoesNothingWithoutADeletedObject(t *testing.T) {
	m, _ := newTestModel(t)
	runPalette(m, "organizations view")
	press(m, "r")
	assert.Equal(t, modeNormal, m.mode)
	assert.Empty(t, openOrgs(t, m))
}

func TestEnterOpensTheEditFormOfThePersonUnderTheCursor(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "enter")
	assert.Equal(t, focusDetail, m.focus)
	press(m, "esc")
	assert.Equal(t, focusList, m.focus)
}

// --- link and unlink ---

// linkVia presses key (L or U), picks the relation from the first picker, and types into the
// second one. It leaves the second picker open so the caller can choose.
func linkVia(m *Model, key, relation, typed string) {
	press(m, key)
	typeText(m, relation)
	press(m, "enter")
	if typed != "" {
		typeText(m, typed)
	}
}

// personOrg is the primary organization of a person, nil when there is none.
func personOrg(t *testing.T, m *Model, id int64) *int64 {
	t.Helper()
	items, err := m.links.Linked(context.Background(), repo.Ref{Kind: "person", ID: id}, "organization")
	require.NoError(t, err)
	for _, li := range items {
		if li.Primary {
			oid := li.ID
			return &oid
		}
	}
	return nil
}

// isLinked reports whether a and b have a live link.
func (m *Model) isLinked(a, b repo.Ref) bool {
	items, err := m.links.Linked(m.ctx, a, b.Kind)
	if err != nil {
		return false
	}
	for _, li := range items {
		if li.ID == b.ID {
			return true
		}
	}
	return false
}

// confirmLink answers the attribute line of the Link form with Enter (no attributes).
func confirmLink(m *Model) {
	if m.mode == modePrompt && m.promptKind == "link-attrs" {
		press(m, "enter")
	}
}

func TestLinkPersonToANewOrganizationAndUndo(t *testing.T) {
	m, _ := newTestModel(t)
	// Zed Adams (id 1) is under the cursor.
	linkVia(m, "l", "organization", "Globex")
	require.NotNil(t, m.ov)
	last := m.ov.items[m.ov.shown[len(m.ov.shown)-1]]
	assert.Equal(t, "create", last.id, "the last row is Create typed text")
	assert.Contains(t, last.label, "Globex")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode, "a membership asks for its attributes")
	assert.Equal(t, "link-attrs", m.promptKind)
	assert.Contains(t, plain(m.footerView()), "role | from | to | primary | notes", "the attributes show as the hint")
	press(m, "enter")

	orgs := openOrgs(t, m)
	require.Len(t, orgs, 1)
	require.NotNil(t, personOrg(t, m, 1), "the first organization of a person is primary")
	assert.Equal(t, orgs[0].ID, *personOrg(t, m, 1))
	assert.Contains(t, m.toast.text, "linked Zed Adams to Globex")
	assert.Contains(t, m.detail.View(), "Globex", "the person page shows an organization row")

	// Creating and linking is one undo step.
	press(m, "ctrl+z")
	assert.Nil(t, personOrg(t, m, 1))
	assert.Empty(t, openOrgs(t, m), "the created organization goes too")
	assert.Contains(t, m.toast.text, "Undone: link Zed Adams to Globex")
	press(m, "ctrl+y")
	require.Len(t, openOrgs(t, m), 1)
	assert.NotNil(t, personOrg(t, m, 1))
}

func TestLinkPicksAnExistingObjectAndAddsASecondOrganization(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	a, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	g, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)

	runPalette(m, "link organization")
	require.NotNil(t, m.ov)
	typeText(m, "acme")
	press(m, "enter")
	typeText(m, "CTO | 2019-03")
	press(m, "enter")
	assert.Equal(t, a.ID, *personOrg(t, m, 1))

	// A second organization is added, the first stays primary.
	runPalette(m, "link organization")
	typeText(m, "globex")
	press(m, "enter")
	typeText(m, "Advisor")
	press(m, "enter")
	assert.Equal(t, a.ID, *personOrg(t, m, 1))
	items, err := m.links.Linked(context.Background(), repo.Ref{Kind: "person", ID: 1}, "organization")
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, g.ID, items[1].ID)
	assert.Equal(t, "Advisor", *items[1].Role)
	assert.Equal(t, "CTO", *items[0].Role)
	assert.Equal(t, "2019-03", *items[0].StartedOn)
	assert.Len(t, openOrgs(t, m), 2, "no duplicate was created")

	press(m, "ctrl+z")
	items, _ = m.links.Linked(context.Background(), repo.Ref{Kind: "person", ID: 1}, "organization")
	require.Len(t, items, 1, "undo removes the second link")
	press(m, "ctrl+z")
	assert.Nil(t, personOrg(t, m, 1))
	press(m, "ctrl+y", "ctrl+y")
	items, _ = m.links.Linked(context.Background(), repo.Ref{Kind: "person", ID: 1}, "organization")
	require.Len(t, items, 2, "redo brings both back")
	assert.True(t, items[0].Primary)
}

func TestUnlinkWithOneLinkActsAtOnceAndUndoRelinks(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	_, err = m.links.Link(ctx, repo.Ref{Kind: "person", ID: 1}, repo.Ref{Kind: "organization", ID: o.ID})
	require.NoError(t, err)
	m.afterWrite()

	runPalette(m, "unlink organization")
	assert.Nil(t, personOrg(t, m, 1))
	assert.Contains(t, m.toast.text, "unlinked Zed Adams from Acme")
	press(m, "ctrl+z")
	require.NotNil(t, personOrg(t, m, 1))
	press(m, "ctrl+y")
	assert.Nil(t, personOrg(t, m, 1))

	// Nothing linked: a message, no picker.
	runPalette(m, "unlink organization")
	assert.Nil(t, m.ov)
	assert.Contains(t, m.status, "has no organization linked")
}

func TestLinkFromTheOrganizationSide(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	runPalette(m, "organizations view")
	linkVia(m, "l", "person", "bob")
	press(m, "enter")
	confirmLink(m)
	assert.Equal(t, o.ID, *personOrg(t, m, 2), "Bob Baker belongs to Acme")
	assert.Contains(t, m.detail.View(), "Bob Baker", "the organization page lists its people")

	// Unlink from the same side.
	press(m, "u")
	typeText(m, "person")
	press(m, "enter")
	assert.Nil(t, personOrg(t, m, 2))
}

func TestEveryRelationLinksFromBothSides(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, err)
	tk, err := m.tr.Create(ctx, model.CreateTaskInput{Title: "Send deck", Priority: "medium"})
	require.NoError(t, err)

	linked := func(a, b repo.Ref) bool { return m.isLinked(a, b) }
	person := repo.Ref{Kind: "person", ID: 1}
	org := repo.Ref{Kind: "organization", ID: o.ID}
	deal := repo.Ref{Kind: "deal", ID: d.ID}
	task := repo.Ref{Kind: "task", ID: tk.ID}

	// person -> deal (from the person), deal -> organization (from the deal), task -> person (from the task).
	runPalette(m, "link deal")
	typeText(m, "site")
	press(m, "enter")
	confirmLink(m)
	assert.True(t, linked(person, deal), "person to deal, from the person")

	goDeals(m)
	runPalette(m, "link organization")
	typeText(m, "acme")
	press(m, "enter")
	confirmLink(m)
	assert.True(t, linked(deal, org), "deal to organization, from the deal")

	runPalette(m, "link task")
	typeText(m, "send")
	press(m, "enter")
	assert.True(t, linked(deal, task), "task to deal, from the deal")

	press(m, "4")
	require.Equal(t, viewTasks, m.view)
	runPalette(m, "link person")
	typeText(m, "cara")
	press(m, "enter")
	assert.True(t, linked(task, repo.Ref{Kind: "person", ID: 3}), "task to person, from the task")

	// And back from the other side: the person page lists the deal and Link works on a task row.
	press(m, "2")
	assert.Contains(t, m.detail.View(), "Site", "the person page lists the deal")
}

func TestLinkTagToAnything(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	_, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)

	// A person.
	linkVia(m, "l", "tag", "vip")
	press(m, "enter")
	assert.True(t, m.hasTag(ctx, "person", 1, "vip"))
	press(m, "ctrl+z")
	assert.False(t, m.hasTag(ctx, "person", 1, "vip"))

	// An organization.
	runPalette(m, "organizations view")
	linkVia(m, "l", "tag", "hot")
	press(m, "enter")
	assert.True(t, m.hasTag(ctx, "organization", openOrgs(t, m)[0].ID, "hot"))
	runPalette(m, "unlink tag")
	assert.False(t, m.hasTag(ctx, "organization", openOrgs(t, m)[0].ID, "hot"))
}

func TestLinkIsBlockedInReadOnlyMode(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	press(m, "l")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.status, "read-only")
	runPalette(m, "link organization")
	assert.Nil(t, m.ov)
}

func TestLinkActionsAreGreyedOutWithoutAnObjectToLinkFrom(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "9") // Archived view: nothing to link
	for _, id := range []string{"link", "unlink", "link-org", "unlink-tag"} {
		assert.False(t, m.actionByID(id).enabled(m), id)
	}
	// An organization has no link to a task.
	runPalette(m, "organizations view")
	assert.False(t, m.actionByID("link-task").enabled(m))
}

// --- the person page ---

func TestPersonPageHasAnOrganizationRowThatLinksAndOpens(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)

	press(m, "tab")
	goTo(t, m, "organization")
	assert.True(t, m.form[m.fcur].empty)
	press(m, "enter") // an empty row links, from a typeahead under the row
	require.NotNil(t, m.edit)
	require.NotNil(t, m.edit.ta)
	typeText(m, "acme")
	press(m, "enter")
	confirmLink(m)
	assert.Equal(t, o.ID, *personOrg(t, m, 1))
	goTo(t, m, "organization")
	assert.Equal(t, "Acme", m.form[m.fcur].value)

	// d on the row asks to unlink.
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "unlink organization")
	press(m, "n")
	assert.NotNil(t, personOrg(t, m, 1))

	// Enter on a linked row opens the organization.
	press(m, "enter")
	assert.Equal(t, viewOrgs, m.view)
	assert.Equal(t, o.ID, m.rows[m.cursor].orgID)
}

// --- task edit ---

func TestEditTaskWithEnterInTheTasksView(t *testing.T) {
	m, id := taskDelModel(t)
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "send deck", m.input.Value())
	submit(m, "send the deck | 2026-12-01")
	got, err := m.tr.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "send the deck", got.Title)
	require.NotNil(t, got.DueAt)
	press(m, "ctrl+z")
	got, err = m.tr.FindByID(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, "send deck", got.Title)
}
