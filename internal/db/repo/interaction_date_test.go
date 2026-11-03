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

func TestInteractionUpdateOccurredAtIsValidated(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	ir := repo.NewInteractionRepo(d)
	ctx := context.Background()
	i, err := ir.Create(ctx, model.CreateInteractionInput{Type: "note", Content: ptr("dated")})
	require.NoError(t, err)

	got, err := ir.Update(ctx, i.ID, model.UpdateInteractionInput{OccurredAt: ptr("2026-09-15")})
	require.NoError(t, err)
	assert.Equal(t, "2026-09-15 00:00:00", got.OccurredAt)

	_, err = ir.Update(ctx, i.ID, model.UpdateInteractionInput{OccurredAt: ptr("garbage")})
	require.ErrorIs(t, err, model.ErrValidation)
	got, err = ir.FindByID(ctx, i.ID)
	require.NoError(t, err)
	assert.Equal(t, "2026-09-15 00:00:00", got.OccurredAt, "an invalid date changes nothing")
}
