package db

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func oldDB(t *testing.T, dir string, version int) string {
	t.Helper()
	path := filepath.Join(dir, "personal.db")
	d, err := OpenUpTo(path, version)
	require.NoError(t, err)
	_, err = d.Exec("CREATE TABLE marker (v TEXT); INSERT INTO marker VALUES ('kept')")
	require.NoError(t, err)
	require.NoError(t, d.Close())
	return path
}

func captureNotice(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := Notice
	Notice = &buf
	t.Cleanup(func() { Notice = old })
	return &buf
}

func backups(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "backups", "*.db"))
	require.NoError(t, err)
	return m
}

func TestBackupTakenWhenMigrationsPending(t *testing.T) {
	dir := t.TempDir()
	path := oldDB(t, dir, LatestVersion()-1)
	notice := captureNotice(t)
	now = func() time.Time { return time.Date(2026, 10, 29, 10, 15, 0, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })

	d, err := Open(path)
	require.NoError(t, err)
	defer d.Close()

	want := filepath.Join(dir, "backups", fmt.Sprintf("personal-pre-v%d-20261029-101500.db", LatestVersion()-1))
	assert.Equal(t, []string{want}, backups(t, dir))
	assert.Contains(t, notice.String(), want)

	// The copy holds the old schema and data.
	b, err := OpenUpTo(want, LatestVersion()-1)
	require.NoError(t, err)
	defer b.Close()
	v, _ := userVersion(b)
	assert.Equal(t, LatestVersion()-1, v)
	var m string
	require.NoError(t, b.QueryRow("SELECT v FROM marker").Scan(&m))
	assert.Equal(t, "kept", m)
	v, _ = userVersion(d)
	assert.Equal(t, LatestVersion(), v)
}

func TestNoBackupWhenCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "personal.db")
	notice := captureNotice(t)
	d, err := Open(path) // brand new file: nothing to back up
	require.NoError(t, err)
	d.Close()
	d, err = Open(path) // current
	require.NoError(t, err)
	d.Close()
	assert.Empty(t, backups(t, dir))
	assert.Empty(t, notice.String())
}

func TestNoBackupForMemory(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { os.Chdir(wd) })
	d, err := Open(":memory:")
	require.NoError(t, err)
	d.Close()
	_, err = os.Stat(filepath.Join(dir, "backups"))
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestBackupFailureAbortsMigration(t *testing.T) {
	dir := t.TempDir()
	path := oldDB(t, dir, LatestVersion()-1)
	// A file named "backups" makes the backup folder impossible to create.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "backups"), []byte("x"), 0o600))

	_, err := Open(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup before migration failed")

	d, err := OpenUpTo(path, LatestVersion()-1)
	require.NoError(t, err)
	defer d.Close()
	v, _ := userVersion(d)
	assert.Equal(t, LatestVersion()-1, v, "database must be untouched")
}

func TestPruneKeepsFiveAndOnlyMatching(t *testing.T) {
	dir := t.TempDir()
	path := oldDB(t, dir, LatestVersion()-1)
	bdir := filepath.Join(dir, "backups")
	require.NoError(t, os.MkdirAll(bdir, 0o700))
	var names []string
	for i := 1; i <= 7; i++ {
		names = append(names, fmt.Sprintf("personal-pre-v9-2026010%d-120000.db", i))
	}
	others := []string{"personal-manual.db", "work-pre-v9-20260101-120000.db", "personal-pre-v9-20260101-120000.db.txt"}
	for _, n := range append(append([]string{}, names...), others...) {
		require.NoError(t, os.WriteFile(filepath.Join(bdir, n), []byte("x"), 0o600))
	}
	captureNotice(t)
	d, err := Open(path)
	require.NoError(t, err)
	d.Close()

	got := backups(t, dir)
	var mine []string
	for _, g := range got {
		if ok, _ := filepath.Match("personal-pre-v*-*.db", filepath.Base(g)); ok {
			mine = append(mine, filepath.Base(g))
		}
	}
	assert.Len(t, mine, KeepPreMigrationBackups)
	assert.NotContains(t, mine, names[0])
	assert.NotContains(t, mine, names[1])
	assert.NotContains(t, mine, names[2])
	assert.Contains(t, mine, names[6])
	for _, o := range others[:2] {
		assert.FileExists(t, filepath.Join(bdir, o))
	}
}

func TestNeverOverwritesBackup(t *testing.T) {
	dir := t.TempDir()
	path := oldDB(t, dir, LatestVersion()-1)
	captureNotice(t)
	fixed := time.Date(2026, 10, 29, 10, 15, 0, 0, time.UTC)
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = time.Now })
	existing := filepath.Join(dir, "backups", fmt.Sprintf("personal-pre-v%d-20261029-101500.db", LatestVersion()-1))
	require.NoError(t, os.MkdirAll(filepath.Dir(existing), 0o700))
	require.NoError(t, os.WriteFile(existing, []byte("precious"), 0o600))

	d, err := Open(path)
	require.NoError(t, err)
	d.Close()
	b, _ := os.ReadFile(existing)
	assert.Equal(t, "precious", string(b))
	assert.Len(t, backups(t, dir), 2)
}

func TestNewerSchemaRefused(t *testing.T) {
	dir := t.TempDir()
	path := oldDB(t, dir, LatestVersion())
	d, err := OpenUpTo(path, LatestVersion())
	require.NoError(t, err)
	_, err = d.Exec(fmt.Sprintf("PRAGMA user_version = %d", LatestVersion()+3))
	require.NoError(t, err)
	d.Close()

	_, err = Open(path)
	var ns *NewerSchemaError
	require.True(t, errors.As(err, &ns))
	assert.Contains(t, err.Error(), fmt.Sprintf("database is schema %d, this build supports %d, update the program", LatestVersion()+3, LatestVersion()))
	assert.Empty(t, backups(t, dir))
}

func TestConcurrentOpenMigratesOnce(t *testing.T) {
	dir := t.TempDir()
	path := oldDB(t, dir, LatestVersion()-1)
	captureNotice(t)

	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := Open(path)
			if err == nil {
				v, _ := userVersion(d)
				if v != LatestVersion() {
					err = fmt.Errorf("version %d after open", v)
				}
				d.Close()
			}
			errs[i] = err
		}()
	}
	wg.Wait()
	for _, err := range errs {
		assert.NoError(t, err)
	}
	assert.Len(t, backups(t, dir), 1)
	_, err := os.Stat(path + ".migrate.lock")
	assert.True(t, errors.Is(err, os.ErrNotExist), "lock released")
}

func TestStaleLockIsReplaced(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "x.migrate.lock")
	require.NoError(t, os.WriteFile(lock, []byte("999"), 0o600))
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(lock, old, old))
	unlock, err := acquireLock(lock)
	require.NoError(t, err)
	unlock()
}

func TestListBackupsNewestFirstAndOnlyThisDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "personal.db")
	bdir := filepath.Join(dir, "backups")
	require.NoError(t, os.MkdirAll(bdir, 0o700))
	for _, n := range []string{"personal-pre-v3-20260101-100000.db", "personal-pre-v5-20260301-090000.db", "personal-pre-v5-20260301-090000-2.db", "work-pre-v5-20260401-090000.db", "notes.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(bdir, n), []byte("x"), 0o600))
	}
	got := ListBackups(path)
	var names []string
	for _, b := range got {
		names = append(names, b.Name)
	}
	assert.Equal(t, []string{"personal-pre-v5-20260301-090000-2.db", "personal-pre-v5-20260301-090000.db", "personal-pre-v3-20260101-100000.db"}, names)
	assert.Equal(t, 5, got[0].Version)
	assert.Equal(t, int64(1), got[0].Size)
	assert.Empty(t, ListBackups(filepath.Join(t.TempDir(), "x.db")), "no folder is an empty list")
}
