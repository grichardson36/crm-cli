package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f *omFx) personAt(t *testing.T, first, company, title string) int64 {
	t.Helper()
	in := model.CreatePersonInput{FirstName: first}
	if company != "" {
		in.Company = &company
	}
	if title != "" {
		in.Title = &title
	}
	p, err := f.people.Create(context.Background(), in)
	require.NoError(t, err)
	return p.ID
}

func (f *omFx) liveOrgs(t *testing.T) int {
	return f.count(t, `SELECT COUNT(*) FROM organizations WHERE archived = 0`)
}

func (f *omFx) primaryOrgName(t *testing.T, personID int64) string {
	t.Helper()
	name, err := repo.PrimaryOrgName(context.Background(), f.d, personID)
	require.NoError(t, err)
	return name
}

func TestApplyCompanyCreateReuseIgnore(t *testing.T) {
	tests := []struct {
		name        string
		existing    string // an organization that already exists
		text        string
		wantCreated bool
		wantIgnored bool
		wantOrgs    int // live organizations afterwards
		wantPrimary string
	}{
		{name: "new text creates", text: "Acme Corp", wantCreated: true, wantOrgs: 1, wantPrimary: "Acme Corp"},
		{name: "exact name reuses", existing: "Acme Corp", text: "Acme Corp", wantOrgs: 1, wantPrimary: "Acme Corp"},
		{name: "case and punctuation reuse", existing: "Acme Corp", text: "ACME corp.", wantOrgs: 1, wantPrimary: "Acme Corp"},
		{name: "legal ending reuses", existing: "Acme", text: "Acme, Inc.", wantOrgs: 1, wantPrimary: "Acme"},
		{name: "ampersand reuses", existing: "Smith & Sons", text: "Smith and Sons LLC", wantOrgs: 1, wantPrimary: "Smith & Sons"},
		{name: "typo does not merge", existing: "Acme Corp", text: "Acme Cop", wantCreated: true, wantOrgs: 2, wantPrimary: "Acme Cop"},
		{name: "variant beyond normalization does not merge", existing: "Acme", text: "Acme Holdings", wantCreated: true, wantOrgs: 2, wantPrimary: "Acme Holdings"},
		{name: "N/A ignored", text: "N/A", wantIgnored: true},
		{name: "none ignored", text: "none", wantIgnored: true},
		{name: "self-employed ignored", text: "Self-Employed", wantIgnored: true},
		{name: "retired ignored", text: " retired ", wantIgnored: true},
		{name: "dash ignored", text: "-", wantIgnored: true},
		{name: "punctuation only ignored", text: "...", wantIgnored: true},
		{name: "empty does nothing", text: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := omSetup(t)
			ctx := context.Background()
			oa := repo.NewOrgAutoRepo(f.d)
			if tt.existing != "" {
				f.org(t, tt.existing, "")
			}
			pid := f.personAt(t, "Pat", tt.text, "")
			ch, err := oa.ApplyCompany(ctx, pid, tt.text, repo.ModePrimary)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, ch.OrgCreated)
			assert.Equal(t, tt.wantIgnored, ch.Ignored)
			assert.Equal(t, tt.wantOrgs, f.liveOrgs(t))
			assert.Equal(t, tt.wantPrimary, f.primaryOrgName(t, pid))
			assert.Equal(t, tt.text, *mustPerson(t, f, pid).Company, "people.company stays the source text")
		})
	}
}

func mustPerson(t *testing.T, f *omFx, id int64) *model.Person {
	t.Helper()
	p, err := f.people.FindByID(context.Background(), id)
	require.NoError(t, err)
	return p
}

func TestApplyCompanyUsesMergeAlias(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	keep := f.org(t, "Acme Holdings", "")
	drop := f.org(t, "Acme", "")
	_, err := f.orgs.MergeOrgs(ctx, keep, []int64{drop})
	require.NoError(t, err)
	pid := f.personAt(t, "Pat", "Acme", "")
	ch, err := repo.NewOrgAutoRepo(f.d).ApplyCompany(ctx, pid, "Acme", repo.ModePrimary)
	require.NoError(t, err)
	assert.False(t, ch.OrgCreated, "the merged-away name resolves to the survivor")
	assert.Equal(t, keep, ch.OrgID)
	assert.Equal(t, 1, f.liveOrgs(t))
}

func TestApplyCompanyModes(t *testing.T) {
	setup := func(t *testing.T) (*omFx, int64, int64) {
		f := omSetup(t)
		old := f.org(t, "Old Co", "")
		pid := f.personAt(t, "Pat", "New Co", "CTO")
		f.link(t, repo.Ref{Kind: "person", ID: pid}, repo.Ref{Kind: "organization", ID: old}, model.LinkAttrs{})
		return f, pid, old
	}
	t.Run("primary makes the new organization primary and keeps the old membership", func(t *testing.T) {
		f, pid, old := setup(t)
		ch, err := repo.NewOrgAutoRepo(f.d).ApplyCompany(context.Background(), pid, "New Co", repo.ModePrimary)
		require.NoError(t, err)
		assert.True(t, ch.OrgCreated)
		assert.Equal(t, "New Co", f.primaryOrgName(t, pid))
		assert.Equal(t, 2, f.count(t, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND deleted_at IS NULL`, pid))
		assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND org_id = ? AND is_primary = 0 AND deleted_at IS NULL`, pid, old))
		assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND role = 'CTO'`, pid), "the title is the role of the new membership")
	})
	t.Run("repoint moves the primary in place", func(t *testing.T) {
		f, pid, _ := setup(t)
		_, err := repo.NewOrgAutoRepo(f.d).ApplyCompany(context.Background(), pid, "New Co", repo.ModeRepoint)
		require.NoError(t, err)
		assert.Equal(t, "New Co", f.primaryOrgName(t, pid))
		assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND deleted_at IS NULL`, pid))
	})
	t.Run("add never moves the primary", func(t *testing.T) {
		f, pid, _ := setup(t)
		_, err := repo.NewOrgAutoRepo(f.d).ApplyCompany(context.Background(), pid, "New Co", repo.ModeAdd)
		require.NoError(t, err)
		assert.Equal(t, "Old Co", f.primaryOrgName(t, pid))
		assert.Equal(t, 2, f.count(t, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND deleted_at IS NULL`, pid))
	})
}

// Undo and redo of one creation are one step, in every mode.
func TestApplyCompanyUndoRedo(t *testing.T) {
	for _, mode := range []string{repo.ModePrimary, repo.ModeRepoint, repo.ModeAdd} {
		t.Run(mode, func(t *testing.T) {
			f := omSetup(t)
			ctx := context.Background()
			oa := repo.NewOrgAutoRepo(f.d)
			old := f.org(t, "Old Co", "")
			pid := f.personAt(t, "Pat", "New Co", "")
			f.link(t, repo.Ref{Kind: "person", ID: pid}, repo.Ref{Kind: "organization", ID: old}, model.LinkAttrs{})
			ch, err := oa.ApplyCompany(ctx, pid, "New Co", mode)
			require.NoError(t, err)
			require.False(t, ch.Empty())
			after := f.primaryOrgName(t, pid)

			require.NoError(t, oa.Undo(ctx, ch))
			assert.Equal(t, 1, f.liveOrgs(t), "the created organization is archived")
			assert.Equal(t, "Old Co", f.primaryOrgName(t, pid), "the old primary is back")
			assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND deleted_at IS NULL`, pid))

			require.NoError(t, oa.Redo(ctx, ch))
			assert.Equal(t, 2, f.liveOrgs(t))
			assert.Equal(t, after, f.primaryOrgName(t, pid))
		})
	}
}

func TestIgnoreListEditable(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	ig := repo.NewIgnoreRepo(f.d)
	oa := repo.NewOrgAutoRepo(f.d)
	apply := func(text string) repo.CompanyChange {
		pid := f.personAt(t, "P"+text, text, "")
		ch, err := oa.ApplyCompany(ctx, pid, text, repo.ModePrimary)
		require.NoError(t, err)
		return ch
	}

	list, err := ig.List(ctx)
	require.NoError(t, err)
	var texts []string
	for _, e := range list {
		texts = append(texts, e.Text)
		assert.True(t, e.Builtin)
		assert.True(t, e.Active)
	}
	for _, want := range []string{"n/a", "none", "self-employed", "retired", "-", "unemployed"} {
		assert.Contains(t, texts, want)
	}

	// A user addition.
	changed, err := ig.Add(ctx, "Between Things")
	require.NoError(t, err)
	assert.True(t, changed)
	changed, err = ig.Add(ctx, "between things")
	require.NoError(t, err)
	assert.False(t, changed, "already ignored")
	assert.True(t, apply("BETWEEN THINGS").Ignored)

	// Turning off a built-in makes that text create an organization.
	require.NoError(t, ig.Remove(ctx, "Retired"))
	ch := apply("Retired")
	assert.False(t, ch.Ignored)
	assert.True(t, ch.OrgCreated)
	list, err = ig.List(ctx)
	require.NoError(t, err)
	for _, e := range list {
		if e.Key == "retired" {
			assert.False(t, e.Active)
		}
	}
	// And back on.
	changed, err = ig.Add(ctx, "retired")
	require.NoError(t, err)
	assert.True(t, changed)
	assert.True(t, apply("retired ").Ignored)

	// Removing an addition deletes it. Removing an unknown text is not found.
	require.NoError(t, ig.Remove(ctx, "between things"))
	assert.False(t, apply("Between Things").Ignored)
	assert.ErrorIs(t, ig.Remove(ctx, "never listed"), model.ErrNotFound)
	_, err = ig.Add(ctx, "  ")
	assert.ErrorIs(t, err, model.ErrValidation)
}

// backfillFixture: Acme x3 (two spellings), Globex x1 existing, junk x2, Zeta x1, one person already linked.
func backfillFixture(t *testing.T) (*omFx, map[string]int64) {
	f := omSetup(t)
	ids := map[string]int64{}
	for _, p := range []struct{ first, company, title string }{
		{"A1", "Acme Inc", "CEO"}, {"A2", "Acme Inc", ""}, {"A3", "acme, inc.", "Dev"},
		{"G1", "Globex", ""}, {"Z1", "Zeta", ""},
		{"J1", "N/A", ""}, {"J2", "retired", ""},
		{"Empty", "", ""},
	} {
		ids[p.first] = f.personAt(t, p.first, p.company, p.title)
	}
	globex := f.org(t, "Globex", "")
	f.link(t, repo.Ref{Kind: "person", ID: ids["G1"]}, repo.Ref{Kind: "organization", ID: globex}, model.LinkAttrs{})
	return f, ids
}

func TestBackfillDryRunWritesNothing(t *testing.T) {
	f, _ := backfillFixture(t)
	ctx := context.Background()
	orgsBefore := f.liveOrgs(t)
	membersBefore := f.count(t, `SELECT COUNT(*) FROM membership`)

	rep, err := repo.NewOrgAutoRepo(f.d).BackfillPlan(ctx)
	require.NoError(t, err)
	assert.Equal(t, 7, rep.PeopleWithCompany)
	assert.Equal(t, 5, rep.Groups, "acme, globex, zeta, n/a, retired")
	assert.Equal(t, 2, rep.OrgsToCreate, "acme and zeta")
	assert.Equal(t, 1, rep.OrgsReused, "globex")
	assert.Equal(t, 2, rep.IgnoredPeople)
	assert.Equal(t, 2, rep.IgnoredGroups)
	assert.Equal(t, 4, rep.PeopleToLink, "3 acme + zeta")
	assert.Equal(t, 1, rep.AlreadyLinked, "globex person")
	assert.Equal(t, 1, rep.GroupsOf3Plus)
	require.NotEmpty(t, rep.List)
	assert.Equal(t, "acme", rep.List[0].Key, "groups of 3 or more come first")
	assert.Equal(t, 3, rep.List[0].People)
	assert.Len(t, rep.List[0].Variants, 2, "two spellings of acme")

	assert.Equal(t, orgsBefore, f.liveOrgs(t))
	assert.Equal(t, membersBefore, f.count(t, `SELECT COUNT(*) FROM membership`))
}

func TestBackfillApplyIsOneBatchAndIdempotent(t *testing.T) {
	f, ids := backfillFixture(t)
	ctx := context.Background()
	oa := repo.NewOrgAutoRepo(f.d)

	_, ch, err := oa.BackfillApply(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, ch.OrgsCreated)
	assert.Equal(t, 4, ch.Linked)
	assert.Equal(t, 3, f.liveOrgs(t), "globex, acme, zeta: no junk organization")
	for _, name := range []string{"A1", "A2", "A3"} {
		assert.Equal(t, "Acme Inc", f.primaryOrgName(t, ids[name]), "the most common spelling names the organization")
	}
	assert.Equal(t, "Globex", f.primaryOrgName(t, ids["G1"]))
	assert.Equal(t, "", f.primaryOrgName(t, ids["J1"]))
	assert.Equal(t, "", f.primaryOrgName(t, ids["Empty"]))
	assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND role = 'CEO'`, ids["A1"]), "the title is the role")
	assert.Equal(t, "acme, inc.", *mustPerson(t, f, ids["A3"]).Company, "people.company is untouched")

	// Dry run now: nothing left to create or link.
	rep, err := oa.BackfillPlan(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, rep.OrgsToCreate)
	assert.Equal(t, 0, rep.PeopleToLink)
	assert.Equal(t, 3, rep.OrgsReused)

	// A second apply changes nothing.
	members := f.count(t, `SELECT COUNT(*) FROM membership`)
	_, ch2, err := oa.BackfillApply(ctx)
	require.NoError(t, err)
	assert.True(t, ch2.Empty())
	assert.Equal(t, 3, f.liveOrgs(t))
	assert.Equal(t, members, f.count(t, `SELECT COUNT(*) FROM membership`))
}

func TestBackfillUndoRedoIsOneStep(t *testing.T) {
	f, ids := backfillFixture(t)
	ctx := context.Background()
	oa := repo.NewOrgAutoRepo(f.d)
	members := f.count(t, `SELECT COUNT(*) FROM membership WHERE deleted_at IS NULL`)

	_, ch, err := oa.BackfillApply(ctx)
	require.NoError(t, err)
	require.False(t, ch.Empty())

	require.NoError(t, oa.UndoBackfill(ctx, ch))
	assert.Equal(t, 1, f.liveOrgs(t), "only the organization that existed before")
	assert.Equal(t, members, f.count(t, `SELECT COUNT(*) FROM membership WHERE deleted_at IS NULL`))
	assert.Equal(t, "Globex", f.primaryOrgName(t, ids["G1"]))
	assert.Equal(t, "", f.primaryOrgName(t, ids["A1"]))

	require.NoError(t, oa.RedoBackfill(ctx, ch))
	assert.Equal(t, 3, f.liveOrgs(t))
	assert.Equal(t, "Acme Inc", f.primaryOrgName(t, ids["A2"]))
	assert.Equal(t, "Zeta", f.primaryOrgName(t, ids["Z1"]))
}

func TestBackfillKeepsAnExistingPrimaryAndRespectsTheIgnoreList(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	oa := repo.NewOrgAutoRepo(f.d)
	old := f.org(t, "Old Co", "")
	pid := f.personAt(t, "Pat", "New Co", "")
	f.link(t, repo.Ref{Kind: "person", ID: pid}, repo.Ref{Kind: "organization", ID: old}, model.LinkAttrs{})
	f.personAt(t, "Quinn", "Gig Work", "")
	require.NoError(t, func() error { _, err := repo.NewIgnoreRepo(f.d).Add(ctx, "gig work"); return err }())

	_, _, err := oa.BackfillApply(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Old Co", f.primaryOrgName(t, pid), "the backfill never moves a primary membership")
	assert.Equal(t, 2, f.count(t, `SELECT COUNT(*) FROM membership WHERE person_id = ? AND deleted_at IS NULL`, pid))
	assert.Equal(t, 0, f.count(t, `SELECT COUNT(*) FROM organizations WHERE name = 'Gig Work'`))
}

func TestOrgCandidatesSurfaceNearDuplicatesButNeverMerge(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	acme := f.org(t, "Acme Corp", "")
	typo := f.org(t, "Acme Cop", "")
	holdings := f.org(t, "Acme Corp Holdings", "")
	f.org(t, "Globex", "")
	d1 := f.org(t, "Umbrella", "umbrella.com")
	d2 := f.org(t, "Hooli", "www.umbrella.com")

	c, err := f.orgs.FindOrgCandidates(ctx, 0)
	require.NoError(t, err)
	pairs := map[[2]int64]string{}
	for _, x := range c {
		pairs[[2]int64{x.AID, x.BID}] = x.Reason
	}
	assert.Contains(t, pairs, [2]int64{acme, typo})
	assert.Contains(t, pairs, [2]int64{acme, holdings})
	assert.Equal(t, "same domain umbrella.com", pairs[[2]int64{d1, d2}])
	assert.Len(t, pairs, 3, "Globex matches nothing")
	assert.Equal(t, 6, f.liveOrgs(t), "finding candidates merges nothing")

	// Company text with a near name creates a separate organization and reports the neighbours.
	pid := f.personAt(t, "Pat", "Acme Corp.", "")
	ch, err := repo.NewOrgAutoRepo(f.d).ApplyCompany(ctx, pid, "Acme Corp.", repo.ModePrimary)
	require.NoError(t, err)
	assert.False(t, ch.OrgCreated, "same normalized name reuses")
	pid = f.personAt(t, "Quinn", "Acme Corpp", "")
	ch, err = repo.NewOrgAutoRepo(f.d).ApplyCompany(ctx, pid, "Acme Corpp", repo.ModePrimary)
	require.NoError(t, err)
	assert.True(t, ch.OrgCreated)
	assert.NotEmpty(t, ch.Similar, "the new organization lists its near neighbours as merge candidates")
}
