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

func TestPurgeArchived(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*repo.PersonRepo, []int64, int64) {
		d, err := db.Open(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { d.Close() })
		pr := repo.NewPersonRepo(d)
		var archived []int64
		for _, n := range []string{"Ann", "Bea", "Cy"} {
			p, err := pr.Create(ctx, model.CreatePersonInput{FirstName: n})
			require.NoError(t, err)
			require.NoError(t, pr.Archive(ctx, p.ID))
			archived = append(archived, p.ID)
		}
		live, err := pr.Create(ctx, model.CreatePersonInput{FirstName: "Live"})
		require.NoError(t, err)
		return pr, archived, live.ID
	}
	tests := []struct {
		name      string
		opts      func(ids []int64, live int64) repo.PurgeArchivedOptions
		wantCount int
		wantLeft  int // archived people left
		wantSkip  int
		wantErr   bool
	}{
		{"dry run changes nothing", func(ids []int64, _ int64) repo.PurgeArchivedOptions { return repo.PurgeArchivedOptions{} }, 3, 3, 0, false},
		{"apply all", func(ids []int64, _ int64) repo.PurgeArchivedOptions { return repo.PurgeArchivedOptions{Apply: true} }, 3, 0, 0, false},
		{"apply some ids", func(ids []int64, _ int64) repo.PurgeArchivedOptions {
			return repo.PurgeArchivedOptions{Apply: true, IDs: ids[:2]}
		}, 2, 1, 0, false},
		{"live and unknown ids are skipped", func(ids []int64, live int64) repo.PurgeArchivedOptions {
			return repo.PurgeArchivedOptions{Apply: true, IDs: []int64{ids[0], live, 999}}
		}, 1, 2, 2, false},
		{"type person is accepted", func(ids []int64, _ int64) repo.PurgeArchivedOptions {
			return repo.PurgeArchivedOptions{Kind: "person", Apply: true}
		}, 3, 0, 0, false},
		{"other types are refused", func(ids []int64, _ int64) repo.PurgeArchivedOptions {
			return repo.PurgeArchivedOptions{Kind: "task", Apply: true}
		}, 0, 3, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr, ids, live := setup(t)
			res, err := pr.PurgeArchived(ctx, tt.opts(ids, live))
			if tt.wantErr {
				assert.ErrorIs(t, err, model.ErrValidation)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCount, res.Count)
			assert.Len(t, res.Skipped, tt.wantSkip)
			left, err := repo.ListArchived(ctx, pr.DB(), "person")
			require.NoError(t, err)
			assert.Len(t, left, tt.wantLeft)
			if res.Applied && !tt.wantErr {
				assert.Equal(t, tt.wantCount, res.Purged)
			}
			var tomb int
			require.NoError(t, pr.DB().QueryRow(`SELECT COUNT(*) FROM tombstones WHERE table_name = 'people'`).Scan(&tomb))
			assert.Equal(t, res.Purged, tomb, "one tombstone per purged person, as a single purge writes")
			_, err = pr.FindByID(ctx, live)
			assert.NoError(t, err, "live people are never touched")
		})
	}
}
