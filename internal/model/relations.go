package model

// ObjectKinds are the objects that link to each other. Tags attach to anything and have their own
// commands (crm tag apply, crm tag remove). A note is the "interaction" kind (the CLI, MCP and TUI
// call it note) and is a registered object like the others: add, edit, delete, restore, link, unlink.
var ObjectKinds = []string{"person", "organization", "deal", "task", "interaction"}

// NoteKind is the object kind of a note. Its rows are the interactions table, its types are the
// note types (call, email, meeting, note, message and the ones the user adds).
const NoteKind = "interaction"

// Attribute keys a link can carry. They are the flag names of `crm <object> link` (--role, --from,
// --to, --primary, --kind, --notes) and the field names in MCP and JSON.
const (
	AttrRole    = "role"
	AttrFrom    = "from"
	AttrTo      = "to"
	AttrPrimary = "primary"
	AttrKind    = "kind"
	AttrNotes   = "notes"
)

// AttrSpec describes one link attribute for forms, flags and help.
type AttrSpec struct {
	Key   string
	Label string
	// Type is text, date, bool or kind.
	Type string
	// Suggest is set for free-text fields that offer the values already in use.
	Suggest bool
}

// AttrSpecs lists every attribute a link can carry.
var AttrSpecs = map[string]AttrSpec{
	AttrRole:    {Key: AttrRole, Label: "Role", Type: "text", Suggest: true},
	AttrFrom:    {Key: AttrFrom, Label: "From", Type: "date"},
	AttrTo:      {Key: AttrTo, Label: "To", Type: "date"},
	AttrPrimary: {Key: AttrPrimary, Label: "Primary", Type: "bool"},
	AttrKind:    {Key: AttrKind, Label: "Kind", Type: "kind"},
	AttrNotes:   {Key: AttrNotes, Label: "Notes", Type: "text"},
}

// Relation is one stored kind of link between two kinds of object, kept in one table. Every link
// is made, viewed and removed from both ends (patterns/link-from-either-side).
//
// A and B are object kinds. For membership A is the person and B the organization. For a relation
// between two objects of the same kind (person_relation, org_relation) the order matters: the row
// (a, b, kind) reads "a is <kind> b". Activity relations have a task or interaction as A.
type Relation struct {
	// Name is the storage table.
	Name string
	A, B string
	// Attrs are the attribute keys the link carries, in form order.
	Attrs []string
	// Kinded relations take their kind from relation_kind.
	Kinded bool
	// PrimaryPerA says at most one live link per A can be primary (one primary organization per person).
	PrimaryPerA bool
	// Unique says a pair is linked at most once at a time. Memberships can repeat (E3).
	Unique bool
	// LabelA is what an A calls its B objects (a person's "Organizations"). LabelB is what a B calls its A objects.
	LabelA, LabelB string
}

// Relations is the one list of links. The CLI, MCP, TUI, doctor and schema all read it.
var Relations = []Relation{
	{Name: "membership", A: "person", B: "organization", Attrs: []string{AttrRole, AttrFrom, AttrTo, AttrPrimary, AttrNotes},
		PrimaryPerA: true, LabelA: "Organizations", LabelB: "Members"},
	{Name: "person_relation", A: "person", B: "person", Attrs: []string{AttrKind, AttrNotes}, Kinded: true, Unique: true,
		LabelA: "Related people", LabelB: "Related people"},
	{Name: "org_relation", A: "organization", B: "organization", Attrs: []string{AttrKind, AttrNotes}, Kinded: true, Unique: true,
		LabelA: "Related organizations", LabelB: "Related organizations"},
	{Name: "deal_person", A: "deal", B: "person", Attrs: []string{AttrRole}, Unique: true, LabelA: "People", LabelB: "Deals"},
	{Name: "deal_org", A: "deal", B: "organization", Attrs: []string{AttrRole, AttrPrimary}, PrimaryPerA: true, Unique: true,
		LabelA: "Organizations", LabelB: "Deals"},
	{Name: "task_person", A: "task", B: "person", Attrs: []string{AttrRole}, Unique: true, LabelA: "People", LabelB: "Tasks"},
	{Name: "task_organization", A: "task", B: "organization", Attrs: []string{AttrRole}, Unique: true, LabelA: "Organizations", LabelB: "Tasks"},
	{Name: "task_deal", A: "task", B: "deal", Attrs: []string{AttrRole}, Unique: true, LabelA: "Deals", LabelB: "Tasks"},
	{Name: "interaction_person", A: "interaction", B: "person", Attrs: []string{AttrRole}, Unique: true, LabelA: "People", LabelB: "Notes"},
	{Name: "interaction_organization", A: "interaction", B: "organization", Attrs: []string{AttrRole}, Unique: true, LabelA: "Organizations", LabelB: "Notes"},
	{Name: "interaction_deal", A: "interaction", B: "deal", Attrs: []string{AttrRole}, Unique: true, LabelA: "Deals", LabelB: "Notes"},
}

// AskAttrs are the attributes the Link form asks on one line (role, dates, primary, notes). Person
// and organization relations ask their kind from a picker instead, and activity links ask nothing.
func (r Relation) AskAttrs() []string {
	if r.Activity() || r.Kinded {
		return nil
	}
	return r.Attrs
}

// SameKind reports whether both ends are the same kind of object.
func (r Relation) SameKind() bool { return r.A == r.B }

// HasAttr reports whether the link carries the attribute.
func (r Relation) HasAttr(key string) bool {
	for _, a := range r.Attrs {
		if a == key {
			return true
		}
	}
	return false
}

// Activity reports whether the relation is stored in activity_target.
func (r Relation) Activity() bool { return r.A == "task" || r.A == "interaction" }

// LabelFor is the heading of the list the given kind sees: its linked objects.
func (r Relation) LabelFor(kind string) string {
	if r.A == kind {
		return r.LabelA
	}
	return r.LabelB
}

// OtherKind is the kind at the far end of the relation, seen from kind.
func (r Relation) OtherKind(kind string) string {
	if r.A == kind {
		return r.B
	}
	return r.A
}

// RelationBetween finds the relation joining two kinds, in either direction.
func RelationBetween(a, b string) (Relation, bool) {
	for _, r := range Relations {
		if (r.A == a && r.B == b) || (r.A == b && r.B == a) {
			return r, true
		}
	}
	return Relation{}, false
}

// RelationNamed finds a relation by its storage name.
func RelationNamed(name string) (Relation, bool) {
	for _, r := range Relations {
		if r.Name == name {
			return r, true
		}
	}
	return Relation{}, false
}

// RelationsOf lists the relations an object kind takes part in.
func RelationsOf(kind string) []Relation {
	var out []Relation
	for _, r := range Relations {
		if r.A == kind || r.B == kind {
			out = append(out, r)
		}
	}
	return out
}

// RelatedKinds lists the kinds an object of the given kind can link to, in ObjectKinds order. A
// kind that links to its own kind (person to person) is included.
func RelatedKinds(kind string) []string {
	var out []string
	for _, k := range ObjectKinds {
		if _, ok := RelationBetween(kind, k); ok {
			out = append(out, k)
		}
	}
	return out
}

// ValidObjectKind reports whether k is one of ObjectKinds.
func ValidObjectKind(k string) bool {
	for _, x := range ObjectKinds {
		if x == k {
			return true
		}
	}
	return false
}
