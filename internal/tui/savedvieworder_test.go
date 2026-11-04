package tui

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Save current view as keeps the Sort and the Order of the list: the saved view opens in the order the
// list was on screen, ascending or descending.

func TestCaptureViewKeepsSortAndOrderOfEveryList(t *testing.T) {
	m := svModel(t)
	tests := []struct {
		view  viewKind
		sort  string
		order string // "" leaves the natural order of the sort
		want  query.Sort
	}{
		{viewPeople, "name", "", query.Sort{Field: "name"}},
		{viewPeople, "name", "desc", query.Sort{Field: "name", Dir: "desc"}},
		{viewPeople, "recent", "", query.Sort{Field: "updated", Dir: "desc"}},
		{viewPeople, "recent", "asc", query.Sort{Field: "updated"}},
		{viewOrgs, "name", "desc", query.Sort{Field: "name", Dir: "desc"}},
		{viewOrgs, "recent", "asc", query.Sort{Field: "updated"}},
		{viewDeals, "recent", "", query.Sort{Field: "created", Dir: "desc"}},
		{viewDeals, "recent", "asc", query.Sort{Field: "created"}},
		{viewDeals, "name", "", query.Sort{Field: "title"}},
		{viewDeals, "name", "desc", query.Sort{Field: "title", Dir: "desc"}},
		{viewTasks, "due", "", query.Sort{Field: "due"}},
		{viewTasks, "due", "desc", query.Sort{Field: "due", Dir: "desc"}},
		{viewTasks, "name", "desc", query.Sort{Field: "title", Dir: "desc"}},
		{viewTags, "name", "desc", query.Sort{Field: "name", Dir: "desc"}},
		{viewTags, "count", "", query.Sort{Field: "used", Dir: "desc"}},
		{viewTags, "count", "asc", query.Sort{Field: "used"}},
	}
	for _, tt := range tests {
		t.Run(viewNames[tt.view]+" "+tt.sort+" "+tt.order, func(t *testing.T) {
			m.switchView(tt.view)
			m.sortChoice = map[viewKind]string{tt.view: tt.sort}
			m.orderChoice = map[viewKind]string{}
			if tt.order != "" {
				m.orderChoice[tt.view] = tt.order
			}
			_, q, ok := m.captureView()
			require.True(t, ok)
			assert.Equal(t, []query.Sort{tt.want}, q.Sort)
		})
	}
}

func TestSavedDealsAndTasksOpenInTheOrderOfTheList(t *testing.T) {
	t.Run("deals by name descending", func(t *testing.T) {
		m := threeDeals(t)
		press(m, "ctrl+o", "right", "down", "right", "esc")
		require.Equal(t, []string{"Cove", "Beta", "Alpha"}, orgTitles(m))
		runPalette(m, "save current view")
		typeText(m, "Deals Z to A")
		press(m, "enter")
		v, err := m.svr.Find(m.ctx, "Deals Z to A")
		require.NoError(t, err)
		m.openSaved(v.UUID)
		assert.Equal(t, []string{"Cove", "Beta", "Alpha"}, orgTitles(m))
	})
	t.Run("tasks by due date descending, undated last", func(t *testing.T) {
		m := threeTasks(t)
		press(m, "ctrl+o", "down", "right", "esc")
		require.Equal(t, []string{"Cove", "Beta", "Alpha"}, orgTitles(m))
		runPalette(m, "save current view")
		typeText(m, "Latest due")
		press(m, "enter")
		v, err := m.svr.Find(m.ctx, "Latest due")
		require.NoError(t, err)
		m.openSaved(v.UUID)
		assert.Equal(t, []string{"Cove", "Beta", "Alpha"}, orgTitles(m))
	})
}
