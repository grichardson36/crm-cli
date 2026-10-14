package db_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration003_UpgradesV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	for _, f := range []string{"migrations/001_initial.sql", "migrations/002_dex_parity.sql"} {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		up := strings.SplitN(strings.TrimPrefix(string(b), "-- up"), "-- down", 2)[0]
		_, err = raw.Exec(up)
		require.NoError(t, err, f)
	}
	_, err = raw.Exec(`INSERT INTO people (uuid, first_name) VALUES ('u1', 'Jane')`)
	require.NoError(t, err)
	_, err = raw.Exec(`PRAGMA user_version = 2`)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()

	var version int
	require.NoError(t, d.QueryRow(`PRAGMA user_version`).Scan(&version))
	assert.GreaterOrEqual(t, version, 3)

	var mergedInto sql.NullInt64
	require.NoError(t, d.QueryRow(`SELECT merged_into FROM people WHERE id = 1`).Scan(&mergedInto))
	assert.False(t, mergedInto.Valid, "existing people are not merged")
}

func TestMigration003_Constraints(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	_, err = d.Exec(`INSERT INTO people (uuid, first_name) VALUES ('a', 'A'), ('b', 'B')`)
	require.NoError(t, err)

	_, err = d.Exec(`INSERT INTO dupe_dismissals (person_a, person_b, uuid) VALUES (2, 1, 'x')`)
	assert.Error(t, err, "person_a must be lower than person_b")
	_, err = d.Exec(`INSERT INTO dupe_dismissals (person_a, person_b, uuid) VALUES (1, 1, 'x')`)
	assert.Error(t, err, "a person cannot be dismissed against itself")
	_, err = d.Exec(`INSERT INTO dupe_dismissals (person_a, person_b, uuid) VALUES (1, 2, 'x')`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO dupe_dismissals (person_a, person_b, uuid) VALUES (1, 2, 'y')`)
	assert.Error(t, err, "pair is the primary key")
	_, err = d.Exec(`INSERT INTO dupe_dismissals (person_a, person_b, uuid) VALUES (1, 99, 'z')`)
	assert.Error(t, err, "foreign key to people")

	var dismissedAt, updatedAt string
	var archived int
	require.NoError(t, d.QueryRow(`SELECT dismissed_at, updated_at, archived FROM dupe_dismissals`).Scan(&dismissedAt, &updatedAt, &archived))
	assert.NotEmpty(t, dismissedAt)
	assert.NotEmpty(t, updatedAt)
	assert.Equal(t, 0, archived)

	_, err = d.Exec(`UPDATE people SET merged_into = 1 WHERE id = 2`)
	require.NoError(t, err)
	_, err = d.Exec(`UPDATE people SET merged_into = 99 WHERE id = 2`)
	assert.Error(t, err, "merged_into references a person")
}

func TestMigration003_DownSection(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()

	b, err := os.ReadFile("migrations/003_dedupe.sql")
	require.NoError(t, err)
	parts := strings.SplitN(string(b), "-- down", 2)
	require.Len(t, parts, 2, "migration has a down section")

	_, err = d.Exec(parts[1])
	require.NoError(t, err)

	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'dupe_dismissals'`).Scan(&n))
	assert.Equal(t, 0, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('people') WHERE name = 'merged_into'`).Scan(&n))
	assert.Equal(t, 0, n)
}
