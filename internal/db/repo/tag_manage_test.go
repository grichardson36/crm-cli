package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tagWorld makes Ann and Ben (people) and Acme (an organization) to put tags on.
func tagWorld(t *testing.T) (*repo.TagRepo, context.Context, map[string]int64) {
	t.Helper()
	tr, pr, or, _ := tagSetup(t)
	ctx := context.Background()
	ids := map[string]int64{}
	for _, n := range []string{"Ann", "Ben"} {
		p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: n})
		require.NoError(t, err)
		ids[n] = p.ID
	}
	o, err := or.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	ids["Acme"] = o.ID
	return tr, ctx, ids
}

func statOf(t *testing.T, tr *repo.TagRepo, name string) repo.TagStat {
	t.Helper()
	all, err := tr.Stats(context.Background())
	require.NoError(t, err)
	for _, s := range all {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no tag %q", name)
	return repo.TagStat{}
}

func TestTagStatsCountsAndBreakdown(t *testing.T) {
	tr, ctx, ids := tagWorld(t)
	require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], "friend"))
	require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "friend"))
	require.NoError(t, tr.Apply(ctx, "organization", ids["Acme"], "friend"))
	require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], "solo"))
	_, err := tr.FindOrCreate(ctx, "empty")
	require.NoError(t, err)

	tests := []struct {
		tag    string
		count  int
		byKind map[string]int
		unused bool
	}{
		{"friend", 3, map[string]int{"person": 2, "organization": 1}, false},
		{"solo", 1, map[string]int{"person": 1}, false},
		{"empty", 0, map[string]int{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			s := statOf(t, tr, tt.tag)
			assert.Equal(t, tt.count, s.Count)
			assert.Equal(t, tt.byKind, s.ByKind)
			assert.Equal(t, tt.unused, s.Unused())
		})
	}

	all, err := tr.Stats(ctx)
	require.NoError(t, err)
	var names []string
	for _, s := range all {
		names = append(names, s.Name)
	}
	assert.Equal(t, []string{"empty", "friend", "solo"}, names, "unused tags are listed, by name")

	require.NoError(t, tr.Remove(ctx, "person", ids["Ben"], "friend"))
	assert.Equal(t, 2, statOf(t, tr, "friend").Count)
}

func TestTagDeleteLeavesTheNameUnused(t *testing.T) {
	tr, ctx, ids := tagWorld(t)
	require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], "gone"))
	res, err := tr.DeleteTag(ctx, "gone")
	require.NoError(t, err)
	assert.True(t, statOf(t, tr, "gone").Unused())
	require.NoError(t, tr.UndoDelete(ctx, res))
	assert.Equal(t, 1, statOf(t, tr, "gone").Count)
}

func TestTaggedObjectsGroupedByKind(t *testing.T) {
	tr, ctx, ids := tagWorld(t)
	for _, tg := range []struct {
		kind string
		id   int64
	}{{"organization", ids["Acme"]}, {"person", ids["Ben"]}, {"person", ids["Ann"]}} {
		require.NoError(t, tr.Apply(ctx, tg.kind, tg.id, "x"))
	}
	objs, err := tr.TaggedObjects(ctx, "x")
	require.NoError(t, err)
	var got []string
	for _, o := range objs {
		got = append(got, o.Kind+":"+o.Name)
	}
	assert.Equal(t, []string{"person:Ann", "person:Ben", "organization:Acme"}, got, "kinds in registry order, names inside a kind")
	_, err = tr.TaggedObjects(ctx, "nope")
	assert.ErrorIs(t, err, model.ErrNotFound)
}

func TestDupKeyAndDuplicates(t *testing.T) {
	tests := []struct {
		a, b string
		same bool
	}{
		{"Friend", "friend", true},
		{"the-well", "The_Well", true},
		{"the well", "thewell", true},
		{"a/b", "a-b", false},
		{"friend", "friends", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+"|"+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.same, repo.DupKey(tt.a) == repo.DupKey(tt.b))
		})
	}

	tr, ctx, ids := tagWorld(t)
	for _, n := range []string{"Friend", "friend", "FRIEND"} {
		require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], n))
	}
	require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "friend"))
	require.NoError(t, tr.Apply(ctx, "organization", ids["Acme"], "friend"))
	require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "lonely"))
	groups, err := tr.Duplicates(ctx)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "friend", groups[0].Keep, "the most used spelling")
	assert.Len(t, groups[0].Tags, 3)
}

func TestMergeDuplicatesDryRunApplyUndoRedo(t *testing.T) {
	tr, ctx, ids := tagWorld(t)
	for _, n := range []string{"Friend", "friend", "FRIEND"} {
		require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], n))
	}
	require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "friend"))
	require.NoError(t, tr.Apply(ctx, "organization", ids["Acme"], "FRIEND"))
	require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "other"))

	dry, err := tr.MergeDuplicates(ctx, true)
	require.NoError(t, err)
	assert.True(t, dry.DryRun)
	assert.Equal(t, 1, dry.Groups)
	assert.Equal(t, 2, dry.Merged)
	all, _ := tr.Stats(ctx)
	assert.Len(t, all, 4, "the dry run changed nothing")

	res, err := tr.MergeDuplicates(ctx, false)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Merged)
	all, _ = tr.Stats(ctx)
	assert.Len(t, all, 2)
	assert.Equal(t, 3, statOf(t, tr, "friend").Count, "Ann, Ben and Acme count once each")

	require.NoError(t, tr.UndoDuplicates(ctx, res))
	all, _ = tr.Stats(ctx)
	assert.Len(t, all, 4, "one undo brings every spelling back")
	require.NoError(t, tr.RedoDuplicates(ctx, res))
	all, _ = tr.Stats(ctx)
	assert.Len(t, all, 2)

	again, err := tr.MergeDuplicates(ctx, false)
	require.NoError(t, err)
	assert.Zero(t, again.Merged, "nothing left to merge")
}

func TestMergeDuplicatesOnlyNamedGroups(t *testing.T) {
	tr, ctx, ids := tagWorld(t)
	for _, n := range []string{"a", "A", "b", "B"} {
		require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], n))
	}
	res, err := tr.MergeDuplicates(ctx, false, "B")
	require.NoError(t, err)
	assert.Equal(t, 1, res.Groups)
	all, _ := tr.Stats(ctx)
	assert.Len(t, all, 3, "only the group with B merged")
}

func TestDeleteUnusedDryRunApplyUndoRedo(t *testing.T) {
	tr, ctx, ids := tagWorld(t)
	require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], "used"))
	require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], "was"))
	require.NoError(t, tr.Remove(ctx, "person", ids["Ann"], "was")) // a soft deleted tagging stays behind
	_, err := tr.FindOrCreate(ctx, "never")
	require.NoError(t, err)

	unused, err := tr.Unused(ctx)
	require.NoError(t, err)
	require.Len(t, unused, 2)

	dry, err := tr.DeleteUnused(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, 2, dry.Count)
	all, _ := tr.Stats(ctx)
	assert.Len(t, all, 3, "the dry run changed nothing")

	res, err := tr.DeleteUnused(ctx, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"was", "never"}, res.Tags)
	all, _ = tr.Stats(ctx)
	require.Len(t, all, 1)
	assert.Equal(t, "used", all[0].Name)

	require.NoError(t, tr.UndoDeleteUnused(ctx, res))
	all, _ = tr.Stats(ctx)
	assert.Len(t, all, 3)
	require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], "was"), "the old tagging row is back and revives")
	assert.Equal(t, 1, statOf(t, tr, "was").Count)
	require.NoError(t, tr.Remove(ctx, "person", ids["Ann"], "was"))

	require.NoError(t, tr.RedoDeleteUnused(ctx, res))
	all, _ = tr.Stats(ctx)
	assert.Len(t, all, 1)
}

func TestDeleteUnusedNamedRefusesAUsedTag(t *testing.T) {
	tr, ctx, ids := tagWorld(t)
	require.NoError(t, tr.Apply(ctx, "person", ids["Ann"], "used"))
	_, err := tr.DeleteUnused(ctx, false, "used")
	assert.ErrorIs(t, err, model.ErrNotFound)
	_, err = tr.FindOrCreate(ctx, "idle")
	require.NoError(t, err)
	res, err := tr.DeleteUnused(ctx, false, "idle")
	require.NoError(t, err)
	assert.Equal(t, "delete unused tag #idle", res.Label())
}
