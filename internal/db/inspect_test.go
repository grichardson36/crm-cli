package db_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspectReadsWithoutChangingTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crm.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	_, err = d.Exec("INSERT INTO people (uuid, first_name) VALUES ('u1', 'Jane'), ('u2', 'Bob')")
	require.NoError(t, err)
	_, err = d.Exec("UPDATE people SET archived = 1 WHERE first_name = 'Bob'")
	require.NoError(t, err)
	require.NoError(t, d.Close())

	before, err := os.Stat(path)
	require.NoError(t, err)

	info, err := db.Inspect(path)
	require.NoError(t, err)
	assert.True(t, info.IsCRM)
	assert.Equal(t, db.LatestVersion(), info.SchemaVersion)
	assert.Equal(t, 1, info.Counts["people"], "archived people are not counted")
	assert.NotEmpty(t, info.DeviceID)

	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, before.Size(), after.Size())
	assert.Equal(t, before.ModTime(), after.ModTime())
}

func TestOpenReadOnlyDoesNotCreateOrWrite(t *testing.T) {
	dir := t.TempDir()
	_, err := db.OpenReadOnly(filepath.Join(dir, "missing.db"))
	assert.Error(t, err)
	_, statErr := os.Stat(filepath.Join(dir, "missing.db"))
	assert.True(t, os.IsNotExist(statErr))

	path := filepath.Join(dir, "crm.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	d.Close()
	ro, err := db.OpenReadOnly(path)
	require.NoError(t, err)
	defer ro.Close()
	_, err = ro.Exec("INSERT INTO people (uuid, first_name) VALUES ('u', 'x')")
	assert.Error(t, err, "read-only handle must reject writes")
}

func TestInspectNonCRMFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.db")
	// A valid SQLite file with no CRM tables. The driver is registered by the db package.
	other, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = other.Exec("CREATE TABLE notes (id INTEGER)")
	require.NoError(t, err)
	require.NoError(t, other.Close())

	info, err := db.Inspect(path)
	require.NoError(t, err)
	assert.False(t, info.IsCRM)
	assert.Empty(t, info.Counts)
}
