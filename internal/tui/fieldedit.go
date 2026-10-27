package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
	"github.com/mattn/go-runewidth"
)

// Typed field editors (owner decision 2026-10-28, patterns/field-types-drive-editors). A record page
// row that carries a model.FieldSpec (formRow.spec) is edited in place by its type:
//
//	text, number, date  one-line input, validated by the spec on save (a date takes "next friday")
//	longtext            the same input, markdown, alt+enter starts a new line
//	choice              the options sit in the row, up and down (or the first letter) pick
//	list                each entry is its own row, a adds one, enter edits it, d removes it
//	relation            a typeahead under the row (typeahead.go)
//
// enter or e on the row starts the edit, enter or tab saves it, esc cancels. A failed check keeps the
// editor open and says what was expected. Hints use the grayscale hint component (hint.go).

// typedEdit is the state of a typed edit in progress.
type typedEdit struct {
	spec   model.FieldSpec
	ref    repo.Ref
	choice int // index into spec.Choices
	// cell is set when the edit is for a cell of a linked row or a note (cells.go).
	cell *cellEdit
}

// parseDateField is the date parser handed to FieldSpec.Normalize: the same phrases as "by <date>".
func (m *Model) parseDateField(raw string) (string, bool) {
	res := shortsyntax.Parse("by "+raw, shortsyntax.Options{Now: m.now()})
	if res.Deadline != nil && res.Title == "" {
		return res.Deadline.String(), true
	}
	return "", false
}

// specRow builds a typed row for a field of a record.
func specRow(ref repo.Ref, spec model.FieldSpec, value string) formRow {
	s := spec
	return formRow{kind: rkTyped, field: spec.Key, label: spec.Label, value: value, spec: &s, ref: ref}
}

// typedDisplay is the text of a typed row: the label of a choice, the value otherwise.
func typedDisplay(r formRow) string {
	if r.spec == nil {
		return r.value
	}
	if r.spec.Type == model.FieldChoice {
		if r.value == "" {
			return ""
		}
		label := r.spec.ChoiceLabel(r.value)
		if label != r.value && r.spec.Clear != "" {
			return fmt.Sprintf("%s (%s)", label, r.value)
		}
		return label
	}
	if r.spec.Type == model.FieldNumber && r.value != "" {
		if f, err := strconv.ParseFloat(r.value, 64); err == nil {
			return money(&f)
		}
	}
	return r.value
}

// beginTyped starts the in-place editor of the typed row under the cursor.
func (m *Model) beginTyped() tea.Cmd {
	r, ok := m.rowFocused()
	if !ok || r.spec == nil {
		return nil
	}
	if m.readOnly {
		m.setStatus("read-only: editing is disabled", true)
		return nil
	}
	spec := *r.spec
	if spec.Type == model.FieldRelation {
		return m.startLink(r.ref, spec.Kind, false)
	}
	return m.startTypedEdit(spec, r.ref, r.value, nil)
}

// startTypedEdit opens the in-place editor of one value by its field type: a field of the record, or
// with ce set a cell of the row under the cursor.
func (m *Model) startTypedEdit(spec model.FieldSpec, ref repo.Ref, value string, ce *cellEdit) tea.Cmd {
	t := &typedEdit{spec: spec, ref: ref, cell: ce}
	m.edit = &editState{row: m.fcur, kind: m.form[m.fcur].kind, field: spec.Key, oldValue: value, typed: t}
	m.focus = focusDetail
	m.mode = modeEdit
	m.setStatus("", false)
	if spec.Type == model.FieldChoice {
		t.choice = 0
		for i, c := range spec.Choices {
			if c.Value == value {
				t.choice = i
			}
		}
		m.syncDetail()
		return nil
	}
	m.rowInput.SetValue(strings.ReplaceAll(value, "\n", "⏎"))
	m.rowInput.CursorEnd()
	m.rowInput.Width = max(5, m.detail.Width-labelWidth-1)
	m.syncDetail()
	return m.rowInput.Focus()
}

// handleTypedKey is a key in a typed edit.
func (m *Model) handleTypedKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := m.edit.typed
	key := msg.String()
	switch key {
	case "esc":
		m.cancelEdit()
		return m, nil
	case "enter", "tab":
		m.commitTyped()
		return m, nil
	}
	if t.spec.Type == model.FieldChoice {
		n := len(t.spec.Choices)
		switch key {
		case "up", "left", "shift+tab":
			t.choice = (t.choice + n - 1) % n
		case "down", "right":
			t.choice = (t.choice + 1) % n
		default:
			if len(msg.Runes) == 1 {
				m.jumpChoice(string(msg.Runes))
			}
		}
		m.setStatus("", false)
		m.syncDetail()
		return m, nil
	}
	if key == "alt+enter" && t.spec.Type == model.FieldLongText {
		m.rowInput.SetValue(m.rowInput.Value() + "⏎")
		m.rowInput.CursorEnd()
		m.syncDetail()
		return m, nil
	}
	var cmd tea.Cmd
	m.rowInput, cmd = m.rowInput.Update(msg)
	m.syncDetail()
	return m, cmd
}

// jumpChoice moves to the next option whose label or value starts with the typed letters.
func (m *Model) jumpChoice(s string) {
	t := m.edit.typed
	s = strings.ToLower(s)
	n := len(t.spec.Choices)
	for i := 1; i <= n; i++ {
		c := t.spec.Choices[(t.choice+i)%n]
		if strings.HasPrefix(strings.ToLower(c.Label), s) || strings.HasPrefix(strings.ToLower(c.Value), s) {
			t.choice = (t.choice + i) % n
			return
		}
	}
}

// commitTyped checks the value with the field type and saves it. A failed check keeps the editor.
func (m *Model) commitTyped() {
	e := m.edit
	t := e.typed
	var raw string
	if t.spec.Type == model.FieldChoice {
		raw = t.spec.Choices[t.choice].Value
	} else {
		raw = strings.ReplaceAll(m.rowInput.Value(), "⏎", "\n")
	}
	v, err := t.spec.Normalize(raw, m.parseDateField)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if t.spec.Type == model.FieldChoice && t.spec.Clear != "" && v == t.spec.Clear {
		v = ""
	}
	if v == strings.TrimSpace(e.oldValue) {
		m.cancelEdit()
		return
	}
	m.beginGroup()
	if t.cell != nil {
		err = m.saveCell(t.cell, t.ref, v)
	} else {
		err = m.saveTyped(t.ref, t.spec, v)
	}
	m.endGroup("")
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.finishEdit()
	m.setStatus("saved "+t.spec.Label+" (ctrl+z undoes)", false)
}

// saveTyped writes one typed field of a record through its recorded hook, so it is one undo step.
func (m *Model) saveTyped(ref repo.Ref, spec model.FieldSpec, v string) error {
	opt := func() *string {
		if v == "" {
			return nil
		}
		return &v
	}
	switch ref.Kind {
	case "person":
		switch spec.Key {
		case "tier":
			n, _ := strconv.Atoi(v)
			_, err := m.hUpdatePerson(m.ctx, ref.ID, model.UpdatePersonInput{PriorityTier: &n})
			return err
		case "birthday":
			if birthdayShort.MatchString(v) {
				v = "--" + v
			}
			_, err := m.hUpdatePerson(m.ctx, ref.ID, model.UpdatePersonInput{Birthday: &v})
			return err
		}
	case "task":
		switch spec.Key {
		case "due":
			_, err := m.hUpdateTask(m.ctx, ref.ID, model.UpdateTaskInput{DueAt: &v})
			return err
		case "priority":
			_, err := m.hUpdateTask(m.ctx, ref.ID, model.UpdateTaskInput{Priority: &v})
			return err
		}
	case "organization":
		before, err := m.orr.FindByID(m.ctx, ref.ID)
		if err != nil {
			return err
		}
		after := *before
		switch spec.Key {
		case "name":
			after.Name = v
		case "domain":
			after.Domain = opt()
		case "website":
			after.Website = opt()
		case "industry":
			after.Industry = opt()
		case "size":
			after.Size = opt()
		case "location":
			after.City, after.Region, after.Country = model.SplitLocation(v)
		case "linkedin":
			after.LinkedinURL = opt()
		case "description":
			after.Description = opt()
		case "notes":
			after.Notes = opt()
		default:
			return fmt.Errorf("organization has no editable field %q: %w", spec.Key, model.ErrValidation)
		}
		return m.hOverwriteOrg(ref.ID, &after)
	case "deal":
		before, err := m.dlr.FindByID(m.ctx, ref.ID)
		if err != nil {
			return err
		}
		after := *before
		switch spec.Key {
		case "title":
			after.Title = v
		case "stage":
			after.Stage = v
		case "value":
			after.Value = nil
			if v != "" {
				f, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return fmt.Errorf("value %q is not a number: %w", v, model.ErrValidation)
				}
				after.Value = &f
			}
		case "notes":
			after.Notes = opt()
		default:
			return fmt.Errorf("deal has no editable field %q: %w", spec.Key, model.ErrValidation)
		}
		return m.hOverwriteDeal(ref.ID, &after)
	}
	return fmt.Errorf("%s has no editable field %q: %w", ref.Kind, spec.Key, model.ErrValidation)
}

// clearTyped is d on a typed row: it empties an optional field after a y/n.
func (m *Model) clearTyped(r formRow) {
	spec := *r.spec
	if spec.Required || (spec.Type == model.FieldChoice && spec.Clear == "") {
		m.setStatus(spec.Label+" cannot be cleared", true)
		return
	}
	if r.value == "" {
		m.setStatus("nothing to delete here", true)
		return
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("clear %s? y/n", spec.Label),
		yes: func() {
			m.beginGroup()
			err := m.saveTyped(r.ref, spec, "")
			m.endGroup("")
			if err != nil {
				m.setStatus(err.Error(), true)
				return
			}
			m.afterWrite()
			m.setStatus("cleared "+spec.Label+" (ctrl+z undoes)", false)
		},
	}
}

// typedEditLine is the row while a typed edit is open: the input with its hint, or the options.
func (m *Model) typedEditLine(label string, w int) string {
	t := m.edit.typed
	if t.spec.Type == model.FieldChoice {
		var parts []string
		for i, c := range t.spec.Choices {
			text := c.Label
			if c.Label != c.Value && t.spec.Clear != "" {
				text = fmt.Sprintf("%s (%s)", c.Label, c.Value)
			}
			if i == t.choice {
				parts = append(parts, m.st.Sel.Render(" "+text+" "))
			} else {
				parts = append(parts, m.st.Dim.Render(text))
			}
		}
		line := m.st.Dim.Render(label) + strings.Join(parts, " ")
		return lipgloss.NewStyle().MaxWidth(w).Render(line)
	}
	line := m.st.Dim.Render(label) + strings.TrimRight(m.rowInput.View(), " ")
	line += m.st.Dim.Render(m.fieldHint(t.spec, m.rowInput.Value(), w-labelWidth-runewidth.StringWidth(m.rowInput.Value())-2))
	return lipgloss.NewStyle().MaxWidth(w).Render(line)
}

// fieldHint is the grayscale text after a field being typed: what the field takes, and for a date or
// a number the value the text will save as.
func (m *Model) fieldHint(spec model.FieldSpec, typed string, room int) string {
	typed = strings.TrimSpace(typed)
	h := spec.DefaultHint()
	if typed != "" {
		h = ""
		if spec.Type == model.FieldDate || spec.Type == model.FieldNumber {
			if v, err := spec.Normalize(typed, m.parseDateField); err == nil && v != typed {
				h = "-> " + v
			} else if err != nil {
				h = "? " + spec.DefaultHint()
			}
		}
	}
	if h == "" {
		return ""
	}
	return cutHint(hintSpacing(typed)+h, room)
}

func pidOf(p *model.Person) int64 {
	if p == nil {
		return 0
	}
	return p.ID
}
