package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tagNames(t *testing.T, m *Model) []string {
	t.Helper()
	tags, err := m.tagr.FindAll(m.ctx)
	require.NoError(t, err)
	var names []string
	for _, tg := range tags {
		names = append(names, tg.Name)
	}
	return names
}

func TestTagRenameAndMergeFromTheBarAndPalette(t *testing.T) {
	m, _ := newTestModel(t) // Bob has "friend"
	_, err := m.tagr.FindOrCreate(m.ctx, "pal")
	require.NoError(t, err)
	require.NoError(t, m.tagr.Apply(m.ctx, "person", 3, "pal"))

	// Rename from the tag row: e opens a prompt (rename is just activate), the bar returns.
	startBar(t, m, "pal", "tag:pal")
	press(m, "tab", "e")
	require.Equal(t, modePrompt, m.mode)
	m.input.SetValue("buddy")
	press(m, "enter")
	assert.Equal(t, modeOverlay, m.mode)
	assert.ElementsMatch(t, []string{"friend", "buddy"}, tagNames(t, m))
	press(m, "ctrl+z")
	assert.ElementsMatch(t, []string{"friend", "pal"}, tagNames(t, m), "undo renames it back")

	// A name that exists offers the merge instead.
	press(m, "esc", "esc")
	startBar(t, m, "pal", "tag:pal")
	press(m, "tab", "e")
	m.input.SetValue("FRIEND")
	press(m, "enter")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "merge #pal into #friend")
	press(m, "y")
	assert.Equal(t, modeOverlay, m.mode)
	ids, _ := m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.ElementsMatch(t, []int64{2, 3}, ids)
	press(m, "ctrl+z") // one undo step brings the tag back
	ids, _ = m.tagr.GetEntities(m.ctx, "pal", "person")
	assert.Equal(t, []int64{3}, ids)

	// Merge from the row with the key m: pick the target, see the counts, confirm.
	press(m, "esc", "esc")
	startBar(t, m, "pal", "tag:pal")
	press(m, "tab", "m")
	require.NotNil(t, m.ov, "a picker of the other tags")
	press(m, "enter")
	require.Equal(t, modeConfirm, m.mode)
	assert.Contains(t, m.confirm.prompt, "records move")
	press(m, "n")
	ids, _ = m.tagr.GetEntities(m.ctx, "pal", "person")
	assert.Equal(t, []int64{3}, ids, "n changes nothing")

	// The palette has both actions, with no key of their own outside a tag row.
	press(m, "esc", "esc", "ctrl+k")
	typeText(m, "rename tag")
	found := false
	for _, idx := range m.ov.shown {
		if m.ov.items[idx].id == "tag-rename" {
			found = true
		}
	}
	assert.True(t, found, "Rename tag is in the palette")
	assert.Equal(t, "", m.actionByID("tag-merge").hint)
}

func TestTagRenameAndMergeAreBlockedInReadOnlyMode(t *testing.T) {
	m, _ := newTestModel(t)
	m.readOnly = true
	startBar(t, m, "friend", "tag:friend")
	press(m, "tab")
	assert.NotContains(t, barFooter(m), "rename")
	press(m, "e")
	assert.NotEqual(t, modePrompt, m.mode)
	assert.Contains(t, m.toast.text, "read-only")
}

func TestBarGroupsObjectsThenActionsThenSettings(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "bob")
	var order []string
	seen := map[string]bool{}
	for _, i := range m.ov.shown {
		g := barGroup[m.ov.items[i].kind]
		if !seen[g] {
			seen[g] = true
			order = append(order, g)
		}
	}
	pos := func(g string) int {
		for i, x := range order {
			if x == g {
				return i
			}
		}
		return -1
	}
	assert.Equal(t, 0, pos("People"), "objects lead: %v", order)
	if pos("Actions") >= 0 {
		assert.Less(t, pos("People"), pos("Actions"))
	}
	if pos("Settings") >= 0 && pos("Actions") >= 0 {
		assert.Less(t, pos("Actions"), pos("Settings"))
	}

	// A typed verb keeps its action line first, ahead of the objects.
	press(m, "esc")
	press(m, "ctrl+k")
	typeText(m, "n")
	assert.Equal(t, kindVerb, m.ov.items[m.ov.shown[0]].kind, "typing n shows Add note first: %v", shownKinds(m))
	press(m, "esc")
	press(m, "ctrl+k")
	typeText(m, "snooze")
	assert.Equal(t, kindVerb, m.ov.items[m.ov.shown[0]].kind, "%v", shownKinds(m))

	// An empty bar still starts with Recent.
	press(m, "esc")
	press(m, "ctrl+k")
	typeText(m, "bob")
	press(m, "enter") // picks Bob, which is remembered
	press(m, "ctrl+k")
	assert.Equal(t, kindRecent, m.ov.items[m.ov.shown[0]].kind)
}
