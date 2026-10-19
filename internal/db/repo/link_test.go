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

type linkFixture struct {
	links  *repo.LinkRepo
	people *repo.PersonRepo
	orgs   *repo.OrgRepo
	person int64
	other  int64
	org    int64
	org2   int64
	deal   int64
	task   int64
}

func setupLinks(t *testing.T) linkFixture {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	people := repo.NewPersonRepo(d)
	p, err := people.Create(ctx, model.CreatePersonInput{FirstName: "Sam"})
	require.NoError(t, err)
	p2, err := people.Create(ctx, model.CreatePersonInput{FirstName: "Alex"})
	require.NoError(t, err)
	orgs := repo.NewOrgRepo(d)
	o1, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Initech"})
	require.NoError(t, err)
	o2, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	dl, err := repo.NewDealRepo(d).Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, err)
	tk, err := repo.NewTaskRepo(d).Create(ctx, model.CreateTaskInput{Title: "Call", Priority: "medium"})
	require.NoError(t, err)
	return linkFixture{links: repo.NewLinkRepo(d), people: people, orgs: orgs, person: p.ID, other: p2.ID, org: o1.ID, org2: o2.ID, deal: dl.ID, task: tk.ID}
}

func ids(items []model.LinkedObject) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func sp(s string) *string { return &s }
func bp(b bool) *bool     { return &b }

func TestLinkBothEndsEveryRelation(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	person := repo.Ref{Kind: "person", ID: f.person}
	org := repo.Ref{Kind: "organization", ID: f.org}
	deal := repo.Ref{Kind: "deal", ID: f.deal}
	task := repo.Ref{Kind: "task", ID: f.task}

	pairs := []struct{ a, b repo.Ref }{{person, org}, {deal, org}, {deal, person}, {task, deal}, {task, person}, {task, org}}
	for _, p := range pairs {
		res, err := f.links.Link(ctx, p.a, p.b)
		require.NoError(t, err, "%s to %s", p.a, p.b)
		assert.True(t, res.Changed)
		again, err := f.links.Link(ctx, p.b, p.a) // from the other side
		require.NoError(t, err)
		assert.False(t, again.Changed, "linking twice changes nothing")

		ab, err := f.links.Linked(ctx, p.a, p.b.Kind)
		require.NoError(t, err)
		assert.Equal(t, []int64{p.b.ID}, ids(ab), "%s sees %s", p.a, p.b)
		ba, err := f.links.Linked(ctx, p.b, p.a.Kind)
		require.NoError(t, err)
		assert.Equal(t, []int64{p.a.ID}, ids(ba), "%s sees %s", p.b, p.a)

		_, err = f.links.Unlink(ctx, p.b, p.a) // remove from the other side
		require.NoError(t, err)
		ab, err = f.links.Linked(ctx, p.a, p.b.Kind)
		require.NoError(t, err)
		assert.Empty(t, ab)
		_, err = f.links.Unlink(ctx, p.a, p.b)
		assert.ErrorIs(t, err, model.ErrNotFound)
	}
}

func TestLinkRefusesBadInput(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	_, err := f.links.Link(ctx, repo.Ref{Kind: "person", ID: f.person}, repo.Ref{Kind: "person", ID: f.person})
	assert.ErrorIs(t, err, model.ErrValidation, "no self link")
	_, err = f.links.Link(ctx, repo.Ref{Kind: "organization", ID: f.org}, repo.Ref{Kind: "task", ID: f.task})
	assert.NoError(t, err, "tasks can be about organizations now")
	_, err = f.links.Link(ctx, repo.Ref{Kind: "deal", ID: f.deal}, repo.Ref{Kind: "task", ID: f.task})
	assert.NoError(t, err)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "person", ID: f.person}, repo.Ref{Kind: "organization", ID: 999})
	assert.ErrorIs(t, err, model.ErrNotFound)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "widget", ID: 1}, repo.Ref{Kind: "organization", ID: f.org})
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "person", ID: f.person}, repo.Ref{Kind: "organization", ID: f.org}, model.LinkAttrs{Kind: sp("friend")})
	assert.ErrorIs(t, err, model.ErrValidation, "a membership has no kind")
	_, err = f.links.Link(ctx, repo.Ref{Kind: "task", ID: f.task}, repo.Ref{Kind: "person", ID: f.person}, model.LinkAttrs{Primary: bp(true)})
	assert.ErrorIs(t, err, model.ErrValidation, "a task link has no primary flag")
}

func TestLinkedHidesArchivedAndRestoreBringsBack(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	person := repo.Ref{Kind: "person", ID: f.person}
	org := repo.Ref{Kind: "organization", ID: f.org}
	_, err := f.links.Link(ctx, person, org, model.LinkAttrs{Role: sp("CTO")})
	require.NoError(t, err)

	require.NoError(t, f.people.Archive(ctx, f.person))
	got, err := f.links.Linked(ctx, org, "person")
	require.NoError(t, err)
	assert.Empty(t, got, "an archived person is hidden from the organization")
	require.NoError(t, f.people.Restore(ctx, f.person))
	got, err = f.links.Linked(ctx, org, "person")
	require.NoError(t, err)
	require.Len(t, got, 1, "restoring brings the link back")
	assert.Equal(t, "CTO", *got[0].Role)

	require.NoError(t, repo.ArchiveObject(ctx, f.links.DB(), "organization", f.org))
	got, err = f.links.Linked(ctx, person, "organization")
	require.NoError(t, err)
	assert.Empty(t, got, "an archived organization is hidden from the person")
	require.NoError(t, repo.RestoreObject(ctx, f.links.DB(), "organization", f.org))
	got, err = f.links.Linked(ctx, person, "organization")
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestMembershipAttributesPrimaryAndRepeats(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	person := repo.Ref{Kind: "person", ID: f.person}
	o1 := repo.Ref{Kind: "organization", ID: f.org}
	o2 := repo.Ref{Kind: "organization", ID: f.org2}

	res, err := f.links.Link(ctx, person, o1, model.LinkAttrs{Role: sp("Engineer"), StartedOn: sp("2019-03")})
	require.NoError(t, err)
	assert.True(t, res.Created)
	list, err := f.links.Linked(ctx, person, "organization")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.True(t, list[0].Primary, "the first current membership becomes primary")
	assert.True(t, list[0].Current)
	assert.Equal(t, "2019-03", *list[0].StartedOn)

	res, err = f.links.Link(ctx, person, o2, model.LinkAttrs{Role: sp("Advisor")})
	require.NoError(t, err)
	list, _ = f.links.Linked(ctx, person, "organization")
	assert.Equal(t, []int64{f.org, f.org2}, ids(list))
	assert.False(t, list[1].Primary, "a second organization is not primary by itself")

	// Moving the primary demotes the old one and reports it for undo.
	res, err = f.links.Link(ctx, person, o2, model.LinkAttrs{Primary: bp(true)})
	require.NoError(t, err)
	assert.True(t, res.Updated)
	assert.Len(t, res.Demoted, 1)
	list, _ = f.links.Linked(ctx, person, "organization")
	assert.Equal(t, f.org2, list[0].ID, "the primary is listed first")
	assert.Equal(t, "Advisor", *list[0].Role, "unrelated attributes stay")

	// Same attributes again changes nothing. A new date updates in place.
	res, err = f.links.Link(ctx, person, o1, model.LinkAttrs{Role: sp("Engineer")})
	require.NoError(t, err)
	assert.False(t, res.Changed)
	res, err = f.links.Link(ctx, person, o1, model.LinkAttrs{EndedOn: sp("2021-06-30")})
	require.NoError(t, err)
	assert.True(t, res.Updated)
	require.NotNil(t, res.Before)
	assert.Nil(t, res.Before.EndedOn)
	list, _ = f.links.Linked(ctx, person, "organization")
	assert.Equal(t, []int64{f.org2, f.org}, ids(list), "past memberships come after current ones")
	assert.False(t, list[1].Current)

	// Coming back is a second membership at the same organization (E3).
	res, err = f.links.LinkNew(ctx, person, o1, model.LinkAttrs{Role: sp("Director"), StartedOn: sp("2023")})
	require.NoError(t, err)
	assert.True(t, res.Created)
	_, err = f.links.Link(ctx, person, o1, model.LinkAttrs{Role: sp("Chief")})
	assert.ErrorIs(t, err, model.ErrConflict, "which of the two links? the caller must say")

	// Unlink removes every link of the pair, restore brings one back.
	removed, err := f.links.Unlink(ctx, o1, person)
	require.NoError(t, err)
	assert.Len(t, removed, 2)
	list, _ = f.links.Linked(ctx, person, "organization")
	assert.Equal(t, []int64{f.org2}, ids(list))
	require.NoError(t, f.links.RestoreLink(ctx, "membership", removed[0].ID))
	list, _ = f.links.Linked(ctx, person, "organization")
	assert.Len(t, list, 2)

	// A bad date and a reversed range are refused.
	_, err = f.links.Link(ctx, person, o2, model.LinkAttrs{StartedOn: sp("last year")})
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = f.links.Link(ctx, person, o2, model.LinkAttrs{StartedOn: sp("2022"), EndedOn: sp("2020")})
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestOverwriteRestoresAnEditedLink(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	person := repo.Ref{Kind: "person", ID: f.person}
	org := repo.Ref{Kind: "organization", ID: f.org}
	_, err := f.links.Link(ctx, person, org, model.LinkAttrs{Role: sp("CEO")})
	require.NoError(t, err)
	res, err := f.links.Link(ctx, person, org, model.LinkAttrs{Role: sp("")})
	require.NoError(t, err)
	assert.True(t, res.Updated, "an empty value clears")
	list, _ := f.links.Linked(ctx, person, "organization")
	assert.Nil(t, list[0].Role)
	_, err = f.links.Overwrite(ctx, *res.Before)
	require.NoError(t, err)
	list, _ = f.links.Linked(ctx, person, "organization")
	assert.Equal(t, "CEO", *list[0].Role)
}

func TestPersonRelationKindsAndLabels(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	sam := repo.Ref{Kind: "person", ID: f.person}
	alex := repo.Ref{Kind: "person", ID: f.other}

	// The default kind is related, symmetric, stored once with the lower id first.
	res, err := f.links.Link(ctx, alex, sam)
	require.NoError(t, err)
	assert.True(t, res.Created)
	rec, err := f.links.Get(ctx, "person_relation", res.LinkID)
	require.NoError(t, err)
	assert.Equal(t, f.person, rec.A)
	assert.Equal(t, f.other, rec.B)
	again, err := f.links.Link(ctx, sam, alex)
	require.NoError(t, err)
	assert.False(t, again.Changed, "the same pair from the other side is the same link")
	for _, a := range []repo.Ref{sam, alex} {
		got, err := f.links.Linked(ctx, a, "person")
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "related", got[0].Label)
	}

	// Directional: Sam is parent of Alex. Sam sees Alex as child, Alex sees Sam as parent.
	_, err = f.links.Link(ctx, sam, alex, model.LinkAttrs{Kind: sp("parent")})
	require.NoError(t, err)
	labels := func(a, other repo.Ref) []string {
		got, err := f.links.Linked(ctx, a, "person")
		require.NoError(t, err)
		var out []string
		for _, g := range got {
			assert.Equal(t, other.ID, g.ID)
			out = append(out, g.Label)
		}
		return out
	}
	assert.ElementsMatch(t, []string{"related", "child"}, labels(sam, alex))
	assert.ElementsMatch(t, []string{"related", "parent"}, labels(alex, sam))

	// Naming the inverse reverses the ends: Alex "child" Sam is the same link.
	same, err := f.links.Link(ctx, alex, sam, model.LinkAttrs{Kind: sp("child")})
	require.NoError(t, err)
	assert.False(t, same.Changed)

	// Unlink by kind leaves the other link.
	_, err = f.links.Unlink(ctx, sam, alex, repo.UnlinkOptions{Kind: "parent"})
	require.NoError(t, err)
	assert.Equal(t, []string{"related"}, labels(sam, alex))

	_, err = f.links.Link(ctx, sam, alex, model.LinkAttrs{Kind: sp("nemesis")})
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestUserDefinedKinds(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	kinds := repo.NewKindRepo(f.links.DB())
	k, err := kinds.Add(ctx, "person", "godparent", "godchild")
	require.NoError(t, err)
	assert.False(t, k.Symmetric)
	sym, err := kinds.Add(ctx, "person", "neighbor", "")
	require.NoError(t, err)
	assert.True(t, sym.Symmetric)
	_, err = kinds.Add(ctx, "person", "Friend", "")
	assert.ErrorIs(t, err, model.ErrConflict)
	_, err = kinds.Add(ctx, "person", "child", "x")
	assert.ErrorIs(t, err, model.ErrConflict, "an inverse name of another kind clashes")

	sam, alex := repo.Ref{Kind: "person", ID: f.person}, repo.Ref{Kind: "person", ID: f.other}
	_, err = f.links.Link(ctx, sam, alex, model.LinkAttrs{Kind: sp("godparent")})
	require.NoError(t, err)
	assert.ErrorIs(t, kinds.Remove(ctx, "person", "godparent"), model.ErrConflict, "in use")
	_, err = f.links.Unlink(ctx, sam, alex)
	require.NoError(t, err)
	require.NoError(t, kinds.Remove(ctx, "person", "godparent"))
	assert.ErrorIs(t, kinds.Remove(ctx, "person", "friend"), model.ErrConflict, "built-in")

	all, err := kinds.List(ctx, "person")
	require.NoError(t, err)
	var names []string
	for _, x := range all {
		names = append(names, x.Name)
	}
	assert.Contains(t, names, "neighbor")
	assert.NotContains(t, names, "godparent")
}

func TestOrgParentsRejectCyclesAndAllowSeveralParents(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	o3, err := f.orgs.Create(ctx, model.CreateOrgInput{Name: "Hooli"})
	require.NoError(t, err)
	a := repo.Ref{Kind: "organization", ID: f.org}
	b := repo.Ref{Kind: "organization", ID: f.org2}
	c := repo.Ref{Kind: "organization", ID: o3.ID}
	parent := model.LinkAttrs{Kind: sp("parent")}

	_, err = f.links.Link(ctx, a, b, parent) // a is parent of b
	require.NoError(t, err)
	_, err = f.links.Link(ctx, b, c, parent) // b is parent of c
	require.NoError(t, err)
	_, err = f.links.Link(ctx, c, a, parent) // c parent of a would close the loop
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = f.links.Link(ctx, b, a, parent)
	assert.ErrorIs(t, err, model.ErrValidation, "two organizations cannot be each other's parent")
	_, err = f.links.Link(ctx, a, c, model.LinkAttrs{Kind: sp("child")})
	assert.ErrorIs(t, err, model.ErrValidation, "a child of c means c is above a, but c is below a")
	_, err = f.links.Link(ctx, c, a, model.LinkAttrs{Kind: sp("child")})
	assert.NoError(t, err, "c child of a is a second path from a to c, not a cycle")

	// Several parents are allowed: c has parents b and a.
	got, err := f.links.Linked(ctx, c, "organization")
	require.NoError(t, err)
	var labels []string
	for _, g := range got {
		labels = append(labels, g.Label)
	}
	assert.Equal(t, []string{"parent", "parent"}, labels)
}

func TestRolesSuggestions(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	person := repo.Ref{Kind: "person", ID: f.person}
	_, err := f.links.Link(ctx, person, repo.Ref{Kind: "organization", ID: f.org}, model.LinkAttrs{Role: sp("Founder")})
	require.NoError(t, err)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "person", ID: f.other}, repo.Ref{Kind: "organization", ID: f.org}, model.LinkAttrs{Role: sp("founder")})
	require.NoError(t, err)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "person", ID: f.other}, repo.Ref{Kind: "organization", ID: f.org2}, model.LinkAttrs{Role: sp("Advisor")})
	require.NoError(t, err)
	roles, err := f.links.Roles(ctx, "membership", 10)
	require.NoError(t, err)
	require.Len(t, roles, 2)
	assert.Equal(t, "Founder", roles[0], "most used first, case folded")
}

func TestActivitiesAttachToAnyObject(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	d := f.links.DB()
	ir := repo.NewInteractionRepo(d)
	note, err := ir.Create(ctx, model.CreateInteractionInput{Type: "note", Subject: sp("Kickoff"), PersonIDs: []int64{f.person}, OrgIDs: []int64{f.org}, DealIDs: []int64{f.deal}})
	require.NoError(t, err)
	assert.Equal(t, []int64{f.org}, note.OrgIDs)
	assert.Equal(t, []int64{f.deal}, note.DealIDs)

	for _, filter := range []model.InteractionFilters{{PersonID: &f.person}, {OrgID: &f.org}, {DealID: &f.deal}} {
		got, err := ir.FindAll(ctx, filter)
		require.NoError(t, err)
		require.Len(t, got, 1)
	}
	got, err := ir.FindAll(ctx, model.InteractionFilters{OrgID: &f.org2})
	require.NoError(t, err)
	assert.Empty(t, got)

	// Attach and detach an existing note from the org side with the one link verb.
	_, err = f.links.Link(ctx, repo.Ref{Kind: "interaction", ID: note.ID}, repo.Ref{Kind: "organization", ID: f.org2})
	require.NoError(t, err)
	got, _ = ir.FindAll(ctx, model.InteractionFilters{OrgID: &f.org2})
	assert.Len(t, got, 1)

	// Tasks too, and an archived org hides its tasks from the link list but not the task.
	tr := repo.NewTaskRepo(d)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "task", ID: f.task}, repo.Ref{Kind: "organization", ID: f.org})
	require.NoError(t, err)
	tasks, err := tr.FindAll(ctx, model.TaskFilters{OrgID: &f.org})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, []int64{f.org}, tasks[0].OrgIDs)
}

func TestMergeCombinesLinks(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	keep := repo.Ref{Kind: "person", ID: f.person}
	drop := repo.Ref{Kind: "person", ID: f.other}
	third, err := f.people.Create(ctx, model.CreatePersonInput{FirstName: "Third"})
	require.NoError(t, err)
	tp := repo.Ref{Kind: "person", ID: third.ID}
	o1, o2 := repo.Ref{Kind: "organization", ID: f.org}, repo.Ref{Kind: "organization", ID: f.org2}

	// Same organization on both with different roles: both stay (CTO and Engineer are two memberships).
	_, err = f.links.Link(ctx, keep, o1, model.LinkAttrs{StartedOn: sp("2020-05"), Role: sp("CTO")})
	require.NoError(t, err)
	_, err = f.links.Link(ctx, drop, o1, model.LinkAttrs{StartedOn: sp("2018"), Role: sp("Engineer"), Notes: sp("hired by Pat")})
	require.NoError(t, err)
	// An identical membership on both (same organization, role, start, end) is dropped once.
	_, err = f.links.Link(ctx, keep, o2, model.LinkAttrs{Role: sp("Advisor")})
	require.NoError(t, err)
	_, err = f.links.Link(ctx, drop, o2, model.LinkAttrs{Role: sp("Advisor")})
	require.NoError(t, err)
	// Related people and a duplicate pair.
	_, err = f.links.Link(ctx, drop, tp, model.LinkAttrs{Kind: sp("friend")})
	require.NoError(t, err)
	_, err = f.links.Link(ctx, keep, tp, model.LinkAttrs{Kind: sp("friend")})
	require.NoError(t, err)
	_, err = f.links.Link(ctx, keep, drop) // becomes a self link and is dropped
	require.NoError(t, err)
	// Deals, tasks, notes.
	_, err = f.links.Link(ctx, repo.Ref{Kind: "deal", ID: f.deal}, drop, model.LinkAttrs{Role: sp("champion")})
	require.NoError(t, err)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "task", ID: f.task}, drop)
	require.NoError(t, err)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "task", ID: f.task}, keep)
	require.NoError(t, err)
	note, err := repo.NewInteractionRepo(f.links.DB()).Create(ctx, model.CreateInteractionInput{Type: "note", PersonIDs: []int64{f.other}})
	require.NoError(t, err)

	res, err := f.people.Merge(ctx, f.person, []int64{f.other}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Moved["memberships"], "only the Engineer stay moved, the identical Advisor row was dropped")
	assert.Equal(t, 1, res.Moved["interactions"])
	assert.Equal(t, 1, res.Moved["deals"])
	assert.Equal(t, 1, res.Moved["aliases"])
	assert.GreaterOrEqual(t, res.Moved["duplicates_dropped"], 4, "identical membership, friend pair, self link, task link")

	orgs, err := f.links.Linked(ctx, keep, "organization")
	require.NoError(t, err)
	require.Len(t, orgs, 3, "CTO and Engineer at Initech, Advisor at Globex")
	assert.True(t, orgs[0].Primary, "the survivor keeps its own primary")
	var roles []string
	for _, o := range orgs {
		roles = append(roles, *o.Role)
	}
	assert.ElementsMatch(t, []string{"CTO", "Engineer", "Advisor"}, roles)
	for _, o := range orgs {
		if *o.Role == "Engineer" {
			assert.Equal(t, "2018", *o.StartedOn, "the moved row keeps its own start and notes")
			assert.Equal(t, "hired by Pat", *o.Notes)
			assert.False(t, o.Primary)
		}
	}

	rel, err := f.links.Linked(ctx, keep, "person")
	require.NoError(t, err)
	require.Len(t, rel, 1, "one friend link, no self link")
	assert.Equal(t, third.ID, rel[0].ID)
	deals, _ := f.links.Linked(ctx, keep, "deal")
	require.Len(t, deals, 1)
	assert.Equal(t, "champion", *deals[0].Role)
	tasks, _ := f.links.Linked(ctx, keep, "task")
	assert.Len(t, tasks, 1)

	got, err := repo.NewInteractionRepo(f.links.DB()).FindByID(ctx, note.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{f.person}, got.PersonIDs)

	survivor, ok := repo.ResolveAlias(ctx, f.links.DB(), "person", f.other)
	assert.True(t, ok)
	assert.Equal(t, f.person, survivor)
}
