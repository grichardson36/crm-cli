package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// LatestVersion is the schema version this build expects (the number of embedded migrations).
func LatestVersion() int {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			n++
		}
	}
	return n
}

// OpenReadOnly opens an existing database without creating it or running migrations. Use it to
// inspect a database that may belong to another version or to another program.
func OpenReadOnly(dbPath string) (*sql.DB, error) {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("stat database: %w", err)
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows drive path: C:/x becomes /C:/x
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro"}
	d, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open database read-only: %w", err)
	}
	d.SetMaxOpenConns(1)
	if _, err := d.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		d.Close()
		return nil, fmt.Errorf("open database read-only: %w", err)
	}
	return d, nil
}

// SchemaVersion returns PRAGMA user_version.
func SchemaVersion(d *sql.DB) (int, error) {
	var v int
	if err := d.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("get user_version: %w", err)
	}
	return v, nil
}

// Info is a read-only summary of a database file.
type Info struct {
	SchemaVersion int
	IsCRM         bool
	DeviceID      string
	Counts        map[string]int
}

// countTables are the entity tables counted by Inspect (archived rows excluded).
var countTables = []string{"people", "organizations", "interactions", "deals", "tasks", "events"}

// Inspect opens the database read-only and summarizes it. It never creates or migrates.
func Inspect(dbPath string) (*Info, error) {
	d, err := OpenReadOnly(dbPath)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return InspectDB(d)
}

// InspectDB summarizes an already open database.
func InspectDB(d *sql.DB) (*Info, error) {
	v, err := SchemaVersion(d)
	if err != nil {
		return nil, err
	}
	info := &Info{SchemaVersion: v, IsCRM: IsCRMDatabase(d), Counts: map[string]int{}}
	if !info.IsCRM {
		return info, nil
	}
	for _, t := range countTables {
		var n int
		// t comes from the fixed list above, never from user input.
		if err := d.QueryRow("SELECT COUNT(*) FROM " + t + " WHERE archived = 0").Scan(&n); err == nil {
			info.Counts[t] = n
		}
	}
	// Absent before schema version 4.
	_ = d.QueryRow("SELECT value FROM meta WHERE key = 'device_id'").Scan(&info.DeviceID)
	return info, nil
}

// IsCRMDatabase reports whether the database has the core CRM tables.
func IsCRMDatabase(d *sql.DB) bool {
	var n int
	q := "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('people', 'organizations', 'interactions')"
	if err := d.QueryRow(q).Scan(&n); err != nil {
		return false
	}
	return n == 3
}
