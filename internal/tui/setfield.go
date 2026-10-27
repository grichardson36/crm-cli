package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Set field on selected (patterns/bulk-actions-always). One palette action covers every field with a
// fixed kind of value, choice, date and number, of every object type, so no field needs a bulk action
// of its own: priority tier of people, stage and value of deals, due date and priority of tasks. The
// value is checked by the field's type (model.FieldSpec.Normalize), the same check as the record page.
// It acts on the selected rows, else on the highlighted one, as one undo step.

// setFieldState is a "set field" waiting for the value the user types.
type setFieldState struct {
	refs []repo.Ref
	spec model.FieldSpec
}

// settableFields are the fields of a kind that a bulk set can take: choice, date and number fields
// the record editors can save.
func settableFields(kind string) []model.FieldSpec {
	var out []model.FieldSpec
	for _, f := range model.FieldsOf(kind) {
		if f.Type != model.FieldChoice && f.Type != model.FieldDate && f.Type != model.FieldNumber {
			continue
		}
		if _, ok := bulkSettable[kind+"."+f.Key]; ok {
			out = append(out, f)
		}
	}
	return out
}

// bulkSettable lists the fields saveTyped writes, by kind and key.
var bulkSettable = map[string]bool{
	"person.tier": true, "person.birthday": true,
	"deal.stage": true, "deal.value": true,
	"task.due": true, "task.priority": true,
}

func whenSetField(m *Model) bool {
	if m.focus != focusList {
		return false
	}
	refs := m.bulkRefs()
	if len(refs) == 0 {
		return false
	}
	return len(settableFields(refs[0].Kind)) > 0
}

// openSetField is the palette action: pick the field, then the value.
func (m *Model) openSetField() tea.Cmd {
	refs := m.bulkRefs()
	if len(refs) == 0 {
		m.setStatus("select rows or highlight an object first", true)
		return nil
	}
	kind := refs[0].Kind
	for _, r := range refs {
		if r.Kind != kind {
			m.setStatus("select objects of one type to set a field on them", true)
			return nil
		}
	}
	fields := settableFields(kind)
	who := nouns(refKinds(refs), len(refs))
	if len(fields) == 0 {
		m.setStatus(relationLabel(kind)+" has no choice, date or number field to set", true)
		return nil
	}
	items := make([]overlayItem, len(fields))
	for i, f := range fields {
		items[i] = overlayItem{id: "field:" + f.Key, label: f.Label, desc: string(f.Type), value: f.Key}
	}
	m.openPicker("Set field on "+who, items, func(m *Model, it overlayItem) {
		spec, _ := model.FieldNamed(kind, it.value)
		m.pendingCmd = m.askFieldValue(refs, spec)
	})
	return nil
}

// askFieldValue asks for the value: a pick list for a choice field, a typed line for a date or number.
func (m *Model) askFieldValue(refs []repo.Ref, spec model.FieldSpec) tea.Cmd {
	who := nouns(refKinds(refs), len(refs))
	if spec.Type == model.FieldChoice {
		items := make([]overlayItem, len(spec.Choices))
		for i, c := range spec.Choices {
			items[i] = overlayItem{id: "choice:" + c.Value, label: c.Label, value: c.Value}
		}
		m.openPicker("Set "+spec.Label+" on "+who, items, func(m *Model, it overlayItem) {
			m.applySetField(refs, spec, it.value)
		})
		return nil
	}
	m.setField = &setFieldState{refs: refs, spec: spec}
	m.startInput("set-field", fmt.Sprintf("set %s on %s: ", spec.Label, who), "")
	return m.input.Focus()
}

// runSetField is the enter of the value prompt.
func (m *Model) runSetField(value string) {
	st := m.setField
	m.setField = nil
	if st == nil {
		return
	}
	v, err := st.spec.Normalize(value, m.parseDateField)
	if err != nil {
		m.setStatus(err.Error(), true)
		m.setField = st
		m.startInput("set-field", fmt.Sprintf("set %s on %s: ", st.spec.Label, nouns(refKinds(st.refs), len(st.refs))), value)
		m.pendingCmd = m.input.Focus()
		return
	}
	m.applySetField(st.refs, st.spec, v)
}

// applySetField writes the value to every object as one undo step.
func (m *Model) applySetField(refs []repo.Ref, spec model.FieldSpec, v string) {
	if spec.Type == model.FieldChoice && spec.Clear != "" && v == spec.Clear {
		v = ""
	}
	who := nouns(refKinds(refs), len(refs))
	m.beginGroup()
	ok := 0
	var first error
	for _, ref := range refs {
		if err := m.saveTyped(ref, spec, v); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		ok++
	}
	m.endGroup(fmt.Sprintf("set %s on %s", spec.Label, who))
	m.marked = map[int64]bool{}
	m.afterWrite()
	shown := spec.ChoiceLabel(v)
	if v == "" {
		shown = "empty"
	}
	switch {
	case first != nil:
		m.setStatus(fmt.Sprintf("%d of %s set, %d failed: %v", ok, who, len(refs)-ok, first), true)
	default:
		msg := fmt.Sprintf("%s set to %s on %s (ctrl+z undoes)", spec.Label, shown, who)
		m.setStatus(msg, false)
		m.pendingCmd = m.showToast(msg, false)
	}
}

func setFieldAction() action {
	return action{id: "set-field", title: "Set field on selected", group: "Bulk",
		desc:  "set a choice, date or number field on the selected rows, or the highlighted one: priority tier of people, stage and value of deals, due date and priority of tasks. Palette, no key. The value is checked like on the record page",
		write: true, when: whenSetField, run: func(m *Model) tea.Cmd { return m.openSetField() }}
}
