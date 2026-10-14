package dav

import "strings"

// mergeMode picks how the phone's version is folded into the CRM.
type mergeMode int

const (
	// modeUpdate is the three-way merge against the base the phone last held.
	modeUpdate mergeMode = iota
	// modeLink fills blanks only. It is used when a new phone contact is attached to an
	// existing person, where the CRM's values are trusted over the phone's.
	modeLink
)

// scalarDecision says what to do with one scalar field.
type scalarDecision struct {
	Field    string
	Take     bool // write the device value into the CRM
	Conflict bool // both sides changed to different values
	Winner   string
	CRM      string
	Device   string
}

// mergeScalars decides every scalar field. crmChangedAt returns when the CRM last changed a
// field ("" when unknown, which counts as oldest). now is the device change time, which is the
// request time because the phone does not say when the edit happened. Times compare as
// "2006-01-02 15:04:05" strings in UTC.
func mergeScalars(base, crm, dev map[string]string, crmChangedAt func(field string) string, now string, mode mergeMode) []scalarDecision {
	var out []scalarDecision
	for _, f := range scalarFields {
		b, c, d := base[f], crm[f], dev[f]
		dec := scalarDecision{Field: f, CRM: c, Device: d}
		switch {
		case d == c:
			continue
		case mode == modeLink:
			if c == "" && d != "" {
				dec.Take = true
			}
		case d == b:
			// The phone did not change it. Whatever the CRM has stays and goes out on the next GET.
		case c == b:
			dec.Take = true
		default:
			dec.Conflict = true
			dec.Winner = "device"
			dec.Take = true
			if at := crmChangedAt(f); at != "" && at > now {
				dec.Winner = "crm"
				dec.Take = false
			}
		}
		if f == fieldFirst && dec.Take && d == "" {
			dec.Take = false // a person always keeps a first name
		}
		if dec.Take || dec.Conflict {
			out = append(out, dec)
		}
	}
	return out
}

// listPlan is the set difference result for emails or phones.
type listPlan struct {
	Add        []Item   // on the phone only, new there
	RemoveKeys []string // on the phone's base, removed there, still in the CRM
	Relabel    []Item   // the phone changed the label
}

func indexItems(items []Item, key func(string) string) map[string]Item {
	m := make(map[string]Item, len(items))
	for _, it := range items {
		k := key(it.Value)
		if k == "" {
			continue
		}
		if _, dup := m[k]; !dup {
			m[k] = it
		}
	}
	return m
}

// mergeItems merges a list by set. Additions from either side are kept. A removal on the phone
// removes only an item that was in the base. An item the CRM removed stays removed.
func mergeItems(base, crm, dev []Item, key func(string) string) listPlan {
	b, c, d := indexItems(base, key), indexItems(crm, key), indexItems(dev, key)
	var plan listPlan
	seen := map[string]bool{}
	for _, it := range dev {
		k := key(it.Value)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		cur, inC := c[k]
		if !inC {
			if _, inB := b[k]; !inB {
				plan.Add = append(plan.Add, it)
			}
			continue
		}
		if it.Label != "" && !strings.EqualFold(it.Label, b[k].Label) && !strings.EqualFold(it.Label, cur.Label) {
			plan.Relabel = append(plan.Relabel, it)
		}
	}
	for k := range b {
		if _, inD := d[k]; inD {
			continue
		}
		if _, inC := c[k]; inC {
			plan.RemoveKeys = append(plan.RemoveKeys, k)
		}
	}
	return plan
}

// handleDecision says what to do with one handle kind.
type handleDecision struct {
	Kind     string
	Value    string // the device value to set, empty when Remove
	Remove   bool
	Conflict bool
	CRM      string
	Device   string
}

// mergeHandles merges social handles and the website by kind. A kind both sides changed to
// different values is a conflict that the phone wins, because the CRM does not record when a
// handle changed.
func mergeHandles(base, crm, dev map[string]string, mode mergeMode) []handleDecision {
	kinds := map[string]bool{}
	for _, m := range []map[string]string{base, crm, dev} {
		for k := range m {
			kinds[k] = true
		}
	}
	same := func(a, b string) bool { return a == b || (a != "" && b != "" && handleKey(a) == handleKey(b)) }
	var out []handleDecision
	for _, k := range sortedKeys(kinds) {
		b, c, d := base[k], crm[k], dev[k]
		dec := handleDecision{Kind: k, CRM: c, Device: d}
		switch {
		case same(d, c):
			continue
		case mode == modeLink:
			if c != "" || d == "" {
				continue
			}
			dec.Value = d
		case same(d, b):
			continue
		case d == "":
			if !same(c, b) {
				continue // the CRM replaced it with a different value, which is a new item
			}
			dec.Remove = true
		case c == "" || same(c, b):
			dec.Value = d
		default:
			dec.Conflict = true
			dec.Value = d
		}
		out = append(out, dec)
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// insertion sort, the sets are tiny
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
