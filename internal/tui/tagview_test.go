package tui

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tagsModel is the test model on the Tags view. Zed, Bob and Cara are people 1 to 3. Bob has "friend".
// Added here: Zed has "friend" and "Client", Cara has "client", Acme (an organization) has "client",
// "idle" is a name no object carries and "old" was removed from Cara.
func tagsModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	o, err := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	for _, a := range []struct {
		kind string
		id   int64
		tag  string
	}{{"person", 1, "friend"}, {"person", 1, "Client"}, {"person", 3, "client"}, {"organization", o.ID, "client"}, {"person", 3, "old"}} {
		require.NoError(t, m.tagr.Apply(m.ctx, a.kind, a.id, a.tag))
	}
	require.NoError(t, m.tagr.Remove(m.ctx, "person", 3, "old"))
	_, err = m.tagr.FindOrCreate(m.ctx, "idle")
	require.NoError(t, err)
	m.afterWrite()
	m.switchView(viewTags)
	return m
}

func tagTitles(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		if !r.skip {
			out = append(out, r.tag)
		}
	}
	return out
}

func cursorTag(t *testing.T, m *Model, name string) {
	t.Helper()
	m.cursorOn(func(r row) bool { return r.tag == name && !r.skip })
	require.Equal(t, name, m.cursorTagName())
}

func TestTagsListCountsAndBreakdown(t *testing.T) {
	m := tagsModel(t)
	assert.Equal(t, []string{"Client", "client", "friend"}, tagTitles(m), "in use, by name, unused ones are filtered out")
	want := map[string][2]string{
		"friend": {"2", "2 people"},
		"client": {"2", "1 person, 1 org"},
		"Client": {"1", "1 person"},
	}
	for _, r := range m.rows {
		assert.Equal(t, want[r.tag][0], r.meta, r.tag)
		assert.Equal(t, want[r.tag][1], r.sub, r.tag)
	}
	assert.Equal(t, "3 tags, in use", m.tagCountText())
	assert.Contains(t, plain(m.screen()), "friend")
}

func TestTagsViewOptionsSortAndFilter(t *testing.T) {
	tests := []struct {
		name         string
		sort, filter string
		want         []string
	}{
		{"default is by name, in use", "", "", []string{"Client", "client", "friend"}},
		{"by number of objects", "count", "", []string{"client", "friend", "Client"}},
		{"unused tags", "", "unused", []string{"idle", "old"}},
		{"possible case duplicates", "", "duplicates", []string{"client", "Client"}},
		{"all tags", "", "all", []string{"Client", "client", "friend", "idle", "old"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tagsModel(t)
			if tt.sort != "" {
				m.sortChoice = map[viewKind]string{viewTags: tt.sort}
			}
			m.tagFilter = tt.filter
			require.NoError(t, m.rebuild(false))
			assert.Equal(t, tt.want, tagTitles(m))
		})
	}

	// The shared ctrl+o overlay offers both options.
	m := tagsModel(t)
	press(m, "ctrl+o")
	require.NotNil(t, m.ov)
	require.Len(t, m.ov.items, 2)
	assert.Contains(t, m.ov.items[0].label, "Sort")
	assert.Contains(t, m.ov.items[1].label, "Filter")
	press(m, "down", "right") // filter: unused tags
	assert.Equal(t, "unused", m.tagFilter)
	assert.Equal(t, []string{"idle", "old"}, tagTitles(m))
	press(m, "esc")
}

func TestTagsDuplicatesFilterGroupsSpellingsUnderADivider(t *testing.T) {
	m := tagsModel(t)
	m.tagFilter = "duplicates"
	require.NoError(t, m.rebuild(false))
	require.Len(t, m.rows, 3)
	assert.True(t, m.rows[0].skip && m.rows[0].rule)
	assert.Equal(t, "merge into client", m.rows[0].title)
	assert.Equal(t, "2 spellings", m.rows[0].meta)
}

func TestTagsSearchNarrowsTheList(t *testing.T) {
	m := tagsModel(t)
	press(m, "ctrl+k")
	typeText(m, "fri")
	var found bool
	for _, idx := range m.ov.shown {
		if m.ov.items[idx].id == "tags-all" {
			found = true
			m.ov.cursor = posOf(m.ov.shown, idx)
		}
	}
	require.True(t, found, "the bar offers to narrow the Tags list")
	press(m, "enter")
	assert.Equal(t, viewTags, m.view)
	assert.Equal(t, []string{"friend"}, tagTitles(m))
	press(m, "esc")
	assert.Equal(t, []string{"Client", "client", "friend"}, tagTitles(m), "esc clears the search")
}

func posOf(list []int, v int) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return 0
}

func TestTagPageGroupsObjectsByKindAndOpensThem(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "client")
	press(m, "enter")
	require.True(t, m.onTagPage())
	var heads, objs []string
	for _, r := range m.rows {
		if r.skip {
			heads = append(heads, r.title+"|"+r.meta+"|"+r.akind)
			assert.True(t, r.rule)
		} else {
			objs = append(objs, r.akind+":"+r.title)
		}
	}
	assert.Equal(t, []string{"People|1|person", "Organizations|1|organization"}, heads, "dim headings with the kind and its count")
	assert.Equal(t, []string{"person:Cara Cole", "organization:Acme"}, objs)
	assert.Equal(t, m.cursor, 1, "the cursor skips the heading")

	// A kind heading is drawn in the kind color of the related segment.
	assert.Contains(t, m.screen(), "People")

	// enter on an object opens it in its own view.
	press(m, "down", "enter")
	assert.Equal(t, viewOrgs, m.view)

	// esc goes back to the list with the cursor on the tag.
	m.switchView(viewTags)
	assert.True(t, m.onTagPage(), "the page is remembered")
	press(m, "esc")
	assert.False(t, m.onTagPage())
	assert.Equal(t, "client", m.cursorTagName())

	cursorTag(t, m, "friend")
	press(m, "enter", "enter")
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, "Bob Baker", m.rows[m.cursor].title, "the first person of the page")
}

func TestTagRenameInPlaceFromTheDetailPane(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "friend")
	press(m, "tab")
	require.Equal(t, focusDetail, m.focus)
	require.Equal(t, rkTyped, m.form[m.fcur].kind, "the name is a typed field")
	press(m, "enter")
	require.Equal(t, modeEdit, m.mode)
	replaceText(m, "pal")
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Equal(t, []string{"Client", "client", "pal"}, tagTitles(m))
	ids, _ := m.tagr.GetEntities(m.ctx, "pal", "person")
	assert.ElementsMatch(t, []int64{1, 2}, ids, "renamed everywhere")
	assert.Equal(t, "pal", m.cursorTagName(), "the cursor stays on the tag")

	press(m, "ctrl+z")
	assert.Contains(t, tagTitles(m), "friend")
}

func TestTagRenameIntoAnExistingNameAsksToMergeAsOneStep(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "client")
	press(m, "tab", "enter")
	require.Equal(t, modeEdit, m.mode)
	replaceText(m, "friend")
	press(m, "enter")
	require.Equal(t, modeConfirm, m.mode, "asks once")
	assert.Contains(t, m.confirm.prompt, "merge #client into #friend")
	press(m, "y")
	assert.Equal(t, []string{"Client", "friend"}, tagTitles(m))
	ids, _ := m.tagr.GetEntities(m.ctx, "friend", "organization")
	assert.Len(t, ids, 1, "the organization moved")

	before := len(m.hist)
	press(m, "ctrl+z")
	assert.Equal(t, before-1, len(m.hist), "one undo step")
	assert.Contains(t, tagTitles(m), "client")
	ids, _ = m.tagr.GetEntities(m.ctx, "client", "organization")
	assert.Len(t, ids, 1)
}

func TestTagsDeleteUntagsEverythingInOneStepAndBulkAsksTypedCount(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "friend")
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "delete tag #friend from 2 records")
	press(m, "y")
	assert.Equal(t, []string{"Client", "client"}, tagTitles(m))
	press(m, "ctrl+z")
	assert.Contains(t, tagTitles(m), "friend")
	ids, _ := m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.Len(t, ids, 2)

	// Bulk: select two tags, d asks for the count typed back, one undo step.
	cursorTag(t, m, "Client")
	press(m, " ", " ") // Client, then client
	require.Len(t, m.marked, 2)
	press(m, "d")
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "delete 2")
	press(m, "enter")
	assert.Equal(t, []string{"friend"}, tagTitles(m))
	before := len(m.hist)
	press(m, "ctrl+z")
	assert.Equal(t, before-1, len(m.hist))
	assert.Equal(t, []string{"Client", "client", "friend"}, tagTitles(m))
}

func TestTagsDeleteOnAnUnusedNameRemovesIt(t *testing.T) {
	m := tagsModel(t)
	m.tagFilter = "unused"
	require.NoError(t, m.rebuild(false))
	cursorTag(t, m, "idle")
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "unused tag #idle")
	press(m, "y")
	assert.Equal(t, []string{"old"}, tagTitles(m))
	press(m, "ctrl+z")
	assert.Equal(t, []string{"idle", "old"}, tagTitles(m))
}

func TestTagsMergeSelectedIntoHighlighted(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "Client")
	press(m, " ", " ") // Client and client selected, cursor on friend
	require.Equal(t, "friend", m.cursorTagName())
	press(m, "m")
	require.Equal(t, modePrompt, m.mode, "several tags ask for the count typed back")
	typeText(m, "merge 2")
	press(m, "enter")
	assert.Equal(t, []string{"friend"}, tagTitles(m))
	ids, _ := m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.ElementsMatch(t, []int64{1, 2, 3}, ids)
	assert.Empty(t, m.marked)

	before := len(m.hist)
	press(m, "ctrl+z")
	assert.Equal(t, before-1, len(m.hist), "one undo step for the whole bulk merge")
	assert.Equal(t, []string{"Client", "client", "friend"}, tagTitles(m))

	// One selected tag and a different highlighted one: y/n.
	cursorTag(t, m, "Client")
	press(m, " ")
	require.Equal(t, "client", m.cursorTagName())
	press(m, "m")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "merge #Client into #client")
	press(m, "esc")

	// Nothing selected: pick the tag to merge into.
	m.marked = map[int64]bool{}
	press(m, "m")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "Merge #client into")
}

func TestTagPageLinkAndUnlinkObjects(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "friend")
	press(m, "enter")
	require.True(t, m.onTagPage())

	// l links another object to the tag.
	press(m, "l")
	require.NotNil(t, m.ov)
	typeText(m, "cara")
	press(m, "enter")
	ids, _ := m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.ElementsMatch(t, []int64{1, 2, 3}, ids)
	assert.Len(t, tagObjectRows(m), 3)

	// u unlinks the highlighted one at once, undoable.
	press(m, "u")
	ids, _ = m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.Len(t, ids, 2)
	press(m, "ctrl+z")
	ids, _ = m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.Len(t, ids, 3)

	// Bulk: select all, u asks for the count typed back.
	press(m, "ctrl+a", "u")
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "unlink 3")
	press(m, "enter")
	ids, _ = m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.Empty(t, ids)
	before := len(m.hist)
	press(m, "ctrl+z")
	assert.Equal(t, before-1, len(m.hist), "one step")
	ids, _ = m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.Len(t, ids, 3)
}

func tagObjectRows(m *Model) []row {
	var out []row
	for _, r := range m.rows {
		if !r.skip {
			out = append(out, r)
		}
	}
	return out
}

func TestTagsLinkAndUnlinkFromTheListAreBulk(t *testing.T) {
	m := tagsModel(t)
	cursorTag(t, m, "Client")
	press(m, " ", " ") // Client and client
	press(m, "l")
	require.NotNil(t, m.ov)
	typeText(m, "bob")
	press(m, "enter")
	for _, tag := range []string{"Client", "client"} {
		ids, _ := m.tagr.GetEntities(m.ctx, tag, "person")
		assert.Contains(t, ids, int64(2), tag)
	}
	before := len(m.hist)
	press(m, "ctrl+z")
	assert.Equal(t, before-1, len(m.hist), "both tags in one undo step")
	ids, _ := m.tagr.GetEntities(m.ctx, "client", "person")
	assert.NotContains(t, ids, int64(2))
}

func TestTagsPaletteDeleteUnusedAndMergeDuplicates(t *testing.T) {
	m := tagsModel(t)
	runID(m, "tag-delete-unused")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.input.Prompt, "2 unused tags")
	typeText(m, "delete 3") // wrong count changes nothing
	press(m, "enter")
	m.tagFilter = "unused"
	require.NoError(t, m.rebuild(false))
	assert.Len(t, tagTitles(m), 2)

	runID(m, "tag-delete-unused")
	typeText(m, "delete 2")
	press(m, "enter")
	assert.Empty(t, tagTitles(m))
	press(m, "ctrl+z")
	assert.Len(t, tagTitles(m), 2, "undoable")

	m.tagFilter = "duplicates"
	require.NoError(t, m.rebuild(false))
	runID(m, "tag-merge-duplicates")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.input.Prompt, "merge 1 tags")
	typeText(m, "merge 1")
	press(m, "enter")
	assert.Empty(t, tagTitles(m), "no duplicates are left")
	m.tagFilter = ""
	require.NoError(t, m.rebuild(false))
	assert.Equal(t, []string{"client", "friend"}, tagTitles(m))
	press(m, "ctrl+z")
	assert.Equal(t, []string{"Client", "client", "friend"}, tagTitles(m))
}

func TestTagsModuleIsListedAfterDeals(t *testing.T) {
	m, _ := newTestModel(t)
	assert.NotNil(t, m.actionByID("view-tags"))
	id, ok := moduleOf(viewTags)
	require.True(t, ok)
	assert.Equal(t, "tags", id)
	assert.Equal(t, "labels on anything, rename, merge, clean up", viewBlurb(viewTags))
}

func TestTagNamesStayPlainText(t *testing.T) {
	m := tagsModel(t)
	require.NoError(t, m.tagr.Apply(m.ctx, "person", 2, "projects/website"))
	m.afterWrite()
	assert.Contains(t, tagTitles(m), "projects/website", "a parent/child name is just a name")
}
