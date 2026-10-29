package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// svModel is a test model with a few tabs, so the saved views get a number key.
func svModel(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	arr := modules.Default()
	var err error
	for _, id := range []string{"birthdays", "merge", "tags", "archived", "sync"} {
		arr, err = arr.With(id, false)
		require.NoError(t, err)
	}
	m.mods = arr.Normalize()
	m.patchModuleActions()
	m.loadSaved()
	return m
}

func svCreate(t *testing.T, m *Model, name, kind string, q query.Query, tabs bool) *repo.SavedView {
	t.Helper()
	v, err := m.svr.Create(context.Background(), repo.SavedViewInput{Name: name, Kind: kind, Query: q, ShowInTabs: tabs})
	require.NoError(t, err)
	m.loadSaved()
	return v
}

func cond(field, op string, vals ...string) query.Condition {
	return query.Condition{Field: field, Op: op, Values: vals}
}

func svTitles(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		out = append(out, strings.TrimPrefix(r.title, "★ "))
	}
	return out
}

func selectable(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		if !r.skip {
			out = append(out, strings.TrimPrefix(r.title, "★ "))
		}
	}
	return out
}

func TestSavedViewShowsInTheViewsScreenHiddenAndOpens(t *testing.T) {
	m := svModel(t)
	press(m, "v")
	var labels []string
	hiddenAt := -1
	for i, idx := range m.ov.shown {
		it := m.ov.items[idx]
		if it.divider && it.label == "Hidden" {
			hiddenAt = i
		}
		if strings.HasPrefix(it.value, savedPrefix) {
			assert.Greater(t, hiddenAt, -1, "examples are off, so they sit under Hidden")
			labels = append(labels, it.label)
		}
	}
	assert.Equal(t, []string{"Overdue follow-up (example)", "Open deals by stage (example)", "Tasks due this week (example)"}, labels)

	// enter opens a hidden view, it renders with the People list and the filter applied.
	for m.ov.items[m.ov.shown[m.ov.cursor]].label != "Overdue follow-up (example)" {
		press(m, "down")
	}
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	require.NotNil(t, m.sv)
	assert.Equal(t, viewPeople, m.view)
	assert.Equal(t, []string{"Bob Baker"}, svTitles(m), "Bob was last contacted in 2020, the other two never")
	assert.Equal(t, "1 persons", m.countText())
	assert.Contains(t, m.screen(), "Overdue follow-up", "the More slot names the hidden view")

	// leaving to the plain view clears it
	m.switchView(viewPeople)
	assert.Nil(t, m.sv)
	assert.Len(t, m.rows, 3)
}

func TestSavedViewXShowsItInTheTabsAndTakesTheNextNumberKey(t *testing.T) {
	m := svModel(t)
	v := svCreate(t, m, "Globex people", "person", query.Query{Filter: []query.Condition{cond("company", query.OpIs, "Globex")}}, false)
	press(m, "v")
	for m.ov.items[m.ov.shown[m.ov.cursor]].value != savedPrefix+v.UUID {
		press(m, "down")
	}
	press(m, "x")
	got, err := m.svr.FindByID(context.Background(), v.ID)
	require.NoError(t, err)
	assert.True(t, got.ShowInTabs)
	press(m, "esc")
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})

	// built-in tabs: home people follow-up tasks organizations deals settings = 7, so the view is 8
	assert.Equal(t, "8", m.svKeyOf(v.UUID))
	var tab *tabSeg
	for _, seg := range m.tabLayout() {
		seg := seg
		if seg.sv == v.UUID {
			tab = &seg
		}
	}
	require.NotNil(t, tab, "the view has a tab")
	assert.Contains(t, tab.label, "8 Globex people")

	press(m, "8")
	require.NotNil(t, m.sv)
	assert.Equal(t, v.UUID, m.sv.v.UUID)
	assert.Equal(t, []string{"Bob Baker"}, svTitles(m))
	assert.True(t, strings.Contains(m.screen(), "8 Globex people"))

	// the tab is on, the People tab is not
	for _, seg := range m.tabLayout() {
		assert.Equal(t, seg.sv == v.UUID, seg.on, seg.label)
	}

	// ] and [ walk through the saved tab, a click on its tab opens it
	m.switchView(viewDeals)
	press(m, "]", "]")
	assert.Equal(t, v.UUID, m.sv.v.UUID, "] from Deals goes to Settings, then to the saved view")
	press(m, "[")
	assert.Nil(t, m.sv)
	press(m, "8")
	m.switchView(viewTasks)
	click(m, tab.x0+1, 0)
	require.NotNil(t, m.sv)

	// hiding it again drops the tab and the key
	press(m, "v")
	for m.ov.items[m.ov.shown[m.ov.cursor]].value != savedPrefix+v.UUID {
		press(m, "down")
	}
	press(m, "x")
	press(m, "esc")
	assert.Equal(t, "", m.svKeyOf(v.UUID))
	for _, seg := range m.tabLayout() {
		assert.NotEqual(t, v.UUID, seg.sv)
	}
}

func TestSavedViewGroupsDealsByStageWithDividers(t *testing.T) {
	m := svModel(t)
	ctx := context.Background()
	dr := repo.NewDealRepo(m.db)
	for _, d := range []struct {
		title, stage string
		value        float64
	}{{"Site", "lead", 100}, {"Big", "negotiation", 900}, {"Done", "won", 50}, {"Small", "lead", 10}} {
		v := d.value
		_, err := dr.Create(ctx, model.CreateDealInput{Title: d.title, Stage: d.stage, Value: &v})
		require.NoError(t, err)
	}
	require.NoError(t, m.reload())
	v, err := m.svr.Find(ctx, "Open deals by stage")
	require.NoError(t, err)
	m.openSaved(v.UUID)
	require.Equal(t, viewDeals, m.view)

	var heads []string
	var deals []string
	for _, r := range m.rows {
		if r.skip {
			assert.True(t, r.rule, "a group heading is drawn as a rule")
			heads = append(heads, r.title)
		} else {
			deals = append(deals, r.title)
		}
	}
	assert.Equal(t, []string{"lead", "negotiation"}, heads, "stages in the order of the stage list, the won deal is not open")
	assert.Equal(t, []string{"Site", "Small", "Big"}, deals, "sorted by value inside the group... lead holds Site 100 then Small 10")
	assert.False(t, m.rows[m.cursor].skip, "the cursor never rests on a heading")

	// bulk verbs and marks work across groups
	press(m, "space")
	press(m, "j", "j")
	if m.rows[m.cursor].skip {
		press(m, "j")
	}
	press(m, "space")
	assert.Len(t, m.markedRefs(), 2)
	for _, ref := range m.markedRefs() {
		assert.Equal(t, "deal", ref.Kind)
	}
	assert.True(t, strings.Contains(m.countText(), "deals"))
}

func TestSavedViewOptionsTweakTheSessionNotTheStoredView(t *testing.T) {
	m := svModel(t)
	ctx := context.Background()
	v := svCreate(t, m, "Sorted people", "person", query.Query{
		Filter: []query.Condition{cond("company", query.OpNotEmpty)}, Sort: []query.Sort{{Field: "name", Dir: "desc"}}}, true)
	m.openSaved(v.UUID)
	assert.Equal(t, []string{"Zed Adams", "Cara Cole", "Bob Baker"}, svTitles(m), "the stored sort: name descending")

	press(m, "ctrl+o")
	require.Equal(t, modeOverlay, m.mode)
	var labels []string
	for _, idx := range m.ov.shown {
		labels = append(labels, m.ov.items[idx].label)
	}
	require.Len(t, labels, 3)
	assert.Contains(t, labels[0], "Filter")
	assert.Contains(t, m.ov.items[m.ov.shown[0]].desc, "company is not empty", "the overlay shows the saved filter")
	assert.Contains(t, m.ov.items[m.ov.shown[1]].desc, "name desc", "and the saved sort")

	// Sort: next choice is "default order", then the fields.
	press(m, "down", "right")
	assert.Equal(t, "none", m.sv.sortKey)
	assert.Equal(t, []string{"Bob Baker", "Cara Cole", "Zed Adams"}, svTitles(m), "default order of people is by name")
	// Filter off shows everyone
	press(m, "up", "right")
	assert.True(t, m.sv.filterOff)
	assert.Len(t, m.rows, 3)
	// Group by company
	press(m, "down", "down")
	for i := 0; i < 40 && m.sv.groupKey != "company"; i++ {
		press(m, "right")
	}
	require.Equal(t, "company", m.sv.groupKey)
	assert.Equal(t, []string{"Acme", "Globex", "Initech"}, groupHeads(m))
	press(m, "esc")

	stored, err := m.svr.FindByID(ctx, v.ID)
	require.NoError(t, err)
	assert.Equal(t, v.Query, stored.Query, "nothing was saved")
	// Reopening starts from the saved view again
	m.openSaved(v.UUID)
	assert.False(t, m.sv.filterOff)
	assert.Equal(t, "", m.sv.groupKey)
}

func groupHeads(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		if r.skip {
			out = append(out, r.title)
		}
	}
	return out
}

func TestSaveCurrentViewAsCapturesKindSearchAndSort(t *testing.T) {
	m := svModel(t)
	ctx := context.Background()
	m.query = "#friend bak"
	m.sortChoice = map[viewKind]string{viewPeople: "recent"}
	require.NoError(t, m.rebuild(false))
	runPalette(m, "save current view")
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "Friends named Baker")
	press(m, "enter")
	v, err := m.svr.Find(ctx, "Friends named Baker")
	require.NoError(t, err)
	assert.Equal(t, "person", v.Kind)
	assert.True(t, v.ShowInTabs)
	assert.Equal(t, query.Query{
		Filter: []query.Condition{cond("tag", query.OpHas, "friend")},
		Search: "bak",
		Sort:   []query.Sort{{Field: "updated", Dir: "desc"}},
	}, v.Query)
	m.openSaved(v.UUID)
	assert.Equal(t, []string{"Bob Baker"}, svTitles(m))

	// ctrl+z takes it back
	press(m, "ctrl+z")
	_, err = m.svr.Find(ctx, "Friends named Baker")
	require.NoError(t, err)
	got, _ := m.svr.Find(ctx, "Friends named Baker")
	assert.True(t, got.Deleted)
	assert.Nil(t, m.sv, "the open view went away with it")
	press(m, "ctrl+y")
	got, _ = m.svr.Find(ctx, "Friends named Baker")
	assert.False(t, got.Deleted)

	t.Run("a duplicate name is refused", func(t *testing.T) {
		m.switchView(viewPeople)
		runPalette(m, "save current view")
		typeText(m, "friends named baker")
		press(m, "enter")
		assert.True(t, m.statusIsErr)
		assert.Contains(t, m.status, "already exists")
	})
	t.Run("from an organizations list, a deals list and a saved view", func(t *testing.T) {
		m.switchView(viewOrgs)
		kind, q, ok := m.captureView()
		require.True(t, ok)
		assert.Equal(t, "organization", kind)
		assert.Equal(t, []query.Sort{{Field: "updated", Dir: "desc"}}, q.Sort, "organizations default to recently updated")
		m.switchView(viewDeals)
		kind, _, ok = m.captureView()
		assert.True(t, ok)
		assert.Equal(t, "deal", kind)
		m.switchView(viewHome)
		_, _, ok = m.captureView()
		assert.False(t, ok, "Home is not a list of one kind")
		runPalette(m, "save current view")
		assert.Equal(t, modeNormal, m.mode)
		assert.True(t, m.statusIsErr)
	})
}

func TestEditViewEditorEditsConditionsInPlace(t *testing.T) {
	m := svModel(t)
	ctx := context.Background()
	orgs := repo.NewOrgRepo(m.db)
	_, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	v := svCreate(t, m, "People view", "person", query.Query{Filter: []query.Condition{cond("company", query.OpContains, "o")}}, true)
	m.openSaved(v.UUID)
	runPalette(m, "edit view")
	require.NotNil(t, m.sve, "Edit view opens the editor")
	assert.Contains(t, m.screen(), "Edit view: People view")
	assert.Contains(t, m.screen(), "company")

	cur := func() svRow { return m.sve.rows()[m.sve.cur] }
	goTo := func(kind svRowKind, idx int) {
		for i, r := range m.sve.rows() {
			if r.kind == kind && r.idx == idx {
				m.sve.cur, m.sve.col = i, 0
				return
			}
		}
		t.Fatalf("no row %v %d", kind, idx)
	}

	t.Run("rename the view", func(t *testing.T) {
		goTo(svrName, 0)
		press(m, "enter")
		require.NotNil(t, m.sve.ed)
		m.rowInput.SetValue("People who matter")
		press(m, "enter")
		assert.Equal(t, "People who matter", m.sve.name)
		assert.True(t, m.sve.dirty)
	})
	t.Run("a adds a condition, field and operator are choices", func(t *testing.T) {
		goTo(svrCond, 0)
		press(m, "a")
		require.NotNil(t, m.sve.ed, "a opens the field cell of the new row")
		assert.Len(t, m.sve.q.Filter, 2)
		assert.Equal(t, svrCond, cur().kind)
		assert.Equal(t, 1, cur().idx)
		// pick the field "tier" by typing its first letters, enter
		for m.sve.ed.choices[m.sve.ed.choice].Value != "tier" {
			press(m, "down")
		}
		press(m, "enter")
		assert.Equal(t, "tier", m.sve.q.Filter[1].Field)
		assert.Equal(t, query.OpIs, m.sve.q.Filter[1].Op, "a new field of another type starts with its first operator")
		// operator cell
		press(m, "right", "enter")
		for m.sve.ed.choices[m.sve.ed.choice].Value != query.OpIn {
			press(m, "down")
		}
		press(m, "enter")
		assert.Equal(t, query.OpIn, m.sve.q.Filter[1].Op)
		// value cell: a choice, space adds a second option (any of)
		press(m, "right", "enter")
		require.Equal(t, "multi", m.sve.ed.mode)
		for m.sve.ed.choices[m.sve.ed.choice].Value != "1" {
			press(m, "down")
		}
		press(m, "space", "down", "space", "enter")
		assert.Equal(t, []string{"1", "2"}, m.sve.q.Filter[1].Values)
		assert.Contains(t, m.screen(), "Top or High")
	})
	t.Run("a typed value is checked by its type", func(t *testing.T) {
		press(m, "a")
		for m.sve.ed.choices[m.sve.ed.choice].Value != "last_contacted" {
			press(m, "down")
		}
		press(m, "enter")
		assert.Equal(t, query.OpBefore, m.sve.q.Filter[2].Op)
		press(m, "right", "right", "enter")
		require.Equal(t, "text", m.sve.ed.mode)
		m.rowInput.SetValue("soon-ish")
		press(m, "enter")
		require.NotNil(t, m.sve.ed, "a failed check keeps the editor open")
		assert.True(t, m.sve.isErr)
		assert.Contains(t, m.sve.msg, "E_QUERY_VALUE_INVALID")
		m.rowInput.SetValue("90 days ago")
		press(m, "enter")
		assert.Nil(t, m.sve.ed)
		assert.Equal(t, []string{"90 days ago"}, m.sve.q.Filter[2].Values)
	})
	t.Run("a relation value uses the typeahead", func(t *testing.T) {
		press(m, "a")
		for m.sve.ed.choices[m.sve.ed.choice].Value != "organization" {
			press(m, "down")
		}
		press(m, "enter")
		assert.Equal(t, query.OpLinkedTo, m.sve.q.Filter[3].Op)
		press(m, "right", "right", "enter")
		require.Equal(t, "rel", m.sve.ed.mode)
		typeText(m, "glo")
		require.NotEmpty(t, m.sve.ed.shown)
		assert.Equal(t, "Globex", m.sve.ed.shown[0].label)
		press(m, "enter") // picks Globex
		press(m, "enter") // empty input: done
		require.Nil(t, m.sve.ed)
		require.Len(t, m.sve.q.Filter[3].Values, 1)
		assert.Equal(t, "Globex", m.sve.labels[m.sve.q.Filter[3].Values[0]])
		assert.Contains(t, m.screen(), "Globex")
	})
	t.Run("d removes a condition", func(t *testing.T) {
		goTo(svrCond, 2)
		press(m, "d")
		require.Len(t, m.sve.q.Filter, 3)
		assert.Equal(t, "organization", m.sve.q.Filter[2].Field)
	})
	t.Run("sort and group cells", func(t *testing.T) {
		goTo(svrSort, 0)
		press(m, "enter")
		for m.sve.ed.choices[m.sve.ed.choice].Value != "last_contacted" {
			press(m, "down")
		}
		press(m, "enter", "right", "enter", "down", "enter")
		assert.Equal(t, []query.Sort{{Field: "last_contacted", Dir: "desc"}}, m.sve.q.Sort)
		goTo(svrGroup, 0)
		press(m, "enter")
		for m.sve.ed.choices[m.sve.ed.choice].Value != "last_contacted" {
			press(m, "down")
		}
		press(m, "enter", "right", "enter")
		for m.sve.ed.choices[m.sve.ed.choice].Value != query.BucketMonth {
			press(m, "down")
		}
		press(m, "enter")
		assert.Equal(t, "last_contacted", m.sve.q.GroupBy)
		assert.Equal(t, query.BucketMonth, m.sve.q.Bucket)
	})
	t.Run("ctrl+s stores it, esc closes, ctrl+z undoes", func(t *testing.T) {
		press(m, "ctrl+s")
		assert.False(t, m.sve.dirty, m.sve.msg)
		got, err := m.svr.FindByID(ctx, v.ID)
		require.NoError(t, err)
		assert.Equal(t, "People who matter", got.Name)
		require.Len(t, got.Query.Filter, 3)
		assert.Equal(t, "last_contacted", got.Query.GroupBy)
		press(m, "esc")
		assert.Nil(t, m.sve)
		assert.Equal(t, "People who matter", m.sv.v.Name, "the open view shows the stored one")
		press(m, "ctrl+z")
		got, _ = m.svr.FindByID(ctx, v.ID)
		assert.Equal(t, "People view", got.Name)
		assert.Len(t, got.Query.Filter, 1)
	})
}

func TestEditViewEditorRefusesWhatTheValidationRefusesAndKeepsChangesUntilSaved(t *testing.T) {
	m := svModel(t)
	v := svCreate(t, m, "Plain", "person", query.Query{}, false)
	m.openSaved(v.UUID)
	runPalette(m, "edit view")
	require.NotNil(t, m.sve)

	// a condition with no value cannot be saved: the code shows in the editor, nothing is stored
	press(m, "down", "down", "down", "down", "down", "down")
	press(m, "a")
	press(m, "enter") // keeps the first field
	press(m, "ctrl+s")
	assert.True(t, m.sve.isErr)
	assert.Contains(t, m.sve.msg, "E_QUERY_VALUE_INVALID")
	got, _ := m.svr.FindByID(context.Background(), v.ID)
	assert.Empty(t, got.Query.Filter)

	// esc with unsaved changes asks for a second esc
	press(m, "esc")
	require.NotNil(t, m.sve)
	assert.Contains(t, m.sve.msg, "unsaved")
	press(m, "esc")
	assert.Nil(t, m.sve)

	// changing the kind drops the conditions the new kind cannot have
	runPalette(m, "edit view")
	m.sve.q.Filter = []query.Condition{cond("company", query.OpEmpty), cond("name", query.OpContains, "x")}
	m.sve.cur, m.sve.col = 1, 0
	press(m, "enter")
	for m.sve.ed.choices[m.sve.ed.choice].Value != "organization" {
		press(m, "down")
	}
	press(m, "enter")
	assert.Equal(t, "organization", m.sve.kind)
	require.Len(t, m.sve.q.Filter, 1)
	assert.Equal(t, "name", m.sve.q.Filter[0].Field, "company is a person field, name is shared")
}

func TestViewsScreenEditDeleteRestoreKeys(t *testing.T) {
	m := svModel(t)
	ctx := context.Background()
	v := svCreate(t, m, "Mine", "task", query.Query{}, true)
	focus := func(value string) {
		for m.ov.items[m.ov.shown[m.ov.cursor]].value != value {
			press(m, "down")
		}
	}

	press(m, "v")
	focus(savedPrefix + v.UUID)
	press(m, "e")
	require.NotNil(t, m.sve, "e edits the saved view")
	assert.Equal(t, v.ID, m.sve.id)
	press(m, "esc")

	press(m, "v")
	focus(savedPrefix + v.UUID)
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode, "d asks y or n")
	assert.Contains(t, m.confirm.prompt, "Mine")
	press(m, "n")
	got, _ := m.svr.FindByID(ctx, v.ID)
	assert.False(t, got.Deleted)

	press(m, "v")
	focus(savedPrefix + v.UUID)
	press(m, "d", "y")
	got, _ = m.svr.FindByID(ctx, v.ID)
	assert.True(t, got.Deleted)
	require.Equal(t, modeOverlay, m.mode, "the Views screen is back")
	var hasDeleted bool
	for _, idx := range m.ov.shown {
		if m.ov.items[idx].label == "Deleted views" {
			hasDeleted = true
		}
	}
	assert.True(t, hasDeleted)
	focus(savedPrefix + v.UUID)
	press(m, "enter")
	assert.True(t, m.statusIsErr, "a deleted view does not open")
	assert.Nil(t, m.sv)

	press(m, "v")
	focus(savedPrefix + v.UUID)
	press(m, "r")
	got, _ = m.svr.FindByID(ctx, v.ID)
	assert.False(t, got.Deleted, "r restores")
	press(m, "esc")

	// built-in views are not edited or deleted from here
	press(m, "v")
	focus("people")
	press(m, "d")
	assert.True(t, m.statusIsErr)
	assert.Equal(t, modeOverlay, m.mode)
}

func TestObjectRegistryGivesTheSavedViewItsVerbs(t *testing.T) {
	m := svModel(t)
	o := objectByKind(model.SavedViewKind)
	require.NotNil(t, o)
	for _, id := range []string{"sview-edit", "sview-delete", "sview-restore", "sview-add"} {
		assert.NotNil(t, m.actionByID(id), id)
	}
	assert.Equal(t, "Edit view", m.actionByID("sview-edit").title)
	assert.Equal(t, "Delete view", m.actionByID("sview-delete").title)
	assert.Equal(t, "Restore view", m.actionByID("sview-restore").title)

	v := svCreate(t, m, "Tasks mine", "task", query.Query{}, true)
	m.openSaved(v.UUID)
	require.Equal(t, viewTasks, m.view)
	assert.Equal(t, v.ID, o.selected(m))

	// Delete view from the palette: y/n, soft delete, back to the plain list, ctrl+z restores
	runPalette(m, "delete view")
	require.Equal(t, modeConfirm, m.mode)
	press(m, "y")
	got, _ := m.svr.FindByID(context.Background(), v.ID)
	assert.True(t, got.Deleted)
	assert.Nil(t, m.sv)
	runPalette(m, "restore view")
	got, _ = m.svr.FindByID(context.Background(), v.ID)
	assert.False(t, got.Deleted, "Restore view brings back the view deleted last")

	// d in a saved list still deletes the object under the cursor, never the view
	m.openSaved(v.UUID)
	assert.Equal(t, "task", o.kindOfRowKey(m))
}

func (o *objDef) kindOfRowKey(m *Model) string { return m.primaryKind() }

func TestSavedNotesAndTagsRender(t *testing.T) {
	m := svModel(t)
	ctx := context.Background()
	ir := repo.NewInteractionRepo(m.db)
	text := "Talked about pricing"
	_, err := ir.Create(ctx, model.CreateInteractionInput{Type: "note", Content: &text})
	require.NoError(t, err)
	require.NoError(t, m.reload())

	nv := svCreate(t, m, "Pricing notes", "note", query.Query{Filter: []query.Condition{cond("text", query.OpContains, "pricing")}}, true)
	m.openSaved(nv.UUID)
	require.Equal(t, viewNotes, m.view)
	require.Len(t, m.rows, 1)
	assert.Equal(t, "Talked about pricing", m.rows[0].title)
	ref, ok := m.rowRef(m.rows[0])
	require.True(t, ok)
	assert.Equal(t, model.NoteKind, ref.Kind, "bulk verbs act on the note")
	assert.Contains(t, m.screen(), "Talked about pricing")

	tv := svCreate(t, m, "Friend tags", "tag", query.Query{Filter: []query.Condition{cond("name", query.OpContains, "friend")}}, true)
	m.openSaved(tv.UUID)
	require.Equal(t, viewTags, m.view)
	assert.Equal(t, []string{"friend"}, svTitles(m))
	ref, ok = m.rowRef(m.rows[0])
	require.True(t, ok)
	assert.Equal(t, tagKind, ref.Kind)
}

func TestSavedViewDeletedElsewhereLeavesTheScreen(t *testing.T) {
	m := svModel(t)
	v := svCreate(t, m, "Gone soon", "person", query.Query{}, true)
	m.openSaved(v.UUID)
	require.NotNil(t, m.sv)
	require.NoError(t, m.svr.Delete(context.Background(), v.ID))
	require.NoError(t, m.reload())
	assert.Nil(t, m.sv)
	assert.Len(t, m.rows, 3)
}
