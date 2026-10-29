package repo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var svNow = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func svNames(vs []repo.SavedView) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = v.Name
	}
	return out
}

func TestSavedView_ExamplesAreSeededOffAndMarked(t *testing.T) {
	f := setupLinks(t)
	r := repo.NewSavedViewRepo(f.links.DB())
	list, err := r.List(context.Background(), false)
	require.NoError(t, err)
	assert.Equal(t, []string{"Overdue follow-up", "Open deals by stage", "Tasks due this week"}, svNames(list))
	for _, v := range list {
		assert.True(t, v.Example, v.Name)
		assert.False(t, v.ShowInTabs, v.Name+" is off by default")
		assert.NoError(t, query.Validate(v.Kind, v.Query, r.Env(context.Background(), svNow)), v.Name+" is a valid query")
	}
}

func TestSavedView_ExamplesRun(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	d := f.links.DB()
	_, err := d.Exec(`UPDATE people SET last_contacted_at = '2026-05-01' WHERE first_name = 'Sam'`)
	require.NoError(t, err)
	_, err = d.Exec(`UPDATE people SET last_contacted_at = '2026-10-20' WHERE first_name = 'Alex'`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO deals (uuid, title, stage, value) VALUES ('d9', 'Won one', 'won', 9), ('d10', 'Late one', 'negotiation', 7)`)
	require.NoError(t, err)
	_, err = d.Exec(`UPDATE tasks SET due_at = '2026-10-31'`)
	require.NoError(t, err)
	r := repo.NewSavedViewRepo(d)

	byName := func(name string) *repo.SavedView {
		v, err := r.Find(ctx, name)
		require.NoError(t, err)
		return v
	}
	rows, err := r.Run(ctx, byName("Overdue follow-up"), svNow)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Sam", rows[0].Label, "only the person last contacted more than 90 days ago")

	rows, err = r.Run(ctx, byName("Open deals by stage"), svNow)
	require.NoError(t, err)
	var groups []string
	for _, row := range rows {
		if len(groups) == 0 || groups[len(groups)-1] != row.GroupLabel {
			groups = append(groups, row.GroupLabel)
		}
		assert.NotEqual(t, "Won one", row.Label, "a won deal is not open")
	}
	assert.Equal(t, []string{"lead", "negotiation"}, groups, "grouped by stage in the order of the stage list")

	rows, err = r.Run(ctx, byName("Tasks due this week"), svNow)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Call", rows[0].Label)
	assert.Equal(t, "2026-10-31 Sat", rows[0].GroupLabel)
}

func TestSavedView_CreateFindUpdateDeleteRestore(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	r := repo.NewSavedViewRepo(f.links.DB())

	v, err := r.Create(ctx, repo.SavedViewInput{Name: "  Hot leads ", Kind: "person", ShowInTabs: true,
		Query: query.Query{Filter: []query.Condition{{Field: "tag", Op: query.OpHas, Values: []string{"lead"}}}, GroupBy: "tier"}})
	require.NoError(t, err)
	assert.Equal(t, "Hot leads", v.Name)
	assert.True(t, v.ShowInTabs)
	assert.False(t, v.Example)
	assert.Equal(t, 4, v.Position, "after the three examples")
	assert.Equal(t, "tier", v.Query.GroupBy)

	t.Run("find by id, uuid and name", func(t *testing.T) {
		for _, ref := range []string{"hot LEADS", v.UUID} {
			got, err := r.Find(ctx, ref)
			require.NoError(t, err, ref)
			assert.Equal(t, v.ID, got.ID)
		}
		got, err := r.Find(ctx, "4")
		require.NoError(t, err)
		assert.Equal(t, v.ID, got.ID)
		_, err = r.Find(ctx, "no such view")
		assert.ErrorIs(t, err, model.ErrNotFound)
	})

	t.Run("name is unique among live views", func(t *testing.T) {
		_, err := r.Create(ctx, repo.SavedViewInput{Name: "HOT leads", Kind: "person"})
		assert.ErrorIs(t, err, model.ErrConflict)
		other, err := r.Create(ctx, repo.SavedViewInput{Name: "Other", Kind: "task"})
		require.NoError(t, err)
		n := "hot leads"
		_, err = r.Update(ctx, other.ID, repo.SavedViewPatch{Name: &n})
		assert.ErrorIs(t, err, model.ErrConflict)
	})

	t.Run("update changes the name, the query and the tab flag", func(t *testing.T) {
		name := "Warm leads"
		off := false
		q := query.Query{Filter: []query.Condition{{Field: "company", Op: query.OpNotEmpty}}}
		got, err := r.Update(ctx, v.ID, repo.SavedViewPatch{Name: &name, ShowInTabs: &off, Query: &q})
		require.NoError(t, err)
		assert.Equal(t, "Warm leads", got.Name)
		assert.False(t, got.ShowInTabs)
		assert.Equal(t, q, got.Query)
		on, err := r.SetTabs(ctx, v.ID, true)
		require.NoError(t, err)
		assert.True(t, on.ShowInTabs)
	})

	t.Run("changing the kind revalidates the query", func(t *testing.T) {
		kind := "task"
		_, err := r.Update(ctx, v.ID, repo.SavedViewPatch{Kind: &kind})
		require.Error(t, err, "company is not a task field")
		ve, ok := validate.As(err)
		require.True(t, ok)
		assert.Equal(t, validate.CodeQueryField, ve.First().Code)
	})

	t.Run("delete is soft, restore brings it back", func(t *testing.T) {
		require.NoError(t, r.Delete(ctx, v.ID))
		live, err := r.List(ctx, false)
		require.NoError(t, err)
		assert.NotContains(t, svNames(live), "Warm leads")
		dead, err := r.List(ctx, true)
		require.NoError(t, err)
		assert.Equal(t, []string{"Warm leads"}, svNames(dead))
		assert.ErrorIs(t, r.Delete(ctx, v.ID), model.ErrConflict, "already deleted")
		name := "x"
		_, err = r.Update(ctx, v.ID, repo.SavedViewPatch{Name: &name})
		assert.ErrorIs(t, err, model.ErrConflict, "a deleted view is not edited")

		got, err := r.Restore(ctx, v.ID)
		require.NoError(t, err)
		assert.False(t, got.Deleted)
		_, err = r.Restore(ctx, v.ID)
		assert.ErrorIs(t, err, model.ErrConflict, "not deleted")
	})

	t.Run("restore is refused when the name was taken", func(t *testing.T) {
		require.NoError(t, r.Delete(ctx, v.ID))
		_, err := r.Create(ctx, repo.SavedViewInput{Name: "Warm leads", Kind: "person"})
		require.NoError(t, err, "a deleted view frees its name")
		_, err = r.Restore(ctx, v.ID)
		assert.ErrorIs(t, err, model.ErrConflict)
	})

	t.Run("not found", func(t *testing.T) {
		assert.ErrorIs(t, r.Delete(ctx, 9999), model.ErrNotFound)
		_, err := r.Restore(ctx, 9999)
		assert.ErrorIs(t, err, model.ErrNotFound)
	})
}

func TestSavedView_ValidationAtTheDoor(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	r := repo.NewSavedViewRepo(f.links.DB())
	tests := []struct {
		name string
		in   repo.SavedViewInput
		code string
	}{
		{"empty name", repo.SavedViewInput{Name: " ", Kind: "person"}, validate.CodeViewName},
		{"unknown kind", repo.SavedViewInput{Name: "A", Kind: "widget"}, validate.CodeViewKind},
		{"unknown field", repo.SavedViewInput{Name: "A", Kind: "person", Query: query.Query{Filter: []query.Condition{{Field: "x", Op: query.OpIs, Values: []string{"1"}}}}}, validate.CodeQueryField},
		{"bad operator", repo.SavedViewInput{Name: "A", Kind: "person", Query: query.Query{Filter: []query.Condition{{Field: "first", Op: "regex", Values: []string{"1"}}}}}, validate.CodeQueryOperator},
		{"bad value", repo.SavedViewInput{Name: "A", Kind: "deal", Query: query.Query{Filter: []query.Condition{{Field: "value", Op: query.OpGt, Values: []string{"lots"}}}}}, validate.CodeQueryValue},
		{"bad group", repo.SavedViewInput{Name: "A", Kind: "person", Query: query.Query{GroupBy: "email"}}, validate.CodeQueryGroup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.Create(ctx, tt.in)
			require.Error(t, err)
			ve, ok := validate.As(err)
			require.True(t, ok)
			assert.Equal(t, tt.code, ve.First().Code)
			assert.ErrorIs(t, err, model.ErrValidation)
			assert.Equal(t, 2, model.ExitCode(err))
		})
	}
	list, _ := r.List(ctx, false)
	assert.Len(t, list, 3, "nothing was stored")
}

func TestSavedView_Move(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	r := repo.NewSavedViewRepo(f.links.DB())
	list, _ := r.List(ctx, false)
	third := list[2]
	moved, err := r.Move(ctx, third.ID, -1)
	require.NoError(t, err)
	assert.True(t, moved)
	list, _ = r.List(ctx, false)
	assert.Equal(t, []string{"Overdue follow-up", "Tasks due this week", "Open deals by stage"}, svNames(list))
	moved, err = r.Move(ctx, list[0].ID, -1)
	require.NoError(t, err)
	assert.False(t, moved, "the first view does not move up")
	moved, err = r.Move(ctx, list[2].ID, 1)
	require.NoError(t, err)
	assert.False(t, moved, "the last view does not move down")
	_, err = r.Move(ctx, 9999, 1)
	assert.ErrorIs(t, err, model.ErrNotFound)
}

func TestSavedView_RelationValuesAreStoredAsUUIDs(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	d := f.links.DB()
	r := repo.NewSavedViewRepo(d)
	_, err := f.links.Link(ctx, repo.Ref{Kind: "person", ID: f.person}, repo.Ref{Kind: "organization", ID: f.org})
	require.NoError(t, err)

	var uuid string
	require.NoError(t, d.QueryRow(`SELECT uuid FROM organizations WHERE name = 'Initech'`).Scan(&uuid))

	for i, val := range []string{"Initech", "initech", "1", uuid} {
		v, err := r.Create(ctx, repo.SavedViewInput{Name: fmt.Sprintf("At %d", i), Kind: "person",
			Query: query.Query{Filter: []query.Condition{{Field: "organization", Op: query.OpLinkedTo, Values: []string{val}}}}})
		require.NoError(t, err, val)
		assert.Equal(t, []string{uuid}, v.Query.Filter[0].Values, "stored as the uuid, the same on every device (%s)", val)
		rows, err := r.Run(ctx, v, svNow)
		require.NoError(t, err)
		require.Len(t, rows, 1, val)
		assert.Equal(t, "Sam", rows[0].Label)
	}
	labels := r.RelationLabels(ctx, "organizations", []string{uuid, "missing"})
	assert.Equal(t, map[string]string{uuid: "Initech"}, labels)
	cands, err := r.RelationCandidates(ctx, "organizations")
	require.NoError(t, err)
	assert.Len(t, cands, 2)

	t.Run("an unknown name is a validation error", func(t *testing.T) {
		_, err := r.Create(ctx, repo.SavedViewInput{Name: "Nobody", Kind: "person",
			Query: query.Query{Filter: []query.Condition{{Field: "organization", Op: query.OpLinkedTo, Values: []string{"No such org"}}}}})
		var ve *validate.Error
		require.True(t, errors.As(err, &ve))
		assert.Equal(t, validate.CodeQueryValue, ve.First().Code)
	})
	t.Run("an ambiguous name asks for the id", func(t *testing.T) {
		_, err := d.Exec(`INSERT INTO organizations (uuid, name) VALUES ('dup-uuid', 'Globex')`)
		require.NoError(t, err)
		_, err = r.Create(ctx, repo.SavedViewInput{Name: "Twice", Kind: "person",
			Query: query.Query{Filter: []query.Condition{{Field: "organization", Op: query.OpLinkedTo, Values: []string{"Globex"}}}}})
		var ve *validate.Error
		require.True(t, errors.As(err, &ve))
		assert.Contains(t, ve.First().Expected, "have that name")
	})
}
