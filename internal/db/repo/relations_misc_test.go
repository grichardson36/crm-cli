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

func TestDealClosedAtFollowsStage(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	ctx := context.Background()
	dr := repo.NewDealRepo(d)

	deal, err := dr.Create(ctx, model.CreateDealInput{Title: "A", Stage: "lead"})
	require.NoError(t, err)
	assert.Nil(t, deal.ClosedAt)

	won := "won"
	deal, err = dr.Update(ctx, deal.ID, model.UpdateDealInput{Stage: &won})
	require.NoError(t, err)
	require.NotNil(t, deal.ClosedAt, "won sets closed_at")
	first := *deal.ClosedAt

	lost := "lost"
	deal, err = dr.Update(ctx, deal.ID, model.UpdateDealInput{Stage: &lost})
	require.NoError(t, err)
	assert.Equal(t, first, *deal.ClosedAt, "won to lost keeps the first close time")

	open := "negotiation"
	deal, err = dr.Update(ctx, deal.ID, model.UpdateDealInput{Stage: &open})
	require.NoError(t, err)
	assert.Nil(t, deal.ClosedAt, "reopening clears closed_at")

	born, err := dr.Create(ctx, model.CreateDealInput{Title: "B", Stage: "won"})
	require.NoError(t, err)
	assert.NotNil(t, born.ClosedAt, "a deal created as won is closed")

	title := "renamed"
	same, err := dr.Update(ctx, born.ID, model.UpdateDealInput{Title: &title})
	require.NoError(t, err)
	assert.Equal(t, *born.ClosedAt, *same.ClosedAt, "other edits leave closed_at alone")
}

func TestDealLinksFromCreateAndUpdate(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	ctx := context.Background()
	p, _ := repo.NewPersonRepo(d).Create(ctx, model.CreatePersonInput{FirstName: "Pat"})
	o1, _ := repo.NewOrgRepo(d).Create(ctx, model.CreateOrgInput{Name: "One"})
	o2, _ := repo.NewOrgRepo(d).Create(ctx, model.CreateOrgInput{Name: "Two"})
	dr := repo.NewDealRepo(d)
	deal, err := dr.Create(ctx, model.CreateDealInput{Title: "D", Stage: "lead", PersonID: &p.ID, OrgID: &o1.ID})
	require.NoError(t, err)
	assert.Equal(t, []int64{p.ID}, deal.PersonIDs)
	assert.Equal(t, o1.ID, *deal.OrgID)
	deal, err = dr.Update(ctx, deal.ID, model.UpdateDealInput{OrgID: &o2.ID})
	require.NoError(t, err)
	assert.Equal(t, o2.ID, *deal.OrgID, "the organization given last becomes primary")
	assert.ElementsMatch(t, []int64{o1.ID, o2.ID}, deal.OrgIDs)
	byOrg, err := dr.FindAll(ctx, model.DealFilters{OrgID: &o1.ID})
	require.NoError(t, err)
	assert.Len(t, byOrg, 1)
}

func TestTaskDueDateCanBeCleared(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	ctx := context.Background()
	tr := repo.NewTaskRepo(d)
	due := "2030-01-01 09:00:00"
	task, err := tr.Create(ctx, model.CreateTaskInput{Title: "T", Priority: "medium", DueAt: &due})
	require.NoError(t, err)
	require.NotNil(t, task.DueAt)

	empty := ""
	task, err = tr.Update(ctx, task.ID, model.UpdateTaskInput{DueAt: &empty})
	require.NoError(t, err)
	assert.Nil(t, task.DueAt)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM tasks WHERE due_at IS NULL`).Scan(&n))
	assert.Equal(t, 1, n, "stored as NULL, not an empty string")

	// A recurring task needs a due date.
	rec := "2 weeks"
	task2, err := tr.Create(ctx, model.CreateTaskInput{Title: "R", Priority: "medium", DueAt: &due, Recurrence: &rec})
	require.NoError(t, err)
	_, err = tr.Update(ctx, task2.ID, model.UpdateTaskInput{DueAt: &empty})
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = tr.Update(ctx, task2.ID, model.UpdateTaskInput{DueAt: &empty, Recurrence: &empty})
	assert.NoError(t, err)
}

func TestPersonOrgIDMakesAPrimaryMembership(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	ctx := context.Background()
	o1, _ := repo.NewOrgRepo(d).Create(ctx, model.CreateOrgInput{Name: "One"})
	o2, _ := repo.NewOrgRepo(d).Create(ctx, model.CreateOrgInput{Name: "Two"})
	pr := repo.NewPersonRepo(d)
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Pat", OrgID: &o1.ID})
	require.NoError(t, err)
	name, err := repo.PrimaryOrgName(ctx, d, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "One", name)
	_, err = pr.Update(ctx, p.ID, model.UpdatePersonInput{OrgID: &o2.ID})
	require.NoError(t, err)
	name, _ = repo.PrimaryOrgName(ctx, d, p.ID)
	assert.Equal(t, "Two", name)
	members, err := repo.NewOrgRepo(d).FindPeople(ctx, o1.ID)
	require.NoError(t, err)
	assert.Len(t, members, 1, "still a member of the first organization")
	byOrg, err := pr.FindAll(ctx, model.PersonFilters{OrgID: &o2.ID})
	require.NoError(t, err)
	assert.Len(t, byOrg, 1)

	problems, err := repo.CheckLinks(ctx, d)
	require.NoError(t, err)
	assert.Zero(t, problems.Total())
}
