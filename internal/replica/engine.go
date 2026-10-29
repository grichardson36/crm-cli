package replica

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrNotReady is returned by a Projector when a record cannot be written yet, for example when a
// required field or a referenced record has not arrived. The engine keeps the record pending and
// tries again after the next batch.
var ErrNotReady = errors.New("record is not ready to project")

// RecordRef names one record.
type RecordRef struct {
	Collection string
	Key        string
}

// Store is what the engine needs from a database. A Store value works inside one transaction.
type Store interface {
	// Get returns the stored change of one device for one field, or nil.
	Get(ctx context.Context, collection, key, field, device string) (*Change, error)
	// Entries returns every stored change of one record, field changes and life changes.
	Entries(ctx context.Context, collection, key string) ([]Change, error)
	// Put stores a change in place of the same device's earlier change for that field.
	Put(ctx context.Context, c Change) error
	// Bump raises the version vector entry of a device.
	Bump(ctx context.Context, device string, seq int64) error
	// SyncReview opens (parked) or closes (not parked) the delete-versus-edit review item of a record.
	SyncReview(ctx context.Context, ref RecordRef, parked bool, detail string) error
	// Pending lists the records that could not be projected yet.
	Pending(ctx context.Context) ([]RecordRef, error)
	// SetPending records why a record is pending. An empty reason clears it.
	SetPending(ctx context.Context, ref RecordRef, reason string) error
	// Unknown lists the changes stored without being applied.
	Unknown(ctx context.Context) ([]Change, error)
}

// Projector writes the resolved state of a record into the application tables.
type Projector interface {
	Project(ctx context.Context, v RecordView) error
}

// Life is the state of a record after delete, restore and conflicts are taken into account.
type Life int

// The three life states. Parked means a delete and an edit that had not seen it exist together.
// A projector treats a parked record like a live one, so nothing is lost, and the review queue
// holds the decision: keep the record (restore) or delete it for good (a new delete that has seen
// the edit).
const (
	Alive Life = iota
	Deleted
	Parked
)

func (l Life) String() string { return [...]string{"alive", "deleted", "parked"}[l] }

// RecordView is the resolved state of one record.
type RecordView struct {
	Collection string
	Key        string
	Life       Life
	Fields     map[string]Resolved
	LatestHLC  string
	Uncovered  []ChangeID // edits a delete had not seen, when Life is Parked
	DeleteID   ChangeID
}

// Outcome of applying one change.
type Outcome string

// Outcomes of Apply.
const (
	Applied   Outcome = "applied"
	Duplicate Outcome = "duplicate"
	Unknown   Outcome = "unknown"
)

// Rejection explains a change that was refused.
type Rejection struct {
	ID     ChangeID `json:"id"`
	Reason string   `json:"reason"`
}

// BatchResult summarises ApplyBatch.
type BatchResult struct {
	Applied    int         `json:"applied"`
	Duplicates int         `json:"duplicates"`
	Unknown    int         `json:"unknown"`
	Rejected   []Rejection `json:"rejected,omitempty"`
	MaxHLC     string      `json:"max_hlc,omitempty"`
}

// Engine applies changes to a Store using the schema.
type Engine struct {
	Schema Schema
	Now    func() time.Time
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Apply stores one change and updates the record it touches. It is safe to call twice with the
// same change and in any order with other changes (contract rule 14).
func (e *Engine) Apply(ctx context.Context, st Store, pr Projector, c Change) (Outcome, error) {
	res, err := e.ApplyBatch(ctx, st, pr, []Change{c})
	if err != nil {
		return "", err
	}
	switch {
	case len(res.Rejected) > 0:
		return "", errors.New(res.Rejected[0].Reason)
	case res.Duplicates > 0:
		return Duplicate, nil
	case res.Unknown > 0:
		return Unknown, nil
	}
	return Applied, nil
}

// ApplyBatch applies changes in order of device and sequence. When a change is rejected (bad shape
// or clock drift) the later changes of the same device are skipped, so the caller's cursor never
// moves past a change that was not stored.
func (e *Engine) ApplyBatch(ctx context.Context, st Store, pr Projector, changes []Change) (BatchResult, error) {
	var res BatchResult
	ordered := append([]Change(nil), changes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].ID.Device != ordered[j].ID.Device {
			return ordered[i].ID.Device < ordered[j].ID.Device
		}
		return ordered[i].ID.Seq < ordered[j].ID.Seq
	})
	blocked := map[string]bool{}
	dirty := map[RecordRef]bool{}
	now := e.now()
	for _, c := range ordered {
		if blocked[c.ID.Device] {
			res.Rejected = append(res.Rejected, Rejection{ID: c.ID, Reason: "skipped after an earlier change of this device was rejected"})
			continue
		}
		if err := c.Validate(); err != nil {
			res.Rejected = append(res.Rejected, Rejection{ID: c.ID, Reason: err.Error()})
			blocked[c.ID.Device] = true
			continue
		}
		if err := CheckDrift(c.HLC, now); err != nil {
			res.Rejected = append(res.Rejected, Rejection{ID: c.ID, Reason: err.Error()})
			blocked[c.ID.Device] = true
			continue
		}
		old, err := st.Get(ctx, c.Collection, c.Key, c.Field, c.ID.Device)
		if err != nil {
			return res, err
		}
		if old != nil && old.ID.Seq >= c.ID.Seq {
			res.Duplicates++
			if err := st.Bump(ctx, c.ID.Device, c.ID.Seq); err != nil {
				return res, err
			}
			continue
		}
		c.Status = StatusApplied
		if !e.Schema.Knows(c.Collection, c.Field) {
			c.Status = StatusUnknown
		}
		if err := st.Put(ctx, c); err != nil {
			return res, err
		}
		if err := st.Bump(ctx, c.ID.Device, c.ID.Seq); err != nil {
			return res, err
		}
		if c.HLC > res.MaxHLC {
			res.MaxHLC = c.HLC
		}
		if c.Status == StatusUnknown {
			res.Unknown++
			continue
		}
		res.Applied++
		dirty[RecordRef{c.Collection, c.Key}] = true
	}
	return res, e.flush(ctx, st, pr, dirty)
}

// RetryUnknown re-checks the changes stored without being applied. Call it after the schema grows.
// It returns how many were applied now.
func (e *Engine) RetryUnknown(ctx context.Context, st Store, pr Projector) (int, error) {
	list, err := st.Unknown(ctx)
	if err != nil {
		return 0, err
	}
	dirty := map[RecordRef]bool{}
	n := 0
	for _, c := range list {
		if !e.Schema.Knows(c.Collection, c.Field) {
			continue
		}
		c.Status = StatusApplied
		if err := st.Put(ctx, c); err != nil {
			return n, err
		}
		dirty[RecordRef{c.Collection, c.Key}] = true
		n++
	}
	return n, e.flush(ctx, st, pr, dirty)
}

// Reproject rebuilds the listed records (all pending ones when none are given).
func (e *Engine) Reproject(ctx context.Context, st Store, pr Projector, refs ...RecordRef) error {
	dirty := map[RecordRef]bool{}
	for _, r := range refs {
		dirty[r] = true
	}
	return e.flush(ctx, st, pr, dirty)
}

// flush projects the dirty records and every pending record, and repeats while that makes progress,
// because a record can wait for another one (a membership waits for its person).
func (e *Engine) flush(ctx context.Context, st Store, pr Projector, dirty map[RecordRef]bool) error {
	pending, err := st.Pending(ctx)
	if err != nil {
		return err
	}
	for _, r := range pending {
		dirty[r] = true
	}
	for pass := 0; pass < 50 && len(dirty) > 0; pass++ {
		progress := false
		refs := make([]RecordRef, 0, len(dirty))
		for r := range dirty {
			refs = append(refs, r)
		}
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Collection != refs[j].Collection {
				return refs[i].Collection < refs[j].Collection
			}
			return refs[i].Key < refs[j].Key
		})
		for _, r := range refs {
			entries, err := st.Entries(ctx, r.Collection, r.Key)
			if err != nil {
				return err
			}
			v := e.View(r.Collection, r.Key, entries)
			if err := st.SyncReview(ctx, r, v.Life == Parked, parkedDetail(v)); err != nil {
				return err
			}
			perr := pr.Project(ctx, v)
			switch {
			case perr == nil:
				if err := st.SetPending(ctx, r, ""); err != nil {
					return err
				}
				delete(dirty, r)
				progress = true
			case errors.Is(perr, ErrNotReady):
				if err := st.SetPending(ctx, r, perr.Error()); err != nil {
					return err
				}
			default:
				return fmt.Errorf("project %s %s: %w", r.Collection, r.Key, perr)
			}
		}
		if !progress {
			break
		}
	}
	return nil
}

func parkedDetail(v RecordView) string {
	if v.Life != Parked {
		return ""
	}
	return fmt.Sprintf("delete %s#%d has %d edit(s) it had not seen", v.DeleteID.Device, v.DeleteID.Seq, len(v.Uncovered))
}

// View resolves the stored changes of one record. Changes stored as unknown are left out.
func (e *Engine) View(collection, key string, entries []Change) RecordView {
	v := RecordView{Collection: collection, Key: key, Fields: map[string]Resolved{}}
	col, _ := e.Schema.Collection(collection)
	byField := map[string][]Change{}
	var life *Change
	for i := range entries {
		c := entries[i]
		if c.Status == StatusUnknown {
			continue
		}
		if c.HLC > v.LatestHLC {
			v.LatestHLC = c.HLC
		}
		if c.IsLife() {
			if life == nil || after(c.HLC, c.ID.Device, life.HLC, life.ID.Device) {
				life = &entries[i]
			}
			continue
		}
		byField[c.Field] = append(byField[c.Field], c)
	}
	for name, list := range byField {
		f, ok := col.Field(name)
		if !ok {
			continue
		}
		p := f.Policy
		if p == "" {
			p = LWW
		}
		if r, ok := ResolveField(p, list); ok {
			v.Fields[name] = r
		}
	}
	if life == nil || life.Op == OpRestore {
		return v
	}
	v.Life, v.DeleteID = Deleted, life.ID
	for _, list := range byField {
		for _, c := range list {
			if !life.Seen.Covers(c.ID.Device, c.ID.Seq) {
				v.Uncovered = append(v.Uncovered, c.ID)
			}
		}
	}
	if len(v.Uncovered) > 0 {
		sort.Slice(v.Uncovered, func(i, j int) bool {
			if v.Uncovered[i].Device != v.Uncovered[j].Device {
				return v.Uncovered[i].Device < v.Uncovered[j].Device
			}
			return v.Uncovered[i].Seq < v.Uncovered[j].Seq
		})
		v.Life = Parked
	}
	return v
}
