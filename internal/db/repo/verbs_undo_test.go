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

// Owner decisions 2026-10-27: link revive (X20), tags are names on objects (X26), dismissals (D7).

func TestUnlinkThenLinkRevivesTheOldRowInEveryLinkTable(t *testing.T) {
	cases := []struct {
		name     string
		relation string
		a, b     func(f linkFixture) repo.Ref
		attrs    model.LinkAttrs
		change   model.LinkAttrs // given when linking again: only this attribute changes
	}{
		{"membership", "membership",
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "person", ID: f.person} },
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "organization", ID: f.org} },
			model.LinkAttrs{Role: sp("CTO"), StartedOn: sp("2019-03-01"), EndedOn: sp("2021-06-30"), Notes: sp("left on good terms")},
			model.LinkAttrs{Role: sp("Advisor")}},
		{"person_relation", "person_relation",
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "person", ID: f.person} },
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "person", ID: f.other} },
			model.LinkAttrs{Kind: sp("friend"), Notes: sp("college")},
			model.LinkAttrs{Notes: sp("work")}},
		{"org_relation", "org_relation",
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "organization", ID: f.org} },
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "organization", ID: f.org2} },
			model.LinkAttrs{Kind: sp("parent"), Notes: sp("holding")},
			model.LinkAttrs{Notes: sp("spun out")}},
		{"deal_person", "deal_person",
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "deal", ID: f.deal} },
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "person", ID: f.person} },
			model.LinkAttrs{Role: sp("champion")},
			model.LinkAttrs{Role: sp("blocker")}},
		{"deal_org", "deal_org",
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "deal", ID: f.deal} },
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "organization", ID: f.org} },
			model.LinkAttrs{Role: sp("buyer"), Primary: bp(true)},
			model.LinkAttrs{Role: sp("partner")}},
		{"activity_target", "task_person",
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "task", ID: f.task} },
			func(f linkFixture) repo.Ref { return repo.Ref{Kind: "person", ID: f.person} },
			model.LinkAttrs{Role: sp("owner")},
			model.LinkAttrs{Role: sp("reviewer")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupLinks(t)
			ctx := context.Background()
			a, b := tc.a(f), tc.b(f)
			first, err := f.links.Link(ctx, a, b, tc.attrs)
			require.NoError(t, err)
			require.Equal(t, tc.relation, first.Relation)
			old, err := f.links.Get(ctx, tc.relation, first.LinkID)
			require.NoError(t, err)

			_, err = f.links.Unlink(ctx, a, b)
			require.NoError(t, err)
			table := tc.relation
			if table == "task_person" {
				table = "activity_target"
			}
			rows := count(t, f.links.DB(), "SELECT COUNT(*) FROM "+table)

			// Link again with nothing new: the same row comes back with everything it had. A kinded
			// link is the same pair and kind, so the kind is named again.
			sameKind := model.LinkAttrs{Kind: tc.attrs.Kind}
			res, err := f.links.Link(ctx, a, b, sameKind)
			require.NoError(t, err)
			assert.True(t, res.Changed)
			assert.True(t, res.Revived, "revived, not inserted")
			assert.Equal(t, first.LinkID, res.LinkID)
			assert.Equal(t, rows, count(t, f.links.DB(), "SELECT COUNT(*) FROM "+table), "no new row")
			back, err := f.links.Get(ctx, tc.relation, first.LinkID)
			require.NoError(t, err)
			assert.False(t, back.Deleted)
			assert.Equal(t, old.Role, back.Role)
			assert.Equal(t, old.StartedOn, back.StartedOn)
			assert.Equal(t, old.EndedOn, back.EndedOn)
			assert.Equal(t, old.Notes, back.Notes)
			assert.Equal(t, old.KindID, back.KindID)
			assert.Equal(t, old.Primary, back.Primary)

			// Linking an already live pair is the usual no-op.
			again, err := f.links.Link(ctx, a, b, sameKind)
			require.NoError(t, err)
			assert.False(t, again.Changed)

			// Unlink, link again with one new attribute: it wins, the rest stays.
			_, err = f.links.Unlink(ctx, a, b)
			require.NoError(t, err)
			change := tc.change
			change.Kind = tc.attrs.Kind
			res, err = f.links.Link(ctx, a, b, change)
			require.NoError(t, err)
			assert.True(t, res.Revived)
			assert.Equal(t, first.LinkID, res.LinkID)
			next, err := f.links.Get(ctx, tc.relation, first.LinkID)
			require.NoError(t, err)
			if tc.change.Role != nil {
				assert.Equal(t, tc.change.Role, next.Role)
				assert.Equal(t, old.Notes, next.Notes)
			}
			if tc.change.Notes != nil {
				assert.Equal(t, tc.change.Notes, next.Notes)
				assert.Equal(t, old.KindID, next.KindID)
			}
			assert.Equal(t, old.StartedOn, next.StartedOn)
			assert.Equal(t, old.EndedOn, next.EndedOn)
		})
	}
}

func TestRevivedPrimaryMembershipDoesNotStealThePrimaryFlag(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	p := repo.Ref{Kind: "person", ID: f.person}
	o1, o2 := repo.Ref{Kind: "organization", ID: f.org}, repo.Ref{Kind: "organization", ID: f.org2}
	r1, err := f.links.Link(ctx, p, o1, model.LinkAttrs{Role: sp("CTO")})
	require.NoError(t, err)
	rec, _ := f.links.Get(ctx, "membership", r1.LinkID)
	require.True(t, rec.Primary, "the first membership is primary")
	_, err = f.links.Unlink(ctx, p, o1)
	require.NoError(t, err)
	r2, err := f.links.Link(ctx, p, o2)
	require.NoError(t, err)
	_, err = f.links.Link(ctx, p, o1)
	require.NoError(t, err)
	old, _ := f.links.Get(ctx, "membership", r1.LinkID)
	assert.False(t, old.Primary, "another link took the flag in the meantime")
	assert.Equal(t, "CTO", *old.Role)
	cur, _ := f.links.Get(ctx, "membership", r2.LinkID)
	assert.True(t, cur.Primary)

	// A person who left and came back on purpose can still get a second membership row.
	_, err = f.links.LinkNew(ctx, p, o1, model.LinkAttrs{Role: sp("Advisor")})
	require.NoError(t, err)
	assert.Equal(t, 2, count(t, f.links.DB(), "SELECT COUNT(*) FROM membership WHERE org_id = ? AND deleted_at IS NULL", f.org))
}

func TestTagsAreNamesOnObjects(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	tags := repo.NewTagRepo(d)
	ann, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Ann"})
	bob, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Bob"})
	names := func() []string {
		all, err := tags.FindAll(ctx)
		require.NoError(t, err)
		var out []string
		for _, tg := range all {
			out = append(out, tg.Name)
		}
		return out
	}
	live := func(tag string) int {
		return count(t, d, "SELECT COUNT(*) FROM taggings g JOIN tags t ON t.id = g.tag_id WHERE t.name = ? AND g.deleted_at IS NULL", tag)
	}

	t.Run("untag and tag again revives the same row", func(t *testing.T) {
		require.NoError(t, tags.Apply(ctx, "person", ann.ID, "vip"))
		var uuid1 string
		require.NoError(t, d.QueryRow("SELECT uuid FROM taggings").Scan(&uuid1))
		require.NoError(t, tags.Remove(ctx, "person", ann.ID, "vip"))
		assert.Equal(t, 1, count(t, d, "SELECT COUNT(*) FROM taggings WHERE deleted_at IS NOT NULL AND updated_at <> ''"), "soft deleted, not removed")
		assert.Empty(t, names(), "a tag with no live tagging is not listed")
		require.NoError(t, tags.Apply(ctx, "person", ann.ID, "vip"))
		var uuid2 string
		require.NoError(t, d.QueryRow("SELECT uuid FROM taggings WHERE deleted_at IS NULL").Scan(&uuid2))
		assert.Equal(t, uuid1, uuid2)
		assert.Equal(t, 1, count(t, d, "SELECT COUNT(*) FROM taggings"))
		assert.Equal(t, []string{"vip"}, names())
		got, _ := tags.GetForEntity(ctx, "person", ann.ID)
		assert.Len(t, got, 1)
		require.NoError(t, tags.Remove(ctx, "person", ann.ID, "vip"))
		assert.ErrorIs(t, tags.Remove(ctx, "person", ann.ID, "vip"), model.ErrNotFound, "an untagged object cannot be untagged again")
		assert.Empty(t, mustEntities(t, tags, "vip"))
	})

	t.Run("delete untags everything as one step and undo restores the same rows", func(t *testing.T) {
		require.NoError(t, tags.Apply(ctx, "person", ann.ID, "client"))
		require.NoError(t, tags.Apply(ctx, "person", bob.ID, "client"))
		o, _ := repo.NewOrgRepo(d).Create(ctx, model.CreateOrgInput{Name: "Acme", AllowDuplicate: true})
		require.NoError(t, tags.Apply(ctx, "organization", o.ID, "client"))
		res, err := tags.DeleteTag(ctx, "client")
		require.NoError(t, err)
		assert.Len(t, res.TaggingIDs, 3)
		assert.Equal(t, map[string]int{"person": 2, "organization": 1}, res.Removed)
		assert.Zero(t, live("client"))
		assert.NotContains(t, names(), "client")
		assert.Equal(t, 1, count(t, d, "SELECT COUNT(*) FROM tags WHERE name = 'client'"), "no tag record is deleted")
		assert.Equal(t, 3, count(t, d, "SELECT COUNT(*) FROM taggings WHERE tag_id = (SELECT id FROM tags WHERE name = 'client') AND deleted_at IS NOT NULL AND uuid <> ''"))
		assert.ErrorIs(t, tags.Delete(ctx, "client"), model.ErrNotFound)

		require.NoError(t, tags.UndoDelete(ctx, res))
		assert.Equal(t, 3, live("client"))
		assert.Contains(t, names(), "client")
		require.NoError(t, tags.RedoDelete(ctx, res))
		assert.Zero(t, live("client"))
		require.NoError(t, tags.UndoDelete(ctx, res))

		// A tagging made again in the meantime is not doubled by the undo.
		require.NoError(t, tags.Delete(ctx, "client"))
		require.NoError(t, tags.Apply(ctx, "person", ann.ID, "client"))
		res2 := &repo.TagDeleteResult{Tag: "client", TaggingIDs: res.TaggingIDs}
		require.NoError(t, tags.UndoDelete(ctx, res2))
		assert.Equal(t, 3, live("client"))
	})

	t.Run("rename and merge keep working", func(t *testing.T) {
		require.NoError(t, tags.Apply(ctx, "person", ann.ID, "old"))
		require.NoError(t, tags.Apply(ctx, "person", bob.ID, "old"))
		require.NoError(t, tags.Apply(ctx, "person", bob.ID, "new"))
		require.NoError(t, tags.Rename(ctx, "old", "older"))
		assert.Equal(t, 2, live("older"))

		// A soft deleted tagging of the source does not break the merge, and the tagging rows that
		// leave for good get tombstones so a sync sees them.
		require.NoError(t, tags.Remove(ctx, "person", ann.ID, "older"))
		res, err := tags.Merge(ctx, "older", "new")
		require.NoError(t, err)
		assert.Equal(t, 1, res.Duplicates["person"], "bob already had new")
		assert.Equal(t, 1, live("new"))
		assert.Equal(t, 0, count(t, d, "SELECT COUNT(*) FROM tags WHERE name = 'older'"))
		assert.Equal(t, 2, count(t, d, "SELECT COUNT(*) FROM tombstones WHERE table_name = 'taggings'"), "the dead row and the duplicate left with the source tag")

		require.NoError(t, tags.UndoMerge(ctx, res))
		assert.Equal(t, 1, live("older"))
		assert.Equal(t, 1, live("new"))
	})
}

func mustEntities(t *testing.T, tags *repo.TagRepo, tag string) []int64 {
	t.Helper()
	ids, err := tags.GetEntities(context.Background(), tag, "person")
	require.NoError(t, err)
	return ids
}

func TestPurgeWritesTaggingTombstones(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	p, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Gone"})
	require.NoError(t, repo.NewTagRepo(d).Apply(ctx, "person", p.ID, "x"))
	require.NoError(t, pr.Archive(ctx, p.ID))
	require.NoError(t, pr.Purge(ctx, p.ID))
	assert.Equal(t, 1, count(t, d, "SELECT COUNT(*) FROM tombstones WHERE table_name = 'taggings'"))
}

func TestOrganizationCandidatesCanBeDismissedAndRestored(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	ctx := context.Background()
	orgs := repo.NewOrgRepo(d)
	var ids []int64
	for _, n := range []string{"Blue Origin", "BLUE ORIGIN", "Other Co", "Other Co."} {
		o, err := orgs.Create(ctx, model.CreateOrgInput{Name: n, AllowDuplicate: true})
		require.NoError(t, err)
		ids = append(ids, o.ID)
	}
	cands, err := orgs.FindOrgCandidates(ctx, 0)
	require.NoError(t, err)
	require.Len(t, cands, 2)

	tests := []struct {
		name string
		do   func() error
		want int
	}{
		{"dismiss one pair", func() error { _, err := orgs.DismissOrgPair(ctx, ids[1], ids[0]); return err }, 1}, // either order
		{"dismissing twice changes nothing", func() error {
			changed, err := orgs.DismissOrgPair(ctx, ids[0], ids[1])
			assert.False(t, changed)
			return err
		}, 1},
		{"undismiss brings it back", func() error { return orgs.UndismissOrgPair(ctx, ids[0], ids[1]) }, 2},
		{"dismiss again revives the row", func() error { _, err := orgs.DismissOrgPair(ctx, ids[0], ids[1]); return err }, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.do())
			got, err := orgs.FindOrgCandidates(ctx, 0)
			require.NoError(t, err)
			assert.Len(t, got, tc.want)
		})
	}
	assert.Equal(t, 1, count(t, d, "SELECT COUNT(*) FROM org_dupe_dismissals"), "one row, soft deleted and revived")
	dismissed, err := orgs.DismissedOrgCandidates(ctx)
	require.NoError(t, err)
	require.Len(t, dismissed, 1)
	assert.Equal(t, ids[0], dismissed[0].AID)

	_, err = orgs.DismissOrgPair(ctx, ids[0], ids[0])
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = orgs.DismissOrgPair(ctx, ids[0], 999)
	assert.ErrorIs(t, err, model.ErrNotFound)
	assert.ErrorIs(t, orgs.UndismissOrgPair(ctx, ids[2], ids[3]), model.ErrNotFound)
}
