package replica

import "sort"

// Field declares one syncable field of a collection.
type Field struct {
	Name   string
	Policy Policy // empty means lww
	Ref    string // collection the value points to, "" for plain values. A ref value is the target's key.
}

// Collection declares a set of records of one kind (a table in SQLite).
type Collection struct {
	Name   string
	Fields []Field
}

// Field finds a field by name.
func (c Collection) Field(name string) (Field, bool) {
	for _, f := range c.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// Schema is the set of collections a build can apply. Version rises whenever it changes, and goes
// into every change a device writes.
type Schema struct {
	Version     int
	Collections map[string]Collection
}

// NewSchema builds a schema from a list of collections.
func NewSchema(version int, cols ...Collection) Schema {
	s := Schema{Version: version, Collections: map[string]Collection{}}
	for _, c := range cols {
		s.Collections[c.Name] = c
	}
	return s
}

// Collection finds a collection by name.
func (s Schema) Collection(name string) (Collection, bool) {
	c, ok := s.Collections[name]
	return c, ok
}

// Knows reports whether the collection and field are both declared. Life changes (empty field)
// only need the collection.
func (s Schema) Knows(collection, field string) bool {
	c, ok := s.Collections[collection]
	if !ok {
		return false
	}
	if field == "" {
		return true
	}
	_, ok = c.Field(field)
	return ok
}

// Names lists the collection names in order.
func (s Schema) Names() []string {
	out := make([]string, 0, len(s.Collections))
	for n := range s.Collections {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
