package repo_test

import (
	"errors"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nestedWorld has Ann, Ben (people) and Acme (organization) with these tags:
//
//	work               Ann
//	work/clients/acme  Ann, Acme
//	work/clients/beta  Ben
//	home               Ben
//
// work/clients is a virtual parent: no tag is named so.
func nestedWorld(t *testing.T) (*repo.TagRepo, map[string]int64) {
	t.Helper()
	tr, ctx, ids := tagWorld(t)
	for _, a := range []struct {
		kind, who, tag string
	}{
		{"person", "Ann", "work"}, {"person", "Ann", "work/clients/acme"}, {"organization", "Acme", "work/clients/acme"},
		{"person", "Ben", "work/clients/beta"}, {"person", "Ben", "home"},
	} {
		require.NoError(t, tr.Apply(ctx, a.kind, ids[a.who], a.tag))
	}
	return tr, ids
}

func treePaths(t *testing.T, tr *repo.TagRepo) []string {
	t.Helper()
	nodes, err := tr.Tree(t.Context(), true)
	require.NoError(t, err)
	var out []string
	for _, n := range nodes {
		out = append(out, n.Path)
	}
	return out
}

func tagNames(t *testing.T, tr *repo.TagRepo) []string {
	t.Helper()
	all, err := tr.Stats(t.Context())
	require.NoError(t, err)
	var out []string
	for _, s := range all {
		out = append(out, s.Name)
	}
	return out
}

func TestTagTreeVirtualParentsAndCounts(t *testing.T) {
	tr, _ := nestedWorld(t)
	nodes, err := tr.Tree(t.Context(), false)
	require.NoError(t, err)
	type want struct {
		virtual    bool
		own, total int
	}
	wants := map[string]want{
		"home":              {false, 1, 1},
		"work":              {false, 1, 3}, // Ann is on work and on acme and counts once, Acme and Ben add two
		"work/clients":      {true, 0, 3},
		"work/clients/acme": {false, 2, 2},
		"work/clients/beta": {false, 1, 1},
	}
	var order []string
	for _, n := range nodes {
		order = append(order, n.Path)
		w, ok := wants[n.Path]
		require.True(t, ok, n.Path)
		assert.Equal(t, w.virtual, n.Virtual, n.Path)
		assert.Equal(t, w.own, n.Own, n.Path+" own")
		assert.Equal(t, w.total, n.Total, n.Path+" total")
	}
	assert.Equal(t, []string{"home", "work", "work/clients", "work/clients/acme", "work/clients/beta"}, order)
}

func TestTagNameValidation(t *testing.T) {
	tr, ctx, ids := tagWorld(t)
	tests := []struct {
		name, tag string
		code      string
	}{
		{"fine", "a/b", ""},
		{"empty segment", "a//b", validate.CodePathEmptySegment},
		{"leading slash", "/a", validate.CodePathEdgeSlash},
		{"trailing slash", "a/", validate.CodePathEdgeSlash},
		{"too deep", "a/b/c/d/e/f/g/h/i", validate.CodePathTooDeep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tr.Apply(ctx, "person", ids["Ann"], tt.tag)
			if tt.code == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, model.ErrValidation)
			var ve *validate.Error
			require.True(t, errors.As(err, &ve))
			assert.Equal(t, tt.code, ve.Issues[0].Code)
		})
	}
	assert.ErrorIs(t, tr.Rename(ctx, "a/b", "x//y"), model.ErrValidation)
}

func TestRenameTreeCascadesToDescendants(t *testing.T) {
	ctx := t.Context()
	tests := []struct {
		name, from, to string
		want           []string
	}{
		{"rename a virtual parent", "work/clients", "work/accounts",
			[]string{"home", "work", "work/accounts/acme", "work/accounts/beta"}},
		{"rename a top level tag with children", "work", "job",
			[]string{"home", "job", "job/clients/acme", "job/clients/beta"}},
		{"rename a leaf into a new branch", "work/clients/beta", "archive/2026/beta",
			[]string{"archive/2026/beta", "home", "work", "work/clients/acme"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, _ := nestedWorld(t)
			res, err := tr.RenameTree(ctx, tt.from, tt.to, false)
			require.NoError(t, err)
			assert.Equal(t, tt.want, tagNames(t, tr))
			require.NoError(t, tr.UndoTree(ctx, res))
			assert.Equal(t, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, tagNames(t, tr), "undo puts every name back")
			require.NoError(t, tr.RedoTree(ctx, res))
			assert.Equal(t, tt.want, tagNames(t, tr), "redo repeats it")
		})
	}
}

func TestRenameTreeRefusesCycleAndBadNames(t *testing.T) {
	tr, _ := nestedWorld(t)
	ctx := t.Context()
	tests := []struct {
		name, from, to string
		err            error
	}{
		{"into its own child", "work", "work/clients/x", model.ErrValidation},
		{"unknown tag", "nope", "x", model.ErrNotFound},
		{"bad path", "work", "a//b", model.ErrValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tr.RenameTree(ctx, tt.from, tt.to, false)
			assert.ErrorIs(t, err, tt.err)
		})
	}
	assert.Equal(t, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, tagNames(t, tr))
}

func TestRenameTreeCollisionOffersMergeThenMergesChildren(t *testing.T) {
	tr, ids := nestedWorld(t)
	ctx := t.Context()
	// job already has a child named clients/acme, with Ben on it, and job itself.
	require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "job"))
	require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "job/clients/acme"))

	_, err := tr.RenameTree(ctx, "work", "job", false)
	var exists *repo.TagExistsError
	require.True(t, errors.As(err, &exists), "the name is taken, the caller offers a merge")
	assert.Equal(t, "job", exists.Existing)
	assert.ErrorIs(t, err, model.ErrConflict)
	assert.Equal(t, []string{"home", "job", "job/clients/acme", "work", "work/clients/acme", "work/clients/beta"}, tagNames(t, tr), "a refused rename changes nothing")

	dry, err := tr.RenameTreeDryRun(ctx, "work", "job", true)
	require.NoError(t, err)
	assert.True(t, dry.DryRun)
	assert.Equal(t, 2, dry.Merged, "work and work/clients/acme merge")
	assert.Equal(t, 1, dry.Renamed, "work/clients/beta moves")
	assert.Len(t, tagNames(t, tr), 6, "a dry run writes nothing")

	res, err := tr.RenameTree(ctx, "work", "job", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"home", "job", "job/clients/acme", "job/clients/beta"}, tagNames(t, tr))
	assert.Equal(t, 3, statOf(t, tr, "job/clients/acme").Count, "Ann, Acme and Ben")

	require.NoError(t, tr.UndoTree(ctx, res))
	assert.Equal(t, []string{"home", "job", "job/clients/acme", "work", "work/clients/acme", "work/clients/beta"}, tagNames(t, tr), "one undo restores the merged tags")
	assert.Equal(t, 2, statOf(t, tr, "work/clients/acme").Count)
	require.NoError(t, tr.RedoTree(ctx, res))
	assert.Equal(t, []string{"home", "job", "job/clients/acme", "job/clients/beta"}, tagNames(t, tr))
}

func TestMergeTreeChildrenFollow(t *testing.T) {
	tests := []struct {
		name, source, target string
		prep                 []string // tags Ben also gets, to make the target
		want                 []string
		wantErr              error
	}{
		{"children become children of the target", "work", "area", []string{"area"},
			[]string{"area", "area/clients/acme", "area/clients/beta", "home"}, nil},
		{"same named children merge", "work", "area", []string{"area", "area/clients/acme"},
			[]string{"area", "area/clients/acme", "area/clients/beta", "home"}, nil},
		{"into a virtual parent", "home", "work/clients", nil,
			[]string{"work", "work/clients", "work/clients/acme", "work/clients/beta"}, nil},
		{"into itself", "work", "work", nil, nil, model.ErrValidation},
		{"into its own child", "work", "work/clients/acme", nil, nil, model.ErrValidation},
		{"unknown target", "work", "zzz", nil, nil, model.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, ids := nestedWorld(t)
			ctx := t.Context()
			for _, p := range tt.prep {
				require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], p))
			}
			res, err := tr.MergeTree(ctx, tt.source, tt.target)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, tagNames(t, tr))
			require.NoError(t, tr.UndoTree(ctx, res))
			assert.Contains(t, tagNames(t, tr), tt.source)
			require.NoError(t, tr.RedoTree(ctx, res))
			assert.Equal(t, tt.want, tagNames(t, tr))
		})
	}
}

func TestNestAndUnnestBulk(t *testing.T) {
	ctx := t.Context()
	t.Run("nest several under a new parent", func(t *testing.T) {
		tr, _ := nestedWorld(t)
		res, err := tr.Nest(ctx, []string{"home", "work/clients"}, "area/life", false)
		require.NoError(t, err)
		assert.Equal(t, []string{"area/life/clients/acme", "area/life/clients/beta", "area/life/home", "work"}, tagNames(t, tr))
		assert.Equal(t, 3, res.Renamed)
		require.NoError(t, tr.UndoTree(ctx, res))
		assert.Equal(t, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, tagNames(t, tr))
	})
	t.Run("a tag under another selected tag follows it", func(t *testing.T) {
		tr, _ := nestedWorld(t)
		_, err := tr.Nest(ctx, []string{"work", "work/clients/acme"}, "area", false)
		require.NoError(t, err)
		assert.Equal(t, []string{"area/work", "area/work/clients/acme", "area/work/clients/beta", "home"}, tagNames(t, tr))
	})
	t.Run("nest to the top level", func(t *testing.T) {
		tr, _ := nestedWorld(t)
		_, err := tr.Nest(ctx, []string{"work/clients/beta"}, "", false)
		require.NoError(t, err)
		assert.Equal(t, []string{"beta", "home", "work", "work/clients/acme"}, tagNames(t, tr))
	})
	t.Run("nest under its own child is refused", func(t *testing.T) {
		tr, _ := nestedWorld(t)
		_, err := tr.Nest(ctx, []string{"work"}, "work/clients", false)
		assert.ErrorIs(t, err, model.ErrValidation)
	})
	t.Run("a name the parent holds is refused, or merged", func(t *testing.T) {
		tr, ids := nestedWorld(t)
		require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "area/home"))
		_, err := tr.Nest(ctx, []string{"home", "work"}, "area", false)
		var exists *repo.TagExistsError
		require.True(t, errors.As(err, &exists))
		assert.Equal(t, []string{"area/home", "home", "work", "work/clients/acme", "work/clients/beta"}, tagNames(t, tr), "the bulk change is all or nothing")
		res, err := tr.Nest(ctx, []string{"home", "work"}, "area", true)
		require.NoError(t, err)
		assert.Equal(t, []string{"area/home", "area/work", "area/work/clients/acme", "area/work/clients/beta"}, tagNames(t, tr))
		assert.Equal(t, 1, res.Merged)
	})
	t.Run("unnest moves up one level", func(t *testing.T) {
		tr, _ := nestedWorld(t)
		res, err := tr.Unnest(ctx, []string{"work/clients/acme", "work/clients/beta"}, false)
		require.NoError(t, err)
		assert.Equal(t, []string{"home", "work", "work/acme", "work/beta"}, tagNames(t, tr))
		require.NoError(t, tr.UndoTree(ctx, res))
		assert.Equal(t, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, tagNames(t, tr))
	})
	t.Run("unnest a top level tag does nothing", func(t *testing.T) {
		tr, _ := nestedWorld(t)
		_, err := tr.Unnest(ctx, []string{"home"}, false)
		assert.ErrorIs(t, err, model.ErrValidation)
	})
}

func TestDeleteTagTreeIsOneUndoStep(t *testing.T) {
	tr, ids := nestedWorld(t)
	ctx := t.Context()
	st, err := tr.SubtreeStat(ctx, "work/clients")
	require.NoError(t, err)
	assert.Equal(t, repo.SubtreeStat{Path: "work/clients", Tags: 2, Taggings: 3, Objects: 3}, st, "a virtual parent counts its children")

	res, err := tr.DeleteTagTree(ctx, "work")
	require.NoError(t, err)
	assert.Equal(t, 4, len(res.TaggingIDs))
	assert.Equal(t, map[string]int{"person": 3, "organization": 1}, res.Removed)
	assert.Equal(t, []string{"work", "work/clients/acme", "work/clients/beta"}, res.Tags)
	assert.Equal(t, []string{"home"}, treePaths(t, tr)[:1], "only home is left in use")
	got, err := tr.GetForEntity(ctx, "person", ids["Ann"])
	require.NoError(t, err)
	assert.Empty(t, got)

	require.NoError(t, tr.UndoDelete(ctx, res))
	got, err = tr.GetForEntity(ctx, "person", ids["Ann"])
	require.NoError(t, err)
	assert.Len(t, got, 2, "one undo restores the whole subtree")
	require.NoError(t, tr.RedoDelete(ctx, res))
	got, _ = tr.GetForEntity(ctx, "person", ids["Ann"])
	assert.Empty(t, got)

	_, err = tr.DeleteTagTree(ctx, "work")
	assert.ErrorIs(t, err, model.ErrNotFound, "nothing left to delete")
	_, err = tr.SubtreeStat(ctx, "nope")
	assert.ErrorIs(t, err, model.ErrNotFound)
}

func TestDeleteTagStaysOnOneTag(t *testing.T) {
	tr, ids := nestedWorld(t)
	_, err := tr.DeleteTag(t.Context(), "work")
	require.NoError(t, err)
	got, _ := tr.GetForEntity(t.Context(), "person", ids["Ann"])
	require.Len(t, got, 1, "the child tag is still on Ann")
	assert.Equal(t, "work/clients/acme", got[0].Name)
}

func TestTaggedObjectsWithChildren(t *testing.T) {
	tr, _ := nestedWorld(t)
	ctx := t.Context()
	tests := []struct {
		name, tag string
		children  bool
		want      []string
		err       error
	}{
		{"own objects only", "work", false, []string{"person:Ann"}, nil},
		{"with children, an object once", "work", true, []string{"person:Ann", "person:Ben", "organization:Acme"}, nil},
		{"a virtual parent has none of its own", "work/clients", false, nil, nil},
		{"a virtual parent with children", "work/clients", true, []string{"person:Ann", "person:Ben", "organization:Acme"}, nil},
		{"a leaf", "work/clients/beta", true, []string{"person:Ben"}, nil},
		{"unknown", "nope", true, nil, model.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []repo.TaggedObject
			var err error
			if tt.children {
				objs, err = tr.TaggedObjectsWithChildren(ctx, tt.tag)
			} else {
				objs, err = tr.TaggedObjects(ctx, tt.tag)
			}
			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			var got []string
			for _, o := range objs {
				got = append(got, o.Kind+":"+o.Name)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOrphanAndDuplicateToolsRespectHierarchy(t *testing.T) {
	tr, ids := nestedWorld(t)
	ctx := t.Context()

	// "work/ghost" has no object and no child: unused. "work" itself has its own object. A parent that
	// only has used children is not unused even with no object of its own.
	_, err := tr.FindOrCreate(ctx, "work/ghost")
	require.NoError(t, err)
	_, err = tr.FindOrCreate(ctx, "work/clients")
	require.NoError(t, err)
	unused, err := tr.Unused(ctx)
	require.NoError(t, err)
	var got []string
	for _, u := range unused {
		got = append(got, u.Name)
	}
	assert.Equal(t, []string{"work/ghost"}, got, "work/clients has used children, so it is a parent in use")
	assert.True(t, statOf(t, tr, "work/clients").Parent)

	t.Run("case duplicates merge with their children", func(t *testing.T) {
		require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "Work/clients/acme"))
		require.NoError(t, tr.Apply(ctx, "person", ids["Ben"], "Work/extra"))
		groups, err := tr.Duplicates(ctx)
		require.NoError(t, err)
		var keys []string
		for _, g := range groups {
			keys = append(keys, g.Key)
		}
		assert.Contains(t, keys, "work/clients/acme", "the slash keeps levels apart, case does not")

		dry, err := tr.MergeDuplicates(ctx, true)
		require.NoError(t, err)
		assert.True(t, dry.DryRun)
		res, err := tr.MergeDuplicates(ctx, false)
		require.NoError(t, err)
		assert.Positive(t, res.Merged)
		for _, n := range tagNames(t, tr) {
			assert.NotContains(t, n, "Work", "every spelling is one now: %s", n)
		}
		assert.Contains(t, tagNames(t, tr), "work/extra", "a child with no twin follows its parent")
		require.NoError(t, tr.UndoDuplicates(ctx, res))
		assert.Contains(t, tagNames(t, tr), "Work/extra")
		assert.Contains(t, tagNames(t, tr), "Work/clients/acme")
	})
}
