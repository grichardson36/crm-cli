// Package replicasql is the SQLite adapter of the sync contract. A registry lists the tables that
// sync. Triggers capture local writes into replica_changes in the same transaction as the row
// edit. A projector writes resolved remote state back into the tables.
package replicasql

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/replica"
)

// SchemaVersion is written into every change. Raise it when the registry changes in a way peers
// should notice (a new table, a renamed field).
const SchemaVersion = 1

// Poly describes a column that holds the id of a row in one of several tables, chosen by the value
// of another column. On the wire the value is the uuid of the target row.
type Poly struct {
	Column     string            // the id column
	KindColumn string            // the column that says which table
	Targets    map[string]string // kind value to table
}

// Table is one registry line. Columns are found from the database, so a column added later syncs
// without a change here. A column that is a foreign key to another registered table syncs as the
// uuid of the target row.
type Table struct {
	Name   string
	Skip   []string                  // columns that never sync (id, uuid and updated_at never do)
	Policy map[string]replica.Policy // conflict policy per column, default lww
	Poly   []Poly
	Local  map[string]string // NOT NULL columns that do not sync, with the SQL literal rows made by sync get
	Pair   [2]string         // two ref columns where the CHECK needs the smaller id first
	// Natural lists the columns of a natural key (a unique name). Rows that exist when sync is
	// turned on get a uuid derived from these values, so the same built-in or same-named row made
	// on two devices is one record.
	Natural []string
	// AddUUID adds a uuid column when sync is turned on, for tables that never had one.
	AddUUID bool
}

// DefaultRegistry lists the synced tables. Adding a table is adding a line.
func DefaultRegistry() []Table {
	person := map[string]string{"person": "people", "organization": "organizations", "deal": "deals", "interaction": "interactions", "task": "tasks"}
	return []Table{
		{Name: "people", Policy: map[string]replica.Policy{"notes": replica.KeepBoth, "last_contacted_at": replica.Max}},
		{Name: "organizations", Policy: map[string]replica.Policy{"notes": replica.KeepBoth}},
		{Name: "interactions", Policy: map[string]replica.Policy{"content": replica.KeepBoth}},
		{Name: "deals", Policy: map[string]replica.Policy{"notes": replica.KeepBoth}},
		{Name: "tasks", Policy: map[string]replica.Policy{"description": replica.KeepBoth}},
		{Name: "tags", Natural: []string{"name"}},
		{Name: "taggings", Poly: []Poly{{Column: "entity_id", KindColumn: "entity_type", Targets: person}}},
		{Name: "person_emails", AddUUID: true},
		{Name: "person_phones", AddUUID: true},
		{Name: "person_handles", AddUUID: true},
		{Name: "custom_fields", AddUUID: true, Poly: []Poly{{Column: "entity_id", KindColumn: "entity_type", Targets: map[string]string{"person": "people", "organization": "organizations", "deal": "deals", "interaction": "interactions", "task": "tasks"}}}},
		{Name: "relation_kind", Natural: []string{"object_type", "name"}},
		{Name: "membership"},
		{Name: "person_relation"},
		{Name: "org_relation"},
		{Name: "deal_person"},
		{Name: "deal_org"},
		{Name: "activity_target", Poly: []Poly{
			{Column: "source_id", KindColumn: "source_kind", Targets: map[string]string{"task": "tasks", "interaction": "interactions"}},
			{Column: "target_id", KindColumn: "target_kind", Targets: map[string]string{"person": "people", "organization": "organizations", "deal": "deals"}},
		}},
		{Name: "merge_alias",
			Poly:  []Poly{{Column: "survivor_id", KindColumn: "kind", Targets: map[string]string{"person": "people", "organization": "organizations"}}},
			Skip:  []string{"old_id"},
			Local: map[string]string{"old_id": "-1"}},
		{Name: "note_types", Natural: []string{"name"}},
		{Name: "vocabulary", Natural: []string{"kind", "name"}},
		{Name: "name_aliases", Natural: []string{"alias"}},
		{Name: "company_ignore", Natural: []string{"key"}},
		{Name: "dupe_dismissals", Pair: [2]string{"person_a", "person_b"}},
		{Name: "org_dupe_dismissals", Pair: [2]string{"org_a", "org_b"}},
	}
}

// NotCovered lists database tables that do not sync and why, for status output and the docs.
func NotCovered() map[string]string {
	return map[string]string{
		"external_ids":   "ties a row to a source account on this device",
		"sync_runs":      "history of source syncs on this device",
		"sync_cursors":   "source sync cursors of this device",
		"sync_inbox":     "source sync inbox of this device",
		"sync_conflicts": "CardDAV conflict log of this device",
		"dav_state":      "CardDAV base state of this device",
		"dav_issues":     "CardDAV cards this device could not read",
		"field_versions": "per-device change times used by source sync",
		"tombstones":     "local purge log, replaced by delete changes",
		"merge_log":      "undo history holds device-local row ids",
		"meta":           "device settings",
	}
}

// Column is one column of a registered table after discovery.
type Column struct {
	Name       string
	NotNull    bool
	HasDefault bool
	Ref        string // target table, "" for a plain column
	Poly       *Poly  // set for the id column of a polymorphic ref
	Policy     replica.Policy
}

// Described is a table with its columns found from the database.
type Described struct {
	Table
	Columns   []Column
	HasUpdate bool     // has an updated_at column
	NullUUID  bool     // the uuid column may be NULL, so capture fills it
	Excluded  []string // columns left out because they point at an unsynced table
}

// Column finds a synced column by name.
func (d *Described) Column(name string) (Column, bool) {
	for _, c := range d.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return Column{}, false
}

// Registry is the described tables of one database.
type Registry struct {
	Tables map[string]*Described
	Order  []string
}

// Describe reads the tables of the registry from the database.
func Describe(db interface {
	Query(string, ...any) (*sql.Rows, error)
}, tables []Table) (*Registry, error) {
	reg := &Registry{Tables: map[string]*Described{}}
	names := map[string]bool{}
	for _, t := range tables {
		names[t.Name] = true
	}
	for _, t := range tables {
		d := &Described{Table: t}
		skip := map[string]bool{"id": true, "uuid": true, "updated_at": true}
		for _, s := range t.Skip {
			skip[s] = true
		}
		polyCol := map[string]*Poly{}
		for i := range t.Poly {
			polyCol[t.Poly[i].Column] = &t.Poly[i]
		}
		fk := map[string]string{}
		fr, err := db.Query(fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, t.Name))
		if err != nil {
			return nil, err
		}
		for fr.Next() {
			var id, seq int
			var table, from string
			var to, onUpdate, onDelete, match sql.NullString
			if err := fr.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				_ = fr.Close()
				return nil, err
			}
			fk[from] = table
		}
		_ = fr.Close()
		rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, t.Name))
		if err != nil {
			return nil, err
		}
		found := false
		var requiredLeft []string
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				_ = rows.Close()
				return nil, err
			}
			found = true
			switch name {
			case "uuid":
				d.NullUUID = notnull == 0
			case "updated_at":
				d.HasUpdate = true
			}
			if skip[name] {
				if t.Skip != nil && contains(t.Skip, name) && notnull == 1 && !dflt.Valid {
					requiredLeft = append(requiredLeft, name)
				}
				continue
			}
			c := Column{Name: name, NotNull: notnull == 1, HasDefault: dflt.Valid, Policy: t.Policy[name]}
			if p, ok := polyCol[name]; ok {
				c.Poly = p
			} else if target, ok := fk[name]; ok {
				if !names[target] {
					d.Excluded = append(d.Excluded, name)
					if notnull == 1 && !dflt.Valid {
						requiredLeft = append(requiredLeft, name)
					}
					continue
				}
				c.Ref = target
			}
			d.Columns = append(d.Columns, c)
		}
		_ = rows.Close()
		if !found {
			return nil, fmt.Errorf("registry table %q does not exist", t.Name)
		}
		for _, c := range d.Columns {
			if c.Poly != nil {
				if _, ok := d.Column(c.Poly.KindColumn); !ok {
					return nil, fmt.Errorf("registry table %q: kind column %q of %q is not a synced column", t.Name, c.Poly.KindColumn, c.Name)
				}
			}
		}
		for _, x := range requiredLeft {
			if _, ok := t.Local[x]; !ok {
				return nil, fmt.Errorf("registry table %q: column %q is required but does not sync, give it a Local default", t.Name, x)
			}
		}
		reg.Tables[t.Name] = d
		reg.Order = append(reg.Order, t.Name)
	}
	return reg, nil
}

// Schema builds the core schema from the described tables.
func (r *Registry) Schema() replica.Schema {
	var cols []replica.Collection
	for _, n := range r.Order {
		d := r.Tables[n]
		c := replica.Collection{Name: n}
		for _, col := range d.Columns {
			f := replica.Field{Name: col.Name, Policy: col.Policy}
			f.Ref = col.Ref
			c.Fields = append(c.Fields, f)
		}
		sort.Slice(c.Fields, func(i, j int) bool { return c.Fields[i].Name < c.Fields[j].Name })
		cols = append(cols, c)
	}
	return replica.NewSchema(SchemaVersion, cols...)
}

// Signature changes whenever the set of synced columns or their rules change. It decides whether
// triggers must be rebuilt.
func (r *Registry) Signature() string {
	var b strings.Builder
	fmt.Fprintf(&b, "v%d;", SchemaVersion)
	for _, n := range r.Order {
		d := r.Tables[n]
		fmt.Fprintf(&b, "%s[upd=%t,nu=%t,pair=%v]", n, d.HasUpdate, d.NullUUID, d.Pair)
		for _, c := range d.Columns {
			fmt.Fprintf(&b, "%s:%t:%t:%s:%s:", c.Name, c.NotNull, c.HasDefault, c.Ref, c.Policy)
			if c.Poly != nil {
				fmt.Fprintf(&b, "poly(%s)", c.Poly.KindColumn)
			}
			b.WriteString(",")
		}
		b.WriteString(";")
	}
	return b.String()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
