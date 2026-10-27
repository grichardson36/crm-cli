// Package keys reads and writes the key bindings a user changed. The TUI action registry in
// internal/tui/actions.go holds the defaults. A [keys] table in ~/.crm/config.toml maps an action
// id to one key or a list of keys and replaces the default keys of that action:
//
//	[keys]
//	undo = "ctrl+z"
//	favorite = ["f", "ctrl+g"]
//
// The rest of config.toml (such as the theme) is kept when the table is written.
package keys

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// ConfigFile is the per-device settings file in the data directory.
func ConfigFile(dataDir string) string { return filepath.Join(dataDir, "config.toml") }

// Overrides maps an action id to the keys that replace its defaults.
type Overrides map[string][]string

var named = map[string]string{
	"enter": "enter", "return": "enter", "esc": "esc", "escape": "esc", "tab": "tab",
	"up": "up", "down": "down", "left": "left", "right": "right", "home": "home", "end": "end",
	"pgup": "pgup", "pageup": "pgup", "pgdown": "pgdown", "pagedown": "pgdown",
	"backspace": "backspace", "delete": "delete", "del": "delete", "insert": "insert",
	"space": " ",
}

// Normalize checks a key name and returns the form Bubble Tea reports (tea.KeyMsg.String), such
// as "ctrl+k", "shift+tab", "enter" or "T". A space is " ". Single characters keep their case.
func Normalize(key string) (string, error) {
	orig := key
	key = strings.TrimSpace(key)
	if key == "" && orig != "" {
		return " ", nil // a lone space
	}
	if key == "" {
		return "", fmt.Errorf("a key cannot be empty")
	}
	if utf8.RuneCountInString(key) == 1 {
		return key, nil
	}
	parts := strings.Split(strings.ToLower(key), "+")
	// A trailing "+" is the plus key itself: "ctrl++" or "alt++".
	if strings.HasSuffix(key, "++") {
		parts = append(strings.Split(strings.ToLower(strings.TrimSuffix(key, "++")), "+"), "+")
	}
	base := parts[len(parts)-1]
	mods := parts[:len(parts)-1]
	var alt, ctrl, shift bool
	for _, m := range mods {
		switch m {
		case "alt", "meta":
			alt = true
		case "ctrl", "control":
			ctrl = true
		case "shift":
			shift = true
		default:
			return "", fmt.Errorf("%q: unknown modifier %q (use ctrl, alt or shift)", key, m)
		}
	}
	var name string
	switch {
	case base == "":
		return "", fmt.Errorf("%q: missing the key after the modifier", key)
	case named[base] != "":
		name = named[base]
	case len(base) >= 2 && base[0] == 'f' && isFunctionKey(base):
		name = base
	case utf8.RuneCountInString(base) == 1:
		name = base
	default:
		return "", fmt.Errorf("%q is not a key name (examples: a, T, ?, enter, esc, tab, up, pgdown, space, f5, ctrl+z, alt+x, shift+tab)", key)
	}
	if ctrl && (len(name) != 1 || name[0] < 'a' || name[0] > 'z') && !isArrowish(name) && !isCtrlPunct(name) {
		return "", fmt.Errorf("%q: ctrl works with a letter a to z or an arrow key", key)
	}
	if shift && name != "tab" && !isArrowish(name) && !isFunctionKey(name) {
		return "", fmt.Errorf("%q: shift works with tab, arrows and function keys; write a capital letter instead (T, not shift+t)", key)
	}
	if !alt && !ctrl && !shift {
		return name, nil
	}
	if name == " " {
		name = "space"
	}
	out := ""
	if alt {
		out += "alt+"
	}
	if ctrl {
		out += "ctrl+"
	}
	if shift {
		out += "shift+"
	}
	return out + name, nil
}

func isFunctionKey(s string) bool {
	if len(s) < 2 || s[0] != 'f' {
		return false
	}
	n := 0
	for _, c := range s[1:] {
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n >= 1 && n <= 20
}

// isCtrlPunct reports the punctuation keys that make a control byte: ctrl+[ is esc, ctrl+] ctrl+\ ctrl+^
// and ctrl+_ are the other control bytes. A terminal may never deliver them as typed (see tui.SwallowedChordWarning).
func isCtrlPunct(s string) bool {
	return len(s) == 1 && strings.ContainsRune("[]\\^_", rune(s[0]))
}

func isArrowish(s string) bool {
	switch s {
	case "up", "down", "left", "right", "home", "end", "pgup", "pgdown":
		return true
	}
	return false
}

// Display writes a normalized key for people: a space is "space".
func Display(key string) string {
	if key == " " {
		return "space"
	}
	return key
}

// Load reads the [keys] table. It never fails: a missing file is empty, and every problem (bad
// TOML, a value that is not a string or list, an invalid key name) is returned as a message and
// that entry is skipped.
func Load(dataDir string) (Overrides, []string) {
	out := Overrides{}
	data, err := os.ReadFile(ConfigFile(dataDir))
	if err != nil {
		return out, nil
	}
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return out, []string{fmt.Sprintf("%s is not valid TOML (%v): key bindings were not loaded", ConfigFile(dataDir), err)}
	}
	table, ok := raw["keys"]
	if !ok {
		return out, nil
	}
	m, ok := table.(map[string]any)
	if !ok {
		return out, []string{"[keys] in config.toml must be a table of action = key"}
	}
	var problems []string
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		var list []string
		switch v := m[id].(type) {
		case string:
			list = []string{v}
		case []any:
			badItem := false
			for _, e := range v {
				s, ok := e.(string)
				if !ok {
					problems = append(problems, fmt.Sprintf("keys.%s: every key must be a string", id))
					badItem = true
					break
				}
				list = append(list, s)
			}
			if badItem {
				continue
			}
		default:
			problems = append(problems, fmt.Sprintf("keys.%s: use a string such as \"ctrl+z\" or a list of strings", id))
			continue
		}
		if len(list) == 0 {
			if _, isList := m[id].([]any); isList {
				problems = append(problems, fmt.Sprintf("keys.%s: the list is empty (remove the line to use the default)", id))
			}
			continue
		}
		var norm []string
		bad := false
		for _, k := range list {
			n, err := Normalize(k)
			if err != nil {
				problems = append(problems, fmt.Sprintf("keys.%s: %v", id, err))
				bad = true
				break
			}
			norm = append(norm, n)
		}
		if !bad {
			out[id] = norm
		}
	}
	return out, problems
}

// Save writes the overrides as the [keys] table and keeps every other setting in the file. An
// empty set removes the table.
func Save(dataDir string, o Overrides) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dataDir, err)
	}
	path := ConfigFile(dataDir)
	raw := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(data), &raw); err != nil {
			return fmt.Errorf("%s is not valid TOML (%v): fix or remove it before changing keys", path, err)
		}
	}
	if len(o) == 0 {
		delete(raw, "keys")
	} else {
		table := map[string]any{}
		for id, list := range o {
			disp := make([]string, len(list))
			for i, k := range list {
				disp[i] = Display(k)
			}
			if len(disp) == 1 {
				table[id] = disp[0]
			} else {
				table[id] = disp
			}
		}
		raw["keys"] = table
	}
	var b strings.Builder
	b.WriteString("# crm settings for this device. Written by `crm theme set` and `crm keys set`.\n")
	if err := toml.NewEncoder(&b).Encode(raw); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
