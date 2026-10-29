package replica

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSchema(version int) Schema {
	return NewSchema(version, Collection{Name: "people", Fields: []Field{
		{Name: "name"},
		{Name: "notes", Policy: KeepBoth},
		{Name: "score", Policy: Max},
		{Name: "labels", Policy: SetUnion},
		{Name: "boss", Ref: "people"},
	}})
}

func fixedNow(s string) func() time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return func() time.Time { return t }
}

func TestHLCFormatAndParse(t *testing.T) {
	tm := time.Date(2026, 10, 28, 12, 0, 0, 123_000_000, time.UTC)
	s := FormatHLC(tm, 175, "0123456789abcdef-aaaa")
	assert.Equal(t, "2026-10-28T12:00:00.123Z-00AF-0123456789abcdef", s)
	assert.Len(t, s, hlcLen)
	h, err := ParseHLC(s)
	require.NoError(t, err)
	assert.Equal(t, 175, h.Counter)
	assert.True(t, h.Time.Equal(tm))
	_, err = ParseHLC("nonsense")
	assert.ErrorIs(t, err, ErrBadHLC)
}

func TestHLCSortsAsString(t *testing.T) {
	c := NewClock("dev-a", fixedNow("2026-10-28T12:00:00Z"))
	prev := ""
	for i := 0; i < 100; i++ {
		s, err := c.Now()
		require.NoError(t, err)
		assert.Greater(t, s, prev)
		prev = s
	}
}

func TestHLCDriftIsFiveMinutes(t *testing.T) {
	assert.Equal(t, 5*time.Minute, MaxDrift)
	now := fixedNow("2026-10-28T12:00:00Z")
	ok := FormatHLC(now().Add(MaxDrift), 0, "n")
	bad := FormatHLC(now().Add(MaxDrift+time.Second), 0, "n")
	assert.NoError(t, CheckDrift(ok, now()))
	assert.ErrorIs(t, CheckDrift(bad, now()), ErrDrift)
	c := NewClock("a", now)
	_, err := c.Recv(bad)
	assert.ErrorIs(t, err, ErrDrift)
	_, err = c.Recv(ok)
	assert.NoError(t, err)
}

func TestClockRecvMovesPastRemote(t *testing.T) {
	now := fixedNow("2026-10-28T12:00:00Z")
	c := NewClock("a", now)
	remote := FormatHLC(now().Add(2*time.Minute), 7, "b")
	got, err := c.Recv(remote)
	require.NoError(t, err)
	assert.Greater(t, got, remote)
	next, err := c.Now()
	require.NoError(t, err)
	assert.Greater(t, next, got)
}

func TestClockNowRefusesWhenFarAhead(t *testing.T) {
	now := fixedNow("2026-10-28T12:00:00Z")
	c := NewClock("a", now)
	c.Restore(now().Add(10*time.Minute), 0)
	_, err := c.Now()
	assert.ErrorIs(t, err, ErrDrift)
}

func TestVersionVector(t *testing.T) {
	a := VersionVector{"x": 3, "y": 1}
	b := VersionVector{"x": 2, "z": 4}
	assert.False(t, a.Dominates(b))
	a.Merge(b)
	assert.Equal(t, VersionVector{"x": 3, "y": 1, "z": 4}, a)
	assert.True(t, a.Dominates(b))
	assert.True(t, a.Covers("z", 4))
	assert.False(t, a.Covers("z", 5))
}

func ch(device string, seq int64, hlcSec int, coll, key, field string, op Op, value string) Change {
	c := Change{
		ID: ChangeID{Device: device, Seq: seq}, Collection: coll, Key: key, Field: field, Op: op, SchemaVersion: 1,
		HLC: FormatHLC(time.Date(2026, 10, 28, 12, 0, hlcSec, 0, time.UTC), 0, device),
	}
	if value != "" {
		c.Value = json.RawMessage(value)
	}
	return c
}

func TestPolicies(t *testing.T) {
	t.Run("lww latest clock wins, device breaks ties", func(t *testing.T) {
		r, _ := ResolveField(LWW, []Change{
			ch("a", 1, 5, "p", "k", "f", OpSet, `"old"`),
			ch("b", 1, 9, "p", "k", "f", OpSet, `"new"`),
		})
		assert.JSONEq(t, `"new"`, string(r.Value))
		x := ch("a", 1, 5, "p", "k", "f", OpSet, `"x"`)
		y := ch("b", 1, 5, "p", "k", "f", OpSet, `"y"`)
		x.HLC = y.HLC
		r, _ = ResolveField(LWW, []Change{x, y})
		assert.Equal(t, "b", r.Device)
	})
	t.Run("lww unset wins when latest", func(t *testing.T) {
		r, _ := ResolveField(LWW, []Change{
			ch("a", 1, 5, "p", "k", "f", OpSet, `"v"`),
			ch("b", 1, 9, "p", "k", "f", OpUnset, ""),
		})
		assert.Equal(t, OpUnset, r.Op)
	})
	t.Run("max", func(t *testing.T) {
		r, _ := ResolveField(Max, []Change{
			ch("a", 1, 9, "p", "k", "f", OpSet, `3`),
			ch("b", 1, 5, "p", "k", "f", OpSet, `10`),
		})
		assert.JSONEq(t, `10`, string(r.Value))
		r, _ = ResolveField(Max, []Change{
			ch("a", 1, 9, "p", "k", "f", OpSet, `"2026-01-02"`),
			ch("b", 1, 5, "p", "k", "f", OpSet, `"2026-03-01"`),
		})
		assert.JSONEq(t, `"2026-03-01"`, string(r.Value))
	})
	t.Run("set-union", func(t *testing.T) {
		r, _ := ResolveField(SetUnion, []Change{
			ch("a", 1, 9, "p", "k", "f", OpSet, `["x","y"]`),
			ch("b", 1, 5, "p", "k", "f", OpSet, `["y","z"]`),
		})
		assert.JSONEq(t, `["x","y","z"]`, string(r.Value))
	})
	t.Run("keep-both keeps concurrent edits and drops seen ones", func(t *testing.T) {
		a := ch("a", 4, 5, "p", "k", "f", OpSet, `"from a"`)
		b := ch("b", 2, 6, "p", "k", "f", OpSet, `"from b"`)
		r, _ := ResolveField(KeepBoth, []Change{a, b})
		want, _ := json.Marshal("from a" + KeepBothSeparator + "from b")
		assert.JSONEq(t, string(want), string(r.Value))
		b.Seen = VersionVector{"a": 4}
		r, _ = ResolveField(KeepBoth, []Change{a, b})
		assert.JSONEq(t, `"from b"`, string(r.Value))
	})
}

func newPair(t *testing.T) (*Mem, *Mem) {
	t.Helper()
	return NewMem("aaaaaaaa", testSchema(1), nil), NewMem("bbbbbbbb", testSchema(1), nil)
}

func syncMem(t *testing.T, from, to *Mem) {
	t.Helper()
	ctx := context.Background()
	cur, _ := to.Vector(ctx)
	cs, err := from.Pull(ctx, cur, 0)
	require.NoError(t, err)
	res, err := to.ApplyBatch(ctx, cs)
	require.NoError(t, err)
	require.Empty(t, res.Rejected)
}

func both(t *testing.T, a, b *Mem) {
	syncMem(t, a, b)
	syncMem(t, b, a)
}

func TestApplyIsIdempotentAndOrderFree(t *testing.T) {
	ctx := context.Background()
	a, b := newPair(t)
	require.NoError(t, a.Set(ctx, "people", "k1", "name", "Ann"))
	require.NoError(t, a.Set(ctx, "people", "k1", "name", "Anne"))
	require.NoError(t, b.Set(ctx, "people", "k1", "score", 4))
	both(t, a, b)
	both(t, a, b)
	assert.Equal(t, a.Rows(), b.Rows())
	assert.Equal(t, `"Anne"`, a.Rows()["people/k1"]["name"])
	cs, _ := a.Pull(ctx, VersionVector{}, 0)
	for _, c := range cs {
		out, err := b.Apply(ctx, c)
		require.NoError(t, err)
		assert.Equal(t, Duplicate, out)
	}
}

func TestPerFieldMergeOfOfflineEdits(t *testing.T) {
	ctx := context.Background()
	a, b := newPair(t)
	require.NoError(t, a.Set(ctx, "people", "k1", "name", "Ann"))
	both(t, a, b)
	require.NoError(t, a.Set(ctx, "people", "k1", "name", "Anne"))
	require.NoError(t, b.Set(ctx, "people", "k1", "score", 9))
	both(t, a, b)
	assert.Equal(t, a.Rows(), b.Rows())
	assert.Equal(t, `"Anne"`, b.Rows()["people/k1"]["name"])
	assert.Equal(t, `9`, a.Rows()["people/k1"]["score"])
}

func TestKeepBothNotes(t *testing.T) {
	ctx := context.Background()
	a, b := newPair(t)
	require.NoError(t, a.Set(ctx, "people", "k1", "name", "Ann"))
	require.NoError(t, a.Set(ctx, "people", "k1", "notes", "base"))
	both(t, a, b)
	require.NoError(t, a.Set(ctx, "people", "k1", "notes", "base, a"))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, b.Set(ctx, "people", "k1", "notes", "base, b"))
	both(t, a, b)
	assert.Equal(t, a.Rows(), b.Rows())
	var got string
	require.NoError(t, json.Unmarshal([]byte(a.Rows()["people/k1"]["notes"]), &got))
	assert.Contains(t, got, "base, a")
	assert.Contains(t, got, "base, b")
	// Editing the merged text on one device supersedes both once the other has seen it.
	require.NoError(t, a.Set(ctx, "people", "k1", "notes", "resolved"))
	both(t, a, b)
	assert.Equal(t, `"resolved"`, b.Rows()["people/k1"]["notes"])
}

func TestDeleteVersusEditParks(t *testing.T) {
	ctx := context.Background()
	a, b := newPair(t)
	require.NoError(t, a.Set(ctx, "people", "k1", "name", "Ann"))
	both(t, a, b)
	require.NoError(t, a.Delete(ctx, "people", "k1"))
	require.NoError(t, b.Set(ctx, "people", "k1", "score", 3))
	both(t, a, b)
	assert.Equal(t, []RecordRef{{"people", "k1"}}, a.Reviews())
	assert.Equal(t, []RecordRef{{"people", "k1"}}, b.Reviews())
	// Both devices show the record with the edit. Nothing was removed.
	_, onA := a.Rows()["people/k1"]
	_, onB := b.Rows()["people/k1"]
	assert.True(t, onA)
	assert.True(t, onB)
	assert.Equal(t, a.Rows(), b.Rows())
	// Resolve by keeping the record on B: the restore closes the item on A too.
	require.NoError(t, b.Restore(ctx, "people", "k1"))
	both(t, a, b)
	assert.Empty(t, a.Reviews())
	assert.Empty(t, b.Reviews())
	assert.Equal(t, a.Rows(), b.Rows())
	assert.Equal(t, `3`, a.Rows()["people/k1"]["score"])
}

func TestDeleteWithoutConcurrentEditDeletes(t *testing.T) {
	ctx := context.Background()
	a, b := newPair(t)
	require.NoError(t, a.Set(ctx, "people", "k1", "name", "Ann"))
	both(t, a, b)
	require.NoError(t, a.Delete(ctx, "people", "k1"))
	both(t, a, b)
	assert.Empty(t, b.Rows())
	assert.Empty(t, b.Reviews())
}

func TestUnknownFieldsAreStoredAndRetried(t *testing.T) {
	ctx := context.Background()
	old := NewMem("cccccccc", NewSchema(1, Collection{Name: "people", Fields: []Field{{Name: "name"}}}), nil)
	a, _ := newPair(t)
	require.NoError(t, a.Set(ctx, "people", "k1", "name", "Ann"))
	require.NoError(t, a.Set(ctx, "people", "k1", "score", 7))
	require.NoError(t, a.Set(ctx, "tasks", "t1", "title", "x"))
	cs, _ := a.Pull(ctx, VersionVector{}, 0)
	res, err := old.ApplyBatch(ctx, cs)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Unknown)
	assert.Equal(t, `"Ann"`, old.Rows()["people/k1"]["name"])
	assert.NotContains(t, old.Rows()["people/k1"], "score")
	snap, _ := old.Snapshot(ctx)
	assert.Len(t, snap.Changes, 3, "unknown changes are kept, never discarded")
	// The upgrade teaches it the field and the collection.
	old.SetSchema(NewSchema(2, testSchema(2).Collections["people"], Collection{Name: "tasks", Fields: []Field{{Name: "title"}}}))
	n, err := old.RetryUnknown(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, `7`, old.Rows()["people/k1"]["score"])
	assert.Equal(t, `"x"`, old.Rows()["tasks/t1"]["title"])
}

func TestDriftIsRejectedAndLaterChangesOfTheDeviceSkipped(t *testing.T) {
	ctx := context.Background()
	now := fixedNow("2026-10-28T12:00:00Z")
	m := NewMem("dddddddd", testSchema(1), now)
	far := ch("a", 1, 0, "people", "k", "name", OpSet, `"x"`)
	far.HLC = FormatHLC(now().Add(MaxDrift+time.Minute), 0, "a")
	next := ch("a", 2, 1, "people", "k", "score", OpSet, `1`)
	fine := ch("b", 1, 1, "people", "k2", "name", OpSet, `"ok"`)
	res, err := m.ApplyBatch(ctx, []Change{far, next, fine})
	require.NoError(t, err)
	require.Len(t, res.Rejected, 2)
	assert.Equal(t, 1, res.Applied)
	v, _ := m.Vector(ctx)
	assert.Equal(t, VersionVector{"b": 1}, v, "the cursor does not move past a rejected change")
}

func TestPendingRefsResolveWhenTargetArrives(t *testing.T) {
	// The in-memory projector accepts everything, so check the engine path with a gate.
	ctx := context.Background()
	m := NewMem("eeeeeeee", testSchema(1), nil)
	gate := &gatedProjector{inner: memTx{m}, blocked: map[string]bool{"people/k2": true}}
	eng := &Engine{Schema: testSchema(1)}
	st := memTx{m}
	c := ch("a", 1, 1, "people", "k2", "name", OpSet, `"child"`)
	_, err := eng.Apply(ctx, st, gate, c)
	require.NoError(t, err)
	assert.Len(t, m.Pending(), 1)
	gate.blocked = map[string]bool{}
	require.NoError(t, eng.Reproject(ctx, st, gate))
	assert.Empty(t, m.Pending())
	assert.Equal(t, `"child"`, m.Rows()["people/k2"]["name"])
}

type gatedProjector struct {
	inner   Projector
	blocked map[string]bool
}

func (g *gatedProjector) Project(ctx context.Context, v RecordView) error {
	if g.blocked[v.Collection+"/"+v.Key] {
		return fmt.Errorf("%w: waiting", ErrNotReady)
	}
	return g.inner.Project(ctx, v)
}

func TestShuffledAndDuplicatedDeliveryConverges(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewSource(7))
	a, b := newPair(t)
	c := NewMem("cccccccc", testSchema(1), nil)
	keys := []string{"k1", "k2", "k3"}
	devs := []*Mem{a, b, c}
	for i := 0; i < 60; i++ {
		d := devs[rng.Intn(3)]
		k := keys[rng.Intn(3)]
		switch rng.Intn(6) {
		case 0, 1:
			require.NoError(t, d.Set(ctx, "people", k, "name", fmt.Sprintf("n%d", i)))
		case 2:
			require.NoError(t, d.Set(ctx, "people", k, "score", rng.Intn(100)))
		case 3:
			require.NoError(t, d.Set(ctx, "people", k, "notes", fmt.Sprintf("note %d", i)))
		case 4:
			require.NoError(t, d.Delete(ctx, "people", k))
		case 5:
			require.NoError(t, d.Restore(ctx, "people", k))
		}
		if rng.Intn(4) == 0 {
			syncMem(t, d, devs[rng.Intn(3)])
		}
	}
	var all []Change
	for _, d := range devs {
		s, _ := d.Snapshot(ctx)
		all = append(all, s.Changes...)
	}
	var want map[string]map[string]string
	for round := 0; round < 20; round++ {
		fresh := NewMem("ffffffff", testSchema(1), nil)
		order := append([]Change(nil), all...)
		order = append(order, all[:len(all)/2]...) // duplicates
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		// Apply one by one in the shuffled order, so the engine's own ordering is not used.
		for _, ch := range order {
			_, err := fresh.Apply(ctx, ch)
			require.NoError(t, err)
		}
		if want == nil {
			want = fresh.Rows()
			continue
		}
		require.Equal(t, want, fresh.Rows())
	}
}
