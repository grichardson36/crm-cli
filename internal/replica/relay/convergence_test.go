package relay_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/replica"
	"github.com/jdanielnd/crm-cli/internal/replica/relay"
	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// peopleState is a canonical text of the people table, for comparing devices.
func (n *node) peopleState() string {
	n.t.Helper()
	rows, err := n.d.Query(`SELECT uuid, first_name, COALESCE(last_name,''), COALESCE(title,''), COALESCE(notes,''), COALESCE(company,''), is_favorite FROM people ORDER BY uuid`)
	require.NoError(n.t, err)
	defer rows.Close()
	var out string
	for rows.Next() {
		var u, f, l, ti, no, co string
		var fav int
		require.NoError(n.t, rows.Scan(&u, &f, &l, &ti, &no, &co, &fav))
		out += u + "|" + f + "|" + l + "|" + ti + "|" + no + "|" + co + "|" + string(rune('0'+fav)) + "\n"
	}
	return out
}

func TestThreeDevicesConvergeWithRandomEdits(t *testing.T) {
	rel, a, b, c := newNode(t), newNode(t), newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	devs := []*node{a, b, c}
	peers := []replicasql.Device{join(t, rel, srv.URL, a, "a"), join(t, rel, srv.URL, b, "b"), join(t, rel, srv.URL, c, "c")}
	keys := []string{"k1", "k2", "k3", "k4"}
	fields := []string{"last_name", "title", "company"}
	step := 0
	for round := 0; round < 12; round++ {
		for i, d := range devs {
			k := keys[(round+i)%len(keys)]
			switch (round*3 + i) % 5 {
			case 0:
				d.exec(`INSERT OR IGNORE INTO people (uuid, first_name) VALUES (?, ?)`, k, "N"+k)
			case 1, 2:
				f := fields[(round+i)%len(fields)]
				step++
				d.exec(`UPDATE people SET `+f+` = ? WHERE uuid = ?`, "v"+string(rune('a'+step%26)), k)
			case 3:
				d.exec(`UPDATE people SET notes = ? WHERE uuid = ?`, "note "+string(rune('a'+step%26)), k)
			case 4:
				d.exec(`UPDATE people SET is_favorite = 1 - is_favorite WHERE uuid = ?`, k)
			}
			time.Sleep(2 * time.Millisecond)
		}
		// Only one device syncs each round, so edits pile up offline.
		syncWith(t, devs[round%3], peers[round%3])
	}
	for pass := 0; pass < 2; pass++ {
		for i, d := range devs {
			syncWith(t, d, peers[i])
		}
	}
	want := rel.peopleState()
	require.NotEmpty(t, want)
	for i, d := range devs {
		assert.Equal(t, want, d.peopleState(), "device %d", i)
	}
}

func TestDeviceThatIsOfflineCatchesUpAndHoldsBackPurge(t *testing.T) {
	ctx := context.Background()
	rel, a, b := newNode(t), newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	pb := join(t, rel, srv.URL, b, "phone")
	a.exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	syncWith(t, a, pa)
	syncWith(t, b, pb) // b has seen the record, then goes offline
	a.exec(`INSERT INTO people (uuid, first_name) VALUES ('p2', 'Bob')`)
	a.exec(`DELETE FROM people WHERE uuid='p1'`)
	syncWith(t, a, pa)

	n, err := rel.a.PurgeTombstones(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "b has not passed the delete")
	assert.Equal(t, 1, rel.count(`SELECT COUNT(*) FROM replica_changes WHERE key='p1' AND op='delete'`))

	// b comes back, catches up, and now every device has passed the delete.
	syncWith(t, b, pb)
	assert.Zero(t, b.count(`SELECT COUNT(*) FROM people WHERE uuid='p1'`))
	assert.Equal(t, "Bob", b.str(`SELECT first_name FROM people WHERE uuid='p2'`))
	syncWith(t, a, pa)
	n, err = rel.a.PurgeTombstones(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Zero(t, rel.count(`SELECT COUNT(*) FROM replica_changes WHERE key='p1'`), "the delete and the history of the record are gone")
	assert.Equal(t, 1, rel.count(`SELECT COUNT(*) FROM replica_changes WHERE key='p2' AND field='first_name'`))
}

func TestRetiredDeviceDoesNotHoldBackPurge(t *testing.T) {
	ctx := context.Background()
	rel, a, lost := newNode(t), newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	join(t, rel, srv.URL, lost, "old-phone") // enrolled, never syncs
	a.exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	a.exec(`DELETE FROM people WHERE uuid='p1'`)
	syncWith(t, a, pa)
	syncWith(t, a, pa)
	n, _ := rel.a.PurgeTombstones(ctx)
	assert.Zero(t, n, "the lost phone has not passed it")
	_, err := rel.a.Retire(ctx, "old-phone")
	require.NoError(t, err)
	n, err = rel.a.PurgeTombstones(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestNoPurgeWithoutOtherDevices(t *testing.T) {
	n := newNode(t)
	n.exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	n.exec(`DELETE FROM people WHERE uuid='p1'`)
	got, err := n.a.PurgeTombstones(context.Background())
	require.NoError(t, err)
	assert.Zero(t, got)
}

func TestParkedRecordIsNotPurged(t *testing.T) {
	ctx := context.Background()
	rel, a, b := newNode(t), newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	pb := join(t, rel, srv.URL, b, "phone")
	a.exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	syncWith(t, a, pa)
	syncWith(t, b, pb)
	a.exec(`DELETE FROM people WHERE uuid='p1'`)
	b.exec(`UPDATE people SET title = 'x' WHERE uuid='p1'`)
	for i := 0; i < 2; i++ {
		syncWith(t, a, pa)
		syncWith(t, b, pb)
	}
	n, err := rel.a.PurgeTombstones(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, "x", rel.str(`SELECT title FROM people WHERE uuid='p1'`))
}

func TestSchemaMismatchKeepsUnknownFieldsAndAppliesThemAfterUpgrade(t *testing.T) {
	ctx := context.Background()
	rel, old := newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	po := join(t, rel, srv.URL, old, "old")
	// A newer device wrote a field and a table this build does not know.
	dev := "ee000000-0000-4000-8000-000000000001"
	ts := time.Now().Add(-time.Minute)
	future := []replica.Change{
		{ID: replica.ChangeID{Device: dev, Seq: 1}, Collection: "people", Key: "p9", Field: "first_name", Op: replica.OpSet, Value: []byte(`"Zoe"`), SchemaVersion: 9, HLC: replica.FormatHLC(ts, 0, dev)},
		{ID: replica.ChangeID{Device: dev, Seq: 2}, Collection: "people", Key: "p9", Field: "nickname", Op: replica.OpSet, Value: []byte(`"Z"`), SchemaVersion: 9, HLC: replica.FormatHLC(ts, 1, dev)},
		{ID: replica.ChangeID{Device: dev, Seq: 3}, Collection: "gadgets", Key: "g1", Field: "name", Op: replica.OpSet, Value: []byte(`"G"`), SchemaVersion: 9, HLC: replica.FormatHLC(ts, 2, dev)},
	}
	res, err := rel.a.ApplyBatch(ctx, future)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Unknown) // the relay is an old build too
	syncWith(t, old, po)
	assert.Equal(t, "Zoe", old.str(`SELECT first_name FROM people WHERE uuid='p9'`))
	assert.Equal(t, 2, old.count(`SELECT COUNT(*) FROM replica_changes WHERE status='unknown'`), "kept, not discarded")

	// The upgrade adds the column. Refresh (run on open) rebuilds the triggers and applies the field.
	old.exec(`ALTER TABLE people ADD COLUMN nickname TEXT`)
	require.NoError(t, replicasql.Refresh(ctx, old.d))
	assert.Equal(t, "Z", old.str(`SELECT nickname FROM people WHERE uuid='p9'`))
	assert.Equal(t, 1, old.count(`SELECT COUNT(*) FROM replica_changes WHERE status='unknown'`), "the table nobody has still waits")
	// Local edits of the new column are captured from now on.
	old.exec(`UPDATE people SET nickname = 'ZZ' WHERE uuid='p9'`)
	assert.Equal(t, `"ZZ"`, old.str(`SELECT value FROM replica_changes WHERE key='p9' AND field='nickname' AND device=?`, old.a.Device()))
}

func TestRepushIsIdempotent(t *testing.T) {
	ctx := context.Background()
	rel, a := newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	a.exec(`INSERT INTO people (uuid, first_name, notes) VALUES ('p1', 'Ann', 'n')`)
	snap, err := a.a.Snapshot(ctx)
	require.NoError(t, err)
	var mine []replica.Change
	for _, c := range snap.Changes {
		if c.ID.Device == a.a.Device() {
			mine = append(mine, c)
		}
	}
	cl := relay.NewClient(pa.URL, pa.Token)
	first, err := cl.Push(ctx, relay.PushRequest{Changes: mine})
	require.NoError(t, err)
	assert.Equal(t, len(mine), first.Applied)
	before := rel.peopleState()
	again, err := cl.Push(ctx, relay.PushRequest{Changes: mine})
	require.NoError(t, err)
	assert.Zero(t, again.Applied)
	assert.Equal(t, len(mine), again.Duplicates)
	assert.Equal(t, before, rel.peopleState())
	assert.Equal(t, first.Vector, again.Vector)
}
