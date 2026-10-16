package model

// ObjectKinds are the objects that link to each other. Tags attach to anything and have their own
// commands (crm tag apply, crm tag remove).
var ObjectKinds = []string{"person", "organization", "deal", "task"}

// Relation is one stored link between two kinds of object. Holder is the kind whose row stores the
// link: a single value (people.org_id) or, when Many is set, a join table (task_people).
// Every relation is made, viewed and removed from both ends (patterns/link-from-either-side).
type Relation struct {
	Holder string
	Target string
	Many   bool
}

// Relations is the one list of links. The CLI, MCP, TUI and schema all read it.
var Relations = []Relation{
	{Holder: "person", Target: "organization"},
	{Holder: "deal", Target: "organization"},
	{Holder: "deal", Target: "person"},
	{Holder: "task", Target: "deal"},
	{Holder: "task", Target: "person", Many: true},
}

// RelationBetween finds the relation joining two kinds, in either direction.
func RelationBetween(a, b string) (Relation, bool) {
	for _, r := range Relations {
		if (r.Holder == a && r.Target == b) || (r.Holder == b && r.Target == a) {
			return r, true
		}
	}
	return Relation{}, false
}

// RelatedKinds lists the kinds an object of the given kind can link to, in ObjectKinds order.
func RelatedKinds(kind string) []string {
	var out []string
	for _, k := range ObjectKinds {
		if _, ok := RelationBetween(kind, k); ok && k != kind {
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
