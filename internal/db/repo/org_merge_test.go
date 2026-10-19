package repo_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type omFx struct {
	d      *sql.DB
	orgs   *repo.OrgRepo
	people *repo.PersonRepo
	links  *repo.LinkRepo
	tags   *repo.TagRepo
}

func omSetup(t *testing.T) *omFx {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return &omFx{d: d, orgs: repo.NewOrgRepo(d), people: repo.NewPersonRepo(d), links: repo.NewLinkRepo(d), tags: repo.NewTagRepo(d)}
}

func (f *omFx) org(t *testing.T, name string, domain string) int64 {
	t.Helper()
	in := model.CreateOrgInput{Name: name, AllowDuplicate: true}
	if domain != "" {
		in.Domain = &domain
	}
	o, err := f.orgs.Create(context.Background(), in)
	require.NoError(t, err)
	return o.ID
}

func (f *omFx) person(t *testing.T, name string) int64 {
	t.Helper()
	p, err := f.people.Create(context.Background(), model.CreatePersonInput{FirstName: name})
	require.NoError(t, err)
	return p.ID
}

func (f *omFx) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.d.QueryRow(q, args...).Scan(&n))
	return n
}

func (f *omFx) link(t *testing.T, a, b repo.Ref, attrs model.LinkAttrs) {
	t.Helper()
	_, err := f.links.Link(context.Background(), a, b, attrs)
	require.NoError(t, err)
}

func oref(id int64) repo.Ref { return repo.Ref{Kind: "organization", ID: id} }
func pref(id int64) repo.Ref { return repo.Ref{Kind: "person", ID: id} }

func TestCreateRefusesADuplicateNameOrDomain(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	id := f.org(t, "Blue Origin", "blueorigin.com")

	for _, name := range []string{"BLUE ORIGIN", "blue origin, Inc.", "The Blue Origin LLC"} {
		_, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: name})
		var dup *repo.DuplicateOrgError
		require.ErrorAs(t, err, &dup, name)
		assert.Equal(t, id, dup.Existing.ID)
		assert.Equal(t, "name", dup.Reason)
		assert.True(t, errors.Is(err, model.ErrConflict))
		assert.Contains(t, err.Error(), "Blue Origin")
		assert.Contains(t, err.Error(), "--allow-duplicate")
	}
	d := "https://www.BlueOrigin.com/about"
	_, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Other Name", Domain: &d})
	var dup *repo.DuplicateOrgError
	require.ErrorAs(t, err, &dup)
	assert.Equal(t, "domain", dup.Reason)

	_, err = f.orgs.Create(ctx, model.CreateOrgInput{Name: "BLUE ORIGIN", AllowDuplicate: true})
	require.NoError(t, err, "allow duplicate creates it")

	// An archived organization is not a duplicate.
	gone := f.org(t, "Old Co", "")
	require.NoError(t, f.orgs.Archive(ctx, gone))
	_, err = f.orgs.Create(ctx, model.CreateOrgInput{Name: "old co"})
	require.NoError(t, err)
}

func TestMergeOrgsMovesEverythingAndUndoesInOneStep(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	keep := f.org(t, "BLUE ORIGIN", "")
	drop := f.org(t, "Blue Origin", "blueorigin.com")
	parent := f.org(t, "Holding", "")
	child := f.org(t, "Launch Unit", "")
	industry := "Space"
	_, err := f.orgs.Update(ctx, drop, model.UpdateOrgInput{Industry: &industry})
	require.NoError(t, err)

	p1, p2, p3 := f.person(t, "A"), f.person(t, "B"), f.person(t, "C")
	yes := true
	cto, eng, adv := "CTO", "Eng", "Advisor"
	f.link(t, pref(p1), oref(drop), model.LinkAttrs{Role: &cto, Primary: &yes})
	f.link(t, pref(p2), oref(keep), model.LinkAttrs{Role: &eng})
	f.link(t, pref(p2), oref(drop), model.LinkAttrs{Role: &eng, Primary: &yes}) // exact duplicate of the row above
	f.link(t, pref(p3), oref(keep), model.LinkAttrs{Role: &cto})
	f.link(t, pref(p3), oref(drop), model.LinkAttrs{Role: &adv, Primary: &yes}) // other role: both stay

	f.link(t, oref(parent), oref(drop), model.LinkAttrs{})
	f.link(t, oref(drop), oref(child), model.LinkAttrs{})
	f.link(t, oref(parent), oref(keep), model.LinkAttrs{}) // same as the moved parent link: duplicate dropped

	deal, err := repo.NewDealRepo(f.d).Create(ctx, model.CreateDealInput{Title: "Launch", Stage: "lead"})
	require.NoError(t, err)
	f.link(t, repo.Ref{Kind: "deal", ID: deal.ID}, oref(drop), model.LinkAttrs{Primary: &yes})
	task, err := repo.NewTaskRepo(f.d).Create(ctx, model.CreateTaskInput{Title: "Call", Priority: "medium"})
	require.NoError(t, err)
	f.link(t, repo.Ref{Kind: "task", ID: task.ID}, oref(drop), model.LinkAttrs{})
	require.NoError(t, f.tags.Apply(ctx, "organization", drop, "space"))
	require.NoError(t, f.tags.Apply(ctx, "organization", drop, "both"))
	require.NoError(t, f.tags.Apply(ctx, "organization", keep, "both"))

	// Preview writes nothing.
	pre, err := f.orgs.MergeOrgsDryRun(ctx, keep, []int64{drop})
	require.NoError(t, err)
	assert.True(t, pre.DryRun)
	assert.Equal(t, 0, f.count(t, `SELECT COUNT(*) FROM organizations WHERE archived = 1`))
	assert.Equal(t, 3, f.count(t, `SELECT COUNT(*) FROM membership WHERE org_id = ?`, drop))

	res, err := f.orgs.MergeOrgs(ctx, keep, []int64{drop})
	require.NoError(t, err)
	assert.Equal(t, []string{"Blue Origin"}, res.DropNames)

	o, err := f.orgs.FindByID(ctx, keep)
	require.NoError(t, err)
	assert.Equal(t, "BLUE ORIGIN", o.Name, "the survivor keeps its name")
	assert.Equal(t, "blueorigin.com", *o.Domain, "an empty field is filled")
	assert.Equal(t, "Space", *o.Industry)
	_, err = f.orgs.FindByID(ctx, drop)
	assert.ErrorIs(t, err, model.ErrNotFound, "the duplicate is archived")

	live := func(q string, args ...any) int { return f.count(t, q+" AND deleted_at IS NULL", args...) }
	assert.Equal(t, 0, live(`SELECT COUNT(*) FROM membership WHERE org_id = ?`, drop))
	assert.Equal(t, 4, live(`SELECT COUNT(*) FROM membership WHERE org_id = ?`, keep), "every row stays but the exact duplicate")
	assert.Equal(t, 1, live(`SELECT COUNT(*) FROM membership WHERE person_id = ? AND org_id = ? AND is_primary = 1`, p2, keep), "the primary flag moved to the surviving twin")
	assert.Equal(t, 1, live(`SELECT COUNT(*) FROM membership WHERE person_id = ? AND is_primary = 1`, p1))
	assert.Equal(t, 2, live(`SELECT COUNT(*) FROM membership WHERE person_id = ? AND org_id = ?`, p3, keep))
	assert.Equal(t, 1, live(`SELECT COUNT(*) FROM org_relation WHERE a_id = ? AND b_id = ?`, parent, keep), "the duplicate parent link is dropped")
	assert.Equal(t, 1, live(`SELECT COUNT(*) FROM org_relation WHERE a_id = ? AND b_id = ?`, keep, child))
	assert.Equal(t, 1, live(`SELECT COUNT(*) FROM deal_org WHERE org_id = ? AND is_primary = 1`, keep))
	assert.Equal(t, 1, live(`SELECT COUNT(*) FROM activity_target WHERE target_kind = 'organization' AND target_id = ?`, keep))
	assert.Equal(t, 2, f.count(t, `SELECT COUNT(*) FROM taggings WHERE entity_type = 'organization' AND entity_id = ?`, keep))
	alias, ok := repo.ResolveAlias(ctx, f.d, "organization", drop)
	assert.True(t, ok)
	assert.Equal(t, keep, alias)

	// One undo step puts all of it back, one redo repeats it.
	require.NoError(t, f.orgs.UndoMerge(ctx, res))
	_, err = f.orgs.FindByID(ctx, drop)
	require.NoError(t, err)
	o, _ = f.orgs.FindByID(ctx, keep)
	assert.Nil(t, o.Domain)
	assert.Equal(t, 3, live(`SELECT COUNT(*) FROM membership WHERE org_id = ?`, drop))
	assert.Equal(t, 1, live(`SELECT COUNT(*) FROM membership WHERE person_id = ? AND org_id = ? AND is_primary = 1`, p2, drop))
	assert.Equal(t, 1, live(`SELECT COUNT(*) FROM deal_org WHERE org_id = ?`, drop))
	assert.Equal(t, 1, live(`SELECT COUNT(*) FROM org_relation WHERE a_id = ? AND b_id = ?`, parent, drop))
	_, ok = repo.ResolveAlias(ctx, f.d, "organization", drop)
	assert.False(t, ok)

	require.NoError(t, f.orgs.RedoMerge(ctx, res))
	_, err = f.orgs.FindByID(ctx, drop)
	assert.ErrorIs(t, err, model.ErrNotFound)
	assert.Equal(t, 4, live(`SELECT COUNT(*) FROM membership WHERE org_id = ?`, keep))
	alias, ok = repo.ResolveAlias(ctx, f.d, "organization", drop)
	assert.True(t, ok)
	assert.Equal(t, keep, alias)
}

func TestMergeOrgsDropsSelfLinksAndRejectsCycles(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	a, b := f.org(t, "Alpha", ""), f.org(t, "Alpha Two", "")
	f.link(t, oref(a), oref(b), model.LinkAttrs{}) // a is the parent of b: after the merge a self link
	res, err := f.orgs.MergeOrgs(ctx, a, []int64{b})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Moved["duplicates_dropped"])
	assert.Equal(t, 0, f.count(t, `SELECT COUNT(*) FROM org_relation WHERE deleted_at IS NULL`))

	c, d, x := f.org(t, "C", ""), f.org(t, "D", ""), f.org(t, "X", "")
	f.link(t, oref(d), oref(x), model.LinkAttrs{}) // D parent of X
	f.link(t, oref(x), oref(c), model.LinkAttrs{}) // X parent of C: merging C and D makes a loop
	_, err = f.orgs.MergeOrgs(ctx, c, []int64{d})
	require.Error(t, err)
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = f.orgs.FindByID(ctx, d)
	assert.NoError(t, err, "a rejected merge changes nothing")
}

func TestMergeOrgsValidatesInput(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	a, b := f.org(t, "A1", ""), f.org(t, "B1", "")
	for _, tc := range []struct {
		keep  int64
		drops []int64
	}{{a, nil}, {a, []int64{a}}, {a, []int64{b, b}}, {a, []int64{9999}}, {9999, []int64{a}}} {
		_, err := f.orgs.MergeOrgs(ctx, tc.keep, tc.drops)
		assert.Error(t, err, "%v", tc)
	}
}

func TestMergeOrgsKeepsTheCompanyReviewWorking(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	keep := f.org(t, "Acme Holdings", "")
	drop := f.org(t, "Acme", "")
	conv := repo.NewConvertRepo(f.d)
	co := func(first, company string) int64 {
		c := company
		p, err := f.people.Create(ctx, model.CreatePersonInput{FirstName: first, Company: &c})
		require.NoError(t, err)
		return p.ID
	}
	done := co("Done", "Acme")
	_, err := conv.Apply(ctx, []repo.ConvertDecision{{Key: "acme", Action: repo.ConvertLink, OrgID: drop}})
	require.NoError(t, err)
	skipped := co("Skipped", "Globex")
	_, err = conv.Apply(ctx, []repo.ConvertDecision{{Key: "globex", Action: repo.ConvertSkip}})
	require.NoError(t, err)
	_ = skipped

	_, err = f.orgs.MergeOrgs(ctx, keep, []int64{drop})
	require.NoError(t, err)

	groups, _, err := conv.Groups(ctx, false)
	require.NoError(t, err)
	for _, g := range groups {
		assert.NotEqual(t, "acme", g.Key, "the converted person does not come back")
		assert.NotEqual(t, "globex", g.Key, "the skip is remembered")
	}
	// New people with the old text match the survivor through the merge alias.
	co("New", "Acme")
	groups, _, err = conv.Groups(ctx, false)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.NotNil(t, groups[0].Match)
	assert.Equal(t, keep, groups[0].Match.OrgID)
	assert.Equal(t, "alias", groups[0].Match.Kind)
	_ = done
}

func TestConvertCreateLinksAnExistingOrganizationInsteadOfDuplicating(t *testing.T) {
	f := omSetup(t)
	ctx := context.Background()
	existing := f.org(t, "Blue Origin", "")
	conv := repo.NewConvertRepo(f.d)
	mk := func(first, company string) {
		c := company
		_, err := f.people.Create(ctx, model.CreatePersonInput{FirstName: first, Company: &c})
		require.NoError(t, err)
	}
	mk("A", "BLUE ORIGIN LLC")
	mk("B", "Acme")
	mk("C", "ACME Inc")
	// Two staged creates that normalize to one name make one organization.
	ds := []repo.ConvertDecision{
		{Key: "blue origin", Action: repo.ConvertCreate, Name: "Blue Origin"},
		{Key: "acme", Action: repo.ConvertCreate, Name: "Acme"},
	}
	plan, err := conv.Plan(ctx, ds)
	require.NoError(t, err)
	assert.Equal(t, existing, plan[0].OrgID)
	assert.Contains(t, plan[0].Note, "existing")

	ch, err := conv.Apply(ctx, ds)
	require.NoError(t, err)
	assert.Len(t, ch.OrgsCreated, 1, "only Acme is new")
	all, err := f.orgs.FindAll(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, all, 2)
	assert.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM membership WHERE org_id = ? AND deleted_at IS NULL`, existing))

	// A second create decision whose name matches an organization made earlier in the batch links it.
	mk("D", "Zeta")
	mk("E", "ZETA CORP")
	ch, err = conv.Apply(ctx, []repo.ConvertDecision{
		{Key: "zeta", Action: repo.ConvertCreate, Name: "Zeta"},
		{Key: "zeta", Action: repo.ConvertCreate, Name: "zeta corp"},
	})
	require.NoError(t, err)
	assert.Len(t, ch.OrgsCreated, 1)
}
