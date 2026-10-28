// Package doctor checks that a crm installation is healthy and says how to fix what is not.
// It only reads: the database is opened read-only, and the one file it writes (a probe in the
// data directory) is removed again.
package doctor

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Status of one check.
const (
	OK   = "ok"
	Warn = "warn"
	Fail = "fail"
)

// Check is the result of one check. Fix is a one-line instruction, empty when status is ok.
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

// Report is the full doctor result.
type Report struct {
	OK      bool           `json:"ok"`
	Summary map[string]int `json:"summary"`
	Checks  []Check        `json:"checks"`
}

// Options are the inputs of a run.
type Options struct {
	DBPath      string
	Version     string
	MCPTools    int
	SyncSources []string
	// FreeBytes returns the free bytes on the volume holding dir. Nil uses the OS.
	FreeBytes func(dir string) (uint64, error)
}

const (
	walWarnBytes   = 64 << 20
	diskWarnBytes  = 200 << 20
	diskFailBytes  = 10 << 20
	diskDBMultiple = 3
)

// Run executes every check.
func Run(ctx context.Context, opt Options) Report {
	if opt.FreeBytes == nil {
		opt.FreeBytes = freeBytes
	}
	var checks []Check
	add := func(c Check) { checks = append(checks, c) }

	add(Check{Name: "binary", Status: OK, Message: fmt.Sprintf("crm %s (%s/%s)", opt.Version, runtime.GOOS, runtime.GOARCH)})

	dbPath := opt.DBPath
	dir := filepath.Dir(dbPath)
	add(checkDataDir(dir))

	exists := false
	var size int64
	if st, err := os.Stat(dbPath); err == nil {
		exists, size = true, st.Size()
	}

	var ro *sql.DB
	if !exists {
		add(Check{Name: "database", Status: Fail, Message: "no database at " + dbPath, Fix: "run: crm init"})
	} else {
		var err error
		ro, err = db.OpenReadOnly(dbPath)
		switch {
		case err != nil:
			add(Check{Name: "database", Status: Fail, Message: "cannot open " + dbPath + ": " + err.Error(), Fix: "check the file permissions, or restore a backup"})
		case !db.IsCRMDatabase(ro):
			add(Check{Name: "database", Status: Fail, Message: dbPath + " is not a CRM database", Fix: "point crm at the right file with --db or CRM_DB"})
			ro.Close()
			ro = nil
		default:
			add(Check{Name: "database", Status: OK, Message: "opens read-only: " + dbPath})
		}
	}
	if ro != nil {
		defer ro.Close()
		add(checkIntegrity(ro))
		add(checkSchema(ro))
		add(checkLinks(ctx, ro))
	}

	add(checkWAL(dbPath))
	add(checkBackups(dir, dbPath))
	add(checkDisk(dir, size, opt.FreeBytes))
	add(checkCloudFolder(dbPath))
	add(checkSync(ctx, ro, opt.SyncSources))
	add(Check{Name: "mcp", Status: OK, Message: fmt.Sprintf("MCP server built in with %d tools (optional)", opt.MCPTools), Fix: ""})

	rep := Report{OK: true, Summary: map[string]int{OK: 0, Warn: 0, Fail: 0}, Checks: checks}
	for _, c := range checks {
		rep.Summary[c.Status]++
		if c.Status == Fail {
			rep.OK = false
		}
	}
	return rep
}

func backupStamp() string { return time.Now().Format("20060102") }

func checkDataDir(dir string) Check {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return Check{Name: "data_dir", Status: Fail, Message: dir + " does not exist", Fix: "run: crm init"}
	}
	probe, err := os.CreateTemp(dir, ".crm-doctor-*")
	if err != nil {
		return Check{Name: "data_dir", Status: Fail, Message: dir + " is not writable: " + err.Error(), Fix: "fix the directory permissions, or choose another location with --db or CRM_DB"}
	}
	name := probe.Name()
	probe.Close()
	_ = os.Remove(name)
	return Check{Name: "data_dir", Status: OK, Message: dir + " exists and is writable"}
}

func checkIntegrity(d *sql.DB) Check {
	rows, err := d.Query("PRAGMA integrity_check")
	if err != nil {
		return Check{Name: "integrity", Status: Fail, Message: "integrity_check could not run: " + err.Error(), Fix: "restore the latest backup, see: crm help install"}
	}
	defer rows.Close()
	var problems []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err == nil && line != "ok" {
			problems = append(problems, line)
		}
	}
	if len(problems) > 0 {
		if len(problems) > 3 {
			problems = append(problems[:3], "...")
		}
		return Check{Name: "integrity", Status: Fail, Message: "integrity_check: " + strings.Join(problems, "; "), Fix: "restore the latest backup, or export with 'crm person list -f csv' before repairing"}
	}
	return Check{Name: "integrity", Status: OK, Message: "PRAGMA integrity_check: ok"}
}

func checkSchema(d *sql.DB) Check {
	v, err := db.SchemaVersion(d)
	if err != nil {
		return Check{Name: "schema", Status: Fail, Message: err.Error(), Fix: "restore the latest backup"}
	}
	latest := db.LatestVersion()
	switch {
	case v == latest:
		return Check{Name: "schema", Status: OK, Message: fmt.Sprintf("schema version %d is current", v)}
	case v < latest:
		return Check{Name: "schema", Status: Warn, Message: fmt.Sprintf("schema version %d, this build expects %d", v, latest), Fix: "run any crm command: the database is backed up to the backups folder and migrated automatically"}
	default:
		return Check{Name: "schema", Status: Fail, Message: fmt.Sprintf("schema version %d is newer than this build (%d)", v, latest), Fix: "install a newer crm binary: see INSTALL.md"}
	}
}

// checkLinks reports links that point at nothing: activity rows whose task, note or target is gone,
// merge aliases without a survivor, and foreign key violations.
func checkLinks(ctx context.Context, d *sql.DB) Check {
	v, err := db.SchemaVersion(d)
	if err != nil || v < 8 {
		return Check{Name: "links", Status: OK, Message: "links are checked after the upgrade to schema 8"}
	}
	p, err := repo.CheckLinks(ctx, d)
	if err != nil {
		return Check{Name: "links", Status: Fail, Message: err.Error(), Fix: "restore the latest backup"}
	}
	if p.Total() > 0 {
		return Check{Name: "links", Status: Fail,
			Message: fmt.Sprintf("%d activity links and %d merge aliases point at nothing, %d foreign key violations", p.ActivityOrphans, p.AliasOrphans, p.ForeignKeys),
			Fix:     "restore the latest backup, or delete the orphan rows from activity_target and merge_alias"}
	}
	return Check{Name: "links", Status: OK, Message: "no orphan links"}
}

func checkWAL(dbPath string) Check {
	wal, err := os.Stat(dbPath + "-wal")
	if err != nil {
		return Check{Name: "wal", Status: OK, Message: "no write-ahead log pending"}
	}
	if wal.Size() > walWarnBytes {
		return Check{Name: "wal", Status: Warn, Message: fmt.Sprintf("write-ahead log is %d MB", wal.Size()>>20), Fix: "close every crm process (TUI, MCP server) so SQLite can checkpoint, then run any crm command"}
	}
	return Check{Name: "wal", Status: OK, Message: fmt.Sprintf("write-ahead log is %d KB", wal.Size()>>10)}
}

// checkBackups looks for backups in <data dir>/backups and for *backup*.db / *.bak files next to the database.
func checkBackups(dir, dbPath string) Check {
	var found []string
	if entries, err := os.ReadDir(filepath.Join(dir, "backups")); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				found = append(found, filepath.Join(dir, "backups", e.Name()))
			}
		}
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			n := strings.ToLower(e.Name())
			full := filepath.Join(dir, e.Name())
			if e.IsDir() || full == dbPath {
				continue
			}
			if strings.HasSuffix(n, ".bak") || (strings.Contains(n, "backup") && strings.HasSuffix(n, ".db")) {
				found = append(found, full)
			}
		}
	}
	if len(found) == 0 {
		return Check{Name: "backups", Status: Warn, Message: "no backup found in " + filepath.Join(dir, "backups"), Fix: "run: crm backup " + filepath.Join(dir, "backups", "crm-"+backupStamp()+".db")}
	}
	return Check{Name: "backups", Status: OK, Message: fmt.Sprintf("%d backup file(s) found", len(found))}
}

func checkDisk(dir string, dbSize int64, free func(string) (uint64, error)) Check {
	n, err := free(dir)
	if err != nil {
		return Check{Name: "disk_space", Status: OK, Message: "free space not available on this system"}
	}
	need := uint64(dbSize) * diskDBMultiple
	if need < diskWarnBytes {
		need = diskWarnBytes
	}
	mb := n >> 20
	switch {
	case n < diskFailBytes:
		return Check{Name: "disk_space", Status: Fail, Message: fmt.Sprintf("only %d MB free", mb), Fix: "free disk space: SQLite cannot write safely when the disk is full"}
	case n < need:
		return Check{Name: "disk_space", Status: Warn, Message: fmt.Sprintf("%d MB free", mb), Fix: "free disk space; backups and migrations need room for a full copy of the database"}
	}
	return Check{Name: "disk_space", Status: OK, Message: fmt.Sprintf("%d MB free", mb)}
}

// cloudMarkers are path components of folders that sync files in the background.
var cloudMarkers = []struct{ marker, name string }{
	{"onedrive", "OneDrive"},
	{"dropbox", "Dropbox"},
	{"icloud", "iCloud"},
	{"mobile documents", "iCloud"},
	{"com~apple~clouddocs", "iCloud"},
}

func checkCloudFolder(dbPath string) Check {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		abs = dbPath
	}
	for _, part := range strings.FieldsFunc(strings.ToLower(filepath.ToSlash(abs)), func(r rune) bool { return r == '/' }) {
		for _, m := range cloudMarkers {
			if strings.HasPrefix(part, m.marker) {
				return Check{Name: "cloud_folder", Status: Warn,
					Message: "the database is inside a " + m.name + " folder: its sync client can copy the database and its -wal file at different moments, which corrupts SQLite",
					Fix:     "move the database outside the synced folder (set CRM_DB), and write backups into the synced folder with 'crm backup'"}
			}
		}
	}
	return Check{Name: "cloud_folder", Status: OK, Message: "the database is not in a OneDrive, Dropbox or iCloud folder"}
}

func checkSync(ctx context.Context, ro *sql.DB, sources []string) Check {
	msg := "no sync sources are registered in this build"
	if len(sources) > 0 {
		msg = fmt.Sprintf("%d sync source(s): %s", len(sources), strings.Join(sources, ", "))
	}
	if ro == nil {
		return Check{Name: "sync", Status: OK, Message: msg}
	}
	n, err := repo.NewSyncRepo(ro).InboxCount(ctx, model.InboxPending)
	if err != nil {
		return Check{Name: "sync", Status: OK, Message: msg}
	}
	if n > 0 {
		return Check{Name: "sync", Status: Warn, Message: fmt.Sprintf("%s; %d record(s) wait in the sync inbox", msg, n), Fix: "review them: crm sync inbox list"}
	}
	return Check{Name: "sync", Status: OK, Message: msg + "; sync inbox is empty"}
}
