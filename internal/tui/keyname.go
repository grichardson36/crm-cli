package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// keyName is the one place that turns a Bubble Tea v2 key press into the key vocabulary of the
// action registry, the key config file and internal/keys. That vocabulary is the one Bubble Tea v1
// reported, so no binding changed when the framework moved to v2:
//
//   - a space is " " (v2 says "space"), a space with a modifier is "alt+space"
//   - modifiers come in the order alt, ctrl, shift ("alt+ctrl+up"), v2 says ctrl, alt, shift
//   - ctrl+shift+letter is "ctrl+<letter>" and alt+shift+letter is "alt+<LETTER>", as before
//   - text keeps its case: "A", "?", and typed text with ctrl or alt held is never taken as text
//
// Everything else (enter, esc, tab, shift+tab, up, pgup, pgdown, home, end, f5, ctrl+shift+up) is
// spelled the same in both versions.
func keyName(msg tea.KeyPressMsg) string {
	k := tea.Key(msg)
	held := k.Mod & (tea.ModCtrl | tea.ModAlt | tea.ModMeta | tea.ModHyper | tea.ModSuper)
	if held == 0 && k.Text != "" && k.Text != " " {
		return k.Text
	}
	// The key without modifiers, spelled by v2 ("up", "enter", "a", "space").
	name := tea.Key{Code: k.Code, BaseCode: k.BaseCode, Text: k.Text}.Keystroke()
	if k.Code == tea.KeyExtended {
		name = k.Text
	}
	if k.Mod == 0 && name == "space" {
		return " "
	}
	shift := k.Mod.Contains(tea.ModShift)
	alt := k.Mod.Contains(tea.ModAlt)
	ctrl := k.Mod.Contains(tea.ModCtrl)
	if r, n := utf8.DecodeRuneInString(name); n == len(name) && unicode.IsLetter(r) && shift {
		if !ctrl {
			name = strings.ToUpper(name)
		}
		shift = false
	}
	var b strings.Builder
	if alt {
		b.WriteString("alt+")
	}
	if ctrl {
		b.WriteString("ctrl+")
	}
	if shift {
		b.WriteString("shift+")
	}
	for _, mod := range []struct {
		m tea.KeyMod
		s string
	}{{tea.ModMeta, "meta+"}, {tea.ModHyper, "hyper+"}, {tea.ModSuper, "super+"}} {
		if k.Mod.Contains(mod.m) {
			b.WriteString(mod.s)
		}
	}
	b.WriteString(name)
	return b.String()
}
