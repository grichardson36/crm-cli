package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// The choice editor of a field backed by a user-managed vocabulary (model.FieldSpec.Vocabulary),
// with pick-or-create (patterns/pick-or-create, patterns/deal-roles). The row shows an input and the
// matching options. Typing filters the list, up and down pick, tab completes the highlighted name,
// enter saves it. Text that is not on the list adds a last row, "Create role X", and enter on it adds
// the value to the list and sets it as one undo step. With the input empty the last row clears a value.
//
// The check is internal/validate (validate.Field with Options.Create), the same one the CLI and MCP
// use. They refuse an unknown value unless told to create it, the editor is the place that offers.

// vocabItem is one row of the option list.
type vocabItem struct {
	value, label string
	create       bool // the "Create role X" row
	clear        bool // the "(no role)" row
}

// vocabEdit is the state of the editor: the rows that match the input and the highlighted one.
type vocabEdit struct {
	items []vocabItem
	sel   int
}

// vocabNoun is the word for one value of the field's vocabulary ("role").
func vocabNoun(spec model.FieldSpec) string {
	if vs, ok := model.VocabularyNamed(spec.Vocabulary); ok {
		return vs.Noun
	}
	return "value"
}

// vocabItemsFor lists the options that match typed: those starting with it first, then those holding
// it, an exact match before both. A text that matches no option exactly adds the create row.
func vocabItemsFor(spec model.FieldSpec, typed, current string) []vocabItem {
	typed = strings.TrimSpace(typed)
	low := strings.ToLower(typed)
	var exact, prefix, inner []vocabItem
	for _, c := range spec.Choices {
		it := vocabItem{value: c.Value, label: c.Label}
		l := strings.ToLower(c.Value)
		switch {
		case typed == "":
			prefix = append(prefix, it)
		case l == low:
			exact = append(exact, it)
		case strings.HasPrefix(l, low):
			prefix = append(prefix, it)
		case strings.Contains(l, low):
			inner = append(inner, it)
		}
	}
	items := append(append(exact, prefix...), inner...)
	switch {
	case typed == "" && current != "":
		items = append(items, vocabItem{label: "(no " + vocabNoun(spec) + ")", clear: true})
	case typed != "" && len(exact) == 0:
		name := model.CleanVocabName(typed)
		items = append(items, vocabItem{value: name, label: "Create " + vocabNoun(spec) + " " + name, create: true})
	}
	return items
}

// startVocabEdit opens the editor on the row under the cursor.
func (m *Model) startVocabEdit(t *typedEdit, spec model.FieldSpec, value string) tea.Cmd {
	t.vocab = &vocabEdit{items: vocabItemsFor(spec, "", value)}
	for i, it := range t.vocab.items {
		if strings.EqualFold(it.value, value) && value != "" {
			t.vocab.sel = i
		}
	}
	m.rowInput.SetValue("")
	m.rowInput.Width = max(5, m.detail.Width-labelWidth-1)
	m.syncDetail()
	return m.rowInput.Focus()
}

// handleVocabKey is a key in the vocabulary editor.
func (m *Model) handleVocabKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := m.edit.typed
	v := t.vocab
	switch msg.String() {
	case "esc":
		m.cancelEdit()
		return m, nil
	case "enter":
		m.commitTyped()
		return m, nil
	case "up", "ctrl+p", "shift+tab":
		if n := len(v.items); n > 0 {
			v.sel = (v.sel + n - 1) % n
		}
	case "down", "ctrl+n":
		if n := len(v.items); n > 0 {
			v.sel = (v.sel + 1) % n
		}
	case "tab":
		if v.sel < len(v.items) {
			if it := v.items[v.sel]; !it.create && !it.clear {
				m.rowInput.SetValue(it.value)
				m.rowInput.CursorEnd()
				v.items = vocabItemsFor(t.spec, it.value, m.edit.oldValue)
				v.sel = 0
			}
		}
	default:
		before := m.rowInput.Value()
		var cmd tea.Cmd
		m.rowInput, cmd = m.rowInput.Update(msg)
		if m.rowInput.Value() != before {
			v.items = vocabItemsFor(t.spec, m.rowInput.Value(), m.edit.oldValue)
			v.sel = 0
		}
		m.setStatus("", false)
		m.syncDetail()
		return m, cmd
	}
	m.setStatus("", false)
	m.syncDetail()
	return m, nil
}

// vocabPick is the highlighted row as the text to check, and whether it is the create row.
func (t *typedEdit) vocabPick() (raw string, create bool, ok bool) {
	v := t.vocab
	if v == nil || v.sel >= len(v.items) {
		return "", false, false
	}
	it := v.items[v.sel]
	return it.value, it.create, true
}

// vocabEditLine is the row while the editor is open: the input, then the matching rows.
func (m *Model) vocabEditLine(label string, w int) string {
	t := m.edit.typed
	v := t.vocab
	var parts []string
	for i, it := range v.items {
		text := it.label
		switch {
		case i == v.sel:
			parts = append(parts, m.st.Sel.Render(" "+text+" "))
		case it.create:
			parts = append(parts, m.st.Name.Render(text))
		default:
			parts = append(parts, m.st.Dim.Render(text))
		}
	}
	line := m.st.Dim.Render(label) + strings.TrimRight(m.rowInput.View(), " ")
	if len(parts) == 0 {
		line += m.st.Dim.Render("  type a " + vocabNoun(t.spec) + " name")
	} else {
		line += "  " + strings.Join(parts, " ")
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(line)
}

// createVocabValue adds a value to a vocabulary and records the add, so it undoes with the change
// that used it (the caller groups them, beginGroup and endGroup).
func (m *Model) createVocabValue(kind, name string) error {
	e, err := m.vr.Add(m.ctx, kind, name)
	if err != nil {
		return err
	}
	entry := *e
	noun := "value"
	if vs, ok := model.VocabularyNamed(kind); ok {
		noun = vs.Noun
	}
	m.record("create "+noun+" "+entry.Name,
		func() error { return m.vr.Discard(m.ctx, entry) },
		func() error { return m.vr.Restore(m.ctx, entry) })
	return nil
}

// roleSuggestions are the roles the link prompt completes: the deal roles for a deal link, else the
// roles already in use.
func (m *Model) roleSuggestions(relation string, limit int) ([]string, error) {
	if relation == model.DealPersonKind {
		names, err := m.vr.Names(m.ctx, model.DealRoleKind)
		if err == nil && len(names) > limit {
			names = names[:limit]
		}
		return names, err
	}
	return m.links.Roles(m.ctx, relation, limit)
}

// roleHint starts the status line that lists the suggestions.
func roleHint(rel model.Relation) string {
	if rel.Name == model.DealPersonKind {
		return "deal roles: "
	}
	return "roles in use: "
}
