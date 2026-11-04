package tui

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Key hints live in one place (patterns/key-hints-in-one-place). Every screen and mode shows the
// keys that work in it on the last rows of the screen, the footer, in one format and one order:
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
// spaces between hints. A hint whose registry action has no key is left out. Hints that do not fit
// the width continue on the next row, up to footerMaxRows rows (footer.go), and the rest are cut.
func (m *Model) renderHintItems(items []hintItem) string {
	var parts []string
	var widths []int
	for _, h := range items {
		k := m.keyText(h)
		if k == "" {
			continue
		}
		widths = append(widths, ansi.StringWidth(k)+1+ansi.StringWidth(h.label))
		parts = append(parts, m.st.Name.Render(k)+" "+m.st.Dim.Render(h.label))
	}
	places := placeHints(widths, 0, m.width, m.footerMax())
	return joinHintRows(parts[:len(places)], places, "")
}

// hintRowsOf is the number of footer rows the hints take on this screen width.
func (m *Model) hintRowsOf(items []hintItem) int { return hintRows(m.renderHintItems(items)) }

// editHints are the hints while a field of a record page is edited in place.
func (m *Model) editHints() []hintItem {
	items := []hintItem{fk("enter", hwSave), fk("esc", hwCancel)}
	e := m.edit
	switch {
	case e != nil && e.typed != nil && e.typed.vocab != nil:
		items = append(items, fk("up/down", hwPick), ak("prompt-tab", hwComplete))
	case e != nil && e.typed != nil && e.typed.spec.Type == model.FieldChoice:
		items = append(items, fk("up/down", hwPick), fk("tab", hwSave))
	case e != nil && e.typed != nil && e.typed.spec.Type == model.FieldLongText:
		items = append(items, fk("alt+enter", hwNewLine), fk("tab", hwSave))
	case e != nil && e.typed != nil:
		items = append(items, fk("tab", hwSave))
	case e != nil && e.ta != nil:
		items = []hintItem{fk("enter", hwPick), fk("esc", hwCancel), fk("up/down", hwMove), ak("prompt-tab", hwComplete)}
	}
	if e != nil && e.kind == rkHandle {
		items = append(items, fk("tab", hwChangeKind))
	}
	return items
}

// promptHints are the hints while the footer input is open.
func (m *Model) promptHints() []hintItem {
	if s := m.pipeRefresh(); s != nil && s.ac.Open() {
		return []hintItem{fk("enter", hwSave), fk("esc", hwClose), fk("up/down", hwChoose), ak("prompt-tab", hwAccept)}
	}
	if m.promptKind == "note" || m.promptKind == "note-object" || m.promptKind == "note-free" {
		return []hintItem{fk("enter", hwSave), fk("esc", hwCancel), ak("prompt-tab", hwType)}
	}
	return []hintItem{fk("enter", hwSave), fk("esc", hwCancel)}
}

// overlayHints are the hints of a picker, the palette or the command bar.
func (m *Model) overlayHints() []hintItem {
	o := m.ov
	switch {
	case o == nil:
		return nil
	case o.cmd != nil && o.cmd.confirming:
		return []hintItem{fk("enter", hwApply), fk("esc", hwBack)}
	case o.cmd != nil:
		return []hintItem{fk("enter", hwRun), fk("esc", hwCancel), ak("prompt-tab", hwComplete)}
	case o.hintsFor != nil:
		return o.hintsFor(m)
	case o.hints != nil:
		return o.hints
	}
	// The keys that work on the highlighted row, one word each.
	it, ok := o.highlighted()
	var h []hintItem
	esc := hwClose // a popup with nothing to choose is read only
	switch {
	case ok && it.selector:
		h = append(h, fk("left/right", hwChange))
		esc = hwCancel
	case ok && it.disabled == "" && !it.divider && o.palette:
		h = append(h, fk("enter", hwRun))
		esc = hwCancel
	case ok && it.disabled == "" && !it.divider:
		h = append(h, fk("enter", hwPick))
		esc = hwCancel
	}
	h = append(h, fk("esc", esc))
	if len(o.shown) > 1 {
		h = append(h, fk("up/down", hwSelect))
	}
	if o.palette {
		h = append(h, fk("tab", hwVerbs))
	}
	return h
}

// namesHints are the hints of the panel that settles the names of a line.
func (m *Model) namesHints() []hintItem {
	return []hintItem{fk("enter", hwSave), fk("esc", hwBack), fk("up/down", hwMove), fk("1-9", hwPickPerson),
		fk("c", hwCreate), fk("x", hwKeepText), fk("A", hwSameAnswer)}
}

// sveHints are the hints of the saved view editor.
func (m *Model) sveHints() []hintItem {
	e := m.sve
	if e != nil && e.ed != nil {
		switch e.ed.mode {
		case "text":
			return []hintItem{fk("enter", hwSave), fk("esc", hwCancel)}
		case "choice":
			return []hintItem{fk("enter", hwSave), fk("esc", hwCancel), fk("up/down", hwPick)}
		case "multi":
			return []hintItem{fk("enter", hwSave), fk("esc", hwCancel), fk("up/down", hwMove), fk("space", hwSelect)}
		}
		return []hintItem{fk("enter", hwPick), fk("esc", hwBack), fk("up/down", hwMove), fk("tab", hwComplete)}
	}
	return []hintItem{fk("enter", hwEdit), fk("a", hwAdd), fk("d", hwDelete), fk("ctrl+s", hwSave), fk("esc", hwCancel),
		fk("up/down", hwRow), fk("left/right", hwCell)}
}

// textHints are the hints of a text page (a help topic, doctor, where).
func (m *Model) textHints() []hintItem {
	return []hintItem{fk("esc", hwBack), fk("up/down", hwScroll), fk("home/end", hwTopBottom), ak("palette", hwEveryThing)}
}

// helpHints are the hints of the keys page.
func (m *Model) helpHints() []hintItem {
	return []hintItem{fk("any key", hwClose), fk("up/down", hwScroll), ak("palette", hwEveryThing)}
}

// modalHintsLine is the footer line of a mode that takes over the screen, "" for the main screen.
func (m *Model) modalHintsLine() (string, bool) {
	if p := m.paneSPFocused(); p != nil && m.mode == modeNormal {
		return m.renderHintItems(p.hints(m)), true
	}
	if le := m.paneListFocused(); le != nil && m.mode == modeNormal {
		return m.renderHintItems(le.hints(m)), true
	}
	switch m.mode {
	case modeEdit:
		return m.renderHintItems(m.editHints()), true
	case modeConfirm:
		return m.renderHintItems([]hintItem{fk("y", hwYes), fk("n", hwNo), fk("esc", hwCancel)}), true
	case modeCapture:
		return m.renderHintItems([]hintItem{fk("any key", hwBindIt), fk("esc", hwCancel)}), true
	case modePrompt:
		return m.renderHintItems(m.promptHints()), true
	}
	return "", false
}
