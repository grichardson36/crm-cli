package replica

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Policy decides what a field holds when several devices changed it (contract rule 7).
type Policy string

// The four policies. The schema declares one per field, lww is the default.
const (
	LWW      Policy = "lww"       // the change with the latest clock wins
	KeepBoth Policy = "keep-both" // concurrent text edits are both kept, one after the other
	Max      Policy = "max"       // the largest value wins (dates and numbers that only grow)
	SetUnion Policy = "set-union" // JSON arrays are merged, values are only added
)

// KeepBothSeparator joins concurrent values of a keep-both field.
const KeepBothSeparator = "\n\n--- edited on another device ---\n\n"

// ParsePolicy reads a policy name. An empty name is lww.
func ParsePolicy(s string) (Policy, error) {
	switch Policy(s) {
	case "", LWW:
		return LWW, nil
	case KeepBoth, Max, SetUnion:
		return Policy(s), nil
	}
	return "", fmt.Errorf("unknown conflict policy %q", s)
}

// Resolved is the value a field holds after policy resolution. Op is set or unset.
type Resolved struct {
	Op     Op
	Value  json.RawMessage
	HLC    string // latest clock among the changes that contributed
	Device string
	Seq    int64
}

// ResolveField merges the latest change of each device for one field. entries holds at most one
// change per device and only set or unset changes. The result depends only on the set of entries,
// never on the order they arrived in. It reports false when there are no entries.
func ResolveField(p Policy, entries []Change) (Resolved, bool) {
	if len(entries) == 0 {
		return Resolved{}, false
	}
	switch p {
	case Max:
		return resolveMax(entries), true
	case SetUnion:
		return resolveUnion(entries), true
	case KeepBoth:
		return resolveKeepBoth(entries), true
	}
	return resolveLWW(entries), true
}

func fromChange(c Change) Resolved {
	r := Resolved{Op: c.Op, HLC: c.HLC, Device: c.ID.Device, Seq: c.ID.Seq}
	if c.Op == OpSet {
		r.Value = c.Value
	} else {
		r.Op = OpUnset
	}
	return r
}

func newest(entries []Change) Change {
	best := entries[0]
	for _, e := range entries[1:] {
		if after(e.HLC, e.ID.Device, best.HLC, best.ID.Device) {
			best = e
		}
	}
	return best
}

func resolveLWW(entries []Change) Resolved { return fromChange(newest(entries)) }

func sets(entries []Change) []Change {
	var out []Change
	for _, e := range entries {
		if e.Op == OpSet {
			out = append(out, e)
		}
	}
	return out
}

func resolveMax(entries []Change) Resolved {
	ss := sets(entries)
	if len(ss) == 0 {
		return resolveLWW(entries)
	}
	best := ss[0]
	for _, e := range ss[1:] {
		c := compareJSON(e.Value, best.Value)
		if c > 0 || (c == 0 && after(e.HLC, e.ID.Device, best.HLC, best.ID.Device)) {
			best = e
		}
	}
	r := fromChange(best)
	r.HLC = newest(entries).HLC
	return r
}

// compareJSON orders two JSON values: numbers by value, strings by text, anything else by bytes.
func compareJSON(a, b json.RawMessage) int {
	var fa, fb float64
	if json.Unmarshal(a, &fa) == nil && json.Unmarshal(b, &fb) == nil {
		switch {
		case fa < fb:
			return -1
		case fa > fb:
			return 1
		}
		return 0
	}
	var sa, sb string
	if json.Unmarshal(a, &sa) == nil && json.Unmarshal(b, &sb) == nil {
		return strings.Compare(sa, sb)
	}
	return bytes.Compare(a, b)
}

func resolveUnion(entries []Change) Resolved {
	ss := sets(entries)
	if len(ss) == 0 {
		return resolveLWW(entries)
	}
	seen := map[string]json.RawMessage{}
	for _, e := range ss {
		var arr []json.RawMessage
		if json.Unmarshal(e.Value, &arr) != nil {
			arr = []json.RawMessage{e.Value}
		}
		for _, v := range arr {
			var buf bytes.Buffer
			if json.Compact(&buf, v) == nil {
				seen[buf.String()] = json.RawMessage(buf.Bytes())
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]json.RawMessage, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	raw, _ := json.Marshal(out)
	n := newest(ss)
	return Resolved{Op: OpSet, Value: raw, HLC: newest(entries).HLC, Device: n.ID.Device, Seq: n.ID.Seq}
}

// resolveKeepBoth keeps every change that no other change had already seen. A change is dropped
// when another change's Seen vector covers it, because that writer saw it and wrote on top of it.
// What is left are concurrent edits. They are joined, oldest first, with KeepBothSeparator.
func resolveKeepBoth(entries []Change) Resolved {
	var heads []Change
	for _, e := range entries {
		dominated := false
		for _, f := range entries {
			if f.ID != e.ID && f.Seen.Covers(e.ID.Device, e.ID.Seq) && f.ID.Device != e.ID.Device {
				dominated = true
				break
			}
		}
		if !dominated {
			heads = append(heads, e)
		}
	}
	if len(heads) == 0 {
		heads = entries
	}
	n := newest(heads)
	var texts []Change
	for _, h := range heads {
		if h.Op == OpSet {
			texts = append(texts, h)
		}
	}
	if len(texts) == 0 {
		r := fromChange(n)
		r.HLC = newest(entries).HLC
		return r
	}
	sort.Slice(texts, func(i, j int) bool {
		return after(texts[j].HLC, texts[j].ID.Device, texts[i].HLC, texts[i].ID.Device)
	})
	var parts []string
	seen := map[string]bool{}
	for _, t := range texts {
		var s string
		if json.Unmarshal(t.Value, &s) != nil {
			s = string(t.Value)
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		parts = append(parts, s)
	}
	raw, _ := json.Marshal(strings.Join(parts, KeepBothSeparator))
	w := newest(texts)
	return Resolved{Op: OpSet, Value: raw, HLC: newest(entries).HLC, Device: w.ID.Device, Seq: w.ID.Seq}
}
