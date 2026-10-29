package tui

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyGroupOfActions(t *testing.T) {
	tests := []struct {
		id, layer, area string
	}{
		{"add-any", keyLayerVerbs, keyAreaApp},
		{"org-delete", keyLayerVerbs, keyAreaOrgs},
		{"deal-delete", keyLayerVerbs, keyAreaDeals},
		{"task-delete", keyLayerVerbs, keyAreaTasks},
		{"favorite", keyLayerVerbs, keyAreaPeople},
		{"tags-delete", keyLayerVerbs, keyAreaTags},
		{"merge", keyLayerVerbs, keyAreaMerge},
		{"cmp-confirm", keyLayerVerbs, keyAreaMerge},
		{"link-person", keyLayerApp, keyAreaPeople},
		{"unlink-note", keyLayerApp, keyAreaNotes},
		{"down", keyLayerNav, keyAreaApp},
		{"focus-detail", keyLayerNav, keyAreaApp},
		{"tag-tree-expand", keyLayerNav, keyAreaTags},
		{"palette", keyLayerView, keyAreaApp},
		{"goto-view", keyLayerView, keyAreaViews},
		{"view-people", keyLayerView, keyAreaViews},
		{"options", keyLayerView, keyAreaViews},
		{"snooze", keyLayerApp, keyAreaPeople},
		{"org-backfill", keyLayerApp, keyAreaOrgs},
		{"deal-stages", keyLayerApp, keyAreaDeals},
		{"task", keyLayerApp, keyAreaTasks},
		{"note-types", keyLayerApp, keyAreaNotes},
		{"tag-rename", keyLayerApp, keyAreaTags},
		{"find-dupes", keyLayerApp, keyAreaMerge},
		{"keys-customize", keyLayerApp, keyAreaViews},
		{"theme-choose", keyLayerApp, keyAreaViews},
		{"doctor", keyLayerApp, keyAreaApp},
		{"sync-now", keyLayerApp, keyAreaApp},
	}
	m := newDefaultModel(t)
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			a := m.actionByID(tt.id)
			require.NotNil(t, a)
			layer, area := keyGroup(a)
			assert.Equal(t, tt.layer, layer)
			assert.Equal(t, tt.area, area)
		})
	}
}

func TestEveryActionHasAKnownGroup(t *testing.T) {
	m := newDefaultModel(t)
	for i := range m.actions {
		layer, area := keyGroup(&m.actions[i])
		assert.Less(t, orderIndex(keyLayerOrder, layer), len(keyLayerOrder), m.actions[i].id)
		assert.Less(t, orderIndex(keyAreaOrder, area), len(keyAreaOrder), m.actions[i].id)
	}
}

// dividerTitles lists the dividers of rows in order, and fails when a divider is not followed by a
// row or a deeper divider.
func dividerTitles(t *testing.T, rows []row) []string {
	t.Helper()
	var out []string
	for i, r := range rows {
		if !r.skip {
			continue
		}
		out = append(out, r.title)
		require.Less(t, i+1, len(rows), "a divider at the end of the list")
	}
	return out
}

func TestSettingsKeysSectionIsGroupedByLayerThenArea(t *testing.T) {
	m := settingsModel(t)
	rows := m.settingRows()
	var keyRows []row
	var heads []string
	inKeys := false
	for _, r := range rows {
		switch {
		case r.skip && r.title == secKeys:
			inKeys = true
			heads = append(heads, r.title)
		case r.skip && r.rule && inKeys && orderIndex(settingSections, r.title) < len(settingSections):
			inKeys = false
		case inKeys && r.skip:
			heads = append(heads, r.title)
		case inKeys && len(r.setting) > 4 && r.setting[:4] == "key:":
			keyRows = append(keyRows, r)
		}
	}
	require.NotEmpty(t, keyRows)
	assert.Equal(t, len(m.actions), len(keyRows), "every action has a row")
	assert.Equal(t, secKeys, heads[0])
	assert.Equal(t, keyLayerVerbs, heads[1], "the first layer is the framework verbs")
	assert.Contains(t, heads, keyLayerNav)
	assert.Contains(t, heads, keyLayerView)
	assert.Contains(t, heads, keyLayerApp)
	for _, area := range keyAreaOrder {
		assert.Contains(t, heads, area)
	}
	// layers appear in order, and the areas inside a layer appear in order
	last, lastArea := -1, -1
	for _, h := range heads[1:] {
		if l := orderIndex(keyLayerOrder, h); l < len(keyLayerOrder) {
			assert.Greater(t, l, last, h)
			last, lastArea = l, -1
			continue
		}
		a := orderIndex(keyAreaOrder, h)
		assert.Greater(t, a, lastArea, h)
		lastArea = a
	}
	_ = dividerTitles(t, rows)

	// a row shows the action, its key at the right and no default while the key is unchanged
	var add row
	for _, r := range keyRows {
		if r.setting == "key:add-any" {
			add = r
		}
	}
	assert.Equal(t, "a", add.meta)
	assert.Empty(t, add.sub)
}

func TestKeyRowShowsTheDefaultKeyWhenChanged(t *testing.T) {
	m := settingsModel(t)
	m.applyKeys(keys.Overrides{"add-any": {"ctrl+n"}})
	for _, r := range m.settingRows() {
		if r.setting == "key:add-any" {
			assert.Equal(t, "ctrl+n", r.meta)
			assert.Equal(t, "default a", r.sub)
			return
		}
	}
	t.Fatal("no row for add-any")
}

func TestEnterOnAKeyRowStartsTheRebind(t *testing.T) {
	m := settingsModel(t)
	require.NoError(t, m.rebuild(false))
	for i, r := range m.rows {
		if r.setting == "key:add-any" {
			m.cursor = i
		}
	}
	m.clamp()
	m.refreshDetail(true)
	assert.Contains(t, m.screen(), "Add")
	pressRun(m, "enter")
	require.Equal(t, modeCapture, m.mode)
	assert.Equal(t, "add-any", m.capture.id)
}

func TestRebindPickerIsGrouped(t *testing.T) {
	m := newDefaultModel(t)
	m.openKeysCustomize()
	require.NotNil(t, m.ov)
	var titles []string
	prevDivider := false
	for _, it := range m.ov.items {
		if it.divider {
			titles = append(titles, it.label)
			prevDivider = true
			continue
		}
		prevDivider = false
		assert.NotEmpty(t, it.id)
	}
	assert.False(t, prevDivider, "the list does not end on a divider")
	assert.Equal(t, keyLayerVerbs, titles[0])
	assert.Contains(t, titles, keyAreaPeople)

	// a custom key puts the default beside the row
	m.closeOverlay()
	m.applyKeys(keys.Overrides{"snooze": {"z"}})
	m.openKeysCustomize()
	for _, it := range m.ov.items {
		if it.id == "snooze" {
			assert.Equal(t, "z", it.hint)
			assert.Contains(t, it.desc, "(default s)")
		}
	}

	// the reset picker is grouped too and only lists the custom keys
	m.closeOverlay()
	m.openKeysReset()
	var ids []string
	for _, it := range m.ov.items {
		if !it.divider {
			ids = append(ids, it.id)
		}
	}
	assert.Equal(t, []string{"*", "snooze"}, ids)
}
