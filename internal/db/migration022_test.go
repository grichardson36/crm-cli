package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ftsHits(t *testing.T, d *sql.DB, q string) int {
	t.Helper()
	return scalar(t, d, `SELECT COUNT(*) FROM people WHERE id IN (SELECT rowid FROM people_fts WHERE people_fts MATCH ?)`, q)
}

// Migration 022 (database version 21) adds people.middle_name, puts it in the people search index and
// moves the additional name a phone already sent (kept aside in dav_state.extra_vcard) into the new field.
// The file is named 022 because the file named 017 does not exist, the version is the position of the file.
func TestMigration022_AddsMiddleNameKeepsPhoneValueAndIndexesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v20.db")
	d, err := db.OpenUpTo(path, 20)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO people (id, uuid, first_name, last_name) VALUES
		(1, 'p1', 'Ann', 'Smith'), (2, 'p2', 'Bob', 'Jones'), (3, 'p3', 'Cy', 'Lee')`)
	require.NoError(t, err)
	// Person 1: the phone sent a middle name, with a comma. Person 2: only a prefix. Person 3: no state.
	_, err = d.Exec(`INSERT INTO dav_state (person_id, base_json, extra_vcard) VALUES
		(1, '{"first":"Ann","last":"Smith"}', 'BEGIN:VCARD' || char(13) || char(10) || 'VERSION:3.0' || char(13) || char(10) || 'N:Smith;Ann;Marie\,Lou;Dr.;' || char(13) || char(10) || 'END:VCARD' || char(13) || char(10)),
		(2, '{"first":"Bob"}', 'BEGIN:VCARD' || char(13) || char(10) || 'VERSION:3.0' || char(13) || char(10) || 'N:Jones;Bob;;Mr.;' || char(13) || char(10) || 'END:VCARD' || char(13) || char(10))`)
	require.NoError(t, err)
	require.NoError(t, d.Close())

	d, err = db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, db.LatestVersion(), scalar(t, d, `PRAGMA user_version`))

	var mid sql.NullString
	require.NoError(t, d.QueryRow(`SELECT middle_name FROM people WHERE id = 1`).Scan(&mid))
	assert.Equal(t, "Marie,Lou", mid.String, "phone value moved into the field")
	require.NoError(t, d.QueryRow(`SELECT middle_name FROM people WHERE id = 2`).Scan(&mid))
	assert.False(t, mid.Valid, "a card with a prefix only has no middle name")
	require.NoError(t, d.QueryRow(`SELECT middle_name FROM people WHERE id = 3`).Scan(&mid))
	assert.False(t, mid.Valid)

	var base string
	require.NoError(t, d.QueryRow(`SELECT base_json FROM dav_state WHERE person_id = 1`).Scan(&base))
	assert.JSONEq(t, `{"first":"Ann","last":"Smith","middle":"Marie,Lou"}`, base, "the merge base knows it, so a later phone edit is not a conflict")

	assert.Equal(t, 1, ftsHits(t, d, `"Marie"*`), "backfilled value is searchable")
	assert.Equal(t, 1, ftsHits(t, d, `"Smith"*`), "old index content survives")

	_, err = d.Exec(`UPDATE people SET middle_name = 'Quincy' WHERE id = 3`)
	require.NoError(t, err)
	assert.Equal(t, 1, ftsHits(t, d, `"Quincy"*`), "update trigger indexes it")
	_, err = d.Exec(`UPDATE people SET middle_name = 'Zed' WHERE id = 3`)
	require.NoError(t, err)
	assert.Equal(t, 0, ftsHits(t, d, `"Quincy"*`), "the old value leaves the index")
	assert.Equal(t, 1, ftsHits(t, d, `"Zed"*`))
}

func TestMigration022_DownRemovesColumnAndRestoresIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	defer d.Close()
	_, err = d.Exec(`INSERT INTO people (uuid, first_name, last_name, middle_name) VALUES ('p1', 'Ann', 'Smith', 'Marie')`)
	require.NoError(t, err)

	_, err = d.Exec(migrationSection(t, "022_person_middle_name.sql", "down"))
	require.NoError(t, err)

	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM pragma_table_info('people') WHERE name = 'middle_name'`))
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM pragma_table_info('people_fts') WHERE name = 'middle_name'`))
	_, err = d.Exec(`INSERT INTO people (uuid, first_name, last_name) VALUES ('p2', 'Bob', 'Jones')`)
	require.NoError(t, err)
	assert.Equal(t, 1, ftsHits(t, d, `"Jones"*`), "triggers work again without the column")
	assert.Equal(t, 1, ftsHits(t, d, `"Smith"*`))
	assert.Equal(t, 0, ftsHits(t, d, `"Marie"*`))
}
