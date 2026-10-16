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
	person int64
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
	orgs := repo.NewOrgRepo(d)
	o1, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Initech"})
	require.NoError(t, err)
	o2, err := orgs.Create(ctx, model.CreateOrgInput{Name: "Globex"})
	require.NoError(t, err)
	dl, err := repo.NewDealRepo(d).Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, err)
	tk, err := repo.NewTaskRepo(d).Create(ctx, model.CreateTaskInput{Title: "Call", Priority: "medium"})
	require.NoError(t, err)
	return linkFixture{links: repo.NewLinkRepo(d), people: people, person: p.ID, org: o1.ID, org2: o2.ID, deal: dl.ID, task: tk.ID}
}

func TestLinkBothEndsEveryRelation(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	person := repo.Ref{Kind: "person", ID: f.person}
	org := repo.Ref{Kind: "organization", ID: f.org}
	deal := repo.Ref{Kind: "deal", ID: f.deal}
	task := repo.Ref{Kind: "task", ID: f.task}

	pairs := []struct{ a, b repo.Ref }{{person, org}, {deal, org}, {deal, person}, {task, deal}, {task, person}}
	for _, p := range pairs {
		res, err := f.links.Link(ctx, p.a, p.b)
		require.NoError(t, err, "%s to %s", p.a, p.b)
		assert.True(t, res.Changed)
		again, err := f.links.Link(ctx, p.b, p.a) // from the other side
		require.NoError(t, err)
		assert.False(t, again.Changed, "linking twice changes nothing")

		ab, err := f.links.Linked(ctx, p.a, p.b.Kind)
		require.NoError(t, err)
		assert.Equal(t, []int64{p.b.ID}, ab, "%s sees %s", p.a, p.b)
		ba, err := f.links.Linked(ctx, p.b, p.a.Kind)
		require.NoError(t, err)
		assert.Equal(t, []int64{p.a.ID}, ba, "%s sees %s", p.b, p.a)

		require.NoError(t, f.links.Unlink(ctx, p.b, p.a)) // remove from the other side
		ab, err = f.links.Linked(ctx, p.a, p.b.Kind)
		require.NoError(t, err)
		assert.Empty(t, ab)
		assert.ErrorIs(t, f.links.Unlink(ctx, p.a, p.b), model.ErrNotFound)
	}
}

func TestLinkReplacesASingleValuedSide(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	person := repo.Ref{Kind: "person", ID: f.person}
	_, err := f.links.Link(ctx, person, repo.Ref{Kind: "organization", ID: f.org})
	require.NoError(t, err)
	res, err := f.links.Link(ctx, person, repo.Ref{Kind: "organization", ID: f.org2})
	require.NoError(t, err)
	require.NotNil(t, res.Replaced)
	assert.Equal(t, repo.Ref{Kind: "organization", ID: f.org}, *res.Replaced)
	got, err := f.links.Linked(ctx, person, "organization")
	require.NoError(t, err)
	assert.Equal(t, []int64{f.org2}, got)
}

func TestLinkRefusesBadInput(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	_, err := f.links.Link(ctx, repo.Ref{Kind: "person", ID: f.person}, repo.Ref{Kind: "person", ID: f.person})
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "organization", ID: f.org}, repo.Ref{Kind: "task", ID: f.task})
	assert.ErrorIs(t, err, model.ErrValidation, "no relation between organization and task")
	_, err = f.links.Link(ctx, repo.Ref{Kind: "person", ID: f.person}, repo.Ref{Kind: "organization", ID: 999})
	assert.ErrorIs(t, err, model.ErrNotFound)
	_, err = f.links.Link(ctx, repo.Ref{Kind: "widget", ID: 1}, repo.Ref{Kind: "organization", ID: f.org})
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestLinkedHidesArchived(t *testing.T) {
	f := setupLinks(t)
	ctx := context.Background()
	person := repo.Ref{Kind: "person", ID: f.person}
	org := repo.Ref{Kind: "organization", ID: f.org}
	_, err := f.links.Link(ctx, person, org)
	require.NoError(t, err)
	require.NoError(t, f.people.Archive(ctx, f.person))
	got, err := f.links.Linked(ctx, org, "person")
	require.NoError(t, err)
	assert.Empty(t, got)
}
