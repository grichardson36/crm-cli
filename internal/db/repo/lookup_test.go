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

func TestLookupRecord(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()

	pr := repo.NewPersonRepo(d)
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith")})
	require.NoError(t, err)

	got, err := repo.LookupRecord(ctx, d, "person", p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Jane Smith", got.Label)
	assert.False(t, got.Archived)

	require.NoError(t, pr.Archive(ctx, p.ID))
	got, err = repo.LookupRecord(ctx, d, "person", p.ID)
	require.NoError(t, err)
	assert.True(t, got.Archived, "archived records are still found")

	_, err = repo.LookupRecord(ctx, d, "person", 999)
	assert.ErrorIs(t, err, model.ErrNotFound)
	_, err = repo.LookupRecord(ctx, d, "robot", 1)
	assert.ErrorIs(t, err, model.ErrValidation)

	for _, entity := range []string{"organization", "deal", "task", "interaction"} {
		_, err = repo.LookupRecord(ctx, d, entity, 1)
		assert.ErrorIs(t, err, model.ErrNotFound, entity)
	}
}

// Regression: listing by tag joined tags, whose updated_at made ORDER BY ambiguous.
func TestPersonFindAllByTagAndOrder(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()

	pr := repo.NewPersonRepo(d)
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	_, err = pr.Create(ctx, model.CreatePersonInput{FirstName: "Bob"})
	require.NoError(t, err)
	require.NoError(t, repo.NewTagRepo(d).Apply(ctx, "person", p.ID, "vip"))

	tag := "vip"
	got, err := pr.FindAll(ctx, model.PersonFilters{Tag: &tag})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Jane", got[0].FirstName)

	org := int64(1)
	_, err = pr.FindAll(ctx, model.PersonFilters{Tag: &tag, OrgID: &org, Limit: 5})
	require.NoError(t, err)
}
