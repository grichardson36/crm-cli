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

func TestPersonUpdateRecordsFieldVersions(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	pr := repo.NewPersonRepo(d)

	fav := true
	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Ann", Title: ptr("Eng")})
	require.NoError(t, err)

	at, err := repo.FieldChangedAt(ctx, d, "person", p.ID, "title")
	require.NoError(t, err)
	assert.Equal(t, "", at, "creation records nothing")

	_, err = pr.Update(ctx, p.ID, model.UpdatePersonInput{
		Title: ptr("CTO"), Company: ptr("Acme"), FirstName: ptr("Ann"), // first name unchanged
		Birthday: ptr("1990-05-06"), IsFavorite: &fav,
	})
	require.NoError(t, err)

	for _, f := range []string{"title", "company", "birthday", "is_favorite"} {
		at, err := repo.FieldChangedAt(ctx, d, "person", p.ID, f)
		require.NoError(t, err)
		assert.NotEmpty(t, at, f)
	}
	for _, f := range []string{"first_name", "notes", "location"} {
		at, err := repo.FieldChangedAt(ctx, d, "person", p.ID, f)
		require.NoError(t, err)
		assert.Empty(t, at, "%s did not change", f)
	}

	var device, metaDevice string
	require.NoError(t, d.QueryRow(`SELECT device_id FROM field_versions WHERE field = 'title'`).Scan(&device))
	require.NoError(t, d.QueryRow(`SELECT value FROM meta WHERE key = 'device_id'`).Scan(&metaDevice))
	assert.Equal(t, metaDevice, device)

	// Only changed fields move. Age the rows, then change title again and re-save company unchanged.
	_, err = d.Exec(`UPDATE field_versions SET changed_at = '2000-01-01 00:00:00'`)
	require.NoError(t, err)
	_, err = pr.Update(ctx, p.ID, model.UpdatePersonInput{Title: ptr("CEO"), Company: ptr("Acme")})
	require.NoError(t, err)
	titleAt, _ := repo.FieldChangedAt(ctx, d, "person", p.ID, "title")
	companyAt, _ := repo.FieldChangedAt(ctx, d, "person", p.ID, "company")
	assert.NotEqual(t, "2000-01-01 00:00:00", titleAt)
	assert.Equal(t, "2000-01-01 00:00:00", companyAt, "same value is not a change")

	// Clearing a birthday is a change.
	_, err = pr.Update(ctx, p.ID, model.UpdatePersonInput{Birthday: ptr("")})
	require.NoError(t, err)
	bAt, _ := repo.FieldChangedAt(ctx, d, "person", p.ID, "birthday")
	assert.NotEqual(t, "2000-01-01 00:00:00", bAt)

	// A failed update (unknown person) writes nothing.
	_, err = pr.Update(ctx, 9999, model.UpdatePersonInput{Title: ptr("X")})
	assert.ErrorIs(t, err, model.ErrNotFound)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM field_versions WHERE local_id = 9999`).Scan(&n))
	assert.Equal(t, 0, n)
}
