package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Key hints live in one place (patterns/key-hints-in-one-place). Every screen and mode shows the
// keys that work in it on the last line of the screen, the footer, in one format and one order:
// verbs first, then navigation, then what is specific to the screen. Nothing else on a screen
// restates a key (rows, descriptions, titles), except the dim syntax hint after the cursor of an
// input (patterns/show-syntax-inline-as-you-type).
//
// The main screen builds its footer from the action registry (footerSegs in view.go). The modes that
// take over the screen (editing a field, a prompt, a question, a picker, the bar, the names panel,
// the view editor, a text page, the help page, the compare screen) build theirs here from the tables
// below. A hint names either a registry action, so the key shown is the key bound today, or a fixed
// key that no action owns (enter, esc, the arrows, y and n).

// hintItem is one hint: a key and what it does. When action is set the key comes from the registry.
type hintItem struct {
	action, key, label string
}

// fk is a fixed key hint.
func fk(key, label string) hintItem { return hintItem{key: key, label: label} }

// ak is a hint for a registry action: the key shown is the first key bound to the action.
func ak(action, label string) hintItem { return hintItem{action: action, label: label} }

// Hint classes give the footer its order: verbs, navigation, screen specifics, then the keys that
// control the view and the app.
const (
	hintVerb = iota
	hintNav
	hintSpecific
	hintChrome
)

// hintClassOf is the class of an action in the main footer, from the same table as the grouped key list.
func hintClassOf(a *action) int {
	switch keyLayerOf(a) {
	case keyLayerVerbs:
		return hintVerb
	case keyLayerNav:
		return hintNav
	case keyLayerView:
		return hintChrome
	}
	return hintSpecific
}

// keyText is the text of a hint's key.
func (m *Model) keyText(h hintItem) string {
	if h.action == "" {
		return h.key
	}
	if a := m.actionByID(h.action); a != nil && len(a.keys) > 0 {
		return joinKeys(a.keys[:1])
	}
	return ""
}

// renderHintItems draws hints in the footer format: the key in the name style, the label dim, two
// spaces between hints. A hint whose registry action has no key is left out. The line is cut to the width.
func (m *Model) renderHintItems(items []hintItem) string {
	var parts []string
	used := 0
	for _, h := range items {
		k := m.keyText(h)
		if k == "" {
			continue
		}
		w := ansi.StringWidth(k) + 1 + ansi.StringWidth(h.label)
		if used+w > m.width {
			break
		}
		parts = append(parts, m.st.Name.Render(k)+" "+m.st.Dim.Render(h.label))
		used += w + 2
	}
	return strings.Join(parts, "  ")
}

// editHints are the hints while a field of a record page is edited in place.
func (m *Model) editHints() []hintItem {
	items := []hintItem{fk("enter", "save"), fk("esc", "cancel")}
	e := m.edit
	switch {
	case e != nil && e.typed != nil && e.typed.vocab != nil:
		items = append(items, fk("up/down", "pick"), ak("prompt-tab", "complete"))
	case e != nil && e.typed != nil && e.typed.spec.Type == model.FieldChoice:
		items = append(items, fk("up/down", "pick"), fk("tab", "save"))
	case e != nil && e.typed != nil && e.typed.spec.Type == model.FieldLongText:
		items = append(items, fk("alt+enter", "new line"), fk("tab", "save"))
	case e != nil && e.typed != nil:
		items = append(items, fk("tab", "save"))
	case e != nil && e.ta != nil:
		items = []hintItem{fk("enter", "pick"), fk("esc", "cancel"), fk("up/down", "move"), ak("prompt-tab", "complete")}
	}
	if e != nil && e.kind == rkHandle {
		items = append(items, fk("tab", "change kind"))
	}
	return items
}

// promptHints are the hints while the footer input is open.
func (m *Model) promptHints() []hintItem {
	if s := m.pipeRefresh(); s != nil && s.ac.Open() {
		return []hintItem{fk("enter", "confirm"), fk("esc", "close list"), fk("up/down", "choose"), ak("prompt-tab", "accept")}
	}
	if m.promptKind == "note" || m.promptKind == "note-object" || m.promptKind == "note-free" {
		return []hintItem{fk("enter", "save"), fk("esc", "cancel"), ak("prompt-tab", "type")}
	}
	return []hintItem{fk("enter", "confirm"), fk("esc", "cancel")}
}

// overlayHints are the hints of a picker, the palette or the command bar.
func (m *Model) overlayHints() []hintItem {
	o := m.ov
	switch {
	case o == nil:
		return nil
	case o.cmd != nil && o.cmd.confirming:
		return []hintItem{fk("enter", "apply"), fk("esc", "back")}
	case o.cmd != nil:
		return []hintItem{fk("enter", "run"), fk("esc", "close"), ak("prompt-tab", "complete")}
	case o.hints != nil:
		return o.hints
	case o.palette:
		return []hintItem{fk("enter", "run"), fk("esc", "close"), fk("up/down", "choose"), fk("tab", "verbs"),
			fk(">", "actions"), fk("@", "people"), fk("#", "tags"), fk("=", "settings"), fk("?", "help")}
	}
	return []hintItem{fk("enter", "pick"), fk("esc", "close"), fk("up/down", "choose")}
}

// namesHints are the hints of the panel that settles the names of a line.
func (m *Model) namesHints() []hintItem {
	return []hintItem{fk("enter", "save"), fk("esc", "back"), fk("up/down", "move"), fk("1-9", "pick a person"),
		fk("c", "create a contact"), fk("x", "keep as text"), fk("A", "same answer for the name")}
}

// sveHints are the hints of the saved view editor.
func (m *Model) sveHints() []hintItem {
	e := m.sve
	if e != nil && e.ed != nil {
		switch e.ed.mode {
		case "text":
			return []hintItem{fk("enter", "save the cell"), fk("esc", "cancel")}
		case "choice":
			return []hintItem{fk("enter", "save"), fk("esc", "cancel"), fk("up/down", "pick")}
		case "multi":
			return []hintItem{fk("enter", "save"), fk("esc", "cancel"), fk("up/down", "move"), fk("space", "add or remove")}
		}
		return []hintItem{fk("enter", "pick"), fk("esc", "save"), fk("up/down", "move"), fk("tab", "complete")}
	}
	return []hintItem{fk("enter", "edit"), fk("a", "add condition"), fk("d", "remove"), fk("ctrl+s", "save"), fk("esc", "close"),
		fk("up/down", "row"), fk("left/right", "cell")}
}

// textHints are the hints of a text page (a help topic, doctor, where).
func (m *Model) textHints() []hintItem {
	return []hintItem{fk("esc", "back"), fk("up/down", "scroll"), fk("home/end", "top or bottom"), ak("palette", "every action")}
}

// helpHints are the hints of the keys page.
func (m *Model) helpHints() []hintItem {
	return []hintItem{fk("any key", "close"), fk("up/down", "scroll"), ak("palette", "every action")}
}

// modalHintsLine is the footer line of a mode that takes over the screen, "" for the main screen.
func (m *Model) modalHintsLine() (string, bool) {
	switch m.mode {
	case modeEdit:
		return m.renderHintItems(m.editHints()), true
	case modeConfirm:
		return m.renderHintItems([]hintItem{fk("y", "yes"), fk("n", "no"), fk("esc", "cancel")}), true
	case modeCapture:
		return m.renderHintItems([]hintItem{fk("any key", "bind it"), fk("esc", "cancel")}), true
	case modePrompt:
		return m.renderHintItems(m.promptHints()), true
	}
	return "", false
}
