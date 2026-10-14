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

func TestExternalIDRepo(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	pr := repo.NewPersonRepo(d)
	er := repo.NewExternalIDRepo(d)

	a, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Ann"})
	require.NoError(t, err)
	b, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Ben"})
	require.NoError(t, err)

	_, err = er.Lookup(ctx, "google", "person", "g-1")
	assert.ErrorIs(t, err, model.ErrNotFound)

	e, err := er.Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: a.ID, Source: "google", ExternalID: "g-1", ETag: ptr("v1")})
	require.NoError(t, err)
	assert.Equal(t, a.UUID, e.LocalUUID, "local uuid is looked up")
	assert.Equal(t, "v1", *e.ETag)
	assert.NotEmpty(t, e.UUID)
	assert.NotNil(t, e.LastSeenAt)

	got, err := er.Lookup(ctx, "google", "person", "g-1")
	require.NoError(t, err)
	assert.Equal(t, a.ID, got.LocalID)
	byLocal, err := er.LookupByLocal(ctx, "google", "person", a.ID)
	require.NoError(t, err)
	assert.Equal(t, "g-1", byLocal.ExternalID)
	_, err = er.LookupByLocal(ctx, "icloud", "person", a.ID)
	assert.ErrorIs(t, err, model.ErrNotFound, "mappings are per source")

	// Same external record again: repointed and refreshed, still one row.
	e2, err := er.Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: a.ID, Source: "google", ExternalID: "g-1", ETag: ptr("v2")})
	require.NoError(t, err)
	assert.Equal(t, e.ID, e2.ID)
	assert.Equal(t, "v2", *e2.ETag)

	// A local row cannot carry two records from one source.
	_, err = er.Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: a.ID, Source: "google", ExternalID: "g-2"})
	assert.ErrorIs(t, err, model.ErrConflict)
	// A different source is fine.
	_, err = er.Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: a.ID, Source: "icloud", ExternalID: "g-1"})
	require.NoError(t, err)

	_, err = er.Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: b.ID, Source: "google", ExternalID: "g-3"})
	require.NoError(t, err)
	list, err := er.ListBySource(ctx, "google", "")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	list, err = er.ListBySource(ctx, "google", "deal")
	require.NoError(t, err)
	assert.Empty(t, list)

	t.Run("validation", func(t *testing.T) {
		_, err := er.Upsert(ctx, repo.ExternalIDInput{Entity: "nope", LocalID: 1, Source: "s", ExternalID: "x"})
		assert.ErrorIs(t, err, model.ErrValidation)
		_, err = er.Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: 1, Source: "", ExternalID: "x"})
		assert.ErrorIs(t, err, model.ErrValidation)
		_, err = er.Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: 999, Source: "s", ExternalID: "x"})
		assert.ErrorIs(t, err, model.ErrNotFound)
	})

	require.NoError(t, er.Delete(ctx, "google", "person", "g-1"))
	assert.ErrorIs(t, er.Delete(ctx, "google", "person", "g-1"), model.ErrNotFound)
	_, err = pr.FindByID(ctx, a.ID)
	assert.NoError(t, err, "deleting a mapping leaves the person")
}

func TestPurgeRemovesExternalIDsAndFieldVersions(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	pr := repo.NewPersonRepo(d)
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Ann"})
	require.NoError(t, err)
	_, err = repo.NewExternalIDRepo(d).Upsert(ctx, repo.ExternalIDInput{Entity: "person", LocalID: p.ID, Source: "google", ExternalID: "g-1"})
	require.NoError(t, err)
	_, err = pr.Update(ctx, p.ID, model.UpdatePersonInput{Title: ptr("CTO")})
	require.NoError(t, err)

	require.NoError(t, pr.Archive(ctx, p.ID))
	require.NoError(t, pr.Purge(ctx, p.ID))
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM external_ids`).Scan(&n))
	assert.Equal(t, 0, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM field_versions`).Scan(&n))
	assert.Equal(t, 0, n)
}
