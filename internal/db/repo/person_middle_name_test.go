package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-019: a person has a middle name that is stored, edited, cleaned like the other name fields and found by search.
func TestPersonMiddleName_CreateStoresAndTrims(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()

	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Ann", LastName: ptr("Smith"), MiddleName: ptr("  Marie ")})
	require.NoError(t, err)
	require.NotNil(t, p.MiddleName)
	assert.Equal(t, "Marie", *p.MiddleName)

	got, err := r.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Marie", *got.MiddleName)
}

func TestPersonMiddleName_UpdateSetsAndClears(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Ann"})
	require.NoError(t, err)
	assert.Nil(t, p.MiddleName)

	u, err := r.Update(ctx, p.ID, model.UpdatePersonInput{MiddleName: ptr(" Marie  ")})
	require.NoError(t, err)
	assert.Equal(t, "Marie", *u.MiddleName, "trimmed like the other names")
	assert.Equal(t, "Ann", u.FirstName, "other fields stay")

	u, err = r.Update(ctx, p.ID, model.UpdatePersonInput{Title: ptr("CTO")})
	require.NoError(t, err)
	assert.Equal(t, "Marie", *u.MiddleName, "an edit that does not give it leaves it")

	u, err = r.Update(ctx, p.ID, model.UpdatePersonInput{MiddleName: ptr("")})
	require.NoError(t, err)
	assert.Equal(t, "", *u.MiddleName, "an empty value clears it")
}

func TestPersonMiddleName_SearchFindsAndFollowsEdits(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	_, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Ann", LastName: ptr("Smith"), MiddleName: ptr("Marigold")})
	require.NoError(t, err)
	_, err = r.Create(ctx, model.CreatePersonInput{FirstName: "Bob", LastName: ptr("Jones")})
	require.NoError(t, err)

	res, err := r.Search(ctx, "Marigold", 10)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, "Ann", res[0].FirstName)

	res, err = r.Search(ctx, "Mari", 10)
	require.NoError(t, err)
	assert.Len(t, res, 1, "prefix match like the other name fields")

	_, err = r.Update(ctx, 1, model.UpdatePersonInput{MiddleName: ptr("Quill")})
	require.NoError(t, err)
	res, err = r.Search(ctx, "Marigold", 10)
	require.NoError(t, err)
	assert.Empty(t, res, "the old value is no longer found")
	res, err = r.Search(ctx, "Quill", 10)
	require.NoError(t, err)
	assert.Len(t, res, 1)
}

func TestPersonMiddleName_ArchiveAndRestoreKeepIt(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Ann", MiddleName: ptr("Marigold")})
	require.NoError(t, err)

	require.NoError(t, r.Archive(ctx, p.ID))
	res, err := r.Search(ctx, "Marigold", 10)
	require.NoError(t, err)
	assert.Empty(t, res, "an archived person is not found")

	require.NoError(t, r.Restore(ctx, p.ID))
	got, err := r.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Marigold", *got.MiddleName)
	res, err = r.Search(ctx, "Marigold", 10)
	require.NoError(t, err)
	assert.Len(t, res, 1)
}

func TestPersonMiddleName_DoesNotChangeDuplicateCheck(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	_, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Ann", LastName: ptr("Smith"), MiddleName: ptr("Marie")})
	require.NoError(t, err)
	_, err = r.Create(ctx, model.CreatePersonInput{FirstName: "Ann", LastName: ptr("Smith"), MiddleName: ptr("Lou")})
	assert.ErrorIs(t, err, model.ErrConflict, "the name check stays first plus last")
}

func TestPersonMiddleName_MergeFillsBlankAndHonorsChoice(t *testing.T) {
	_, pr := openMergeDB(t)
	ctx := context.Background()
	keep, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith")})
	require.NoError(t, err)
	drop, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Janet", LastName: ptr("Smith"), MiddleName: ptr("Marie")})
	require.NoError(t, err)

	_, err = pr.Merge(ctx, keep.ID, []int64{drop.ID}, nil)
	require.NoError(t, err)
	got, err := pr.FindByID(ctx, keep.ID)
	require.NoError(t, err)
	require.NotNil(t, got.MiddleName)
	assert.Equal(t, "Marie", *got.MiddleName, "an empty survivor field takes the dropped value")

	keep2, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Ann", LastName: ptr("Lee"), MiddleName: ptr("Keep")})
	drop2, _ := pr.Create(ctx, model.CreatePersonInput{FirstName: "Anna", LastName: ptr("Lee"), MiddleName: ptr("Drop")})
	_, err = pr.Merge(ctx, keep2.ID, []int64{drop2.ID}, map[string]int64{"middle_name": drop2.ID})
	require.NoError(t, err)
	got, _ = pr.FindByID(ctx, keep2.ID)
	assert.Equal(t, "Drop", *got.MiddleName, "a choice names the person to take it from")
}

func TestPersonMiddleName_UpdateRecordsFieldVersion(t *testing.T) {
	r := setupTestDB(t)
	ctx := context.Background()
	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Ann"})
	require.NoError(t, err)
	_, err = r.Update(ctx, p.ID, model.UpdatePersonInput{MiddleName: ptr("Marie")})
	require.NoError(t, err)
	at, err := repo.FieldChangedAt(ctx, r.DB(), "person", p.ID, "middle_name")
	require.NoError(t, err)
	assert.NotEmpty(t, at, "two-way sync can tell when the field last changed")
}
