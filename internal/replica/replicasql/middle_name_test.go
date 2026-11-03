package replicasql_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-019, device sync door: middle_name is a plain people column, so the generated capture triggers
// carry it with the last-writer-wins default. No registry line and no contract change are needed.
func TestMiddleNameReplicatesSetAndUnset(t *testing.T) {
	a := enable(t, newDB(t))
	b := enable(t, newDB(t))
	mustExec(t, a.DB(), `INSERT INTO people (uuid, first_name, last_name, middle_name) VALUES ('p1', 'Ann', 'Lee', 'Marie')`)
	syncOnce(t, a, b)
	assert.Equal(t, "Marie", str(t, b.DB(), `SELECT middle_name FROM people WHERE uuid='p1'`))
	assert.Equal(t, 1, count(t, b.DB(), `SELECT COUNT(*) FROM people WHERE id IN (SELECT rowid FROM people_fts WHERE people_fts MATCH 'Marie')`), "the receiver's search index follows")

	mustExec(t, a.DB(), `UPDATE people SET middle_name = 'Lou' WHERE uuid='p1'`)
	syncOnce(t, a, b)
	assert.Equal(t, "Lou", str(t, b.DB(), `SELECT middle_name FROM people WHERE uuid='p1'`))

	mustExec(t, a.DB(), `UPDATE people SET middle_name = NULL WHERE uuid='p1'`)
	syncOnce(t, a, b)
	assert.Equal(t, 1, count(t, b.DB(), `SELECT COUNT(*) FROM people WHERE uuid='p1' AND middle_name IS NULL`))
	assert.Equal(t, 0, count(t, b.DB(), `SELECT COUNT(*) FROM people WHERE id IN (SELECT rowid FROM people_fts WHERE people_fts MATCH 'Lou')`))
}

// A database that already syncs gets the column by migration. The capture triggers are rebuilt on open,
// so the first edit of the new field is recorded. The database is taken back one version with the
// migration's own down section, on a temporary file.
func TestMiddleNameIsCapturedAfterTheMigrationOnASyncingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	ctx := context.Background()
	_, err = replicasql.Enable(ctx, d, replicasql.DefaultRegistry())
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "022_person_middle_name.sql"))
	require.NoError(t, err)
	_, down, ok := strings.Cut(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n-- down\n")
	require.True(t, ok)
	_, err = d.Exec(down)
	require.NoError(t, err, "the down section works on a database with capture triggers")
	_, err = d.Exec(`PRAGMA user_version = 20`)
	require.NoError(t, err)
	_, err = replicasql.Enable(ctx, d, replicasql.DefaultRegistry()) // triggers as an older build made them, without the column
	require.NoError(t, err)
	mustExec(t, d, `INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	require.NoError(t, d.Close())

	d, err = db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	mustExec(t, d, `UPDATE people SET middle_name = 'Marie' WHERE uuid='p1'`)
	assert.Equal(t, `"Marie"`, str(t, d, `SELECT value FROM replica_changes WHERE key='p1' AND field='middle_name'`))
}
