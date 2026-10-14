package doctor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/doctor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDB(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "crm.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	require.NoError(t, d.Close())
	return path
}

func plenty(string) (uint64, error) { return 100 << 30, nil }

func byName(rep doctor.Report) map[string]doctor.Check {
	m := map[string]doctor.Check{}
	for _, c := range rep.Checks {
		m[c.Name] = c
	}
	return m
}

func TestHealthyDatabase(t *testing.T) {
	dir := t.TempDir()
	path := newDB(t, dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "backups"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "backups", "crm-1.db"), []byte("x"), 0o600))

	rep := doctor.Run(context.Background(), doctor.Options{DBPath: path, Version: "1.2.3", MCPTools: 41, FreeBytes: plenty})
	c := byName(rep)
	assert.True(t, rep.OK)
	for _, name := range []string{"binary", "data_dir", "database", "integrity", "schema", "wal", "backups", "disk_space", "cloud_folder", "sync", "mcp"} {
		require.Contains(t, c, name)
		assert.Equal(t, doctor.OK, c[name].Status, "%s: %s", name, c[name].Message)
		assert.Empty(t, c[name].Fix)
	}
	assert.Contains(t, c["binary"].Message, "1.2.3")
	assert.Equal(t, 0, rep.Summary[doctor.Fail])
}

func TestNoBackupIsWarning(t *testing.T) {
	path := newDB(t, t.TempDir())
	rep := doctor.Run(context.Background(), doctor.Options{DBPath: path, FreeBytes: plenty})
	c := byName(rep)
	assert.True(t, rep.OK, "warnings do not fail the run")
	assert.Equal(t, doctor.Warn, c["backups"].Status)
	assert.Contains(t, c["backups"].Fix, "crm backup")
}

func TestMissingDatabaseFails(t *testing.T) {
	dir := t.TempDir()
	rep := doctor.Run(context.Background(), doctor.Options{DBPath: filepath.Join(dir, "crm.db"), FreeBytes: plenty})
	c := byName(rep)
	assert.False(t, rep.OK)
	assert.Equal(t, doctor.Fail, c["database"].Status)
	assert.Contains(t, c["database"].Fix, "crm init")
	_, hasIntegrity := c["integrity"]
	assert.False(t, hasIntegrity, "no integrity check without a database")
	_, err := os.Stat(filepath.Join(dir, "crm.db"))
	assert.True(t, os.IsNotExist(err), "doctor must not create the database")
}

func TestMissingDataDirFails(t *testing.T) {
	rep := doctor.Run(context.Background(), doctor.Options{DBPath: filepath.Join(t.TempDir(), "nope", "crm.db"), FreeBytes: plenty})
	c := byName(rep)
	assert.Equal(t, doctor.Fail, c["data_dir"].Status)
	assert.Contains(t, c["data_dir"].Fix, "crm init")
}

func TestNotACRMDatabaseFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "other.db")
	require.NoError(t, os.WriteFile(path, []byte("this is not sqlite at all, just text padded to be long enough"), 0o600))
	rep := doctor.Run(context.Background(), doctor.Options{DBPath: path, FreeBytes: plenty})
	assert.False(t, rep.OK)
	assert.Equal(t, doctor.Fail, byName(rep)["database"].Status)
}

func TestOlderAndNewerSchema(t *testing.T) {
	path := newDB(t, t.TempDir())
	d, err := db.Open(path)
	require.NoError(t, err)
	_, err = d.Exec("PRAGMA user_version = 2")
	require.NoError(t, err)
	d.Close()
	// Re-opening with Open would migrate, so go straight to doctor, which reads only.
	rep := doctor.Run(context.Background(), doctor.Options{DBPath: path, FreeBytes: plenty})
	assert.Equal(t, doctor.Warn, byName(rep)["schema"].Status)
	assert.True(t, rep.OK)

	d, err = db.OpenReadOnly(path)
	require.NoError(t, err)
	d.Close()
}

func TestNewerSchemaFails(t *testing.T) {
	path := newDB(t, t.TempDir())
	d, err := db.Open(path)
	require.NoError(t, err)
	_, err = d.Exec("PRAGMA user_version = 999")
	require.NoError(t, err)
	d.Close()
	rep := doctor.Run(context.Background(), doctor.Options{DBPath: path, FreeBytes: plenty})
	c := byName(rep)
	assert.Equal(t, doctor.Fail, c["schema"].Status)
	assert.Contains(t, c["schema"].Fix, "newer")
	assert.False(t, rep.OK)
}

func TestCloudFolderWarning(t *testing.T) {
	for _, folder := range []string{"OneDrive", "Dropbox", "iCloudDrive", "OneDrive - Contoso"} {
		t.Run(folder, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), folder, "crm")
			require.NoError(t, os.MkdirAll(dir, 0o700))
			path := newDB(t, dir)
			rep := doctor.Run(context.Background(), doctor.Options{DBPath: path, FreeBytes: plenty})
			c := byName(rep)["cloud_folder"]
			assert.Equal(t, doctor.Warn, c.Status)
			assert.Contains(t, c.Message, "corrupt")
			assert.Contains(t, c.Fix, "CRM_DB")
			assert.True(t, rep.OK)
		})
	}
}

func TestDiskSpace(t *testing.T) {
	path := newDB(t, t.TempDir())
	run := func(free uint64) doctor.Check {
		rep := doctor.Run(context.Background(), doctor.Options{DBPath: path, FreeBytes: func(string) (uint64, error) { return free, nil }})
		return byName(rep)["disk_space"]
	}
	assert.Equal(t, doctor.OK, run(50<<30).Status)
	assert.Equal(t, doctor.Warn, run(50<<20).Status)
	assert.Equal(t, doctor.Fail, run(1<<20).Status)
}

func TestLargeWALWarns(t *testing.T) {
	path := newDB(t, t.TempDir())
	f, err := os.Create(path + "-wal")
	require.NoError(t, err)
	require.NoError(t, f.Truncate(70<<20))
	f.Close()
	rep := doctor.Run(context.Background(), doctor.Options{DBPath: path, FreeBytes: plenty})
	assert.Equal(t, doctor.Warn, byName(rep)["wal"].Status)
}
