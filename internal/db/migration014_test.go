package db_test

import (
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 014 adds the outcome column, seeds the six deal stages and rebuilds deals without the
// CHECK that fixed the stage names. A schema 13 database keeps its deals, ids and links.
func TestMigration014_SeedsDealStagesAndFreesTheStageColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v13.db")
	d, err := db.OpenUpTo(path, 13)
	require.NoError(t, err)
	for _, q := range []string{
		`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`,
		`INSERT INTO deals (uuid, title, stage, closed_at) VALUES ('d1', 'Won deal', 'won', '2026-01-02 00:00:00')`,
		`INSERT INTO deals (uuid, title, stage) VALUES ('d2', 'Early deal', 'prospect')`,
		`INSERT INTO deals (uuid, title, stage) VALUES ('d3', 'Gone deal', 'lead')`,
		`DELETE FROM deals WHERE uuid = 'd3'`,
		`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES ('l1', 1, 1, 'champion')`,
	} {
		_, err = d.Exec(q)
		require.NoError(t, err, q)
	}
	require.NoError(t, d.Close())

	d, err = db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, db.LatestVersion(), scalar(t, d, `PRAGMA user_version`))

	t.Run("the six stages are seeded in order with outcomes", func(t *testing.T) {
		vs, ok := model.VocabularyNamed(model.DealStageKind)
		require.True(t, ok)
		rows, err := d.Query(`SELECT name, outcome FROM vocabulary WHERE kind = 'deal_stage' AND archived = 0 ORDER BY position`)
		require.NoError(t, err)
		defer rows.Close()
		var names []string
		outcomes := map[string]string{}
		for rows.Next() {
			var n, o string
			require.NoError(t, rows.Scan(&n, &o))
			names = append(names, n)
			outcomes[n] = o
		}
		assert.Equal(t, vs.Seeds, names, "the migration seeds exactly model.Vocabularies[deal_stage].Seeds")
		for _, n := range names {
			want := "open"
			if o, ok := vs.SeedOutcomes[n]; ok {
				want = o
			}
			assert.Equal(t, want, outcomes[n], n)
		}
		assert.Equal(t, "won", outcomes["won"])
		assert.Equal(t, "lost", outcomes["lost"])
	})
	t.Run("the deal roles keep the default outcome", func(t *testing.T) {
		assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM vocabulary WHERE kind = 'deal_role' AND outcome <> 'open'`))
	})
	t.Run("deals keep their rows, stage text and links", func(t *testing.T) {
		assert.Equal(t, 2, scalar(t, d, `SELECT COUNT(*) FROM deals`))
		assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM deals WHERE id = 1 AND stage = 'won' AND closed_at = '2026-01-02 00:00:00'`))
		assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM deal_person WHERE deal_id = 1`))
		assert.Equal(t, 2, scalar(t, d, `SELECT COUNT(*) FROM deals d JOIN vocabulary v ON v.kind = 'deal_stage' AND v.name = d.stage`),
			"every existing stage text matches a seeded name")
	})
	t.Run("the id counter does not go back", func(t *testing.T) {
		_, err := d.Exec(`INSERT INTO deals (uuid, title) VALUES ('d4', 'Next')`)
		require.NoError(t, err)
		assert.Equal(t, 4, scalar(t, d, `SELECT id FROM deals WHERE uuid = 'd4'`))
	})
	t.Run("a stage outside the six names is allowed", func(t *testing.T) {
		_, err := d.Exec(`UPDATE deals SET stage = 'qualified' WHERE id = 2`)
		assert.NoError(t, err)
	})
	t.Run("outcome is open, won or lost", func(t *testing.T) {
		_, err := d.Exec(`UPDATE vocabulary SET outcome = 'maybe' WHERE kind = 'deal_stage' AND name = 'lead'`)
		assert.Error(t, err)
	})
	t.Run("search and link cleanup still follow deals", func(t *testing.T) {
		assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM deals_fts WHERE deals_fts MATCH 'Early'`))
		_, err := d.Exec(`INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES ('a1', 'task', 999, 'deal', 1)`)
		assert.Error(t, err, "the activity_target triggers were created again and still check their source")
	})
}
