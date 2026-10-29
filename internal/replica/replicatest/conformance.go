// Package replicatest holds the conformance test of the sync contract (rule 14): fixed change sets,
// shuffled and duplicated, applied through an adapter must give identical state. Every adapter
// runs the same test.
package replicatest

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/replica"
	"github.com/stretchr/testify/require"
)

// Config describes the adapter under test and the collection the fixed change sets use.
type Config struct {
	// New returns an empty adapter. It is called once per run.
	New func(t *testing.T) replica.Adapter
	// State returns the records of the collection as a canonical string, read from the adapter's
	// own storage (rows of a table, entries of a map).
	State func(t *testing.T, a replica.Adapter) string

	Collection string // for example "people"
	// Required are fields every record needs before the adapter can store it, with JSON values.
	Required map[string]string
	// LWW, Notes and Max name one field of each policy: lww, keep-both and max.
	LWW, Notes, Max string
}

const nDevices = 3

// ChangeSet builds the fixed change set. Three devices write: concurrent lww edits, concurrent
// keep-both edits, max values, a delete that saw everything, a delete that missed an edit (parks)
// and a delete followed by a restore. The result is the same on every run.
func ChangeSet(cfg Config) []replica.Change {
	devs := [nDevices]string{"d1000000-0000-4000-8000-000000000001", "d2000000-0000-4000-8000-000000000002", "d3000000-0000-4000-8000-000000000003"}
	seq := map[string]int64{}
	sec := 0
	var out []replica.Change
	emit := func(d int, key, field string, op replica.Op, value string, seen replica.VersionVector) {
		sec++
		dev := devs[d]
		seq[dev]++
		c := replica.Change{
			ID: replica.ChangeID{Device: dev, Seq: seq[dev]}, Collection: cfg.Collection, Key: key, Field: field, Op: op, SchemaVersion: 1,
			HLC:  replica.FormatHLC(time.Date(2026, 1, 1, 0, sec/60, sec%60, 0, time.UTC), 0, dev),
			Seen: seen,
		}
		if value != "" {
			c.Value = json.RawMessage(value)
		}
		out = append(out, c)
	}
	create := func(d int, key string) {
		names := make([]string, 0, len(cfg.Required))
		for f := range cfg.Required {
			names = append(names, f)
		}
		sort.Strings(names)
		for _, f := range names {
			emit(d, key, f, replica.OpSet, cfg.Required[f], nil)
		}
	}
	vec := func() replica.VersionVector {
		v := replica.VersionVector{}
		for d, n := range seq {
			v[d] = n
		}
		return v
	}
	for i := 0; i < 6; i++ {
		key := fmt.Sprintf("00000000-0000-4000-8000-0000000000%02d", i+1)
		create(0, key)
		// lww: three devices write the same field, the latest clock wins.
		emit(0, key, cfg.LWW, replica.OpSet, `"from d1"`, nil)
		emit(1, key, cfg.LWW, replica.OpSet, `"from d2"`, nil)
		emit(2, key, cfg.LWW, replica.OpSet, fmt.Sprintf(`"from d3 #%d"`, i), nil)
		// keep-both: two devices edit notes without having seen each other.
		emit(1, key, cfg.Notes, replica.OpSet, `"note from d2"`, replica.VersionVector{})
		emit(2, key, cfg.Notes, replica.OpSet, `"note from d3"`, replica.VersionVector{})
		// max: the largest date wins whatever the order.
		emit(0, key, cfg.Max, replica.OpSet, `"2026-03-01"`, nil)
		emit(1, key, cfg.Max, replica.OpSet, `"2026-09-30"`, nil)
		emit(2, key, cfg.Max, replica.OpSet, `"2026-05-15"`, nil)
		switch i {
		case 3: // deleted by a device that had seen every edit
			emit(0, key, "", replica.OpDelete, "", vec())
		case 4: // deleted by a device that had not seen d3's last edit: parks
			seen := vec()
			emit(2, key, cfg.LWW, replica.OpSet, `"late edit"`, nil)
			emit(0, key, "", replica.OpDelete, "", seen)
		case 5: // deleted, then restored by another device
			emit(0, key, "", replica.OpDelete, "", vec())
			emit(1, key, "", replica.OpRestore, "", nil)
		}
	}
	// A field no build knows and a collection no build knows: stored, never applied, never lost.
	emit(1, "00000000-0000-4000-8000-000000000001", "field_from_the_future", replica.OpSet, `"x"`, nil)
	emit(2, "00000000-0000-4000-8000-0000000000ff", "anything", replica.OpSet, `1`, nil)
	out[len(out)-1].Collection = "collection_from_the_future"
	return out
}

func isWriter(device string) bool {
	return strings.HasPrefix(device, "d1000000") || strings.HasPrefix(device, "d2000000") || strings.HasPrefix(device, "d3000000")
}

func canonicalSnapshot(t *testing.T, a replica.Adapter) string {
	t.Helper()
	s, err := a.Snapshot(context.Background())
	require.NoError(t, err)
	// Only the three writers of the fixed set are compared. The adapter's own changes (its baseline,
	// which a relay passes on) and anything else it holds are not part of the comparison.
	var keep []replica.Change
	for _, c := range s.Changes {
		if !isWriter(c.ID.Device) {
			continue
		}
		c.Status = ""
		keep = append(keep, c)
	}
	replica.SortChanges(keep)
	sort.SliceStable(keep, func(i, j int) bool {
		if keep[i].ID.Device != keep[j].ID.Device {
			return keep[i].ID.Device < keep[j].ID.Device
		}
		return keep[i].ID.Seq < keep[j].ID.Seq
	})
	b, err := json.Marshal(keep)
	require.NoError(t, err)
	vec := map[string]int64{}
	for d, n := range s.Vector {
		if isWriter(d) {
			vec[d] = n
		}
	}
	vb, _ := json.Marshal(vec)
	return string(b) + "\n" + string(vb)
}

// Run is the conformance test.
func Run(t *testing.T, cfg Config) {
	set := ChangeSet(cfg)
	ctx := context.Background()

	reference := cfg.New(t)
	res, err := reference.ApplyBatch(ctx, set)
	require.NoError(t, err)
	require.Empty(t, res.Rejected)
	require.Equal(t, 2, res.Unknown, "the future field and collection are stored, not applied")
	wantState := cfg.State(t, reference)
	wantSnap := canonicalSnapshot(t, reference)
	require.NotEmpty(t, wantState)

	t.Run("in order twice", func(t *testing.T) {
		a := cfg.New(t)
		for i := 0; i < 2; i++ {
			_, err := a.ApplyBatch(ctx, set)
			require.NoError(t, err)
		}
		require.Equal(t, wantState, cfg.State(t, a))
		require.Equal(t, wantSnap, canonicalSnapshot(t, a))
	})

	t.Run("shuffled and duplicated, one change at a time", func(t *testing.T) {
		for seed := int64(1); seed <= 12; seed++ {
			rng := rand.New(rand.NewSource(seed))
			order := append([]replica.Change(nil), set...)
			for _, c := range set {
				if rng.Intn(3) == 0 {
					order = append(order, c)
				}
			}
			rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
			a := cfg.New(t)
			for _, c := range order {
				_, err := a.Apply(ctx, c)
				require.NoError(t, err, "seed %d", seed)
			}
			require.Equal(t, wantState, cfg.State(t, a), "seed %d", seed)
			require.Equal(t, wantSnap, canonicalSnapshot(t, a), "seed %d", seed)
		}
	})

	t.Run("shuffled batches", func(t *testing.T) {
		for seed := int64(100); seed < 108; seed++ {
			rng := rand.New(rand.NewSource(seed))
			order := append([]replica.Change(nil), set...)
			order = append(order, set[:len(set)/2]...)
			rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
			a := cfg.New(t)
			for len(order) > 0 {
				n := 1 + rng.Intn(17)
				if n > len(order) {
					n = len(order)
				}
				_, err := a.ApplyBatch(ctx, order[:n])
				require.NoError(t, err)
				order = order[n:]
			}
			require.Equal(t, wantState, cfg.State(t, a), "seed %d", seed)
			require.Equal(t, wantSnap, canonicalSnapshot(t, a), "seed %d", seed)
		}
	})

	t.Run("replay is a no-op", func(t *testing.T) {
		a := cfg.New(t)
		_, err := a.ApplyBatch(ctx, set)
		require.NoError(t, err)
		res, err := a.ApplyBatch(ctx, set)
		require.NoError(t, err)
		require.Zero(t, res.Applied)
		require.Equal(t, len(set), res.Duplicates)
	})

	t.Run("a relay passes the state on", func(t *testing.T) {
		a := cfg.New(t)
		_, err := a.ApplyBatch(ctx, set)
		require.NoError(t, err)
		b := cfg.New(t)
		cursor := replica.VersionVector{}
		for {
			page, err := a.Pull(ctx, cursor, 7)
			require.NoError(t, err)
			if len(page) == 0 {
				break
			}
			_, err = b.ApplyBatch(ctx, page)
			require.NoError(t, err)
			cursor, err = b.Vector(ctx)
			require.NoError(t, err)
		}
		require.Equal(t, wantState, cfg.State(t, b))
		require.Equal(t, wantSnap, canonicalSnapshot(t, b))
	})
}
