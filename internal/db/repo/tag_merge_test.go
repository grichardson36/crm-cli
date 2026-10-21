package repo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tagSetup(t *testing.T) (*repo.TagRepo, *repo.PersonRepo, *repo.OrgRepo, func(q string, args ...any) int) {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	count := func(q string, args ...any) int {
		var n int
		require.NoError(t, d.QueryRow(q, args...).Scan(&n))
		return n
	}
	return repo.NewTagRepo(d), repo.NewPersonRepo(d), repo.NewOrgRepo(d), count
}

func TestTagRenameRefusesAnExistingNameInAnyCase(t *testing.T) {
	tr, pr, _, _ := tagSetup(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Ann"})
	require.NoError(t, err)
	require.NoError(t, tr.Apply(ctx, "person", p.ID, "friends"))
	require.NoError(t, tr.Apply(ctx, "person", p.ID, "Friend"))

	err = tr.Rename(ctx, "friends", "friend")
	var exists *repo.TagExistsError
	require.ErrorAs(t, err, &exists)
	assert.Equal(t, "Friend", exists.Existing)
	assert.True(t, errors.Is(err, model.ErrConflict))
	assert.Contains(t, err.Error(), "merge")

	require.NoError(t, tr.Rename(ctx, "friends", "#pals"), "a leading # is ignored")
	tags, _ := tr.GetForEntity(ctx, "person", p.ID)
	var names []string
	for _, tg := range tags {
		names = append(names, tg.Name)
	}
	assert.ElementsMatch(t, []string{"Friend", "pals"}, names)

	require.NoError(t, tr.Rename(ctx, "pals", "Pals"), "changing only the case of its own name is fine")
	assert.ErrorIs(t, tr.Rename(ctx, "nope", "x"), model.ErrNotFound)
	assert.ErrorIs(t, tr.Rename(ctx, "Pals", "  "), model.ErrValidation)
}

func TestTagMergeMovesDedupesAndUndoes(t *testing.T) {
	tr, pr, or, count := tagSetup(t)
	ctx := context.Background()
	mk := func(name string) int64 {
		p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: name})
		require.NoError(t, err)
		return p.ID
	}
	a, b, c := mk("A"), mk("B"), mk("C")
	o, err := or.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	for _, id := range []int64{a, b} {
		require.NoError(t, tr.Apply(ctx, "person", id, "old"))
	}
	require.NoError(t, tr.Apply(ctx, "organization", o.ID, "old"))
	for _, id := range []int64{b, c} {
		require.NoError(t, tr.Apply(ctx, "person", id, "new"))
	}

	pre, err := tr.MergeDryRun(ctx, "old", "new")
	require.NoError(t, err)
	assert.True(t, pre.DryRun)
	assert.Equal(t, map[string]int{"person": 1, "organization": 1}, pre.Moved)
	assert.Equal(t, map[string]int{"person": 1}, pre.Duplicates)
	assert.Equal(t, 2, count(`SELECT COUNT(*) FROM tags`), "a preview writes nothing")

	res, err := tr.Merge(ctx, "old", "new")
	require.NoError(t, err)
	assert.Equal(t, 2, res.Total)
	tags, _ := tr.FindAll(ctx)
	require.Len(t, tags, 1)
	assert.Equal(t, "new", tags[0].Name)
	ids, _ := tr.GetEntities(ctx, "new", "person")
	assert.ElementsMatch(t, []int64{a, b, c}, ids)
	oids, _ := tr.GetEntities(ctx, "new", "organization")
	assert.Equal(t, []int64{o.ID}, oids)
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM tombstones WHERE table_name = 'tags'`), "the removed tag leaves a tombstone")

	require.NoError(t, tr.UndoMerge(ctx, res))
	old, _ := tr.GetEntities(ctx, "old", "person")
	assert.ElementsMatch(t, []int64{a, b}, old, "undo gives the source tag its records back, duplicates included")
	nw, _ := tr.GetEntities(ctx, "new", "person")
	assert.ElementsMatch(t, []int64{b, c}, nw)
	assert.Equal(t, 0, count(`SELECT COUNT(*) FROM tombstones WHERE table_name = 'tags'`))

	require.NoError(t, tr.RedoMerge(ctx, res))
	tags, _ = tr.FindAll(ctx)
	assert.Len(t, tags, 1)

	_, err = tr.Merge(ctx, "new", "new")
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = tr.Merge(ctx, "ghost", "new")
	assert.ErrorIs(t, err, model.ErrNotFound)
}
