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

// Purge is a verb of every archivable object type: the object and its links go and a tombstone is
// written for every deleted synced row.
func TestPurgeEveryType(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	pr := repo.NewPersonRepo(d)
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Pat"})
	require.NoError(t, err)
	org, err := repo.NewOrgRepo(d).Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	deal, err := repo.NewDealRepo(d).Create(ctx, model.CreateDealInput{Title: "Big deal", Stage: "lead", OrgID: &org.ID, PersonID: &p.ID})
	require.NoError(t, err)
	task, err := repo.NewTaskRepo(d).Create(ctx, model.CreateTaskInput{Title: "Call", Priority: "medium", PersonIDs: []int64{p.ID}})
	require.NoError(t, err)
	text := "met at lunch"
	note, err := repo.NewInteractionRepo(d).Create(ctx, model.CreateInteractionInput{Type: "note", Content: &text, OrgIDs: []int64{org.ID}})
	require.NoError(t, err)
	_, err = repo.NewLinkRepo(d).Link(ctx, repo.Ref{Kind: "person", ID: p.ID}, repo.Ref{Kind: "organization", ID: org.ID}, model.LinkAttrs{})
	require.NoError(t, err)
	tags := repo.NewTagRepo(d)
	require.NoError(t, tags.Apply(ctx, "organization", org.ID, "vip"))
	require.NoError(t, tags.Apply(ctx, "deal", deal.ID, "vip"))

	live := func(kind string) int64 {
		switch kind {
		case "organization":
			return org.ID
		case "deal":
			return deal.ID
		case "task":
			return task.ID
		}
		return note.ID
	}
	// A live object cannot be purged.
	for _, k := range []string{"organization", "deal", "task", "interaction"} {
		err := repo.PurgeObject(ctx, d, k, live(k))
		assert.ErrorIs(t, err, model.ErrConflict, k)
	}
	for _, k := range []string{"organization", "deal", "task", "interaction"} {
		require.NoError(t, repo.ArchiveObject(ctx, d, k, live(k)), k)
	}

	// Dry run lists every kind and writes nothing.
	res, err := repo.PurgeArchived(ctx, d, repo.PurgeArchivedOptions{})
	require.NoError(t, err)
	assert.Equal(t, 4, res.Count)
	assert.False(t, res.Applied)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM tombstones`).Scan(&n))
	assert.Zero(t, n)

	// One type at a time by id.
	res, err = repo.PurgeArchived(ctx, d, repo.PurgeArchivedOptions{Kind: "org", IDs: []int64{org.ID}, Apply: true})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Purged)
	for _, q := range []string{
		`SELECT COUNT(*) FROM organizations`,
		`SELECT COUNT(*) FROM membership`,
		`SELECT COUNT(*) FROM deal_org`,
		`SELECT COUNT(*) FROM taggings WHERE entity_type = 'organization'`,
		`SELECT COUNT(*) FROM activity_target WHERE target_kind = 'organization'`,
	} {
		require.NoError(t, d.QueryRow(q).Scan(&n))
		assert.Zero(t, n, q)
	}
	for _, table := range []string{"organizations", "membership", "deal_org", "taggings", "activity_target"} {
		require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM tombstones WHERE table_name = ?`, table).Scan(&n))
		assert.Positive(t, n, "tombstone for "+table)
	}

	// The rest at once.
	res, err = repo.PurgeArchived(ctx, d, repo.PurgeArchivedOptions{Apply: true})
	require.NoError(t, err)
	assert.Equal(t, 3, res.Purged)
	assert.Empty(t, res.Errors)
	for _, table := range []string{"deals", "tasks", "interactions"} {
		require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n))
		assert.Zero(t, n, table)
		require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM tombstones WHERE table_name = ?`, table).Scan(&n))
		assert.Equal(t, 1, n, "one tombstone for "+table)
	}
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM deal_person`).Scan(&n))
	assert.Zero(t, n)
	// The live person is untouched.
	_, err = pr.FindByID(ctx, p.ID)
	assert.NoError(t, err)
}
