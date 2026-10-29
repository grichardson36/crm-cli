package replica

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Mem is an in-memory Adapter. It is the reference implementation of the contract: the engine
// tests and the conformance tests run against it and against the SQLite adapter.
type Mem struct {
	mu      sync.Mutex
	device  string
	schema  Schema
	engine  *Engine
	clock   *Clock
	seq     int64
	changes map[[4]string]Change
	vector  VersionVector
	pending map[RecordRef]string
	reviews map[RecordRef]string
	rows    map[RecordRef]map[string]json.RawMessage
	remote  bool
}

// NewMem makes an empty in-memory adapter. A nil now uses the wall clock.
func NewMem(device string, schema Schema, now func() time.Time) *Mem {
	m := &Mem{
		device: device, schema: schema, clock: NewClock(device, now),
		changes: map[[4]string]Change{}, vector: VersionVector{}, pending: map[RecordRef]string{},
		reviews: map[RecordRef]string{}, rows: map[RecordRef]map[string]json.RawMessage{},
	}
	m.engine = &Engine{Schema: schema, Now: now}
	return m
}

// Device implements Adapter.
func (m *Mem) Device() string { return m.device }

// --- Store ---

type memTx struct{ m *Mem }

func (t memTx) Get(_ context.Context, coll, key, field, device string) (*Change, error) {
	if c, ok := t.m.changes[[4]string{coll, key, field, device}]; ok {
		return &c, nil
	}
	return nil, nil
}

func (t memTx) Entries(_ context.Context, coll, key string) ([]Change, error) {
	var out []Change
	for k, c := range t.m.changes {
		if k[0] == coll && k[1] == key {
			out = append(out, c)
		}
	}
	SortChanges(out)
	return out, nil
}

func (t memTx) Put(_ context.Context, c Change) error {
	t.m.changes[[4]string{c.Collection, c.Key, c.Field, c.ID.Device}] = c
	return nil
}

func (t memTx) Bump(_ context.Context, device string, seq int64) error {
	if seq > t.m.vector[device] {
		t.m.vector[device] = seq
	}
	return nil
}

func (t memTx) SyncReview(_ context.Context, ref RecordRef, parked bool, detail string) error {
	if parked {
		t.m.reviews[ref] = detail
	} else {
		delete(t.m.reviews, ref)
	}
	return nil
}

func (t memTx) Pending(context.Context) ([]RecordRef, error) {
	var out []RecordRef
	for r := range t.m.pending {
		out = append(out, r)
	}
	return out, nil
}

func (t memTx) SetPending(_ context.Context, ref RecordRef, reason string) error {
	if reason == "" {
		delete(t.m.pending, ref)
	} else {
		t.m.pending[ref] = reason
	}
	return nil
}

func (t memTx) Unknown(context.Context) ([]Change, error) {
	var out []Change
	for _, c := range t.m.changes {
		if c.Status == StatusUnknown {
			out = append(out, c)
		}
	}
	SortChanges(out)
	return out, nil
}

// --- Projector ---

func (t memTx) Project(_ context.Context, v RecordView) error {
	ref := RecordRef{v.Collection, v.Key}
	switch v.Life {
	case Deleted:
		delete(t.m.rows, ref)
	default: // Alive and Parked: a parked record stays visible until someone decides
		row := map[string]json.RawMessage{}
		for name, r := range v.Fields {
			if r.Op == OpSet {
				row[name] = r.Value
			}
		}
		t.m.rows[ref] = row
	}
	return nil
}

// --- Adapter ---

// Apply implements Adapter.
func (m *Mem) Apply(ctx context.Context, c Change) (Outcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remote = true
	defer func() { m.remote = false }()
	return m.engine.Apply(ctx, memTx{m}, memTx{m}, c)
}

// ApplyBatch implements Adapter.
func (m *Mem) ApplyBatch(ctx context.Context, cs []Change) (BatchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remote = true
	defer func() { m.remote = false }()
	res, err := m.engine.ApplyBatch(ctx, memTx{m}, memTx{m}, cs)
	if res.MaxHLC != "" {
		if _, rerr := m.clock.Recv(res.MaxHLC); rerr != nil && err == nil {
			err = rerr
		}
	}
	return res, err
}

// RetryUnknown applies stored changes that the schema knows now.
func (m *Mem) RetryUnknown(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.engine.RetryUnknown(ctx, memTx{m}, memTx{m})
}

// SetSchema swaps the schema, as an upgrade of the program would.
func (m *Mem) SetSchema(s Schema) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schema = s
	m.engine.Schema = s
}

// ReadFieldClock implements Adapter.
func (m *Mem) ReadFieldClock(_ context.Context, coll, key, field string) (FieldClock, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, _ := memTx{m}.Entries(context.Background(), coll, key)
	v := m.engine.View(coll, key, entries)
	r, ok := v.Fields[field]
	if !ok {
		return FieldClock{}, false, nil
	}
	return FieldClock{HLC: r.HLC, Device: r.Device, Seq: r.Seq}, true, nil
}

// EmitLocal implements Adapter. It stores the change as this device's own and updates the rows.
func (m *Mem) EmitLocal(ctx context.Context, c Change) (Change, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hlc, err := m.clock.Now()
	if err != nil {
		return Change{}, err
	}
	pol := LWW
	if col, ok := m.schema.Collection(c.Collection); ok {
		if f, ok := col.Field(c.Field); ok && f.Policy != "" {
			pol = f.Policy
		}
	}
	if c.Op == OpDelete || pol == KeepBoth {
		c.Seen = m.vector.Clone()
	}
	m.seq++
	c.ID = ChangeID{Device: m.device, Seq: m.seq}
	c.HLC, c.SchemaVersion, c.Status = hlc, m.schema.Version, StatusApplied
	if err := c.Validate(); err != nil {
		m.seq--
		return Change{}, err
	}
	tx := memTx{m}
	_ = tx.Put(ctx, c)
	_ = tx.Bump(ctx, m.device, c.ID.Seq)
	err = m.engine.Reproject(ctx, tx, tx, RecordRef{c.Collection, c.Key})
	return c, err
}

// Set, Unset, Delete and Restore are local writes for tests.
func (m *Mem) Set(ctx context.Context, coll, key, field string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = m.EmitLocal(ctx, Change{Collection: coll, Key: key, Field: field, Op: OpSet, Value: raw})
	return err
}

// Unset clears a field.
func (m *Mem) Unset(ctx context.Context, coll, key, field string) error {
	_, err := m.EmitLocal(ctx, Change{Collection: coll, Key: key, Field: field, Op: OpUnset})
	return err
}

// Delete deletes a record.
func (m *Mem) Delete(ctx context.Context, coll, key string) error {
	_, err := m.EmitLocal(ctx, Change{Collection: coll, Key: key, Op: OpDelete})
	return err
}

// Restore brings a record back.
func (m *Mem) Restore(ctx context.Context, coll, key string) error {
	_, err := m.EmitLocal(ctx, Change{Collection: coll, Key: key, Op: OpRestore})
	return err
}

// BeginRemote implements Adapter. Mem has no capture, so the flag only documents the contract.
func (m *Mem) BeginRemote(context.Context) error { m.remote = true; return nil }

// EndRemote implements Adapter.
func (m *Mem) EndRemote(context.Context) error { m.remote = false; return nil }

// Vector implements Adapter.
func (m *Mem) Vector(context.Context) (VersionVector, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.vector.Clone(), nil
}

// Snapshot implements Adapter.
func (m *Mem) Snapshot(context.Context) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Snapshot{Vector: m.vector.Clone()}
	for _, c := range m.changes {
		s.Changes = append(s.Changes, c)
	}
	SortChanges(s.Changes)
	return s, nil
}

// Pull implements Adapter.
func (m *Mem) Pull(_ context.Context, cursor VersionVector, limit int) ([]Change, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	per := map[string][]Change{}
	for _, c := range m.changes {
		if c.ID.Seq > cursor[c.ID.Device] {
			per[c.ID.Device] = append(per[c.ID.Device], c)
		}
	}
	var out []Change
	for _, list := range per {
		SortChanges(list)
		if limit > 0 && len(list) > limit {
			list = list[:limit]
		}
		out = append(out, list...)
	}
	SortChanges(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Rows returns the projected records as "collection/key" to field to JSON text, for comparison.
func (m *Mem) Rows() map[string]map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]map[string]string{}
	for ref, row := range m.rows {
		r := map[string]string{}
		for k, v := range row {
			r[k] = string(v)
		}
		out[ref.Collection+"/"+ref.Key] = r
	}
	return out
}

// Reviews lists the records that wait for a decision.
func (m *Mem) Reviews() []RecordRef {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []RecordRef
	for r := range m.reviews {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Pending lists the records that could not be projected yet.
func (m *Mem) Pending() []RecordRef {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []RecordRef
	for r := range m.pending {
		out = append(out, r)
	}
	return out
}

func (m *Mem) String() string { return fmt.Sprintf("mem(%s)", m.device) }
