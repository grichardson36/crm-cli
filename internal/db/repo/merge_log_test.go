package repo_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveState dumps the live rows of every table a merge can touch, without ids of time columns, so
// two dumps can be compared for "the same data".
func liveState(t *testing.T, d *sql.DB) string {
	t.Helper()
	var b strings.Builder
	for _, tbl := range []string{"people", "person_emails", "person_phones", "person_handles", "custom_fields", "taggings", "membership",
		"person_relation", "deal_person", "activity_target", "merge_alias", "dupe_dismissals", "organizations", "org_relation", "deal_org"} {
		cols, err := d.Query("SELECT name FROM pragma_table_info('" + tbl + "') WHERE name NOT IN ('updated_at', 'uuid', 'created_at', 'deleted_at')")
		require.NoError(t, err)
		var names []string
		for cols.Next() {
			var n string
			require.NoError(t, cols.Scan(&n))
			names = append(names, n)
		}
		require.NoError(t, cols.Close())
		where := ""
		if count(t, d, "SELECT COUNT(*) FROM pragma_table_info('"+tbl+"') WHERE name = 'deleted_at'") > 0 {
			where = " WHERE deleted_at IS NULL"
		}
		rows, err := d.Query("SELECT " + strings.Join(names, ", ") + " FROM " + tbl + where + " ORDER BY rowid")
		require.NoError(t, err)
		vals := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		fmt.Fprintf(&b, "[%s]\n", tbl)
		for rows.Next() {
			require.NoError(t, rows.Scan(ptrs...))
			fmt.Fprintln(&b, vals...)
		}
		require.NoError(t, rows.Close())
	}
	return b.String()
}

func TestMergeLog_PersonMergeUndoRedo(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	keep, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith"), Email: ptr("jane@work.com"), Title: ptr("CEO")})
	require.NoError(t, err)
	drop, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet", LastName: ptr("Smith"), Email: ptr("jane@home.com"), Title: ptr("CTO"), Company: ptr("Acme")})
	require.NoError(t, err)
	third, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Other"})
	require.NoError(t, err)
	require.NoError(t, pr.SetHandle(ctx, drop.ID, "twitter", "janet"))
	tags := repo.NewTagRepo(d)
	require.NoError(t, tags.Apply(ctx, "person", drop.ID, "friend"))
	require.NoError(t, tags.Apply(ctx, "person", keep.ID, "vip"))
	orgs := repo.NewOrgRepo(d)
	o, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Acme", AllowDuplicate: true})
	require.NoError(t, err)
	links := repo.NewLinkRepo(d)
	_, err = links.Link(ctx, repo.Ref{Kind: "person", ID: drop.ID}, repo.Ref{Kind: "organization", ID: o.ID}, model.LinkAttrs{Role: ptr("CTO")})
	require.NoError(t, err)
	_, err = links.Link(ctx, repo.Ref{Kind: "person", ID: drop.ID}, repo.Ref{Kind: "person", ID: third.ID}, model.LinkAttrs{Kind: ptr("friend")})
	require.NoError(t, err)
	addTask(t, d, "t1", drop.ID)
	addDeal(t, d, "d1", drop.ID, "2000-01-01 00:00:00")
	require.NoError(t, repo.NewDupeRepo(d).Dismiss(ctx, drop.ID, third.ID))

	before := liveState(t, d)
	res, err := pr.Merge(ctx, keep.ID, []int64{drop.ID}, map[string]int64{"title": drop.ID})
	require.NoError(t, err)
	require.NotZero(t, res.LogID, "an applied merge is logged")
	after := liveState(t, d)
	require.NotEqual(t, before, after)
	assert.Equal(t, 1, count(t, d, "SELECT archived FROM people WHERE id = ?", drop.ID))
	assert.Equal(t, 1, count(t, d, "SELECT COUNT(*) FROM merge_alias WHERE kind = 'person' AND old_id = ? AND deleted_at IS NULL", drop.ID))

	// A dry run writes no log entry.
	dry, err := pr.MergeDryRun(ctx, keep.ID, []int64{third.ID}, nil)
	require.NoError(t, err)
	assert.Zero(t, dry.LogID)
	assert.Equal(t, 1, count(t, d, "SELECT COUNT(*) FROM merge_log"))

	logs := repo.NewMergeLogRepo(d)
	list, err := logs.List(ctx, "", 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "person", list[0].Kind)
	assert.Equal(t, keep.ID, list[0].SurvivorID)
	assert.Equal(t, []int64{drop.ID}, list[0].LoserIDs)
	assert.Equal(t, "merged", list[0].Status)

	undo, err := logs.Undo(ctx, res.LogID)
	require.NoError(t, err)
	assert.Zero(t, undo.Skipped)
	assert.Equal(t, before, liveState(t, d), "undo puts back the loser, its links, the dismissal and removes the alias")
	assert.Equal(t, 0, count(t, d, "SELECT archived FROM people WHERE id = ?", drop.ID))
	assert.Equal(t, 0, count(t, d, "SELECT COUNT(*) FROM merge_alias WHERE deleted_at IS NULL"))

	_, err = logs.Undo(ctx, res.LogID)
	assert.ErrorIs(t, err, model.ErrConflict, "undoing twice is refused")

	_, err = logs.Redo(ctx, res.LogID)
	require.NoError(t, err)
	assert.Equal(t, after, liveState(t, d), "redo repeats the merge exactly")
	_, err = logs.Redo(ctx, res.LogID)
	assert.ErrorIs(t, err, model.ErrConflict)

	e, err := logs.Get(ctx, res.LogID)
	require.NoError(t, err)
	assert.Equal(t, "merged", e.Status)
	_, err = logs.Get(ctx, 999)
	assert.ErrorIs(t, err, model.ErrNotFound)
}

func TestMergeLog_UndoWorksInALaterSessionAndKeepsLaterEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crm.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	pr := repo.NewPersonRepo(d)
	ctx := context.Background()
	keep, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Title: ptr("CEO")})
	drop, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet", Title: ptr("CTO"), Location: ptr("Paris")})
	res, err := pr.Merge(ctx, keep.ID, []int64{drop.ID}, map[string]int64{"title": drop.ID})
	require.NoError(t, err)
	require.NoError(t, d.Close())

	// Later session: a new connection, the title edited again after the merge.
	d, err = db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	_, err = d.Exec("UPDATE people SET title = 'Founder' WHERE id = ?", keep.ID)
	require.NoError(t, err)
	undo, err := repo.NewMergeLogRepo(d).Undo(ctx, res.LogID)
	require.NoError(t, err)
	assert.Equal(t, 1, undo.Skipped, "the title was edited after the merge and is kept")
	var title, loc sql.NullString
	require.NoError(t, d.QueryRow("SELECT title, location FROM people WHERE id = ?", keep.ID).Scan(&title, &loc))
	assert.Equal(t, "Founder", title.String)
	assert.False(t, loc.Valid, "the location the merge copied is removed again")
	assert.Equal(t, 0, count(t, d, "SELECT archived FROM people WHERE id = ?", drop.ID))
}

func TestMergeLog_UndoRefusesWhenTheLoserWasChangedByHand(t *testing.T) {
	d, pr := openMergeDB(t)
	ctx := context.Background()
	keep, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	drop, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet"})
	res, err := pr.Merge(ctx, keep.ID, []int64{drop.ID}, nil)
	require.NoError(t, err)
	require.NoError(t, pr.Restore(ctx, drop.ID))
	_, err = repo.NewMergeLogRepo(d).Undo(ctx, res.LogID)
	assert.ErrorIs(t, err, model.ErrConflict)
}

func TestMergeLog_OrganizationMergeUndoRedo(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	keep := f.org(t, "Acme", "")
	drop := f.org(t, "Acme Inc", "acme.com")
	ann, bob := f.person(t, "Ann"), f.person(t, "Bob")
	f.link(t, pref(ann), oref(drop), model.LinkAttrs{Role: ptr("CTO")})
	f.link(t, pref(bob), oref(keep), model.LinkAttrs{})
	require.NoError(t, f.tags.Apply(ctx, "organization", drop, "client"))
	other := f.org(t, "Globex", "")
	f.link(t, oref(drop), oref(other), model.LinkAttrs{Kind: ptr("parent")})

	before := liveState(t, f.d)
	res, err := f.orgs.MergeOrgs(ctx, keep, []int64{drop})
	require.NoError(t, err)
	require.NotZero(t, res.LogID)
	after := liveState(t, f.d)
	require.NotEqual(t, before, after)

	logs := repo.NewMergeLogRepo(f.d)
	list, err := logs.List(ctx, "organization", 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Acme", list[0].SurvivorName)
	assert.Equal(t, []string{"Acme Inc"}, list[0].LoserNames)

	_, err = logs.Undo(ctx, res.LogID)
	require.NoError(t, err)
	assert.Equal(t, before, liveState(t, f.d), "domain filled in, memberships, parent link, tag and alias all go back")
	_, err = logs.Redo(ctx, res.LogID)
	require.NoError(t, err)
	assert.Equal(t, after, liveState(t, f.d))

	// The older entry points go through the log too.
	require.NoError(t, f.orgs.UndoMerge(ctx, res))
	assert.Equal(t, before, liveState(t, f.d))
	require.NoError(t, f.orgs.RedoMerge(ctx, res))
	assert.Equal(t, after, liveState(t, f.d))
}
