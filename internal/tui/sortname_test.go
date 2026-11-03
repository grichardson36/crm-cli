package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-020: Deals and Tasks get the Sort and Order rows of ctrl+o, with a Name sort. Their default
// order does not change: Deals newest first, Tasks soonest due first (undated last).

// threeDeals makes Cove (oldest), Alpha and Beta (newest) and shows the Deals view.
func threeDeals(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	for i, n := range []string{"Cove", "Alpha", "Beta"} {
		d, err := m.dlr.Create(context.Background(), model.CreateDealInput{Title: n, Stage: "lead"})
		require.NoError(t, err)
		_, err = m.db.Exec(`UPDATE deals SET created_at = ? WHERE id = ?`, "2026-01-0"+string(rune('1'+i))+" 00:00:00", d.ID)
		require.NoError(t, err)
	}
	m.afterWrite()
	m.switchView(viewDeals)
	m.settle()
	return m
}

// threeTasks makes Beta due first, Cove due second and Alpha without a due date.
func threeTasks(t *testing.T) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	d1, d2 := "2026-03-01", "2026-03-02"
	for _, in := range []model.CreateTaskInput{
		{Title: "Alpha", Priority: "medium"},
		{Title: "Cove", Priority: "medium", DueAt: &d2},
		{Title: "Beta", Priority: "medium", DueAt: &d1},
	} {
		_, err := m.tr.Create(context.Background(), in)
		require.NoError(t, err)
	}
	m.afterWrite()
	m.switchView(viewTasks)
	m.settle()
	return m
}

func TestDealsAndTasksHaveSortAndOrderRows(t *testing.T) {
	for _, v := range []viewKind{viewDeals, viewTasks} {
		var ids []string
		for _, o := range viewOptionsFor(v) {
			ids = append(ids, o.id)
		}
		assert.Equal(t, []string{"sort", "order"}, ids, viewNames[v])
	}
}

func TestDealsSortByNameAndOrder(t *testing.T) {
	m := threeDeals(t)
	assert.Equal(t, []string{"Beta", "Alpha", "Cove"}, orgTitles(m), "default: newest first, as before")

	press(m, "ctrl+o", "right", "esc")
	assert.Equal(t, []string{"Alpha", "Beta", "Cove"}, orgTitles(m), "sorted by name, A to Z")

	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, []string{"Cove", "Beta", "Alpha"}, orgTitles(m), "order descending, Z to A")

	press(m, "ctrl+o", "right", "esc") // sort back to newest, the order the user set (descending) is kept
	assert.Equal(t, []string{"Beta", "Alpha", "Cove"}, orgTitles(m))
	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, []string{"Cove", "Alpha", "Beta"}, orgTitles(m), "newest sort, ascending is oldest first")
}

func TestTasksSortByNameAndOrder(t *testing.T) {
	m := threeTasks(t)
	assert.Equal(t, []string{"Beta", "Cove", "Alpha"}, orgTitles(m), "default: soonest due first, undated last, as before")

	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, []string{"Cove", "Beta", "Alpha"}, orgTitles(m), "due date descending: latest first, undated still last")

	press(m, "ctrl+o", "right", "esc")
	assert.Equal(t, []string{"Cove", "Beta", "Alpha"}, orgTitles(m), "the order the user set is kept: name descending is Z to A")

	press(m, "ctrl+o", "down", "right", "esc")
	assert.Equal(t, []string{"Alpha", "Beta", "Cove"}, orgTitles(m), "name ascending, A to Z")
}

func TestDealsLetterJumpFollowsTheNameSort(t *testing.T) {
	m := threeDeals(t)
	press(m, "ctrl+o", "right", "esc")
	press(m, "home", "ctrl+down")
	assert.Equal(t, "Beta", m.rows[m.cursor].title)
}

func TestSortAndOrderAreRememberedPerDealsAndTasksView(t *testing.T) {
	m := threeDeals(t)
	press(m, "ctrl+o", "right", "esc")
	m.switchView(viewTasks)
	m.settle()
	assert.Equal(t, "due", m.sortOf(viewTasks), "Tasks keep their own sort")
	m.switchView(viewDeals)
	m.settle()
	assert.Equal(t, "name", m.sortOf(viewDeals))
}
