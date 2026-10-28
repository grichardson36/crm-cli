package tui

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// runID runs a registry action by id the way the palette does, for actions that have no key. An
// action that does not apply in the current context does nothing, as in the palette ("not here").
func runID(m *Model, id string) tea.Cmd {
	a := m.actionByID(id)
	if a == nil || !a.enabled(m) || (a.keyIf != nil && !a.keyIf(m)) {
		return nil
	}
	return m.runAction(a)
}

// runIDCmd is runID plus the commands it returned, the way pressRun does for keys.
func runIDCmd(m *Model, id string) {
	runCmds(m, runID(m, id))
}

// typeLinked types a line into the add prompt that is open and then accepts the highlighted person in
// its people segment with tab. It is the way the old grayscale link offer was taken: the text first,
// then the link.
func typeLinked(m *Model, text string) {
	pl := m.pipeFor(m.promptKind)
	kind := "person"
	if m.addSugg != nil && m.addSugg.ref.Kind != "" {
		kind = m.addSugg.ref.Kind
	}
	idx := segFor(pl, kind)
	typeText(m, text)
	if more := idx - strings.Count(text, "|"); more > 0 {
		typeText(m, strings.Repeat("|", more))
	}
	press(m, "tab")
}

// submitLinked is typeLinked followed by enter.
func submitLinked(m *Model, line string) {
	typeLinked(m, line)
	press(m, "enter")
}

var namedKeys = map[string]rune{
	"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "backspace": tea.KeyBackspace,
	"delete": tea.KeyDelete, "insert": tea.KeyInsert, "up": tea.KeyUp, "down": tea.KeyDown,
	"left": tea.KeyLeft, "right": tea.KeyRight, "home": tea.KeyHome, "end": tea.KeyEnd,
	"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "space": tea.KeySpace,
	"f1": tea.KeyF1, "f2": tea.KeyF2, "f3": tea.KeyF3, "f4": tea.KeyF4, "f5": tea.KeyF5, "f6": tea.KeyF6,
}

// keyPress builds the Bubble Tea v2 key press for a key in the registry vocabulary ("ctrl+f",
// "shift+tab", "alt+enter", "pgdown", " ", "A"). It is the one place the tests turn a key name into
// a tea.KeyPressMsg. Any other text becomes one multi-character press, like a typed or pasted word.
func keyPress(name string) tea.KeyPressMsg {
	var mod tea.KeyMod
	rest := name
	for {
		switch {
		case strings.HasPrefix(rest, "alt+") && len(rest) > 4:
			mod |= tea.ModAlt
			rest = rest[4:]
			continue
		case strings.HasPrefix(rest, "ctrl+") && len(rest) > 5:
			mod |= tea.ModCtrl
			rest = rest[5:]
			continue
		case strings.HasPrefix(rest, "shift+") && len(rest) > 6:
			mod |= tea.ModShift
			rest = rest[6:]
			continue
		}
		break
	}
	if rest == " " {
		rest = "space"
	}
	if code, ok := namedKeys[rest]; ok {
		k := tea.Key{Code: code, Mod: mod}
		if code == tea.KeySpace && mod == 0 {
			k.Text = " "
		}
		return tea.KeyPressMsg(k)
	}
	if utf8.RuneCountInString(rest) == 1 {
		r, _ := utf8.DecodeRuneInString(rest)
		k := tea.Key{Code: r, Mod: mod}
		if mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			k.Text = rest
		}
		return tea.KeyPressMsg(k)
	}
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyExtended, Text: name})
}
