package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open opens a SQLite database, sets pragmas, and runs migrations.
func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Set a single connection — SQLite doesn't benefit from a pool
	db.SetMaxOpenConns(1)

	if err := setPragmas(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}

	if err := migrateWithBackup(db, dbPath); err != nil {
		db.Close()
		var ns *NewerSchemaError
		if errors.As(err, &ns) {
			return nil, err
		}
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// With device sync on, keep the capture triggers in step with this build's registry.
	if err := replicasql.Refresh(context.Background(), db); err != nil {
		db.Close()
		return nil, fmt.Errorf("refresh sync triggers: %w", err)
	}

	return db, nil
}

func setPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA cache_size = -64000",
		"PRAGMA temp_store = MEMORY",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

// runMigrations applies the embedded migrations above the database's user_version. A limit above
// zero stops after that version, which lets tests build the database as it looked earlier.
func runMigrations(db *sql.DB, limit int) error {
	var currentVersion int
	err := db.QueryRow("PRAGMA user_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("get user_version: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Filter to .sql files only
	var sqlEntries []fs.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			sqlEntries = append(sqlEntries, entry)
		}
	}

	for i, entry := range sqlEntries {
		version := i + 1
		if version <= currentVersion {
			continue
		}
		if limit > 0 && version > limit {
			break
		}

		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		upSQL := extractUpSection(string(content))
		if upSQL == "" {
			return fmt.Errorf("migration %s: no -- up section found", entry.Name())
		}

		// A migration that rebuilds tables with foreign keys needs them off. The pragma is a no-op
		// inside a transaction, so it is switched here and foreign_key_check runs before the commit.
		fkOff := needsForeignKeysOff(string(content))
		if fkOff {
			if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
				return fmt.Errorf("migration %s: foreign keys off: %w", entry.Name(), err)
			}
		}
		err = applyMigration(db, version, entry.Name(), upSQL, fkOff)
		if fkOff {
			if _, ferr := db.Exec("PRAGMA foreign_keys = ON"); ferr != nil && err == nil {
				err = fmt.Errorf("migration %s: foreign keys on: %w", entry.Name(), ferr)
			}
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// needsForeignKeysOff reports whether the migration file asks for foreign keys to be off while it
// runs, with the line "-- foreign_keys: off" before "-- up".
func needsForeignKeysOff(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if t == "-- up" {
			return false
		}
		if t == "-- foreign_keys: off" {
			return true
		}
	}
	return false
}

func applyMigration(db *sql.DB, version int, name, upSQL string, checkFK bool) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction for migration %d: %w", version, err)
	}
	if _, err := tx.Exec(upSQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("execute migration %s: %w", name, err)
	}
	if checkFK {
		rows, err := tx.Query("PRAGMA foreign_key_check")
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s: foreign key check: %w", name, err)
		}
		bad := rows.Next()
		_ = rows.Close()
		if bad {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s left foreign key violations, nothing was changed", name)
		}
	}

	// PRAGMA user_version does not support ? placeholders in SQLite.
	// version is an internal loop counter, never from user input.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("set user_version to %d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", version, err)
	}
	return nil
}

func extractUpSection(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inUp := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "-- up" {
			inUp = true
			continue
		}
		if trimmed == "-- down" {
			break
		}
		if inUp {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
