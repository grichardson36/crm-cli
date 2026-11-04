package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-027: a tag page uses the standard verbs. a adds an object to the tag, d takes the tag off the
// highlighted or marked objects (never deletes them), l and u are the standard Link and Unlink.

// friendPage opens the page of #friend with Zed, Bob and Cara (people 1 to 3) on it. Zed also has
// #Client, Acme (an organization) has #client.
func friendPage(t *testing.T) *Model {
	t.Helper()
	m := tagsModel(t)
	require.NoError(t, m.tagr.Apply(m.ctx, "person", 3, "friend"))
	m.afterWrite()
	cursorTag(t, m, "friend")
	press(m, "enter")
	require.True(t, m.onTagPage())
	require.Len(t, tagObjectRows(m), 3)
	return m
}

func personTags(t *testing.T, m *Model, id int64) []string {
	t.Helper()
	tags, err := m.tagr.GetForEntity(m.ctx, "person", id)
	require.NoError(t, err)
	var out []string
	for _, tg := range tags {
		out = append(out, tg.Name)
	}
	return out
}

func carriers(t *testing.T, m *Model, tag string) []int64 {
	t.Helper()
	ids, err := m.tagr.GetEntities(m.ctx, tag, "person")
	require.NoError(t, err)
	return ids
}

// S1
func TestTagPageLinkOnMarkedPeopleGivesThemAnotherTag(t *testing.T) {
	m := friendPage(t)
	press(m, "ctrl+a")
	press(m, "l")
	require.NotNil(t, m.ov)
	assert.NotContains(t, m.ov.title, "Give", "not the old give-this-tag picker")
	assert.Equal(t, "Link 3 people to", m.ov.title, "the relation picker of the marked people")

	typeText(m, "tag")
	press(m, "enter")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Link tag to 3 people", m.ov.title)
	typeText(m, "uf")
	press(m, "enter")

	assert.ElementsMatch(t, []int64{1, 2, 3}, carriers(t, m, "uf"))
	assert.ElementsMatch(t, []int64{1, 2, 3}, carriers(t, m, "friend"), "they keep the page's tag")
	assert.Empty(t, m.marked, "the selection is used up")

	before := len(m.hist)
	press(m, "ctrl+z")
	assert.Equal(t, before-1, len(m.hist), "one undo step")
	assert.Empty(t, carriers(t, m, "uf"))
	assert.ElementsMatch(t, []int64{1, 2, 3}, carriers(t, m, "friend"))
}

func TestTagPageLinkOnTheHighlightedObject(t *testing.T) {
	m := friendPage(t)
	press(m, "l")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Link Bob Baker to", m.ov.title, "the highlighted object, not the tag")
	press(m, "esc")

	// A page that lists an organization links the organization.
	m.switchView(viewTags)
	press(m, "esc")
	cursorTag(t, m, "client")
	press(m, "enter", "down", "down")
	require.Equal(t, "organization", m.rows[m.cursor].akind)
	press(m, "l")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Link Acme to", m.ov.title)
}

func TestTagPageLinkOfMixedKindsSaysSelectOneType(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "client")
	press(m, "enter", "ctrl+a", "l")
	assert.Nil(t, m.ov, "the existing bulk link takes one kind only")
	assert.Equal(t, "select objects of one type to link them together", m.status)
	assert.True(t, m.statusIsErr)
}

// S2
func TestTagPageUnlinkTakesAnotherTagOffTheMarkedPeopleOnly(t *testing.T) {
	m := friendPage(t)
	for id := int64(1); id <= 3; id++ {
		require.NoError(t, m.tagr.Apply(m.ctx, "person", id, "uf"))
	}
	m.afterWrite()
	press(m, "space", "space") // the page lists Bob, Cara, Zed
	press(m, "u")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Unlink from 2 people", m.ov.title)
	typeText(m, "tag")
	press(m, "enter")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Unlink tag from 2 people", m.ov.title)
	typeText(m, "uf")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode, "several ask for the count typed back")
	typeText(m, "unlink 2")
	press(m, "enter")

	assert.ElementsMatch(t, []int64{1}, carriers(t, m, "uf"), "Zed was not marked")
	assert.ElementsMatch(t, []int64{1, 2, 3}, carriers(t, m, "friend"), "the page's tag stays")
	before := len(m.hist)
	press(m, "ctrl+z")
	assert.Equal(t, before-1, len(m.hist))
	assert.ElementsMatch(t, []int64{1, 2, 3}, carriers(t, m, "uf"))
}

func TestTagPageUnlinkCanTakeThePageTagOffToo(t *testing.T) {
	m := friendPage(t)
	press(m, "space", "space")
	press(m, "u")
	typeText(m, "tag")
	press(m, "enter")
	typeText(m, "friend")
	press(m, "enter")
	typeText(m, "unlink 2")
	press(m, "enter")
	assert.ElementsMatch(t, []int64{1}, carriers(t, m, "friend"))
}

// S3
func TestTagPageAddGivesAnObjectThePageTag(t *testing.T) {
	m := friendPage(t)
	press(m, "a")
	require.NotNil(t, m.ov, "a opens the picker, not the add-an-object kind picker")
	assert.Equal(t, "Add to #friend", m.ov.title)
	var kinds []string
	for _, it := range m.ov.items {
		kinds = append(kinds, it.value)
	}
	assert.Contains(t, kinds, "person")
	assert.Contains(t, kinds, "organization", "objects of every kind")

	before := len(m.hist)
	typeText(m, "acme")
	press(m, "enter")
	orgs, err := m.tagr.GetEntities(m.ctx, "friend", "organization")
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	assert.Len(t, tagObjectRows(m), 4, "the page lists it now")
	assert.Equal(t, before+1, len(m.hist), "one undo step")
	press(m, "ctrl+z")
	orgs, _ = m.tagr.GetEntities(m.ctx, "friend", "organization")
	assert.Empty(t, orgs)
	assert.Len(t, tagObjectRows(m), 3)
}

// S4
func TestTagPageDRemovesTheTagFromTheHighlightedObjectOnly(t *testing.T) {
	m := friendPage(t)
	m.cursorOn(func(r row) bool { return r.title == "Zed Adams" })
	press(m, "d")
	assert.Equal(t, modeNormal, m.mode, "one object is removed at once")
	assert.ElementsMatch(t, []int64{2, 3}, carriers(t, m, "friend"))
	assert.Equal(t, []string{"Client"}, personTags(t, m, 1), "other tags are untouched")
	_, err := m.pr.FindByID(m.ctx, 1)
	assert.NoError(t, err, "the person still exists, not archived")
	assert.Equal(t, "removed #friend from 1 (ctrl+z undoes)", m.status)
	assert.Len(t, tagObjectRows(m), 2)

	press(m, "ctrl+z")
	assert.ElementsMatch(t, []int64{1, 2, 3}, carriers(t, m, "friend"))
}

func TestTagPageDWithMarksAsksTheCountThenOneUndoStep(t *testing.T) {
	m := friendPage(t)
	press(m, "ctrl+a", "d")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.input.Prompt, "remove #friend from 3 objects")
	typeText(m, "remove 2") // wrong count changes nothing
	press(m, "enter")
	assert.ElementsMatch(t, []int64{1, 2, 3}, carriers(t, m, "friend"))

	press(m, "d")
	require.Equal(t, modePrompt, m.mode)
	before := len(m.hist)
	typeText(m, "remove 3")
	press(m, "enter")
	assert.Empty(t, carriers(t, m, "friend"))
	assert.Equal(t, "removed #friend from 3 (ctrl+z undoes)", m.status)
	assert.Equal(t, before+1, len(m.hist), "one undo step")
	assert.Empty(t, m.marked)
	press(m, "ctrl+z")
	assert.ElementsMatch(t, []int64{1, 2, 3}, carriers(t, m, "friend"))
	assert.Equal(t, before, len(m.hist))
}

func TestTagPageDActsOnTheMarkedNotTheHighlighted(t *testing.T) {
	m := friendPage(t)
	press(m, "down", "space") // Cara marked, the cursor moves to Zed
	press(m, "d")
	assert.ElementsMatch(t, []int64{1, 2}, carriers(t, m, "friend"), "Cara only")
}

// S5
func TestTagPageDNeverDeletesOrArchivesAnObject(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "client")
	press(m, "enter")
	require.Len(t, tagObjectRows(m), 2) // Cara and Acme
	live := func(stage string) {
		t.Helper()
		for _, id := range []int64{1, 2, 3} {
			_, err := m.pr.FindByID(m.ctx, id)
			assert.NoError(t, err, "%s: person %d", stage, id)
		}
		orgs, err := m.orr.FindAll(m.ctx, 0)
		require.NoError(t, err)
		assert.Len(t, orgs, 1, "%s: Acme", stage)
	}
	histBefore := len(m.hist)
	press(m, "ctrl+a", "d")
	require.Equal(t, modePrompt, m.mode)
	live("before confirming")
	typeText(m, "remove 2")
	press(m, "enter")
	live("after confirming")
	assert.Empty(t, m.deleted["person"], "nothing went to the session's deleted list")
	assert.Empty(t, m.deleted["organization"])
	require.Len(t, m.hist, histBefore+1)
	assert.NotContains(t, strings.ToLower(m.hist[len(m.hist)-1].label), "delete")
	assert.NotContains(t, strings.ToLower(m.hist[len(m.hist)-1].label), "archive")
	press(m, "ctrl+z")
	live("after undo")
	assert.Len(t, tagObjectRows(m), 2)
}

// S6
func TestTagsListKeepsGiveAndTakeOffAndTheBarTagRowIsUnchanged(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "friend")
	require.True(t, m.actionByID("tag-link").enabled(m))
	require.True(t, m.actionByID("tag-unlink").enabled(m))
	assert.False(t, m.actionByID("tag-page-add").enabled(m), "a page action is off on the list")
	assert.False(t, m.actionByID("tag-page-remove").enabled(m))
	press(m, "l")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Give #friend to", m.ov.title)
	press(m, "esc")
	press(m, "u")
	require.NotNil(t, m.ov)
	assert.Equal(t, "Take #friend off", m.ov.title)
	press(m, "esc")

	saved := m.enterBarContext(barTarget{kind: "tag", id: 1, name: "friend"})
	defer m.leaveBarContext(saved)
	assert.True(t, m.actionByID("tag-link").enabled(m), "the bar row gives the tag")
	assert.True(t, m.actionByID("tag-unlink").enabled(m))
	assert.True(t, m.actionByID("tags-delete").enabled(m))
	assert.False(t, m.actionByID("tag-page-add").enabled(m))
	assert.False(t, m.actionByID("tag-page-remove").enabled(m))
}

// S7
func TestTagPageFooterPaletteAndTexts(t *testing.T) {
	m := friendPage(t)
	pairs := footPairs(m)
	assert.Equal(t, "add", labelFor(pairs, "a"))
	assert.Equal(t, "delete", labelFor(pairs, "d"))
	assert.Equal(t, "link", labelFor(pairs, "l"))
	assert.Equal(t, "unlink", labelFor(pairs, "u"))
	n := 0
	for _, p := range pairs {
		if p.key == "a" {
			n++
		}
	}
	assert.Equal(t, 1, n, "one add hint")

	assert.True(t, m.actionByID("tag-page-add").enabled(m))
	assert.True(t, m.actionByID("tag-page-remove").enabled(m))
	assert.Equal(t, "Add to this tag", m.actionByID("tag-page-add").title)
	assert.Equal(t, "Remove from this tag", m.actionByID("tag-page-remove").title)
	assert.False(t, m.actionByID("tag-link").enabled(m), "Give tag to an object is a Tags list action")
	assert.False(t, m.actionByID("tag-unlink").enabled(m))
	assert.False(t, m.actionByID("tags-delete").enabled(m), "d never deletes the tag from a page")
	assert.True(t, m.actionByID("link").enabled(m))
	assert.True(t, m.actionByID("unlink").enabled(m))

	m.refreshDetail(false)
	var preview []string
	for _, r := range m.form {
		preview = append(preview, r.value)
	}
	assert.Contains(t, preview, "enter opens it, d takes the tag off")

	// Footer on the list is unchanged.
	press(m, "esc")
	cursorTag(t, m, "friend")
	pairs = footPairs(m)
	assert.Equal(t, "delete", labelFor(pairs, "d"))
	assert.Equal(t, "link", labelFor(pairs, "l"))
}

func TestTagPageEmptyText(t *testing.T) {
	m := friendPage(t)
	press(m, "ctrl+a", "d")
	typeText(m, "remove 3")
	press(m, "enter")
	require.Empty(t, tagObjectRows(m))
	assert.Equal(t, "No object carries #friend. a adds one, esc goes back.", m.tagEmptyText())
	assert.Contains(t, m.screen(), "a adds one")
	press(m, "d") // nothing to remove
	assert.Equal(t, modeNormal, m.mode)
	press(m, "a")
	require.NotNil(t, m.ov, "a adds one to an empty page")
	assert.Equal(t, "Add to #friend", m.ov.title)
}

// S8
func TestTagPageWithChildrenRemovesOnlyFromObjectsThatCarryTheExactTag(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "work")
	press(m, "enter")
	m.setTagChildren(true)
	require.NoError(t, m.rebuild(false))
	require.Len(t, tagObjectRows(m), 2, "Zed (work) and Bob (work/clients/beta)")

	// Bob carries the tag only through a child: nothing to remove, says why.
	m.cursorOn(func(r row) bool { return r.title == "Bob Baker" })
	press(m, "d")
	assert.Equal(t, modeNormal, m.mode)
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "child tag")
	assert.Contains(t, personTags(t, m, 2), "work/clients/beta", "untouched")

	// Both marked: Zed loses #work, Bob is skipped and still has his child tag.
	press(m, "ctrl+a", "d")
	assert.Equal(t, modeNormal, m.mode, "one object is removed, so no typed count")
	assert.Equal(t, "removed #work from 1, skipped 1 that carry it through a child tag (ctrl+z undoes)", m.status)
	assert.NotContains(t, personTags(t, m, 1), "work")
	assert.Contains(t, personTags(t, m, 1), "work/clients/acme", "its child tag stays")
	assert.ElementsMatch(t, []string{"friend", "work/clients/beta"}, personTags(t, m, 2))
	press(m, "ctrl+z")
	assert.Contains(t, personTags(t, m, 1), "work")
}
