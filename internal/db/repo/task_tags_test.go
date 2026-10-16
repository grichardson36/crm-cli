package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskTags(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	tr, tags := repo.NewTaskRepo(d), repo.NewTagRepo(d)
	ctx := context.Background()

	a, err := tr.Create(ctx, model.CreateTaskInput{Title: "A", Priority: "medium", Tags: []string{"work", "urgent"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"urgent", "work"}, a.Tags, "tags come back sorted")
	b, err := tr.Create(ctx, model.CreateTaskInput{Title: "B", Priority: "medium"})
	require.NoError(t, err)
	assert.Empty(t, b.Tags)

	require.NoError(t, tags.Apply(ctx, "task", b.ID, "home"))
	require.NoError(t, tags.Apply(ctx, "task", b.ID, "work"))
	require.NoError(t, tags.Apply(ctx, "task", b.ID, "work"), "applying twice is a no-op")
	got, err := tr.FindByID(ctx, b.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"home", "work"}, got.Tags)

	work := "work"
	list, err := tr.FindAll(ctx, model.TaskFilters{Tag: &work})
	require.NoError(t, err)
	require.Len(t, list, 2)
	home := "home"
	list, err = tr.FindAll(ctx, model.TaskFilters{Tag: &home})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "B", list[0].Title)
	assert.Equal(t, []string{"home", "work"}, list[0].Tags)
	none := "nothing"
	list, err = tr.FindAll(ctx, model.TaskFilters{Tag: &none})
	require.NoError(t, err)
	assert.Empty(t, list)

	require.NoError(t, tags.Remove(ctx, "task", b.ID, "home"))
	got, _ = tr.FindByID(ctx, b.ID)
	assert.Equal(t, []string{"work"}, got.Tags)
	assert.ErrorIs(t, tags.Remove(ctx, "task", b.ID, "home"), model.ErrNotFound)

	// An unknown or archived task cannot be tagged, and tagging bumps updated_at.
	assert.ErrorIs(t, tags.Apply(ctx, "task", 999, "x"), model.ErrNotFound)
	require.NoError(t, tr.Archive(ctx, b.ID))
	assert.ErrorIs(t, tags.Apply(ctx, "task", b.ID, "x"), model.ErrNotFound)
	entities, err := tags.GetEntities(ctx, "work", "task")
	require.NoError(t, err)
	assert.Contains(t, entities, a.ID)

	// Search still rejects "task" (it is taggable, not searchable), tagging accepts it.
	assert.True(t, model.ValidTaggable("task"))
	assert.False(t, model.ValidEntityType("task"))
}

func TestTaskRestoreAndReopen(t *testing.T) {
	tr := setupTaskTestDB(t)
	ctx := context.Background()
	task, err := tr.Create(ctx, model.CreateTaskInput{Title: "A", Priority: "medium"})
	require.NoError(t, err)

	require.NoError(t, tr.Archive(ctx, task.ID))
	assert.ErrorIs(t, tr.Restore(ctx, 999), model.ErrNotFound)
	require.NoError(t, tr.Restore(ctx, task.ID))
	assert.ErrorIs(t, tr.Restore(ctx, task.ID), model.ErrNotFound, "not archived any more")

	_, err = tr.Complete(ctx, task.ID)
	require.NoError(t, err)
	require.NoError(t, tr.Reopen(ctx, task.ID))
	got, err := tr.FindByID(ctx, task.ID)
	require.NoError(t, err)
	assert.False(t, got.Completed)
	assert.Nil(t, got.CompletedAt)
	assert.ErrorIs(t, tr.Reopen(ctx, task.ID), model.ErrNotFound, "already open")
}
