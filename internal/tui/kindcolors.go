package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// Per-kind colors (patterns/kind-colors-are-chosen). The theme gives every object kind a color role
// (internal/theme). The user may override the color of a kind on this device, so a kind is
// recognizable in a related chip, a popup row, a list heading or a record page heading by color alone:
//
//	[colors]
//	person = "accent"      # a name from the theme palette, follows the theme
//	deal   = "#e8a33d"     # or a custom hex color
//
// resolveKindColor is the one function that decides the color of a kind. NewStyles output is patched
// with it in applyKindColors, so every view that asks Styles.KindStyle gets the resolved color. The
// short kind prefixes (p:, o:, #) stay, so nothing depends on color.

// colorKind is a kind in the Colors settings.
type colorKind struct {
	kind, label string
}

// colorKinds lists the kinds that can be colored: the kinds the object registry knows.
func colorKinds() []colorKind {
	var out []colorKind
	for _, o := range objectDefs() {
		label := o.label
		out = append(out, colorKind{kind: o.kind, label: label})
	}
	return out
}

// paletteEntry is a named color of the theme palette.
type paletteEntry struct {
	name, role string
}

// colorPalette lists the named colors of a theme that a kind may take.
var colorPalette = []paletteEntry{
	{"accent", theme.RoleAccent}, {"good", theme.RoleGood}, {"warn", theme.RoleWarn}, {"danger", theme.RoleDanger},
	{"mark", theme.RoleMark}, {"fg", theme.RoleFg}, {"dim", theme.RoleDim}, {"tab", theme.RoleTabBg},
}

// paletteColor is the color of a palette name in the theme, "" for an unknown name.
func paletteColor(t theme.Look, name string) string {
	for _, p := range colorPalette {
		if p.name == name {
			return t.Color(p.role)
		}
	}
	return ""
}

// validKindColor says whether v is a palette name or a hex color, and returns it cleaned.
func validKindColor(v string) (string, bool) {
	v = strings.TrimSpace(v)
	for _, p := range colorPalette {
		if strings.EqualFold(v, p.name) {
			return p.name, true
		}
	}
	if res := validate.HexColor(v); res.OK() {
		return res.Value, true
	}
	return "", false
}

// resolveKindColor is the color of an object kind: the user's override (a palette name resolved in
// the theme, or a hex color), else the theme's kind color. It is "" for a kind without a color.
func resolveKindColor(t theme.Look, overrides map[string]string, kind string) string {
	if v, ok := overrides[kind]; ok {
		if _, valid := validKindColor(v); valid {
			if c := paletteColor(t, strings.ToLower(v)); c != "" {
				return c
			}
			if strings.HasPrefix(v, "#") {
				return strings.ToLower(v)
			}
		}
	}
	return t.KindColor(kind)
}

// loadKindColors reads [colors] of config.toml. Entries that are not a palette name or a hex color,
// or not a known kind, are left out.
func loadKindColors(dataDir string) map[string]string {
	out := map[string]string{}
	known := map[string]bool{}
	for _, k := range colorKinds() {
		known[k.kind] = true
	}
	for kind, raw := range modules.Table(dataDir, "colors") {
		s, _ := raw.(string)
		if v, ok := validKindColor(s); ok && known[kind] {
			out[kind] = v
		}
	}
	return out
}

// saveKindColors writes [colors]. An empty map removes the table.
func saveKindColors(dataDir string, ov map[string]string) error {
	if dataDir == "" {
		return fmt.Errorf("no data directory, the color lasts for this session only")
	}
	t := map[string]any{}
	for k, v := range ov {
		t[k] = v
	}
	return modules.SetTable(dataDir, "colors", t)
}

// applyKindColors patches the kind styles of m.st with the resolved colors. With no color support the
// styles stay plain.
func (m *Model) applyKindColors() {
	if m.profile <= colorprofile.Ascii {
		return
	}
	kinds := map[string]lipgloss.Style{}
	for k, st := range m.st.Kind {
		kinds[k] = st
	}
	for _, ck := range colorKinds() {
		if c := resolveKindColor(m.st.Look, m.kindColors, ck.kind); c != "" {
			kinds[ck.kind] = theme.ForegroundStyle(c)
		}
	}
	m.st.Kind = kinds
}

// setKindColor stores (or, with "", removes) the override of a kind and redraws.
func (m *Model) setKindColor(kind, v string) error {
	next := map[string]string{}
	for k, c := range m.kindColors {
		next[k] = c
	}
	if v == "" {
		delete(next, kind)
	} else {
		next[kind] = v
	}
	if err := saveKindColors(m.env.DataDir, next); err != nil {
		return err
	}
	m.kindColors = next
	m.setTheme(m.st.Look)
	return nil
}

// headingKind is the kind of the record page on screen, "" when the detail pane is not a record page.
func (m *Model) headingKind() string {
	switch m.view {
	case viewOrgs:
		return "organization"
	case viewDeals:
		return "deal"
	case viewTasks:
		return "task"
	case viewTags:
		return "tag"
	case viewSettings, viewMerge, viewArchived, viewSync, viewBirthdays:
		return ""
	}
	return "person"
}

// headingStyle styles the title of a record page: the kind color when it has one, else the accent.
func (m *Model) headingStyle() lipgloss.Style {
	if k := m.headingKind(); k != "" {
		if _, ok := m.st.Kind[k]; ok {
			return m.st.KindStyle(k).Bold(true)
		}
	}
	return m.st.Name
}

