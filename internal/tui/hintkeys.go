package tui

import "strings"

// The key text of the key hints (patterns/key-hints-in-one-place, CRM-025). hintwords.go holds the
// WORD of a hint, this file holds the KEY. Every place that shows a key to a person (the footer, the
// modes, popups, the keys page, the palette key column, the Settings Keys pane, messages) calls
// showKey, so a key reads the same on every screen. Key MATCHING stays on the plain names ("up",
// "shift+tab", " "), the table only changes what is drawn.
//
// The canonical text is what the main footer showed before this table: the arrows are glyphs, every
// other key keeps its name. A modifier prefix keeps its name and the key after it follows the table
// ("shift+up" reads "shift+↑"). A key that is not in the table reads as typed, so a key the owner
// rebinds to "ctrl+g" or "x" displays as that.
const (
	// Key groups hints use. The value is a key name, showKey turns it into the text.
	hkUp            = "up"
	hkDown          = "down"
	hkLeft          = "left"
	hkRight         = "right"
	hkUpDown        = "up/down"
	hkLeftRight     = "left/right"
	hkShiftUpDown   = "shift+up/down"
	hkHomeEnd       = "home/end"
	hkShiftUp       = "shift+up"
	hkShiftDown     = "shift+down"
	hkCtrlUp        = "ctrl+up"
	hkCtrlDown      = "ctrl+down"
	hkCtrlShiftUp   = "ctrl+shift+up"
	hkCtrlShiftDown = "ctrl+shift+down"
)

// keyTexts maps a key name or key group to its display text.
var keyTexts = map[string]string{
	"up":         "↑",
	"down":       "↓",
	"left":       "←",
	"right":      "→",
	"up/down":    "↑/↓",
	"left/right": "←/→",
	// Chrome keys keep their names.
	"enter":     "enter",
	"esc":       "esc",
	"tab":       "tab",
	"shift+tab": "shift+tab",
	" ":         "space",
	"space":     "space",
	"home":      "home",
	"end":       "end",
	"home/end":  "home/end",
	"pgup":      "pgup",
	"pgdown":    "pgdown",
}

// showKey is the one function that turns a key name (or a group such as "up/down") into the text a
// person reads. Modifiers stay words and the key after the last "+" follows keyTexts.
func showKey(name string) string {
	if t, ok := keyTexts[name]; ok {
		return t
	}
	if i := strings.LastIndex(name, "+"); i > 0 && i < len(name)-1 {
		if t, ok := keyTexts[name[i+1:]]; ok {
			return name[:i+1] + t
		}
	}
	return name
}
