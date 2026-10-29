package tui

import (
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// treeTagsModel is the Tags view in the tree layout. Zed (person 1) has work and work/clients/acme, Bob (2)
// has friend and work/clients/beta, Cara (3) has home. work/clients is a virtual parent.
func treeTagsModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	for _, a := range []struct {
		id  int64
		tag string
	}{{1, "work"}, {1, "work/clients/acme"}, {2, "work/clients/beta"}, {3, "home"}} {
		require.NoError(t, m.tagr.Apply(m.ctx, "person", a.id, a.tag))
	}
	m.tagLayout = layoutTree
	m.afterWrite()
	m.switchView(viewTags)
	return m
}

// treePaths lists the tag path of every visible row.
func treePaths(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		if !r.skip {
			out = append(out, r.tag)
		}
	}
	return out
}

func cursorPath(m *Model) string { return m.cursorTagName() }

func TestTagTreeLayoutRowsDepthMarksAndCounts(t *testing.T) {
	m := treeTagsModel(t)
	assert.Equal(t, []string{"friend", "home", "work", "work/clients", "work/clients/acme", "work/clients/beta"}, treePaths(m))
	type want struct {
		depth   int
		mark    string
		meta    string
		virtual bool
		title   string
	}
	tests := map[string]want{
		"friend":            {1, "·", "1", false, "friend"},
		"work":              {1, "▾", "1 (2)", false, "work"},
		"work/clients":      {2, "▾", "(2)", true, "clients"},
		"work/clients/acme": {3, "·", "1", false, "acme"},
	}
	for _, r := range m.rows {
		w, ok := tests[r.tag]
		if !ok {
			continue
		}
		t.Run(r.tag, func(t *testing.T) {
			assert.Equal(t, w.depth, r.depth)
			assert.Equal(t, w.mark, r.mark)
			assert.Equal(t, w.meta, r.meta)
			assert.Equal(t, w.virtual, r.virtual)
			assert.Equal(t, w.title, r.title)
			assert.True(t, r.node)
		})
	}
	screen := plain(m.screen())
	assert.Contains(t, screen, "▾ work")
	assert.Contains(t, screen, "  ▾ clients", "a child is indented")
	assert.Contains(t, screen, "    · acme")
	assert.Equal(t, "6 tags, in use", m.tagCountText())
}

func TestTagTreeArrowNavigation(t *testing.T) {
	steps := []struct {
		name   string
		keys   []string
		cursor string
		paths  []string // nil: not checked
	}{
		{"start on the first row", nil, "friend", nil},
		{"down", []string{"down", "down"}, "work", nil},
		{"right on an open node goes to its first child", []string{"right"}, "work/clients", nil},
		{"left on a leaf-or-open child with a parent: collapses an open node first", []string{"left"}, "work/clients",
			[]string{"friend", "home", "work", "work/clients"}},
		{"left on a closed node goes to the parent", []string{"left"}, "work", nil},
		{"left collapses an open top node", []string{"left"}, "work", []string{"friend", "home", "work"}},
		{"left on a closed top node does nothing", []string{"left"}, "work", nil},
		{"right on a closed node opens it", []string{"right"}, "work", []string{"friend", "home", "work", "work/clients"}},
		{"right on an open node enters the first child", []string{"right"}, "work/clients", nil},
		{"right on a closed child opens it", []string{"right"}, "work/clients", []string{"friend", "home", "work", "work/clients", "work/clients/acme", "work/clients/beta"}},
		{"right again goes in", []string{"right"}, "work/clients/acme", nil},
		{"right on a leaf moves to the detail pane like a list", []string{"right"}, "work/clients/acme", nil},
	}
	m := treeTagsModel(t)
	for _, st := range steps {
		press(m, st.keys...)
		assert.Equal(t, st.cursor, cursorPath(m), st.name)
		if st.paths != nil {
			assert.Equal(t, st.paths, treePaths(m), st.name)
		}
	}
	assert.Equal(t, focusDetail, m.focus, "right on a leaf is the focus-detail key of every list")
	press(m, "left") // back to the list
	assert.Equal(t, focusList, m.focus)
}

func TestTagTreeJumpUnitIsTheSiblingGroup(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "work")
	press(m, "ctrl+down")
	assert.Equal(t, "work/clients/beta", cursorPath(m), "no next sibling at the top: the last row")
	cursorTag(t, m, "work/clients/acme")
	press(m, "ctrl+down")
	assert.Equal(t, "work/clients/beta", cursorPath(m), "the next sibling")
	press(m, "ctrl+up")
	assert.Equal(t, "work/clients/acme", cursorPath(m), "the previous sibling")
	press(m, "ctrl+up")
	assert.Equal(t, "work/clients", cursorPath(m), "the first sibling goes out one level")
	press(m, "ctrl+up")
	assert.Equal(t, "work", cursorPath(m))
	press(m, "ctrl+up")
	assert.Equal(t, "home", cursorPath(m), "work follows home at the top level")
	press(m, "home")
	assert.Equal(t, "friend", cursorPath(m))
	press(m, "end")
	assert.Equal(t, "work/clients/beta", cursorPath(m))

	// A flat list keeps the page jump.
	m.tagLayout = layoutFlat
	require.NoError(t, m.rebuild(false))
	assert.False(t, whenTagTree(m))
}

func TestTagTreeSearchShowsMatchesInsideCollapsedNodes(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "work")
	press(m, "left")
	require.Equal(t, []string{"friend", "home", "work"}, treePaths(m))
	m.showTagQuery("acme")
	assert.Equal(t, []string{"work", "work/clients", "work/clients/acme"}, treePaths(m), "ancestors show so the match has a place")
	for _, r := range m.rows {
		if r.tag == "work/clients" {
			assert.True(t, r.virtual)
		}
	}
	press(m, "esc")
	assert.Equal(t, []string{"friend", "home", "work"}, treePaths(m), "esc clears the search and the node is closed again")
}

func TestTagTreeRenameParentRenamesChildrenAndUndoes(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "work")
	m.tagRenaming = "work"
	m.runTagRename("job")
	assert.Equal(t, []string{"friend", "home", "job", "job/clients", "job/clients/acme", "job/clients/beta"}, treePaths(m))
	press(m, "ctrl+z")
	assert.Equal(t, []string{"friend", "home", "work", "work/clients", "work/clients/acme", "work/clients/beta"}, treePaths(m), "one undo step")
	press(m, "ctrl+y")
	assert.Contains(t, treePaths(m), "job/clients/beta", "redo")
}

func TestTagTreeRenameCollisionAsksToMerge(t *testing.T) {
	m := treeTagsModel(t)
	require.NoError(t, m.tagr.Apply(m.ctx, "person", 3, "job/clients/acme"))
	m.afterWrite()

	// A child meets a child of the same name: one question, then the merge.
	m.tagRenaming = "work"
	m.runTagRename("job")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "exists")
	assert.Contains(t, m.confirm.prompt, "rename and merge into it")
	press(m, "y")
	assert.Equal(t, []string{"friend", "home", "job", "job/clients", "job/clients/acme", "job/clients/beta"}, treePaths(m))
	press(m, "ctrl+z")
	assert.Contains(t, treePaths(m), "work/clients/acme", "one undo restores the merged tags")

	// The tag itself meets a tag: the existing prompt, and the children follow.
	m.tagRenaming = "work"
	m.runTagRename("home")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "merge #work into #home")
	assert.Contains(t, m.confirm.prompt, "tags below follow")
	press(m, "n")
}

func TestTagTreeDeleteParentDeletesTheSubtreeWithTypedCount(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "work")
	press(m, "d")
	require.Equal(t, modePrompt, m.mode, "a parent asks for the count typed back")
	assert.Contains(t, m.input.Prompt, "delete #work and 2 tags below it (3 taggings)")
	assert.Contains(t, m.input.Prompt, `"delete 3"`)
	typeText(m, "delete 4")
	press(m, "enter")
	assert.Contains(t, treePaths(m), "work", "a wrong count changes nothing")

	press(m, "d")
	typeText(m, "delete 3")
	press(m, "enter")
	assert.Equal(t, []string{"friend", "home"}, treePaths(m))
	press(m, "ctrl+z")
	assert.Equal(t, []string{"friend", "home", "work", "work/clients", "work/clients/acme", "work/clients/beta"}, treePaths(m), "one undo restores the whole subtree")

	// A virtual parent deletes the tags below it.
	cursorTag(t, m, "work/clients")
	press(m, "d")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.input.Prompt, "delete the 2 tags below #work/clients")
	press(m, "esc")

	// A tag with no children still asks y/n.
	cursorTag(t, m, "home")
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	press(m, "n")
}

func TestTagTreeMergeSelectedTagsChildrenFollow(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "work")
	press(m, " ") // select work
	cursorTag(t, m, "home")
	press(m, "m")
	require.Equal(t, modeConfirm, m.mode, "one selected tag asks y/n")
	assert.Contains(t, m.confirm.prompt, "merge #work into #home")
	assert.Contains(t, m.confirm.prompt, "tags below follow")
	press(m, "y")
	assert.Equal(t, []string{"friend", "home", "home/clients", "home/clients/acme", "home/clients/beta"}, treePaths(m))
	press(m, "ctrl+z")
	assert.Contains(t, treePaths(m), "work/clients/beta", "one undo step")
}

func TestTagTreeNestAndUnnestSelectedTags(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "home")
	press(m, " ")
	cursorTag(t, m, "friend")
	press(m, " ")
	require.NotNil(t, m.nestVerb)
	m.nestVerb()
	require.NotNil(t, m.ov, "a picker for the parent")
	require.NotNil(t, m.ov.create, "type a new path to create the parent")
	var labels []string
	for _, i := range m.ov.shown {
		labels = append(labels, m.ov.items[i].label)
	}
	assert.Contains(t, labels, "(top level)")
	assert.Contains(t, labels, "#work/clients", "a virtual parent is a place to nest under")
	typeText(m, "people/close")
	press(m, "enter")
	assert.Equal(t, []string{"people", "people/close", "people/close/friend", "people/close/home", "work", "work/clients", "work/clients/acme", "work/clients/beta"}, treePaths(m))
	assert.Equal(t, "people/close/friend", treePaths(m)[2])
	press(m, "ctrl+z")
	assert.Equal(t, []string{"friend", "home", "work", "work/clients", "work/clients/acme", "work/clients/beta"}, treePaths(m), "bulk nest is one undo step")

	// Unnest moves a tag up one level, the children go along.
	cursorTag(t, m, "work/clients")
	m.unnestVerb()
	assert.Equal(t, []string{"clients", "clients/acme", "clients/beta", "friend", "home", "work"}, treePaths(m))
	press(m, "ctrl+z")
	assert.Contains(t, treePaths(m), "work/clients/acme")

	// Nesting under an own child is not offered.
	cursorTag(t, m, "work")
	m.nestVerb()
	for _, i := range m.ov.shown {
		assert.False(t, strings.HasPrefix(m.ov.items[i].label, "#work"), m.ov.items[i].label)
	}
	press(m, "esc")
}

func TestTagViewOptionsToggleLayoutAndKeepItPerDevice(t *testing.T) {
	m, _ := newTestModel(t)
	m.env.DataDir = t.TempDir()
	m.switchView(viewTags)
	assert.False(t, m.tagTreeOn(), "flat is the default")
	press(m, "ctrl+o")
	require.NotNil(t, m.ov)
	require.Len(t, m.ov.items, 4)
	assert.Contains(t, m.ov.items[2].label, "Layout")
	assert.Contains(t, m.ov.items[3].label, "Tag page")
	press(m, "down", "down", "right")
	assert.True(t, m.tagTreeOn())
	press(m, "down", "right")
	assert.True(t, m.tagChildren)
	press(m, "esc")

	again, _ := newTestModel(t)
	again.env.DataDir = m.env.DataDir
	again.loadTagPrefs()
	assert.Equal(t, layoutTree, again.tagLayout, "remembered on this device")
	assert.True(t, again.tagChildren)

	// The palette action toggles it back.
	m.actionByID("tag-layout").run(m)
	assert.Equal(t, layoutFlat, m.tagLayout)
	again.loadTagPrefs()
	assert.Equal(t, layoutFlat, again.tagLayout)
}

func TestTagPageOfAParentIncludesChildrenOnRequest(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "work")
	press(m, "enter")
	require.True(t, m.onTagPage())
	count := func() (n int) {
		for _, r := range m.rows {
			if !r.skip {
				n++
			}
		}
		return n
	}
	assert.Equal(t, 1, count(), "own objects only: Zed")
	m.setTagChildren(true)
	require.NoError(t, m.rebuild(false))
	assert.Equal(t, 2, count(), "Zed once and Bob through work/clients/beta")
	assert.Contains(t, m.tagCountText(), "with children")
	found := false
	for _, r := range m.rows {
		found = found || strings.Contains(r.sub, "#work/clients/beta")
	}
	assert.True(t, found, "an object that came in through a child names that tag")
	press(m, "esc")

	// A virtual parent has no objects of its own, so its page is empty until children are included.
	m.setTagChildren(false)
	cursorTag(t, m, "work/clients")
	press(m, "enter")
	require.True(t, m.onTagPage())
	assert.Equal(t, 0, count())
	m.setTagChildren(true)
	require.NoError(t, m.rebuild(false))
	assert.Equal(t, 2, count())
	press(m, "esc")
}

func TestTagTreeDetailPaneOfAVirtualParent(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "work/clients")
	m.refreshDetail(false)
	var labels []string
	for _, r := range m.form {
		labels = append(labels, r.label)
	}
	assert.Contains(t, labels, "children")
	assert.Equal(t, "#work/clients", m.formTitle)
	assert.Equal(t, "work/clients", m.form[0].value, "the name row holds the full path, editing it renames the subtree")
}

func TestTagTreeBulkSelectionOnVirtualParents(t *testing.T) {
	m := treeTagsModel(t)
	cursorTag(t, m, "work/clients")
	press(m, " ")
	cursorTag(t, m, "home")
	press(m, " ")
	assert.ElementsMatch(t, []string{"work/clients", "home"}, m.bulkTagNames(), "a virtual parent can be selected like any tag")
	assert.Len(t, m.tagRoots(), 2)
	press(m, "esc")
}

func TestTagTreeKeysStayOffFlatListsAndOtherViews(t *testing.T) {
	m, _ := newTestModel(t)
	// In the People list right still moves into the detail pane.
	press(m, "right")
	assert.Equal(t, focusDetail, m.focus)
	press(m, "esc")

	m = treeTagsModel(t)
	m.tagLayout = layoutFlat
	require.NoError(t, m.rebuild(false))
	press(m, "right")
	assert.Equal(t, focusDetail, m.focus, "a flat Tags list keeps right as the focus key")
}

// A note line reads #parent/child as one tag, and the related segment of the pipe line takes a path too.
func TestPipeLineAndTokensCarryTagPaths(t *testing.T) {
	m, _ := newTestModel(t)
	m.startInput("note-free", "note: ", "")
	_ = m.input.Focus()
	typeText(m, "met them #work/clients/acme|Bob Baker, #area/life")
	press(m, "enter")
	bob := int64(2)
	notes, err := m.ir.FindAll(m.ctx, model.InteractionFilters{PersonID: &bob, Limit: 10})
	require.NoError(t, err)
	require.Len(t, notes, 1)
	tags, err := m.tagr.GetForEntity(m.ctx, "interaction", notes[0].ID)
	require.NoError(t, err)
	var got []string
	for _, tg := range tags {
		got = append(got, tg.Name)
	}
	assert.ElementsMatch(t, []string{"work/clients/acme", "area/life"}, got)

	// A path the validator refuses is not applied and the error names the code.
	err = m.tagr.Apply(m.ctx, "person", 1, "work//x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "E_PATH_EMPTY_SEGMENT")
}
