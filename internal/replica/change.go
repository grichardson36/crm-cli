package replica

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Op is what a change does.
type Op string

// The four operations of a change record.
const (
	OpSet     Op = "set"     // the field has this value
	OpUnset   Op = "unset"   // the field is empty
	OpDelete  Op = "delete"  // the record is deleted (field is empty)
	OpRestore Op = "restore" // the record is alive again (field is empty)
)

// Status of a stored change.
const (
	StatusApplied = "applied"
	StatusUnknown = "unknown" // collection or field not in this build's schema, kept and retried
)

// ChangeID identifies a change. A device numbers its own changes 1, 2, 3 and so on, so applying
// the same change twice is harmless.
type ChangeID struct {
	Device string `json:"device"`
	Seq    int64  `json:"seq"`
}

// Change is one field change, the unit of replication (contract rule 2). Seen is an optional
// addition used by keep-both fields and by delete: the writer's version vector, which tells a
// reader what the writer had already seen when it wrote.
type Change struct {
	ID            ChangeID        `json:"id"`
	HLC           string          `json:"hlc"`
	Collection    string          `json:"collection"`
	Key           string          `json:"key"`
	Field         string          `json:"field"`
	Op            Op              `json:"op"`
	Value         json.RawMessage `json:"value,omitempty"`
	SchemaVersion int             `json:"schema_version"`
	Tx            string          `json:"tx,omitempty"`
	Seen          VersionVector   `json:"seen,omitempty"`
	Status        string          `json:"-"`
}

// IsLife reports whether the change is a record-level delete or restore.
func (c Change) IsLife() bool { return c.Op == OpDelete || c.Op == OpRestore }

// Validate checks the shape of a change.
func (c Change) Validate() error {
	if c.ID.Device == "" || c.ID.Seq < 1 {
		return errors.New("change needs id.device and id.seq of 1 or more")
	}
	if c.Collection == "" || c.Key == "" {
		return errors.New("change needs a collection and a key")
	}
	if _, err := ParseHLC(c.HLC); err != nil {
		return err
	}
	switch c.Op {
	case OpSet:
		if c.Field == "" || len(c.Value) == 0 {
			return errors.New("set needs a field and a value")
		}
		if !json.Valid(c.Value) {
			return errors.New("value is not valid JSON")
		}
	case OpUnset:
		if c.Field == "" {
			return errors.New("unset needs a field")
		}
	case OpDelete, OpRestore:
		if c.Field != "" {
			return fmt.Errorf("%s applies to the record, field must be empty", c.Op)
		}
	default:
		return fmt.Errorf("unknown op %q", c.Op)
	}
	return nil
}

// after reports whether a sorts after b by clock, then device (the total order for lww).
func after(aHLC, aDev, bHLC, bDev string) bool {
	if aHLC != bHLC {
		return aHLC > bHLC
	}
	return aDev > bDev
}

// VersionVector maps a device to the highest sequence number seen from it. It is the pull cursor
// (contract rule 4).
type VersionVector map[string]int64

// Clone copies the vector.
func (v VersionVector) Clone() VersionVector {
	out := make(VersionVector, len(v))
	for k, n := range v {
		out[k] = n
	}
	return out
}

// Covers reports whether the vector includes sequence seq of device.
func (v VersionVector) Covers(device string, seq int64) bool { return v[device] >= seq }

// Merge raises every entry to at least the other vector's value.
func (v VersionVector) Merge(o VersionVector) {
	for k, n := range o {
		if n > v[k] {
			v[k] = n
		}
	}
}

// Dominates reports whether v has seen everything o has.
func (v VersionVector) Dominates(o VersionVector) bool {
	for k, n := range o {
		if v[k] < n {
			return false
		}
	}
	return true
}
