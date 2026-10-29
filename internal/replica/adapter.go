package replica

import (
	"context"
	"sort"
)

// FieldClock is the clock of the change that decides a field today.
type FieldClock struct {
	HLC    string `json:"hlc"`
	Device string `json:"device"`
	Seq    int64  `json:"seq"`
}

// Snapshot is the per-field state of a database plus the cursor that describes it (contract rule
// 11). A new device or a reset device applies the changes and keeps the vector as its cursor.
type Snapshot struct {
	Changes []Change      `json:"changes"`
	Vector  VersionVector `json:"cursor"`
}

// Adapter is what a database must provide to take part in sync (contract rule 13). SQLite is the
// only adapter built.
type Adapter interface {
	// Device is this database's device id.
	Device() string
	// Apply applies one remote change.
	Apply(ctx context.Context, c Change) (Outcome, error)
	// ApplyBatch applies remote changes inside one transaction.
	ApplyBatch(ctx context.Context, cs []Change) (BatchResult, error)
	// ReadFieldClock returns the clock of the change that decides a field.
	ReadFieldClock(ctx context.Context, collection, key, field string) (FieldClock, bool, error)
	// EmitLocal records a local write that the adapter did not capture by itself. It fills in the
	// device, sequence, clock and schema version and returns the stored change.
	EmitLocal(ctx context.Context, c Change) (Change, error)
	// BeginRemote and EndRemote bracket a remote apply so that the writes it makes are not captured
	// again as local writes.
	BeginRemote(ctx context.Context) error
	EndRemote(ctx context.Context) error
	// Snapshot returns the full per-field state and its cursor.
	Snapshot(ctx context.Context) (Snapshot, error)
	// Vector returns the version vector: the highest sequence seen from each device.
	Vector(ctx context.Context) (VersionVector, error)
	// Pull returns up to limit stored changes the cursor has not seen, for each device in order of
	// sequence. Any adapter can answer, which is what makes every device a possible relay.
	Pull(ctx context.Context, cursor VersionVector, limit int) ([]Change, error)
}

// SortChanges orders changes by device, then sequence.
func SortChanges(cs []Change) {
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].ID.Device != cs[j].ID.Device {
			return cs[i].ID.Device < cs[j].ID.Device
		}
		if cs[i].ID.Seq != cs[j].ID.Seq {
			return cs[i].ID.Seq < cs[j].ID.Seq
		}
		return cs[i].Field < cs[j].Field
	})
}
