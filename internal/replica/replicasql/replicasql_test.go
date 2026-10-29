package replicasql_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/replica"
	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func enable(t *testing.T, d *sql.DB) *replicasql.Adapter {
	t.Helper()
	ctx := context.Background()
	_, err := replicasql.Enable(ctx, d, replicasql.DefaultRegistry())
	require.NoError(t, err)
	a, err := replicasql.Open(ctx, d, replicasql.DefaultRegistry())
	require.NoError(t, err)
	return a
}

func mustExec(t *testing.T, d *sql.DB, q string, args ...any) {
	t.Helper()
	_, err := d.Exec(q, args...)
	require.NoError(t, err, q)
}

func count(t *testing.T, d *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, d.QueryRow(q, args...).Scan(&n), q)
	return n
}

func str(t *testing.T, d *sql.DB, q string, args ...any) string {
	t.Helper()
	var s sql.NullString
	require.NoError(t, d.QueryRow(q, args...).Scan(&s), q)
	return s.String
}

// syncOnce moves everything from one adapter to another.
func syncOnce(t *testing.T, from, to *replicasql.Adapter) replica.BatchResult {
	t.Helper()
	ctx := context.Background()
	cur, err := to.Vector(ctx)
	require.NoError(t, err)
	cs, err := from.Pull(ctx, cur, 100000)
	require.NoError(t, err)
	res, err := to.ApplyBatch(ctx, cs)
	require.NoError(t, err)
	require.Empty(t, res.Rejected)
	return res
}

func TestEnableBaselinesWithoutTouchingRows(t *testing.T) {
	d := newDB(t)
	mustExec(t, d, `INSERT INTO people (uuid, first_name, last_name, notes) VALUES ('p1', 'Ann', 'Lee', 'hello')`)
	mustExec(t, d, `INSERT INTO people (uuid, first_name) VALUES ('p2', 'Bob')`)
	mustExec(t, d, `INSERT INTO tags (name) VALUES ('vip')`)
	mustExec(t, d, `INSERT INTO person_emails (person_id, email) VALUES (1, 'ann@example.com')`)
	before := str(t, d, `SELECT group_concat(id || first_name || COALESCE(last_name,'') || updated_at, '|') FROM people`)
	res, err := replicasql.Enable(context.Background(), d, replicasql.DefaultRegistry())
	require.NoError(t, err)
	assert.NotEmpty(t, res.Device)
	assert.Greater(t, res.Baseline, int64(5))
	assert.Equal(t, before, str(t, d, `SELECT group_concat(id || first_name || COALESCE(last_name,'') || updated_at, '|') FROM people`))
	assert.Equal(t, `"Ann"`, str(t, d, `SELECT value FROM replica_changes WHERE collection='people' AND key='p1' AND field='first_name'`))
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM replica_changes WHERE collection='people' AND key='p2' AND field='last_name'`), "NULL columns are not written")
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM person_emails WHERE uuid IS NOT NULL`), "contact methods got a uuid")
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM replica_changes WHERE collection='tags' AND field='name'`))
	// Enabling again keeps the identity and adds nothing.
	again, err := replicasql.Enable(context.Background(), d, replicasql.DefaultRegistry())
	require.NoError(t, err)
	assert.Equal(t, res.Device, again.Device)
	assert.True(t, again.Already)
	assert.Zero(t, again.Baseline)
}

func TestTriggersCaptureInsertUpdateDelete(t *testing.T) {
	d := newDB(t)
	enable(t, d)
	dev := str(t, d, `SELECT device FROM replica_clock`)
	mustExec(t, d, `INSERT INTO people (uuid, first_name, notes) VALUES ('p1', 'Ann', 'n')`)
	assert.Equal(t, `"Ann"`, str(t, d, `SELECT value FROM replica_changes WHERE key='p1' AND field='first_name' AND device=?`, dev))
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM replica_changes WHERE key='p1' AND field='last_name'`))
	assert.NotEmpty(t, str(t, d, `SELECT seen FROM replica_changes WHERE key='p1' AND field='notes'`), "keep-both fields carry the writer's vector")
	seq1 := count(t, d, `SELECT MAX(seq) FROM replica_changes`)
	mustExec(t, d, `UPDATE people SET last_name = 'Lee', first_name = 'Ann' WHERE uuid='p1'`)
	assert.Equal(t, `"Lee"`, str(t, d, `SELECT value FROM replica_changes WHERE key='p1' AND field='last_name'`))
	assert.Equal(t, 1, count(t, d, `SELECT COUNT(*) FROM replica_changes WHERE seq > ? `, seq1), "only the changed column is captured")
	mustExec(t, d, `UPDATE people SET last_name = NULL WHERE uuid='p1'`)
	assert.Equal(t, "unset", str(t, d, `SELECT op FROM replica_changes WHERE key='p1' AND field='last_name'`))
	mustExec(t, d, `UPDATE people SET updated_at = datetime('now') WHERE uuid='p1'`)
	assert.Equal(t, seq1+2, count(t, d, `SELECT MAX(seq) FROM replica_changes`), "updated_at is not synced")
	mustExec(t, d, `DELETE FROM people WHERE uuid='p1'`)
	assert.Equal(t, "delete", str(t, d, `SELECT op FROM replica_changes WHERE key='p1' AND field=''`))
	mustExec(t, d, `INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann again')`)
	assert.Equal(t, "restore", str(t, d, `SELECT op FROM replica_changes WHERE key='p1' AND field=''`), "re-creating a deleted record restores it")
	// Sequence numbers are unique.
	assert.Equal(t, count(t, d, `SELECT COUNT(*) FROM replica_changes`), count(t, d, `SELECT COUNT(DISTINCT device||seq) FROM replica_changes`))
	assert.Equal(t, count(t, d, `SELECT MAX(seq) FROM replica_changes`), count(t, d, `SELECT seq FROM replica_vector WHERE device=(SELECT device FROM replica_clock)`))
}

func TestCaptureHappensInTheSameTransaction(t *testing.T) {
	d := newDB(t)
	enable(t, d)
	seqBefore := count(t, d, `SELECT seq FROM replica_vector WHERE device=(SELECT device FROM replica_clock)`)
	tx, err := d.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO people (uuid, first_name) VALUES ('p9', 'Zed')`)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	assert.Equal(t, 0, count(t, d, `SELECT COUNT(*) FROM replica_changes WHERE key='p9'`))
	assert.Equal(t, seqBefore, count(t, d, `SELECT seq FROM replica_vector WHERE device=(SELECT device FROM replica_clock)`))
}

func TestRemoteApplyDoesNotReEmit(t *testing.T) {
	a := enable(t, newDB(t))
	b := enable(t, newDB(t))
	ownBefore := count(t, b.DB(), `SELECT COUNT(*) FROM replica_changes WHERE device=?`, b.Device())
	mustExec(t, a.DB(), `INSERT INTO people (uuid, first_name, last_name) VALUES ('p1', 'Ann', 'Lee')`)
	syncOnce(t, a, b)
	assert.Equal(t, "Ann", str(t, b.DB(), `SELECT first_name FROM people WHERE uuid='p1'`))
	assert.Equal(t, ownBefore, count(t, b.DB(), `SELECT COUNT(*) FROM replica_changes WHERE device=?`, b.Device()), "b wrote nothing of its own")
	assert.Zero(t, count(t, b.DB(), `SELECT applying FROM replica_clock`))
	// Built-in rows have the same name on both devices, so they are the same record.
	assert.Zero(t, count(t, b.DB(), `SELECT COUNT(*) FROM replica_pending`))
	assert.Equal(t, count(t, a.DB(), `SELECT COUNT(*) FROM vocabulary`), count(t, b.DB(), `SELECT COUNT(*) FROM vocabulary`))
}

func TestLinksResolveThroughUUIDs(t *testing.T) {
	a := enable(t, newDB(t))
	b := enable(t, newDB(t))
	// Different row ids on b, so integer ids must not leak.
	mustExec(t, b.DB(), `INSERT INTO people (uuid, first_name) VALUES ('other', 'Pad')`)
	mustExec(t, b.DB(), `INSERT INTO organizations (uuid, name) VALUES ('other-org', 'Pad Org')`)
	mustExec(t, a.DB(), `INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	mustExec(t, a.DB(), `INSERT INTO organizations (uuid, name) VALUES ('o1', 'Acme')`)
	mustExec(t, a.DB(), `INSERT INTO membership (uuid, person_id, org_id, role) SELECT 'm1', p.id, o.id, 'CEO' FROM people p, organizations o WHERE p.uuid='p1' AND o.uuid='o1'`)
	mustExec(t, a.DB(), `INSERT INTO tags (name, uuid) VALUES ('vip', 't1')`)
	mustExec(t, a.DB(), `INSERT INTO taggings (tag_id, entity_type, entity_id) SELECT t.id, 'person', p.id FROM tags t, people p WHERE t.uuid='t1' AND p.uuid='p1'`)
	mustExec(t, a.DB(), `INSERT INTO tasks (uuid, title) VALUES ('k1', 'Call')`)
	mustExec(t, a.DB(), `INSERT INTO activity_target (uuid, source_kind, source_id, target_kind, target_id) SELECT 'a1', 'task', k.id, 'person', p.id FROM tasks k, people p WHERE k.uuid='k1' AND p.uuid='p1'`)
	syncOnce(t, a, b)
	assert.Equal(t, "CEO", str(t, b.DB(), `SELECT m.role FROM membership m JOIN people p ON p.id=m.person_id JOIN organizations o ON o.id=m.org_id WHERE p.uuid='p1' AND o.uuid='o1'`))
	assert.Equal(t, 1, count(t, b.DB(), `SELECT COUNT(*) FROM taggings g JOIN tags t ON t.id=g.tag_id JOIN people p ON p.id=g.entity_id WHERE g.entity_type='person' AND p.uuid='p1' AND t.uuid='t1'`))
	assert.Equal(t, 1, count(t, b.DB(), `SELECT COUNT(*) FROM activity_target x JOIN tasks k ON k.id=x.source_id JOIN people p ON p.id=x.target_id WHERE k.uuid='k1' AND p.uuid='p1'`))
	assert.Zero(t, count(t, b.DB(), `SELECT COUNT(*) FROM replica_pending`))
}

func TestChildArrivingBeforeParentWaits(t *testing.T) {
	ctx := context.Background()
	a := enable(t, newDB(t))
	b := enable(t, newDB(t))
	mustExec(t, a.DB(), `INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	mustExec(t, a.DB(), `INSERT INTO organizations (uuid, name) VALUES ('o1', 'Acme')`)
	mustExec(t, a.DB(), `INSERT INTO membership (uuid, person_id, org_id) SELECT 'm1', p.id, o.id FROM people p, organizations o`)
	snap, _ := a.Snapshot(ctx)
	var first, rest []replica.Change
	for _, c := range snap.Changes {
		if c.Collection == "membership" {
			first = append(first, c)
		} else {
			rest = append(rest, c)
		}
	}
	_, err := b.ApplyBatch(ctx, first)
	require.NoError(t, err)
	assert.Equal(t, 1, count(t, b.DB(), `SELECT COUNT(*) FROM replica_pending`))
	assert.Zero(t, count(t, b.DB(), `SELECT COUNT(*) FROM membership`))
	_, err = b.ApplyBatch(ctx, rest)
	require.NoError(t, err)
	assert.Zero(t, count(t, b.DB(), `SELECT COUNT(*) FROM replica_pending`))
	assert.Equal(t, 1, count(t, b.DB(), `SELECT COUNT(*) FROM membership`))
}

func TestRegistryCoversEveryTableOrExplainsWhy(t *testing.T) {
	d := newDB(t)
	rows, err := d.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '%\_fts%' ESCAPE '\' AND name NOT LIKE 'replica\_%' ESCAPE '\'`)
	require.NoError(t, err)
	defer rows.Close()
	synced := map[string]bool{}
	for _, tb := range replicasql.DefaultRegistry() {
		synced[tb.Name] = true
	}
	skipped := replicasql.NotCovered()
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		_, why := skipped[n]
		assert.True(t, synced[n] || why, "table %s is neither synced nor listed in NotCovered", n)
	}
}

func insertContacts(t *testing.T, d *sql.DB, from, n int) {
	t.Helper()
	tx, err := d.Begin()
	require.NoError(t, err)
	st, err := tx.Prepare(`INSERT INTO people (uuid, first_name, last_name, email, phone, title, company, location, notes, birthday_month, birthday_day, keep_in_touch, is_favorite)
		VALUES (?, ?, ?, ?, ?, 'Engineer', 'Acme', 'Austin', 'met at a conference', 3, 14, '3 mons', 1)`)
	require.NoError(t, err)
	for i := from; i < from+n; i++ {
		_, err := st.Exec(fmt.Sprintf("u-%06d", i), fmt.Sprintf("First%d", i), fmt.Sprintf("Last%d", i), fmt.Sprintf("p%d@example.com", i), fmt.Sprintf("555-%07d", i))
		require.NoError(t, err)
	}
	require.NoError(t, st.Close())
	require.NoError(t, tx.Commit())
}

// TestCaptureOverheadOn5000Contacts reports what capture costs. It only fails when the cost is
// absurd, the numbers are in the log (go test -v -run Overhead).
func TestCaptureOverheadOn5000Contacts(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	off := newDB(t)
	start := time.Now()
	insertContacts(t, off, 0, 5000)
	tOff := time.Since(start)

	on := newDB(t)
	enable(t, on)
	start = time.Now()
	insertContacts(t, on, 0, 5000)
	tOn := time.Since(start)

	base := newDB(t)
	insertContacts(t, base, 0, 5000)
	start = time.Now()
	res, err := replicasql.Enable(context.Background(), base, replicasql.DefaultRegistry())
	require.NoError(t, err)
	tBase := time.Since(start)

	start = time.Now()
	for i := 0; i < 500; i++ {
		mustExec(t, on, `UPDATE people SET title = ? WHERE uuid = ?`, fmt.Sprintf("T%d", i), fmt.Sprintf("u-%06d", i))
	}
	tEdit := time.Since(start) / 500

	start = time.Now()
	a, err := replicasql.Open(context.Background(), on, replicasql.DefaultRegistry())
	require.NoError(t, err)
	pulled := 0
	cur := replica.VersionVector{}
	for {
		cs, err := a.Pull(context.Background(), cur, 5000)
		require.NoError(t, err)
		if len(cs) == 0 {
			break
		}
		for _, c := range cs {
			cur[c.ID.Device] = max(cur[c.ID.Device], c.ID.Seq)
		}
		pulled += len(cs)
	}
	tPull := time.Since(start)
	t.Logf("insert 5000 contacts: sync off %v, sync on %v (x%.1f, +%v per contact)", tOff.Round(time.Millisecond), tOn.Round(time.Millisecond), float64(tOn)/float64(tOff), ((tOn - tOff) / 5000).Round(time.Microsecond))
	t.Logf("first enable on 5000 contacts: %v, %d baseline changes", tBase.Round(time.Millisecond), res.Baseline)
	t.Logf("single field edit with capture: %v each (average of 500)", tEdit.Round(time.Microsecond))
	t.Logf("pull of %d changes in pages of 5000: %v", pulled, tPull.Round(time.Millisecond))
	assert.Less(t, tOn, 120*time.Second)
}
