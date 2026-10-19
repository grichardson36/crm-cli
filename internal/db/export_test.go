package db

import "database/sql"

// OpenUpTo opens a database and applies migrations only up to the given version, so a test can
// check what an earlier migration did before later ones changed the schema.
func OpenUpTo(path string, version int) (*sql.DB, error) {
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	if err := setPragmas(d); err != nil {
		d.Close()
		return nil, err
	}
	if err := runMigrations(d, version); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}
