// Package theme holds every color the terminal UI uses. A Theme names the color roles, the
// built-in themes are defined here, and custom themes are TOML files in ~/.crm/themes. This is
// the only package allowed to contain color literals (a test enforces it).
package theme

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Role names, as written in a theme file under [colors].
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
	RoleSelFg  = "sel_fg" // text of the selected row (optional)
	RoleSelBg  = "sel_bg" // background of the selected row (optional)
)

// Roles lists every role in documentation order. Optional roles are marked in Optional.
var Roles = []string{RoleFg, RoleDim, RoleAccent, RoleDanger, RoleGood, RoleWarn, RoleMark, RoleTabFg, RoleTabBg, RoleSelFg, RoleSelBg}

// Optional roles may be empty. Without sel_fg and sel_bg the selected row is drawn in reverse video.
var Optional = map[string]bool{RoleSelFg: true, RoleSelBg: true}

// Theme is a named set of colors. Each color is "#rrggbb", "#rgb" or an ANSI number 0-255.
type Theme struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Builtin     bool   `json:"builtin"`
	Path        string `json:"path,omitempty"` // file of a custom theme
	Base        string `json:"base,omitempty"` // built-in the custom theme starts from

	Fg, Dim, Accent, Danger, Good, Warn, Mark string
	TabFg, TabBg, SelFg, SelBg                string
}

// slot returns a pointer to the field of a role.
func (t *Theme) slot(role string) *string {
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
	}
	return nil
}

// Color returns the color of a role ("" when unset).
func (t Theme) Color(role string) string {
	if p := t.slot(role); p != nil {
		return *p
	}
	return ""
}

// Colors returns the roles that are set, in documentation order.
func (t Theme) Colors() []RoleColor {
	var out []RoleColor
	for _, r := range Roles {
		out = append(out, RoleColor{Role: r, Color: t.Color(r)})
	}
	return out
}

// RoleColor is one role and its color.
type RoleColor struct {
	Role  string `json:"role"`
	Color string `json:"color"`
}

var (
	hexColor  = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	nameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
)

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

// ValidName reports whether s is usable as a theme name (lower case letters, digits, - and _).
func ValidName(s string) error {
	if !nameRegex.MatchString(s) {
		return fmt.Errorf("theme name %q is invalid: use 1-40 lower case letters, digits, '-' or '_', starting with a letter or digit", s)
	}
	return nil
}

// Validate checks every role. It collects all problems so the user can fix them in one pass.
func (t Theme) Validate() error {
	var problems []string
	for _, r := range Roles {
		v := t.Color(r)
		if v == "" {
			if !Optional[r] {
				problems = append(problems, fmt.Sprintf("colors.%s is missing", r))
			}
			continue
		}
		if err := ValidColor(v); err != nil {
			problems = append(problems, fmt.Sprintf("colors.%s: %v", r, err))
		}
	}
	if (t.SelFg == "") != (t.SelBg == "") {
		problems = append(problems, "colors.sel_fg and colors.sel_bg must be set together (or both left out for reverse video)")
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// mk builds a built-in theme from the colors in role order.
func mk(name, desc string, c [9]string, sel [2]string) Theme {
	return Theme{Name: name, Description: desc, Builtin: true,
		Fg: c[0], Dim: c[1], Accent: c[2], Danger: c[3], Good: c[4], Warn: c[5], Mark: c[6], TabFg: c[7], TabBg: c[8],
		SelFg: sel[0], SelBg: sel[1]}
}

// builtins are listed in menu order. "auto" is first: it picks dark or light from the terminal.
var builtins = []Theme{
	mk("dark", "default colors for dark terminals",
		[9]string{"#E6E6E6", "#888888", "#9D9BFF", "#FF7B72", "#7EE787", "#E3B341", "#D2A8FF", "#FFFFFF", "#5A56E0"}, [2]string{}),
	mk("light", "default colors for light terminals",
		[9]string{"#1F2328", "#777777", "#5A56E0", "#C0392B", "#1E7E34", "#9A6700", "#8250DF", "#FFFFFF", "#5A56E0"}, [2]string{}),
	mk("high-contrast", "maximum contrast, bold selection, for low vision or bright rooms",
		[9]string{"#FFFFFF", "#C0C0C0", "#00FFFF", "#FF5555", "#55FF55", "#FFFF00", "#FF79FF", "#000000", "#FFFF00"}, [2]string{"#000000", "#FFFFFF"}),
	mk("catppuccin", "Catppuccin Mocha",
		[9]string{"#CDD6F4", "#7F849C", "#CBA6F7", "#F38BA8", "#A6E3A1", "#F9E2AF", "#F5C2E7", "#1E1E2E", "#CBA6F7"}, [2]string{"#CDD6F4", "#45475A"}),
	mk("nord", "Nord",
		[9]string{"#D8DEE9", "#7B88A1", "#88C0D0", "#BF616A", "#A3BE8C", "#EBCB8B", "#B48EAD", "#2E3440", "#88C0D0"}, [2]string{"#ECEFF4", "#434C5E"}),
	mk("gruvbox", "Gruvbox dark",
		[9]string{"#EBDBB2", "#928374", "#83A598", "#FB4934", "#B8BB26", "#FABD2F", "#D3869B", "#282828", "#FE8019"}, [2]string{"#EBDBB2", "#504945"}),
	mk("solarized", "Solarized dark",
		[9]string{"#93A1A1", "#657B83", "#268BD2", "#DC322F", "#859900", "#B58900", "#D33682", "#FDF6E3", "#268BD2"}, [2]string{"#EEE8D5", "#073642"}),
}

// AutoName is the default theme: dark or light, chosen from the terminal background.
const AutoName = "auto"

// Builtins returns the built-in themes, "auto" first. The auto entry is the dark theme here
// and is resolved to dark or light by Resolve.
func Builtins() []Theme {
	out := []Theme{{Name: AutoName, Description: "dark or light, chosen from the terminal background (default)", Builtin: true}}
	return append(out, builtins...)
}

// Builtin finds a built-in by name. "auto" returns the dark theme when darkBackground is true.
func Builtin(name string, darkBackground bool) (Theme, bool) {
	if name == AutoName {
		pick := "light"
		if darkBackground {
			pick = "dark"
		}
		t, _ := Builtin(pick, darkBackground)
		t.Name, t.Description = AutoName, "dark or light, chosen from the terminal background (default)"
		return t, true
	}
	for _, t := range builtins {
		if t.Name == name {
			return t, true
		}
	}
	return Theme{}, false
}

// BuiltinNames lists the built-in names in menu order.
func BuiltinNames() []string {
	names := []string{AutoName}
	for _, t := range builtins {
		names = append(names, t.Name)
	}
	return names
}

// suggest returns the closest known name, or "".
func suggest(name string, known []string) string {
	best, bestD := "", 3
	for _, k := range known {
		if d := editDistance(name, k); d < bestD {
			best, bestD = k, d
		}
		if strings.HasPrefix(k, name) && len(name) >= 2 {
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

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
