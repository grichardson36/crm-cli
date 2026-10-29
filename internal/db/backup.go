package db

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// KeepPreMigrationBackups is how many pre-migration backups of one database are kept.
const KeepPreMigrationBackups = 5

// Notice receives the one-line message printed after a pre-migration backup. It is standard error
// by default. The MCP server sets it to io.Discard, and standard output is never used.
var Notice io.Writer = os.Stderr

var (
	now          = time.Now
	lockWait     = 60 * time.Second
	lockStaleAge = 2 * time.Minute
)

// NewerSchemaError reports a database whose schema is newer than this build supports.
type NewerSchemaError struct{ Have, Supported int }

func (e *NewerSchemaError) Error() string {
	return fmt.Sprintf("database is schema %d, this build supports %d, update the program (replace the crm binary with a newer one)", e.Have, e.Supported)
}

func isFileDatabase(path string) bool {
	return path != "" && path != ":memory:" && !strings.HasPrefix(path, "file:") && !strings.Contains(path, "mode=memory")
}

// migrateWithBackup applies pending migrations. For an existing file database that has data, it
// first writes a consistent copy to <data dir>/backups and aborts if that fails. A lock file next
// to the database lets only one process migrate while others wait and then find nothing to do.
func migrateWithBackup(d *sql.DB, path string) error {
	cur, err := userVersion(d)
	if err != nil {
		return err
	}
	latest := LatestVersion()
	if cur > latest {
		return &NewerSchemaError{Have: cur, Supported: latest}
	}
	if cur == latest {
		return nil
	}
	if !isFileDatabase(path) {
		return runMigrations(d, 0)
	}

	unlock, err := acquireLock(path + ".migrate.lock")
	if err != nil {
		return err
	}
	defer unlock()

	// Another process may have migrated while this one waited.
	cur, err = userVersion(d)
	if err != nil {
		return err
	}
	if cur > latest {
		return &NewerSchemaError{Have: cur, Supported: latest}
	}
	if cur == latest {
		return nil
	}
	if cur > 0 {
		dest, err := backupBeforeMigration(d, path, cur)
		if err != nil {
			return fmt.Errorf("backup before migration failed, database left unchanged: %w", err)
		}
		fmt.Fprintf(Notice, "crm: database is schema %d, this build is %d. Backed up to %s before migrating.\n", cur, latest, dest)
	}
	return runMigrations(d, 0)
}

func userVersion(d *sql.DB) (int, error) {
	var v int
	if err := d.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("get user_version: %w", err)
	}
	return v, nil
}

func baseName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// backupBeforeMigration copies the database with VACUUM INTO and prunes old copies.
func backupBeforeMigration(d *sql.DB, path string, version int) (string, error) {
	dir := filepath.Join(filepath.Dir(path), "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := baseName(path)
	stem := fmt.Sprintf("%s-pre-v%d-%s", base, version, now().Format("20060102-150405"))
	dest := filepath.Join(dir, stem+".db")
	for n := 2; ; n++ {
		if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
			break
		}
		if n > 1000 {
			return "", errors.New("cannot find a free backup name")
		}
		dest = filepath.Join(dir, fmt.Sprintf("%s-%d.db", stem, n))
	}
	if _, err := d.Exec("VACUUM INTO ?", dest); err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	pruneBackups(dir, base)
	return dest, nil
}

type backupFile struct {
	name string
	ts   string
	n    int
}

// pruneBackups keeps the newest KeepPreMigrationBackups files named like this database's
// pre-migration backups. Other files in the folder are never touched.
func pruneBackups(dir, base string) {
	re := regexp.MustCompile("^" + regexp.QuoteMeta(base) + `-pre-v\d+-(\d{8}-\d{6})(?:-(\d+))?\.db$`)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var files []backupFile
	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		n := 1
		if m[2] != "" {
			n, _ = strconv.Atoi(m[2])
		}
		files = append(files, backupFile{e.Name(), m[1], n})
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].ts != files[j].ts {
			return files[i].ts > files[j].ts
		}
		return files[i].n > files[j].n
	})
	for i := KeepPreMigrationBackups; i < len(files); i++ {
		_ = os.Remove(filepath.Join(dir, files[i].name))
	}
}

// acquireLock creates the lock file exclusively, waiting for another holder. A lock older than
// lockStaleAge belongs to a crashed process and is removed.
func acquireLock(lock string) (func(), error) {
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return func() { _ = os.Remove(lock) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("migration lock %s: %w", lock, err)
		}
		if st, serr := os.Stat(lock); serr == nil && time.Since(st.ModTime()) > lockStaleAge {
			_ = os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another crm process is migrating the database (lock %s), try again in a minute", lock)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Backup is one automatic pre-migration copy of a database.
type Backup struct {
	Path    string // full path of the copy
	Name    string // file name
	Version int    // the schema version the database had when the copy was taken
	Stamp   string // YYYYMMDD-HHMMSS
	Size    int64  // bytes
}

// ListBackups lists the pre-migration backups of the database at dbPath, newest first. It reads
// <data dir>/backups and never fails: a missing folder is an empty list.
func ListBackups(dbPath string) []Backup {
	dir := filepath.Join(filepath.Dir(dbPath), "backups")
	re := regexp.MustCompile("^" + regexp.QuoteMeta(baseName(dbPath)) + `-pre-v(\d+)-(\d{8}-\d{6})(?:-(\d+))?\.db$`)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type item struct {
		b Backup
		n int
	}
	var items []item
	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		n := 1
		if m[3] != "" {
			n, _ = strconv.Atoi(m[3])
		}
		var size int64
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		items = append(items, item{Backup{Path: filepath.Join(dir, e.Name()), Name: e.Name(), Version: v, Stamp: m[2], Size: size}, n})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].b.Stamp != items[j].b.Stamp {
			return items[i].b.Stamp > items[j].b.Stamp
		}
		return items[i].n > items[j].n
	})
	out := make([]Backup, len(items))
	for i, it := range items {
		out[i] = it.b
	}
	return out
}
