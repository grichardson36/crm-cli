package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInteractionUpdate(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	ir, pr := repo.NewInteractionRepo(d), repo.NewPersonRepo(d)
	ctx := context.Background()

	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	i, err := ir.Create(ctx, model.CreateInteractionInput{Type: "note", Content: ptr("original pelican"), PersonIDs: []int64{p.ID}})
	require.NoError(t, err)

	// Make updated_at visibly old so the bump is detectable.
	_, err = d.Exec(`UPDATE interactions SET updated_at = '2000-01-01 00:00:00' WHERE id = ?`, i.ID)
	require.NoError(t, err)

	got, err := ir.Update(ctx, i.ID, model.UpdateInteractionInput{
		Type: ptr("CALL"), Content: ptr("edited heron"), OccurredAt: ptr("2026-02-03 04:05:06"), Direction: ptr("inbound"),
	})
	require.NoError(t, err)
	assert.Equal(t, "call", got.Type)
	assert.Equal(t, "edited heron", *got.Content)
	assert.Equal(t, "2026-02-03 04:05:06", got.OccurredAt)
	assert.Equal(t, "inbound", *got.Direction)
	assert.NotEqual(t, "2000-01-01 00:00:00", got.UpdatedAt)
	assert.Equal(t, []int64{p.ID}, got.PersonIDs)

	t.Run("fts follows the edit", func(t *testing.T) {
		old, err := ir.Search(ctx, "pelican", 10)
		require.NoError(t, err)
		assert.Empty(t, old)
		fresh, err := ir.Search(ctx, "heron", 10)
		require.NoError(t, err)
		assert.Len(t, fresh, 1)
	})

	t.Run("clear fields", func(t *testing.T) {
		got, err := ir.Update(ctx, i.ID, model.UpdateInteractionInput{Content: ptr(""), Direction: ptr("")})
		require.NoError(t, err)
		assert.Nil(t, got.Content)
		assert.Nil(t, got.Direction)
	})

	t.Run("validation", func(t *testing.T) {
		_, err := ir.Update(ctx, i.ID, model.UpdateInteractionInput{Type: ptr("nonsense")})
		assert.ErrorIs(t, err, model.ErrValidation)
		_, err = ir.Update(ctx, i.ID, model.UpdateInteractionInput{Direction: ptr("sideways")})
		assert.ErrorIs(t, err, model.ErrValidation)
		_, err = ir.Update(ctx, i.ID, model.UpdateInteractionInput{OccurredAt: ptr(" ")})
		assert.ErrorIs(t, err, model.ErrValidation)
		_, err = ir.Update(ctx, 9999, model.UpdateInteractionInput{Content: ptr("x")})
		assert.ErrorIs(t, err, model.ErrNotFound)
	})

	t.Run("archived type rejected", func(t *testing.T) {
		nt := repo.NewNoteTypeRepo(d)
		_, err := nt.Add(ctx, "standup", true)
		require.NoError(t, err)
		_, err = ir.Update(ctx, i.ID, model.UpdateInteractionInput{Type: ptr("standup")})
		require.NoError(t, err)
		require.NoError(t, nt.Archive(ctx, "standup"))
		_, err = ir.Update(ctx, i.ID, model.UpdateInteractionInput{Type: ptr("standup")})
		assert.ErrorIs(t, err, model.ErrValidation)
	})
}

func TestInteractionArchiveRestore(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	ir, pr := repo.NewInteractionRepo(d), repo.NewPersonRepo(d)
	ctx := context.Background()

	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	i, err := ir.Create(ctx, model.CreateInteractionInput{Type: "call", Content: ptr("walrus"), PersonIDs: []int64{p.ID}})
	require.NoError(t, err)
	_, err = d.Exec(`UPDATE interactions SET updated_at = '2000-01-01 00:00:00' WHERE id = ?`, i.ID)
	require.NoError(t, err)

	require.NoError(t, ir.Archive(ctx, i.ID))
	var updated string
	require.NoError(t, d.QueryRow(`SELECT updated_at FROM interactions WHERE id = ?`, i.ID).Scan(&updated))
	assert.NotEqual(t, "2000-01-01 00:00:00", updated, "archive bumps updated_at")

	_, err = ir.FindByID(ctx, i.ID)
	assert.ErrorIs(t, err, model.ErrNotFound)
	found, err := ir.Search(ctx, "walrus", 10)
	require.NoError(t, err)
	assert.Empty(t, found, "archived rows are not searchable")
	list, err := ir.FindAll(ctx, model.InteractionFilters{})
	require.NoError(t, err)
	assert.Empty(t, list)
	assert.ErrorIs(t, ir.Archive(ctx, i.ID), model.ErrNotFound, "double archive")
	_, err = ir.Update(ctx, i.ID, model.UpdateInteractionInput{Content: ptr("x")})
	assert.ErrorIs(t, err, model.ErrNotFound, "archived rows cannot be edited")

	require.NoError(t, ir.Restore(ctx, i.ID))
	found, err = ir.Search(ctx, "walrus", 10)
	require.NoError(t, err)
	assert.Len(t, found, 1)
	assert.ErrorIs(t, ir.Restore(ctx, i.ID), model.ErrNotFound, "restore of an active row")
}

func TestInteractionArchiveStopsKeepInTouchCredit(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	ir, pr := repo.NewInteractionRepo(d), repo.NewPersonRepo(d)
	ctx := context.Background()

	p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	_, err = pr.Update(ctx, p.ID, model.UpdatePersonInput{KeepInTouch: ptr("7 days")})
	require.NoError(t, err)
	at := time.Now().UTC().Format("2006-01-02 15:04:05")
	i, err := ir.Create(ctx, model.CreateInteractionInput{Type: "call", OccurredAt: &at, PersonIDs: []int64{p.ID}})
	require.NoError(t, err)

	var seen *string
	require.NoError(t, d.QueryRow(`SELECT last_seen FROM keep_in_touch_status WHERE person_id = ?`, p.ID).Scan(&seen))
	require.NotNil(t, seen)
	require.NoError(t, ir.Archive(ctx, i.ID))
	require.NoError(t, d.QueryRow(`SELECT last_seen FROM keep_in_touch_status WHERE person_id = ?`, p.ID).Scan(&seen))
	assert.Nil(t, seen)
}
