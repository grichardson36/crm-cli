// Package modules holds the arrangement of the program's views: which are on and in what order.
// Every view is a module (Home, People, Follow-up, Tasks, Organizations, Birthdays, Merge,
// Archived, Sync, Deals). Turning one off hides it from the tabs, the number keys, [ and ], the
// view list and the palette. Its data, its links and every other way to reach it stay. The
// arrangement is per device, in the [modules] table of ~/.crm/config.toml next to [keys]:
//
//	[modules]
//	order  = ["home", "people", "follow-up", "tasks", "organizations", "birthdays", "merge", "archived", "sync", "deals"]
//	hidden = ["deals"]
//
// The rest of config.toml is kept when the table is written.
package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Module is one view that can be turned on or off.
type Module struct {
	ID    string // the name used in config.toml and on the command line
	Name  string // the name shown in tabs and lists
	Blurb string
}

// All lists every module in the default order.
var All = []Module{
	{"home", "Home", "what needs attention today"},
	{"people", "People", "everyone, find with / or ctrl+k"},
	{"follow-up", "Follow-up", "people to reach out to"},
	{"tasks", "Tasks", "open tasks"},
	{"organizations", "Organizations", "companies and groups"},
	{"birthdays", "Birthdays", "upcoming birthdays"},
	{"merge", "Merge", "probable duplicates"},
	{"archived", "Archived", "archived records, restore or purge"},
	{"sync", "Sync", "sources and the inbox"},
	{"deals", "Deals", "deals with stage and value"},
}

var defaultHidden = []string{"deals"}

var aliases = map[string]string{
	"due": "follow-up", "followup": "follow-up", "follow": "follow-up", "orgs": "organizations", "org": "organizations",
	"organisations": "organizations", "birthday": "birthdays", "task": "tasks", "person": "people", "contacts": "people",
	"deal": "deals", "archive": "archived", "duplicates": "merge", "dupes": "merge",
}

// Lookup resolves a module name: its id, its shown name, or a common alias, in any case.
func Lookup(name string) (Module, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, " ", "-")
	if a, ok := aliases[n]; ok {
		n = a
	}
	for _, m := range All {
		if m.ID == n || strings.ToLower(m.Name) == n {
			return m, true
		}
	}
	return Module{}, false
}

// Arrangement is the order of all modules and which are hidden. Both always cover every module
// once the arrangement went through Normalize.
type Arrangement struct {
	Order  []string
	Hidden []string
}

// Default is the arrangement the program ships with.
func Default() Arrangement {
	a := Arrangement{Hidden: append([]string{}, defaultHidden...)}
	for _, m := range All {
		a.Order = append(a.Order, m.ID)
	}
	return a
}

// IsDefault reports whether a equals the shipped arrangement.
func (a Arrangement) IsDefault() bool {
	d := Default()
	n := a.Normalize()
	if len(n.Order) != len(d.Order) || len(n.Hidden) != len(d.Hidden) {
		return false
	}
	for i := range d.Order {
		if n.Order[i] != d.Order[i] {
			return false
		}
	}
	for i := range d.Hidden {
		if n.Hidden[i] != d.Hidden[i] {
			return false
		}
	}
	return true
}

// Normalize drops unknown and repeated names and appends modules missing from the order in the
// default order. Hidden keeps the order of the arrangement.
func (a Arrangement) Normalize() Arrangement {
	var out Arrangement
	seen := map[string]bool{}
	for _, n := range a.Order {
		if m, ok := Lookup(n); ok && !seen[m.ID] {
			seen[m.ID] = true
			out.Order = append(out.Order, m.ID)
		}
	}
	for _, m := range All {
		if !seen[m.ID] {
			out.Order = append(out.Order, m.ID)
		}
	}
	hid := map[string]bool{}
	for _, n := range a.Hidden {
		if m, ok := Lookup(n); ok {
			hid[m.ID] = true
		}
	}
	for _, id := range out.Order {
		if hid[id] {
			out.Hidden = append(out.Hidden, id)
		}
	}
	return out
}

// IsHidden reports whether the module is off.
func (a Arrangement) IsHidden(id string) bool {
	for _, h := range a.Hidden {
		if h == id {
			return true
		}
	}
	return false
}

// Visible lists the modules that are on, in order.
func (a Arrangement) Visible() []string {
	n := a.Normalize()
	var out []string
	for _, id := range n.Order {
		if !n.IsHidden(id) {
			out = append(out, id)
		}
	}
	return out
}

// Position is the 1-based place of the module among the visible ones, 0 when it is off.
func (a Arrangement) Position(id string) int {
	for i, v := range a.Visible() {
		if v == id {
			return i + 1
		}
	}
	return 0
}

// Check returns why the arrangement is not allowed: one module must stay on, and Home or People.
func (a Arrangement) Check() error {
	vis := a.Visible()
	if len(vis) == 0 {
		return fmt.Errorf("at least one module must stay on")
	}
	for _, id := range vis {
		if id == "home" || id == "people" {
			return nil
		}
	}
	return fmt.Errorf("Home or People must stay on, so the program always has a start screen")
}

func resolve(name string) (Module, error) {
	m, ok := Lookup(name)
	if !ok {
		ids := make([]string, len(All))
		for i, m := range All {
			ids[i] = m.ID
		}
		return Module{}, fmt.Errorf("no module %q (modules: %s)", name, strings.Join(ids, ", "))
	}
	return m, nil
}

// With returns the arrangement with the module turned on or off. A change that breaks Check is refused.
func (a Arrangement) With(name string, on bool) (Arrangement, error) {
	m, err := resolve(name)
	if err != nil {
		return a, err
	}
	n := a.Normalize()
	var hidden []string
	for _, h := range n.Hidden {
		if h != m.ID {
			hidden = append(hidden, h)
		}
	}
	if !on {
		hidden = append(hidden, m.ID)
	}
	n.Hidden = hidden
	n = n.Normalize()
	if err := n.Check(); err != nil {
		return a, err
	}
	return n, nil
}

// Moved returns the arrangement with the module at a place among all modules (1 is first). Moving
// does not turn it on or off.
func (a Arrangement) Moved(name string, pos int) (Arrangement, error) {
	m, err := resolve(name)
	if err != nil {
		return a, err
	}
	n := a.Normalize()
	if pos < 1 || pos > len(n.Order) {
		return a, fmt.Errorf("position %d is out of range, use 1 to %d", pos, len(n.Order))
	}
	var rest []string
	for _, id := range n.Order {
		if id != m.ID {
			rest = append(rest, id)
		}
	}
	order := append(append(append([]string{}, rest[:pos-1]...), m.ID), rest[pos-1:]...)
	n.Order = order
	return n.Normalize(), nil
}

// ConfigFile is the per-device settings file in the data directory.
func ConfigFile(dataDir string) string { return filepath.Join(dataDir, "config.toml") }

// Load reads the [modules] table. It never fails: a missing file or table is the default, and every
// problem (bad TOML, wrong types, unknown module names, an arrangement with nothing to start from)
// is returned as a message and falls back to the default for that part.
func Load(dataDir string) (Arrangement, []string) {
	data, err := os.ReadFile(ConfigFile(dataDir))
	if err != nil {
		return Default(), nil
	}
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return Default(), []string{fmt.Sprintf("%s is not valid TOML (%v): the module arrangement was not loaded", ConfigFile(dataDir), err)}
	}
	table, ok := raw["modules"]
	if !ok {
		return Default(), nil
	}
	m, ok := table.(map[string]any)
	if !ok {
		return Default(), []string{"[modules] in config.toml must be a table with order and hidden lists"}
	}
	var problems []string
	list := func(key string) ([]string, bool) {
		v, present := m[key]
		if !present {
			return nil, false
		}
		arr, ok := v.([]any)
		if !ok {
			problems = append(problems, fmt.Sprintf("modules.%s: use a list of module names", key))
			return nil, false
		}
		var out []string
		for _, e := range arr {
			s, ok := e.(string)
			if !ok {
				problems = append(problems, fmt.Sprintf("modules.%s: every entry must be a module name in quotes", key))
				return nil, false
			}
			if _, err := resolve(s); err != nil {
				problems = append(problems, fmt.Sprintf("modules.%s: %v", key, err))
				continue
			}
			out = append(out, s)
		}
		return out, true
	}
	a := Default()
	if order, ok := list("order"); ok {
		a.Order = order
	}
	if hidden, ok := list("hidden"); ok {
		a.Hidden = hidden
	}
	a = a.Normalize()
	if err := a.Check(); err != nil {
		problems = append(problems, fmt.Sprintf("modules: %v, so the default arrangement is used", err))
		return Default(), problems
	}
	return a, problems
}

// Save writes the arrangement as the [modules] table and keeps every other setting in the file. The
// default arrangement removes the table.
func Save(dataDir string, a Arrangement) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dataDir, err)
	}
	path := ConfigFile(dataDir)
	raw := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(data), &raw); err != nil {
			return fmt.Errorf("%s is not valid TOML (%v): fix or remove it before changing modules", path, err)
		}
	}
	a = a.Normalize()
	if err := a.Check(); err != nil {
		return err
	}
	if a.IsDefault() {
		delete(raw, "modules")
	} else {
		hidden := a.Hidden
		if hidden == nil {
			hidden = []string{}
		}
		raw["modules"] = map[string]any{"order": a.Order, "hidden": hidden}
	}
	var b strings.Builder
	b.WriteString("# crm settings for this device. Written by `crm theme set`, `crm keys set` and `crm modules`.\n")
	if err := toml.NewEncoder(&b).Encode(raw); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Row is one module as the CLI and MCP list it.
type Row struct {
	Order    int    `json:"order"`    // place among all modules, 1 is first
	ID       string `json:"id"`       // the name for on, off and move
	Name     string `json:"name"`     // the name shown in tabs
	On       bool   `json:"on"`       // false: hidden from tabs, keys, [ ], the view list and the palette
	Position int    `json:"position"` // place among the visible modules, 0 when off
	Key      string `json:"key"`      // the number key that opens it, empty when off or past 9
	Default  bool   `json:"default"`  // true when this module is on by default
	Blurb    string `json:"description"`
}

// Rows lists every module in order.
func (a Arrangement) Rows() []Row {
	n := a.Normalize()
	rows := make([]Row, 0, len(n.Order))
	for i, id := range n.Order {
		mod, _ := Lookup(id)
		r := Row{Order: i + 1, ID: id, Name: mod.Name, On: !n.IsHidden(id), Position: n.Position(id), Blurb: mod.Blurb}
		if r.Position >= 1 && r.Position <= 9 {
			r.Key = fmt.Sprint(r.Position)
		}
		r.Default = !contains(defaultHidden, id)
		rows = append(rows, r)
	}
	return rows
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
