package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// openAtV7 builds a database as it looked after migration 007 and loads sample data that uses
// every column and table 008 moves.
func openAtV7(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v7.db")
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = raw.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	for _, f := range []string{"001_initial.sql", "002_dex_parity.sql", "003_dedupe.sql", "004_kit_notetypes_archive.sql", "005_external_ids_sync.sql", "006_dav.sql", "007_task_tags.sql"} {
		_, err = raw.Exec(migrationSection(t, f, "up"))
		require.NoError(t, err, f)
	}
	for _, s := range []string{
		`INSERT INTO organizations (uuid, name) VALUES ('o1', 'Acme'), ('o2', 'Globex')`,
		`INSERT INTO people (uuid, first_name, last_name, company, title, org_id, keep_in_touch) VALUES
			('p1', 'Ann', 'Lee', 'Acme Inc', 'CTO', 1, '1 mon'),
			('p2', 'Bob', 'Roe', NULL, NULL, NULL, NULL),
			('p3', 'Cy', 'Poe', NULL, NULL, 2, NULL),
			('p4', 'Dee', 'Fox', NULL, NULL, NULL, NULL)`,
		`INSERT INTO deals (uuid, title, stage, person_id, org_id) VALUES ('d1', 'Big deal', 'won', 1, 1), ('d2', 'Small', 'lead', NULL, 2)`,
		`INSERT INTO tasks (uuid, title, person_id, deal_id) VALUES ('t1', 'Call Ann', 1, 1), ('t2', 'Free', NULL, NULL)`,
		`INSERT INTO task_people (task_id, person_id) VALUES (1, 1), (1, 2), (2, 3)`,
		`INSERT INTO interactions (uuid, type, subject) VALUES ('i1', 'call', 'hello'), ('i2', 'note', 'later')`,
		`INSERT INTO interaction_people (interaction_id, person_id) VALUES (1, 1), (1, 2), (2, 1)`,
		`INSERT INTO relationships (person_id, related_person_id, type, notes) VALUES
			(1, 2, 'colleague', 'same team'), (2, 1, 'colleague', NULL), (3, 4, 'referred-by', NULL), (4, 3, 'mentor', NULL)`,
		// Dex related contacts: names and one Dex id. Ann lists Bob (by name) and Dee (by dex id), Bob lists Ann
		// again (the pair must be stored once), and a name nobody has stays unresolved.
		`INSERT INTO external_ids (uuid, entity, local_id, local_uuid, source, external_id) VALUES ('e4', 'person', 4, 'p4', 'dex', 'dex-dee-1')`,
		`INSERT INTO custom_fields (entity_type, entity_id, field_name, field_value) VALUES
			('person', 1, 'related_contacts', 'Bob Roe, dex-dee-1, Nobody Known'),
			('person', 2, 'related_contacts', 'ann lee')`,
		`PRAGMA user_version = 7`,
	} {
		_, err = raw.Exec(s)
		require.NoError(t, err, s)
	}
	require.NoError(t, raw.Close())
	return path
}

func scalar(t *testing.T, d *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, d.QueryRow(q, args...).Scan(&n), q)
	return n
}

func TestMigration008_MovesDataAndDropsOldColumns(t *testing.T) {
	d, err := db.Open(openAtV7(t))
	require.NoError(t, err)
	defer d.Close()

	assert.Equal(t, 14, scalar(t, d, `PRAGMA user_version`))

	// Memberships: people.org_id became a primary membership.
	assert.Equal(t, 2, scalar(t, d, `SELECT COUNT(*) FROM membership`))
	assert.Equal(t, 2, scalar(t, d, `SELECT COUNT(*) FROM membership WHERE is_primary = 1 AND deleted_at IS NULL AND started_on IS NULL AND ended_on IS NULL`))
	assert.Equal(t, 1, scalar(t, d, `SELECT org_id FROM membership WHERE person_id = 1`))
	assert.Equal(t, 2, scalar(t, d, `SELECT org_id FROM membership WHERE person_id = 3`))

	// Deals.
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM deal_person`))
	assert.Equal(t, 2, scalar(t, d, `SELECT COUNT(*) FROM deal_org WHERE is_primary = 1`))

	// Activities: tasks.person_id + task_people (one overlap), tasks.deal_id, interaction_people.
	assert.Equal(t, 3, scalar(t, d, `SELECT COUNT(*) FROM activity_target WHERE source_kind = 'task' AND target_kind = 'person'`), "task 1 to persons 1 and 2, task 2 to person 3, overlap stored once")
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM activity_target WHERE source_kind = 'task' AND target_kind = 'deal'`))
	assert.Equal(t, 3, scalar(t, d, `SELECT COUNT(*) FROM activity_target WHERE source_kind = 'interaction' AND target_kind = 'person'`))

	// Relationships: the two colleague rows are one symmetric pair, referred-by and mentor keep their direction.
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM person_relation pr JOIN relation_kind k ON k.id = pr.kind_id WHERE k.name = 'colleague'`))
	assert.Equal(t, 1, scalar(t, d, `SELECT a_id FROM person_relation pr JOIN relation_kind k ON k.id = pr.kind_id WHERE k.name = 'colleague'`), "symmetric pair stores the lower id first")
	assert.Equal(t, 3, scalar(t, d, `SELECT a_id FROM person_relation pr JOIN relation_kind k ON k.id = pr.kind_id WHERE k.name = 'referred-by'`))
	assert.Equal(t, 1, scalar(t, d, `SELECT phrase FROM relation_kind WHERE name = 'referred-by'`), "referred-by is a phrase kind")
	assert.Equal(t, 4, scalar(t, d, `SELECT a_id FROM person_relation pr JOIN relation_kind k ON k.id = pr.kind_id WHERE k.name = 'mentor'`))
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM person_relation WHERE notes = 'same team'`), "notes of the first duplicate are kept")

	// Dex related contacts: Ann-Bob (name, listed twice, one pair) and Ann-Dee (dex id). One name is unresolved.
	assert.Equal(t, 2, scalar(t, d, `SELECT COUNT(*) FROM person_relation pr JOIN relation_kind k ON k.id = pr.kind_id WHERE k.name = 'related'`))
	var report string
	require.NoError(t, d.QueryRow(`SELECT value FROM meta WHERE key = 'migration_008'`).Scan(&report))
	assert.Contains(t, report, "related_contacts_items=4")
	assert.Contains(t, report, "related_contacts_resolved=3")
	assert.Contains(t, report, "related_contacts_unresolved=1")
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM custom_fields WHERE field_name = 'related_contacts' AND entity_id = 1`), "the custom field is kept")

	// Old columns and tables are gone, other columns and ids are kept.
	for _, c := range []struct{ table, col string }{{"people", "org_id"}, {"deals", "person_id"}, {"deals", "org_id"}, {"tasks", "person_id"}, {"tasks", "deal_id"}} {
		assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, c.table, c.col), c.table+"."+c.col)
	}
	for _, tbl := range []string{"relationships", "task_people", "interaction_people"} {
		assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tbl), tbl)
	}
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM people WHERE id = 1 AND uuid = 'p1' AND company = 'Acme Inc' AND title = 'CTO' AND keep_in_touch = '1 mon'`))
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM deals WHERE id = 1 AND stage = 'won'`))
	assert.Equal(t, 4, scalar(t, d, `SELECT COUNT(*) FROM people`))

	// Integrity and foreign keys are clean, foreign keys are back on.
	var ok string
	require.NoError(t, d.QueryRow(`PRAGMA integrity_check`).Scan(&ok))
	assert.Equal(t, "ok", ok)
	rows, err := d.Query(`PRAGMA foreign_key_check`)
	require.NoError(t, err)
	assert.False(t, rows.Next(), "no foreign key violations")
	rows.Close()
	assert.Equal(t, 1, scalar(t, d, `PRAGMA foreign_keys`))

	// FTS still follows edits, the keep-in-touch view still works through activity_target.
	_, err = d.Exec(`UPDATE people SET first_name = 'Anna' WHERE id = 1`)
	require.NoError(t, err)
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM people_fts WHERE people_fts MATCH 'Anna'`))
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM people_fts WHERE people_fts MATCH 'Ann'`))
	assert.Equal(t, 1, scalar(t, d, `SELECT COUNT(*) FROM keep_in_touch_status WHERE person_id = 1 AND last_seen IS NOT NULL`))

	// The id counters did not go backward: the next person is id 5.
	res, err := d.Exec(`INSERT INTO people (uuid, first_name) VALUES ('p5', 'New')`)
	require.NoError(t, err)
	id, _ := res.LastInsertId()
	assert.Equal(t, int64(5), id)
}

func TestMigration008_Constraints(t *testing.T) {
	d, err := db.Open(openAtV7(t))
	require.NoError(t, err)
	defer d.Close()

	// One live primary membership per person.
	_, err = d.Exec(`INSERT INTO membership (uuid, person_id, org_id, is_primary) VALUES ('m9', 1, 2, 1)`)
	require.Error(t, err, "person 1 already has a primary")
	_, err = d.Exec(`INSERT INTO membership (uuid, person_id, org_id, is_primary) VALUES ('m9', 1, 2, 0)`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO membership (uuid, person_id, org_id, is_primary) VALUES ('m10', 1, 2, 0)`)
	require.NoError(t, err, "repeated and overlapping memberships are allowed")
	_, err = d.Exec(`UPDATE membership SET deleted_at = datetime('now') WHERE person_id = 1 AND is_primary = 1`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO membership (uuid, person_id, org_id, is_primary) VALUES ('m11', 1, 1, 1)`)
	require.NoError(t, err, "a soft-deleted primary frees the slot")
	_, err = d.Exec(`INSERT INTO membership (uuid, person_id, org_id, started_on, ended_on) VALUES ('m12', 2, 1, '2021-01-01', '2020-01-01')`)
	require.Error(t, err, "dates must be in order")

	// One live primary organization per deal.
	_, err = d.Exec(`INSERT INTO deal_org (uuid, deal_id, org_id, is_primary) VALUES ('x1', 1, 2, 1)`)
	require.Error(t, err)

	// Relation kinds: kind type must match the table, no self links, no duplicate live pair.
	_, err = d.Exec(`INSERT INTO org_relation (uuid, a_id, b_id, kind_id) VALUES ('r1', 1, 2, (SELECT id FROM relation_kind WHERE name = 'friend'))`)
	require.Error(t, err, "a person kind is not valid for organizations")
	_, err = d.Exec(`INSERT INTO org_relation (uuid, a_id, b_id, kind_id) VALUES ('r1', 1, 2, (SELECT id FROM relation_kind WHERE object_type = 'organization' AND name = 'parent'))`)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO org_relation (uuid, a_id, b_id, kind_id) VALUES ('r2', 1, 2, (SELECT id FROM relation_kind WHERE object_type = 'organization' AND name = 'parent'))`)
	require.Error(t, err, "duplicate live pair")
	_, err = d.Exec(`INSERT INTO person_relation (uuid, a_id, b_id, kind_id) VALUES ('r3', 1, 1, 1)`)
	require.Error(t, err, "no self link")

	// Seeded kinds.
	assert.Equal(t, 10, scalar(t, d, `SELECT COUNT(*) FROM relation_kind WHERE builtin = 1`))
	assert.Equal(t, 5, scalar(t, d, `SELECT COUNT(*) FROM relation_kind WHERE symmetric = 1`))
	_, err = d.Exec(`INSERT INTO relation_kind (uuid, object_type, name, inverse_name, symmetric) VALUES ('k1', 'person', 'rival', 'foe', 1)`)
	require.Error(t, err, "symmetric kinds have one name")
	_, err = d.Exec(`INSERT INTO relation_kind (uuid, object_type, name, inverse_name) VALUES ('k2', 'person', 'FRIEND', 'friend')`)
	require.Error(t, err, "names are unique without regard to case")
}

func TestMigration008_ActivityTargetHasNoOrphans(t *testing.T) {
	d, err := db.Open(openAtV7(t))
	require.NoError(t, err)
	defer d.Close()

	_, err = d.Exec(`INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES ('a1', 'task', 999, 'person', 1)`)
	require.Error(t, err, "missing source")
	_, err = d.Exec(`INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES ('a1', 'task', 1, 'organization', 999)`)
	require.Error(t, err, "missing target")
	_, err = d.Exec(`INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) VALUES ('a1', 'task', 1, 'organization', 1)`)
	require.NoError(t, err, "tasks can be about organizations")
	_, err = d.Exec(`UPDATE activity_target SET target_id = 999 WHERE uuid = 'a1'`)
	require.Error(t, err, "re-pointing to a missing target")

	// Hard deleting a target or a source removes its rows.
	before := scalar(t, d, `SELECT COUNT(*) FROM activity_target`)
	_, err = d.Exec(`DELETE FROM organizations WHERE id = 1`)
	require.NoError(t, err)
	assert.Equal(t, before-1, scalar(t, d, `SELECT COUNT(*) FROM activity_target`))
	_, err = d.Exec(`DELETE FROM interactions WHERE id = 1`)
	require.NoError(t, err)
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM activity_target WHERE source_kind = 'interaction' AND source_id = 1`))
	_, err = d.Exec(`DELETE FROM people WHERE id = 1`)
	require.NoError(t, err)
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM activity_target WHERE target_kind = 'person' AND target_id = 1`))
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM membership WHERE person_id = 1`), "memberships cascade")
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM person_relation WHERE a_id = 1 OR b_id = 1`), "person links cascade")
}

func TestMigration008_FreshDatabase(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	assert.Equal(t, 14, scalar(t, d, `PRAGMA user_version`))
	assert.Equal(t, 10, scalar(t, d, `SELECT COUNT(*) FROM relation_kind`))
	assert.Equal(t, 0, scalar(t, d, `SELECT COUNT(*) FROM membership`))
	var ok string
	require.NoError(t, d.QueryRow(`PRAGMA integrity_check`).Scan(&ok))
	assert.Equal(t, "ok", ok)
	rows, err := d.Query(`PRAGMA foreign_key_check`)
	require.NoError(t, err)
	assert.False(t, rows.Next())
	rows.Close()
	assert.Equal(t, 1, scalar(t, d, `PRAGMA foreign_keys`))
}
