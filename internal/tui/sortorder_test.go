package tui

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reversed(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}

func orgTitles(m *Model) []string {
	var out []string
	for _, r := range m.rows {
		out = append(out, r.title)
	}
	return out
}

// twoOrgs makes Acme (older) and Bolt (newer) and shows the Organizations view.
func twoOrgs(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	_, err := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	b, err := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Bolt"})
	require.NoError(t, err)
	_, err = m.db.Exec(`UPDATE organizations SET updated_at = '2099-01-01 00:00:00' WHERE id = ?`, b.ID)
	require.NoError(t, err)
	m.switchView(viewOrgs)
	return m
}

func TestOrderRowSitsNextToSortInPeopleOrgsAndTags(t *testing.T) {
	ids := func(v viewKind) []string {
		var out []string
		for _, o := range viewOptionsFor(v) {
			out = append(out, o.id)
		}
		return out
	}
	assert.Equal(t, []string{"sort", "order"}, ids(viewPeople))
	assert.Equal(t, []string{"sort", "order"}, ids(viewOrgs))
	assert.Equal(t, []string{"sort", "order", "filter", "layout", "page"}, ids(viewTags))
	assert.Equal(t, []string{"level"}, ids(viewMerge), "no Sort row, no Order row")
	assert.Equal(t, []string{"type"}, ids(viewArchived), "no Sort row, no Order row")

	m, _ := newTestModel(t)
	press(m, "ctrl+o")
	require.NotNil(t, m.ov)
	require.Len(t, m.ov.items, 2)
	assert.Equal(t, "Order", m.ov.items[1].label)
	assert.Equal(t, "ascending", m.ov.items[1].current, "People by name start ascending")
	press(m, "esc")
}

func TestPeopleByNameOrderDescendingListsZToA(t *testing.T) {
	m, _ := newTestModel(t)
	asc := names(m)
	require.Len(t, asc, 3)
	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, reversed(asc), names(m))
	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, asc, names(m), "the toggle goes back to ascending")
}

func TestOrgsRecentDefaultIsNewestFirstAndAscendingIsOldestFirst(t *testing.T) {
	m := twoOrgs(t)
	assert.Equal(t, []string{"Bolt", "Acme"}, orgTitles(m), "default order of recently updated is newest first, as before")
	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, []string{"Acme", "Bolt"}, orgTitles(m), "ascending recent is oldest first")
	// Sort by name keeps the chosen order: ascending is A to Z.
	press(m, "ctrl+o", "right", "esc")
	assert.Equal(t, []string{"Acme", "Bolt"}, orgTitles(m))
	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, []string{"Bolt", "Acme"}, orgTitles(m), "descending name is Z to A")
}

func TestOrderDefaultFollowsTheSortUntilTheUserSetsIt(t *testing.T) {
	m := twoOrgs(t)
	press(m, "ctrl+o", "right", "esc") // sort by name, order untouched
	assert.Equal(t, "asc", m.orderOf(viewOrgs))
	assert.Equal(t, []string{"Acme", "Bolt"}, orgTitles(m))
	press(m, "ctrl+o", "right", "esc") // back to recent, order untouched
	assert.Equal(t, "desc", m.orderOf(viewOrgs))
	assert.Equal(t, []string{"Bolt", "Acme"}, orgTitles(m))
}

func TestOrderIsRememberedPerView(t *testing.T) {
	m, _ := newTestModel(t)
	asc := names(m)
	press(m, "ctrl+o", "down", "right", "esc")
	require.Equal(t, reversed(asc), names(m))
	m.switchView(viewOrgs)
	assert.Equal(t, "desc", m.orderOf(viewOrgs), "Organizations keep their own default")
	m.switchView(viewPeople)
	assert.Equal(t, "desc", m.orderOf(viewPeople))
	assert.Equal(t, reversed(asc), names(m), "People are still Z to A")
}

func TestTagsOrderFlipsNameAndCount(t *testing.T) {
	m := tagsModel(t)
	assert.Equal(t, []string{"Client", "client", "friend"}, tagTitles(m))
	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, []string{"friend", "client", "Client"}, tagTitles(m), "name descending")

	m = tagsModel(t)
	m.sortChoice = map[viewKind]string{viewTags: "count"}
	require.NoError(t, m.rebuild(false))
	assert.Equal(t, []string{"client", "friend", "Client"}, tagTitles(m), "count: most objects first by default")
	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, []string{"Client", "client", "friend"}, tagTitles(m), "count ascending: fewest first")
}
