package relay_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/replica"
	"github.com/jdanielnd/crm-cli/internal/replica/relay"
	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// node is one device: an in-memory database with sync on.
type node struct {
	t *testing.T
	d *sql.DB
	a *replicasql.Adapter
}

func newNode(t *testing.T) *node {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	_, err = replicasql.Enable(context.Background(), d, replicasql.DefaultRegistry())
	require.NoError(t, err)
	a, err := replicasql.Open(context.Background(), d, replicasql.DefaultRegistry())
	require.NoError(t, err)
	return &node{t: t, d: d, a: a}
}

func (n *node) exec(q string, args ...any) {
	n.t.Helper()
	_, err := n.d.Exec(q, args...)
	require.NoError(n.t, err, q)
}

func (n *node) str(q string, args ...any) string {
	n.t.Helper()
	var s sql.NullString
	err := n.d.QueryRow(q, args...).Scan(&s)
	if err != nil && err != sql.ErrNoRows {
		require.NoError(n.t, err, q)
	}
	return s.String
}

func (n *node) count(q string, args ...any) int {
	n.t.Helper()
	var c int
	require.NoError(n.t, n.d.QueryRow(q, args...).Scan(&c), q)
	return c
}

// join enrolls dev at the relay behind url and returns the peer row.
func join(t *testing.T, rel *node, url string, dev *node, name string) replicasql.Device {
	t.Helper()
	ctx := context.Background()
	tok, err := rel.a.CreateEnrollToken(ctx, name, time.Hour)
	require.NoError(t, err)
	resp, err := relay.NewClient(url, "").Enroll(ctx, relay.EnrollRequest{Token: tok, Device: dev.a.Device(), Name: name})
	require.NoError(t, err)
	require.NoError(t, dev.a.AddPeer(ctx, "relay", url, resp.Token, resp.Device))
	p, err := dev.a.FindDevice(ctx, "relay")
	require.NoError(t, err)
	return p
}

func syncWith(t *testing.T, n *node, peer replicasql.Device) relay.Report {
	t.Helper()
	rep, err := relay.Sync(context.Background(), n.a, peer, relay.Options{Pull: true, Push: true})
	require.NoError(t, err)
	require.Empty(t, rep.Rejected)
	return rep
}

func TestTwoDevicesThroughARealRelay(t *testing.T) {
	rel, a, b := newNode(t), newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	pb := join(t, rel, srv.URL, b, "phone")

	a.exec(`INSERT INTO people (uuid, first_name, last_name, notes) VALUES ('p1', 'Ann', 'Lee', 'base')`)
	a.exec(`INSERT INTO organizations (uuid, name) VALUES ('o1', 'Acme')`)
	a.exec(`INSERT INTO membership (uuid, person_id, org_id, role) SELECT 'm1', p.id, o.id, 'CEO' FROM people p, organizations o`)
	syncWith(t, a, pa)
	syncWith(t, b, pb)
	assert.Equal(t, "Ann", b.str(`SELECT first_name FROM people WHERE uuid='p1'`))
	assert.Equal(t, "CEO", b.str(`SELECT role FROM membership WHERE uuid='m1'`))
	assert.Equal(t, "Acme", rel.str(`SELECT name FROM organizations WHERE uuid='o1'`), "the relay keeps a full copy")

	// Offline edits on both devices touch different fields and merge per field.
	a.exec(`UPDATE people SET last_name = 'Leigh' WHERE uuid='p1'`)
	b.exec(`UPDATE people SET title = 'CTO' WHERE uuid='p1'`)
	syncWith(t, a, pa)
	syncWith(t, b, pb)
	syncWith(t, a, pa)
	for _, n := range []*node{a, b, rel} {
		assert.Equal(t, "Leigh", n.str(`SELECT last_name FROM people WHERE uuid='p1'`))
		assert.Equal(t, "CTO", n.str(`SELECT title FROM people WHERE uuid='p1'`))
	}

	// Concurrent edits to one text field keep both.
	a.exec(`UPDATE people SET notes = 'base, from laptop' WHERE uuid='p1'`)
	time.Sleep(3 * time.Millisecond)
	b.exec(`UPDATE people SET notes = 'base, from phone' WHERE uuid='p1'`)
	syncWith(t, a, pa)
	syncWith(t, b, pb)
	syncWith(t, a, pa)
	for _, n := range []*node{a, b} {
		got := n.str(`SELECT notes FROM people WHERE uuid='p1'`)
		assert.Contains(t, got, "base, from laptop")
		assert.Contains(t, got, "base, from phone")
	}
	assert.Equal(t, a.str(`SELECT notes FROM people WHERE uuid='p1'`), b.str(`SELECT notes FROM people WHERE uuid='p1'`))

	// Syncing again moves nothing.
	rep := syncWith(t, a, pa)
	assert.Zero(t, rep.Applied)
	assert.Zero(t, rep.PushApplied)
}

func TestDeleteVersusEditParksThenResolves(t *testing.T) {
	rel, a, b := newNode(t), newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	pb := join(t, rel, srv.URL, b, "phone")
	a.exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	syncWith(t, a, pa)
	syncWith(t, b, pb)

	a.exec(`DELETE FROM people WHERE uuid='p1'`)
	b.exec(`UPDATE people SET title = 'Boss' WHERE uuid='p1'`)
	syncWith(t, a, pa)
	syncWith(t, b, pb)
	syncWith(t, a, pa)
	for _, n := range []*node{a, b} {
		assert.Equal(t, 1, n.count(`SELECT COUNT(*) FROM people WHERE uuid='p1'`), "the record is visible with the edit while it waits")
		assert.Equal(t, "Boss", n.str(`SELECT title FROM people WHERE uuid='p1'`))
		rs, err := n.a.Reviews(context.Background(), false)
		require.NoError(t, err)
		require.Len(t, rs, 1)
		assert.Equal(t, "Ann", rs[0].Label)
	}

	// Keep it on the phone. The laptop closes its own item when the restore arrives.
	rs, _ := b.a.Reviews(context.Background(), false)
	require.NoError(t, b.a.Resolve(context.Background(), rs[0].ID, true))
	syncWith(t, b, pb)
	syncWith(t, a, pa)
	for _, n := range []*node{a, b} {
		open, err := n.a.Reviews(context.Background(), false)
		require.NoError(t, err)
		assert.Empty(t, open)
		assert.Equal(t, "Boss", n.str(`SELECT title FROM people WHERE uuid='p1'`))
	}
}

func TestResolveDeleteRemovesEverywhere(t *testing.T) {
	rel, a, b := newNode(t), newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	pb := join(t, rel, srv.URL, b, "phone")
	a.exec(`INSERT INTO people (uuid, first_name) VALUES ('p1', 'Ann')`)
	syncWith(t, a, pa)
	syncWith(t, b, pb)
	a.exec(`DELETE FROM people WHERE uuid='p1'`)
	b.exec(`UPDATE people SET title = 'Boss' WHERE uuid='p1'`)
	syncWith(t, a, pa)
	syncWith(t, b, pb)
	syncWith(t, a, pa)
	rs, _ := a.a.Reviews(context.Background(), false)
	require.Len(t, rs, 1)
	require.NoError(t, a.a.Resolve(context.Background(), rs[0].ID, false))
	syncWith(t, a, pa)
	syncWith(t, b, pb)
	for _, n := range []*node{a, b} {
		assert.Zero(t, n.count(`SELECT COUNT(*) FROM people WHERE uuid='p1'`))
		open, _ := n.a.Reviews(context.Background(), false)
		assert.Empty(t, open)
	}
}

func TestAuthAndEnrollRules(t *testing.T) {
	rel, a := newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	ctx := context.Background()

	_, err := relay.NewClient(srv.URL, "wrong").Pull(ctx, relay.PullRequest{})
	var se *relay.StatusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusUnauthorized, se.Code)

	tok, _ := rel.a.CreateEnrollToken(ctx, "x", time.Hour)
	c := relay.NewClient(srv.URL, "")
	_, err = c.Enroll(ctx, relay.EnrollRequest{Token: tok, Device: a.a.Device(), Name: "laptop"})
	require.NoError(t, err)
	_, err = c.Enroll(ctx, relay.EnrollRequest{Token: tok, Device: "other", Name: "x"})
	require.ErrorAs(t, err, &se, "an enroll token works once")
	assert.Equal(t, http.StatusUnauthorized, se.Code)

	// The same device id cannot enroll twice (a copied database).
	tok2, _ := rel.a.CreateEnrollToken(ctx, "y", time.Hour)
	_, err = c.Enroll(ctx, relay.EnrollRequest{Token: tok2, Device: a.a.Device(), Name: "laptop2"})
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusConflict, se.Code)

	// Expired tokens are refused.
	old, _ := rel.a.CreateEnrollToken(ctx, "z", time.Millisecond)
	time.Sleep(1100 * time.Millisecond)
	_, err = c.Enroll(ctx, relay.EnrollRequest{Token: old, Device: "fresh", Name: "fresh"})
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusUnauthorized, se.Code)
}

func TestRetireRevokesTheToken(t *testing.T) {
	rel, a := newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	syncWith(t, a, pa)
	_, err := rel.a.Retire(context.Background(), "laptop")
	require.NoError(t, err)
	_, err = relay.Sync(context.Background(), a.a, pa, relay.Options{Pull: true})
	var se *relay.StatusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusUnauthorized, se.Code)
}

func TestReadOnlyRelayRefusesPushes(t *testing.T) {
	rel, a := newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a, ReadOnly: true}).Handler())
	defer srv.Close()
	ctx := context.Background()
	// Enrollment is refused too, so enroll through the database and talk to the server directly.
	tok, _ := rel.a.CreateEnrollToken(ctx, "x", time.Hour)
	dtok, err := rel.a.Enroll(ctx, tok, a.a.Device(), "laptop")
	require.NoError(t, err)
	c := relay.NewClient(srv.URL, dtok)
	_, err = c.Pull(ctx, relay.PullRequest{})
	require.NoError(t, err)
	_, err = c.Push(ctx, relay.PushRequest{})
	var se *relay.StatusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusForbidden, se.Code)
}

func TestPushWithFarFutureClockIsRejected(t *testing.T) {
	rel, a := newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	ctx := context.Background()
	bad := replica.Change{ID: replica.ChangeID{Device: "ffff0000-0000-0000-0000-000000000001", Seq: 1}, Collection: "people", Key: "px", Field: "first_name",
		Op: replica.OpSet, Value: []byte(`"Future"`), SchemaVersion: 1,
		HLC: replica.FormatHLC(time.Now().Add(replica.MaxDrift+time.Minute), 0, "ffff0000")}
	resp, err := relay.NewClient(pa.URL, pa.Token).Push(ctx, relay.PushRequest{Changes: []replica.Change{bad}})
	require.NoError(t, err)
	require.Len(t, resp.Rejected, 1)
	assert.Contains(t, resp.Rejected[0].Reason, "drift")
	assert.Zero(t, rel.count(`SELECT COUNT(*) FROM people WHERE uuid='px'`))
}

func TestPullPagesAndRelaysThirdPartyChanges(t *testing.T) {
	rel, a, b := newNode(t), newNode(t), newNode(t)
	srv := httptest.NewServer((&relay.Server{A: rel.a}).Handler())
	defer srv.Close()
	pa := join(t, rel, srv.URL, a, "laptop")
	pb := join(t, rel, srv.URL, b, "phone")
	for i := 0; i < 40; i++ {
		a.exec(`INSERT INTO people (uuid, first_name) VALUES (?, ?)`, "u"+string(rune('a'+i%26))+string(rune('a'+i/26)), "P")
	}
	rep, err := relay.Sync(context.Background(), a.a, pa, relay.Options{Pull: true, Push: true, Limit: 25})
	require.NoError(t, err)
	assert.Greater(t, rep.Pushed, 40, "pushed in several pages")
	_, err = relay.Sync(context.Background(), b.a, pb, relay.Options{Pull: true, Push: true, Limit: 25})
	require.NoError(t, err)
	assert.Equal(t, 40, b.count(`SELECT COUNT(*) FROM people`))
	// b did not write them, but a third device pulling from b would see them: b relays.
	snapB, _ := b.a.Snapshot(context.Background())
	fromA := 0
	for _, c := range snapB.Changes {
		if c.ID.Device == a.a.Device() {
			fromA++
		}
	}
	assert.Greater(t, fromA, 40)
}

func TestCheckBind(t *testing.T) {
	ok := []string{"127.0.0.1:7478", "localhost:7478", "192.168.1.5:7478", "10.0.0.4:1", "100.101.102.103:7478", "172.16.3.4:9", "[::1]:7478", "[fd7a:115c:a1e0::1]:7478", "169.254.1.1:5"}
	for _, a := range ok {
		assert.NoError(t, relay.CheckBind(a, false), a)
	}
	bad := []string{"0.0.0.0:7478", ":7478", "[::]:7478", "8.8.8.8:7478", "example.com:7478", "100.128.0.1:1", "172.32.0.1:1"}
	for _, a := range bad {
		assert.Error(t, relay.CheckBind(a, false), a)
		assert.NoError(t, relay.CheckBind(a, true), a)
	}
	assert.Error(t, relay.CheckBind("nonsense", false))
}
