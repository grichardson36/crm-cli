// Package theme holds every color the terminal UI uses. A Theme is one named entry with a dark and a
// light palette, and each palette sets every color role. The built-in themes are defined here, and
// custom themes are TOML files in ~/.crm/themes. This is the only package allowed to contain color
// literals (a test enforces it).
package theme

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Role names, as written in a theme file under [dark.colors] and [light.colors].
const (
	RoleFg     = "fg"     // headings and strong text
	RoleDim    = "dim"    // secondary text, hints, borders
	RoleAccent = "accent" // names, key hints, the focused divider
	RoleDanger = "danger" // errors, destructive prompts
	RoleGood   = "good"   // success messages
	RoleWarn   = "warn"   // cautions, the READ-ONLY badge
	RoleMark   = "mark"   // marked rows and the marked count
	RoleTabFg  = "tab_fg" // text of the active tab
	RoleTabBg  = "tab_bg" // background of the active tab
	RoleSelFg  = "sel_fg" // text of the selected row
	RoleSelBg  = "sel_bg" // background of the selected row

	// The color of each object kind. They tell people, organizations, deals, tasks, notes and tags
	// apart in the related segment of a pipe line.
	RoleKindPerson = "kind_person"
	RoleKindOrg    = "kind_org"
	RoleKindDeal   = "kind_deal"
	RoleKindTask   = "kind_task"
	RoleKindNote   = "kind_note"
	RoleKindTag    = "kind_tag"

	// RoleBg is the background of the whole screen, painted when the mode is light or dark.
	RoleBg = "bg"
)

// Roles lists every role in documentation order. Every palette sets every role.
var Roles = []string{RoleFg, RoleDim, RoleAccent, RoleDanger, RoleGood, RoleWarn, RoleMark, RoleTabFg, RoleTabBg, RoleSelFg, RoleSelBg,
	RoleKindPerson, RoleKindOrg, RoleKindDeal, RoleKindTask, RoleKindNote, RoleKindTag, RoleBg}

// RoleLabel is a role and the plain name shown for it.
type RoleLabel struct{ Role, Label string }

// EditableRoles are the roles the Theme pane edits, in its order, with the plain name it shows. The
// object kind colors are not here: Settings > Colors overrides them without touching the theme.
var EditableRoles = []RoleLabel{
	{RoleFg, "text"}, {RoleDim, "secondary text"}, {RoleAccent, "accent"}, {RoleDanger, "error"}, {RoleGood, "success"},
	{RoleWarn, "warning"}, {RoleMark, "marked"}, {RoleTabFg, "tab text"}, {RoleTabBg, "tab background"},
	{RoleSelFg, "selection text"}, {RoleSelBg, "selection background"}, {RoleBg, "background"},
}

// KindRoles maps an object kind (the model's kind names, and "tag") to its color role.
var KindRoles = map[string]string{
	"person": RoleKindPerson, "organization": RoleKindOrg, "deal": RoleKindDeal,
	"task": RoleKindTask, "interaction": RoleKindNote, "tag": RoleKindTag,
}

// Palette is one set of colors. Each color is "#rrggbb", "#rgb" or an ANSI number 0-255.
type Palette struct {
	Fg, Dim, Accent, Danger, Good, Warn, Mark string
	TabFg, TabBg, SelFg, SelBg                string

	KindPerson, KindOrg, KindDeal, KindTask, KindNote, KindTag string

	Bg string
}

// KindColor is the color of an object kind, "" for a kind without a role.
func (p Palette) KindColor(kind string) string {
	if role, ok := KindRoles[kind]; ok {
		return p.Color(role)
	}
	return ""
}

// Theme is one named entry with a dark and a light palette. The mode picks the palette.
type Theme struct {
	Name        string
	Description string
	Builtin     bool
	Path        string // file of a custom theme
	CopiedFrom  string // the theme a custom theme started from, what shift+r resets to
	Dark        Palette
	Light       Palette
}

// Look is the palette that is drawn: one theme resolved for dark or light.
type Look struct {
	Name    string // the theme
	Variant string // "dark" or "light"
	Palette
}

// Pick returns the palette of the variant.
func (t Theme) Pick(dark bool) Palette {
	if dark {
		return t.Dark
	}
	return t.Light
}

// Resolve returns the theme as drawn in dark or light.
func (t Theme) Resolve(dark bool) Look {
	v := "light"
	if dark {
		v = "dark"
	}
	return Look{Name: t.Name, Variant: v, Palette: t.Pick(dark)}
}

// Set replaces the palette of the variant.
func (t *Theme) Set(dark bool, p Palette) {
	if dark {
		t.Dark = p
	} else {
		t.Light = p
	}
}

// slot returns a pointer to the field of a role.
func (t *Palette) slot(role string) *string {
	switch role {
	case RoleFg:
		return &t.Fg
	case RoleDim:
		return &t.Dim
	case RoleAccent:
		return &t.Accent
	case RoleDanger:
		return &t.Danger
	case RoleGood:
		return &t.Good
	case RoleWarn:
		return &t.Warn
	case RoleMark:
		return &t.Mark
	case RoleTabFg:
		return &t.TabFg
	case RoleTabBg:
		return &t.TabBg
	case RoleSelFg:
		return &t.SelFg
	case RoleSelBg:
		return &t.SelBg
	case RoleKindPerson:
		return &t.KindPerson
	case RoleKindOrg:
		return &t.KindOrg
	case RoleKindDeal:
		return &t.KindDeal
	case RoleKindTask:
		return &t.KindTask
	case RoleKindNote:
		return &t.KindNote
	case RoleKindTag:
		return &t.KindTag
	case RoleBg:
		return &t.Bg
	}
	return nil
}

// Color returns the color of a role ("" when unset).
func (t Palette) Color(role string) string {
	if p := t.slot(role); p != nil {
		return *p
	}
	return ""
}

// SetColor sets the color of a role. It reports false for an unknown role.
func (t *Palette) SetColor(role, color string) bool {
	p := t.slot(role)
	if p == nil {
		return false
	}
	*p = color
	return true
}

// Colors returns every role with its color, in documentation order.
func (t Palette) Colors() []RoleColor {
	var out []RoleColor
	for _, r := range Roles {
		out = append(out, RoleColor{Role: r, Color: t.Color(r)})
	}
	return out
}

// Map returns the colors as role -> color.
func (t Palette) Map() map[string]string {
	out := map[string]string{}
	for _, r := range Roles {
		out[r] = t.Color(r)
	}
	return out
}

// RoleColor is one role and its color.
type RoleColor struct {
	Role  string `json:"role"`
	Color string `json:"color"`
}

var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// ValidColor reports whether s is a color the UI accepts, and says what is wrong when not.
func ValidColor(s string) error {
	if hexColor.MatchString(s) {
		return nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > 255 {
			return fmt.Errorf("ANSI color %d is out of range, use 0 to 255", n)
		}
		return nil
	}
	return fmt.Errorf("%q is not a color, use \"#rrggbb\", \"#rgb\" or an ANSI number 0-255", s)
}

// MaxName is the longest theme name.
const MaxName = 40

// ValidName reports whether s is usable as a theme name: 1 to 40 characters, no control characters.
func ValidName(s string) error {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > MaxName {
		return fmt.Errorf("theme name %q is invalid: use 1 to %d characters", s, MaxName)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return fmt.Errorf("theme name %q is invalid: no control characters", s)
		}
	}
	return nil
}

// Validate checks every role. It collects all problems so the user can fix them in one pass. The
// prefix names the table in messages, for example "dark.colors".
func (t Palette) Validate(prefix string) error {
	var problems []string
	for _, r := range Roles {
		v := t.Color(r)
		if v == "" {
			problems = append(problems, fmt.Sprintf("%s.%s is missing", prefix, r))
			continue
		}
		if err := ValidColor(v); err != nil {
			problems = append(problems, fmt.Sprintf("%s.%s: %v", prefix, r, err))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// Validate checks both palettes.
func (t Theme) Validate() error {
	var problems []string
	if err := t.Dark.Validate("dark.colors"); err != nil {
		problems = append(problems, err.Error())
	}
	if err := t.Light.Validate("light.colors"); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// pal builds a palette from the colors in role order.
// kinds are the colors of person, organization, deal, task, note and tag, in that order.
func pal(bg string, c [9]string, sel [2]string, kinds [6]string) Palette {
	return Palette{Bg: bg,
		Fg: c[0], Dim: c[1], Accent: c[2], Danger: c[3], Good: c[4], Warn: c[5], Mark: c[6], TabFg: c[7], TabBg: c[8],
		SelFg: sel[0], SelBg: sel[1],
		KindPerson: kinds[0], KindOrg: kinds[1], KindDeal: kinds[2], KindTask: kinds[3], KindNote: kinds[4], KindTag: kinds[5]}
}

// DefaultName is the theme used when none is chosen or the chosen one does not exist.
const DefaultName = "default"

// builtins are the built-in themes, each with both palettes.
var builtins = []Theme{
	{Name: "default", Builtin: true,
		Dark:  pal("#0D1117", [9]string{"#E6E6E6", "#888888", "#9D9BFF", "#FF7B72", "#7EE787", "#E3B341", "#D2A8FF", "#FFFFFF", "#5A56E0"}, [2]string{"#0D1117", "#E6E6E6"}, [6]string{"#79C0FF", "#FFA657", "#56D364", "#D2A8FF", "#F0C674", "#56D4DD"}),
		Light: pal("#FFFFFF", [9]string{"#1F2328", "#777777", "#5A56E0", "#C0392B", "#1E7E34", "#9A6700", "#8250DF", "#FFFFFF", "#5A56E0"}, [2]string{"#FFFFFF", "#1F2328"}, [6]string{"#0969DA", "#BC4C00", "#1A7F37", "#8250DF", "#9A6700", "#0A7E8C"})},
	{Name: "high-contrast", Builtin: true,
		Dark:  pal("#000000", [9]string{"#FFFFFF", "#C0C0C0", "#00FFFF", "#FF5555", "#55FF55", "#FFFF00", "#FF79FF", "#000000", "#FFFF00"}, [2]string{"#000000", "#FFFFFF"}, [6]string{"#55AAFF", "#FFA500", "#55FF55", "#FF79FF", "#FFFF55", "#00FFFF"}),
		Light: pal("#FFFFFF", [9]string{"#000000", "#404040", "#0000CC", "#B00000", "#006600", "#7A4A00", "#8000A0", "#FFFFFF", "#0000CC"}, [2]string{"#FFFFFF", "#000000"}, [6]string{"#0044CC", "#A04000", "#006600", "#8000A0", "#6A5A00", "#007070"})},
	{Name: "catppuccin", Builtin: true, Description: "Catppuccin Mocha and Latte",
		Dark:  pal("#1E1E2E", [9]string{"#CDD6F4", "#7F849C", "#CBA6F7", "#F38BA8", "#A6E3A1", "#F9E2AF", "#F5C2E7", "#1E1E2E", "#CBA6F7"}, [2]string{"#CDD6F4", "#45475A"}, [6]string{"#89B4FA", "#FAB387", "#A6E3A1", "#CBA6F7", "#F9E2AF", "#94E2D5"}),
		Light: pal("#EFF1F5", [9]string{"#4C4F69", "#6C6F85", "#8839EF", "#D20F39", "#3F9E2A", "#C27B19", "#CD67B2", "#EFF1F5", "#8839EF"}, [2]string{"#4C4F69", "#CCD0DA"}, [6]string{"#1E66F5", "#EC5D0A", "#3F9E2A", "#8839EF", "#C27B19", "#179299"})},
	{Name: "gruvbox", Builtin: true, Description: "Gruvbox dark and light",
		Dark:  pal("#282828", [9]string{"#EBDBB2", "#928374", "#83A598", "#FB4934", "#B8BB26", "#FABD2F", "#D3869B", "#282828", "#FE8019"}, [2]string{"#EBDBB2", "#504945"}, [6]string{"#83A598", "#FE8019", "#B8BB26", "#D3869B", "#FABD2F", "#8EC07C"}),
		Light: pal("#FBF1C7", [9]string{"#3C3836", "#7C6F64", "#076678", "#9D0006", "#79740E", "#B57614", "#8F3F71", "#FBF1C7", "#AF3A03"}, [2]string{"#3C3836", "#D5C4A1"}, [6]string{"#076678", "#AF3A03", "#79740E", "#8F3F71", "#B57614", "#427B58"})},
	{Name: "nord", Builtin: true, Description: "Nord, and a light variant from Snow Storm",
		Dark:  pal("#2E3440", [9]string{"#D8DEE9", "#7B88A1", "#88C0D0", "#BF616A", "#A3BE8C", "#EBCB8B", "#B48EAD", "#2E3440", "#88C0D0"}, [2]string{"#ECEFF4", "#434C5E"}, [6]string{"#81A1C1", "#D08770", "#A3BE8C", "#B48EAD", "#EBCB8B", "#8FBCBB"}),
		Light: pal("#ECEFF4", [9]string{"#2E3440", "#616E88", "#5E81AC", "#BF616A", "#5A8445", "#9A7418", "#8A5F96", "#ECEFF4", "#5E81AC"}, [2]string{"#2E3440", "#D8DEE9"}, [6]string{"#5E81AC", "#B5603F", "#5A8445", "#8A5F96", "#9A7418", "#2F7F7A"})},
	{Name: "solarized", Builtin: true, Description: "Solarized dark and light",
		Dark:  pal("#002B36", [9]string{"#93A1A1", "#657B83", "#268BD2", "#DC322F", "#859900", "#B58900", "#D33682", "#FDF6E3", "#268BD2"}, [2]string{"#EEE8D5", "#073642"}, [6]string{"#268BD2", "#CB4B16", "#859900", "#8A8FD8", "#B58900", "#2AA198"}),
		Light: pal("#FDF6E3", [9]string{"#586E75", "#657B83", "#268BD2", "#DC322F", "#839700", "#B38700", "#D33682", "#FDF6E3", "#268BD2"}, [2]string{"#073642", "#EEE8D5"}, [6]string{"#268BD2", "#CB4B16", "#839700", "#6C71C4", "#B38700", "#299D94"})},
	{Name: "anthropocene", Builtin: true, Description: "warm paper-like colors with a terracotta accent",
		Dark:  pal("#141413", [9]string{"#FAF9F5", "#9C9A91", "#D97757", "#E5736F", "#8DAA6B", "#E2B259", "#6A9BCC", "#141413", "#D97757"}, [2]string{"#FAF9F5", "#2E2D2A"}, [6]string{"#6A9BCC", "#D4A27F", "#8DAA6B", "#A99BCF", "#E8E6DC", "#7FB5AB"}),
		Light: pal("#FAF9F5", [9]string{"#141413", "#6B695F", "#B4532F", "#B3392F", "#5A7240", "#8A5A00", "#3F73A6", "#FAF9F5", "#B4532F"}, [2]string{"#141413", "#E8E6DC"}, [6]string{"#3F73A6", "#9A6234", "#5A7240", "#6F5FA3", "#4A4944", "#2F7C72"})},
}

// Builtins returns the built-in themes sorted by name.
func Builtins() []Theme {
	out := append([]Theme{}, builtins...)
	SortThemes(out)
	return out
}

// SortThemes orders themes alphanumerically by name, ignoring case.
func SortThemes(ts []Theme) {
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := strings.ToLower(ts[i].Name), strings.ToLower(ts[j].Name)
		if a != b {
			return a < b
		}
		return ts[i].Name < ts[j].Name
	})
}

// Builtin finds a built-in by name, ignoring case.
func Builtin(name string) (Theme, bool) {
	for _, t := range builtins {
		if strings.EqualFold(t.Name, strings.TrimSpace(name)) {
			return t, true
		}
	}
	return Theme{}, false
}

// BuiltinNames lists the built-in names, sorted.
func BuiltinNames() []string {
	var names []string
	for _, t := range Builtins() {
		names = append(names, t.Name)
	}
	return names
}

// suggest returns the closest known name, or "".
func suggest(name string, known []string) string {
	name = strings.ToLower(name)
	best, bestD := "", 3
	for _, k := range known {
		lk := strings.ToLower(k)
		if d := editDistance(name, lk); d < bestD {
			best, bestD = k, d
		}
		if strings.HasPrefix(lk, name) && len(name) >= 2 {
			return k
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
