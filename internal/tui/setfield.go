package tui

import (
	"strings"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
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
// the record editors can save. The deal role of people (the role of a person on a deal, a field of
// the deal link) is one of them, backed by the deal role list.
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
	if kind == "person" {
		out = append(out, dealRoleSpec(nil))
	}
	return out
}

// dealRoleKey is the key of the deal role in the set-field picker. The field itself is the role of
// model.Fields[deal_person].
const dealRoleKey = "deal_role"

// dealRoleSpec is the deal role field as a bulk-settable field of a person, with its options set from
// names (nil leaves them empty, the picker loads them).
func dealRoleSpec(names []string) model.FieldSpec {
	spec, _ := model.FieldNamed(model.DealPersonKind, model.AttrRole)
	spec.Key = dealRoleKey
	return spec.WithVocabulary(names)
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
		if kind == "person" && it.value == dealRoleKey {
			m.pendingCmd = m.askDealForRole(refs)
			return
		}
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
	res := validate.Field(st.spec, value, validate.Options{ParseDate: m.parseDateField})
	v := res.Value
	if err := res.Err(); err != nil {
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
	if name := m.setFieldCreate; name != "" {
		m.setFieldCreate = ""
		if err := m.createVocabValue(model.DealRoleKind, name); err != nil {
			m.endGroup("")
			m.setStatus(err.Error(), true)
			return
		}
	}
	ok := 0
	var first error
	for _, ref := range refs {
		save := func() error { return m.saveTyped(ref, spec, v) }
		if spec.Vocabulary != "" {
			save = func() error { return m.saveDealRole(ref, v) }
		}
		if err := save(); err != nil {
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
		desc:  "set a choice, date or number field on the selected rows, or the highlighted one: priority tier and deal role of people, stage and value of deals, due date and priority of tasks. Palette, no key. The value is checked like on the record page. A deal role asks for the deal first and can create a new role",
		write: true, when: whenSetField, run: func(m *Model) tea.Cmd { return m.openSetField() }}
}

// --- the deal role of people (the role on a deal link) ---

// askDealForRole is the first step of a bulk deal role: the role of a person belongs to a deal, so the
// deal is picked first from the deals the selected people are on.
func (m *Model) askDealForRole(refs []repo.Ref) tea.Cmd {
	var items []overlayItem
	seen := map[int64]bool{}
	for _, r := range refs {
		deals, err := m.links.Linked(m.ctx, r, "deal")
		if err != nil {
			m.setStatus(err.Error(), true)
			return nil
		}
		for _, d := range deals {
			if !seen[d.ID] {
				seen[d.ID] = true
				items = append(items, overlayItem{id: fmt.Sprintf("deal:%d", d.ID), label: d.Name, value: fmt.Sprint(d.ID)})
			}
		}
	}
	if len(items) == 0 {
		m.setStatus("the selected people are on no deal, link them to a deal first", true)
		return nil
	}
	who := nouns(refKinds(refs), len(refs))
	m.openPicker("Set deal role on "+who+": which deal?", items, func(m *Model, it overlayItem) {
		var id int64
		fmt.Sscan(it.value, &id)
		m.pendingCmd = m.askDealRoleValue(refs, repo.Ref{Kind: "deal", ID: id})
	})
	return nil
}

// askDealRoleValue picks the role from the deal role list. Typing a name that is not on the list adds
// the last row "Create role X" (pick or create).
func (m *Model) askDealRoleValue(refs []repo.Ref, deal repo.Ref) tea.Cmd {
	names, err := m.vr.Names(m.ctx, model.DealRoleKind)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	spec := dealRoleSpec(names)
	items := make([]overlayItem, len(spec.Choices))
	for i, c := range spec.Choices {
		items[i] = overlayItem{id: "choice:" + c.Value, label: c.Label, value: c.Value}
	}
	who := nouns(refKinds(refs), len(refs))
	m.openPicker("Set deal role on "+who+" for "+m.refName(deal), items, func(m *Model, it overlayItem) {
		m.setFieldDeal = deal
		defer func() { m.setFieldDeal = repo.Ref{} }()
		if it.id == "create" {
			m.setFieldCreate = it.value
		}
		m.applySetField(refs, spec, it.value)
	})
	m.ov.create = func(q string) (overlayItem, bool) {
		q = model.CleanVocabName(q)
		if q == "" {
			return overlayItem{}, false
		}
		for _, c := range spec.Choices {
			if strings.EqualFold(c.Value, q) {
				return overlayItem{}, false
			}
		}
		return overlayItem{id: "create", label: "Create role " + q, desc: "add it to the deal roles and set it", value: q}, true
	}
	m.ov.refilter()
	return nil
}

// saveDealRole sets the role of one person on the deal of m.setFieldDeal, as a recorded link change.
func (m *Model) saveDealRole(person repo.Ref, role string) error {
	deal := m.setFieldDeal
	linked, err := m.links.Linked(m.ctx, deal, "person")
	if err != nil {
		return err
	}
	for _, l := range linked {
		if l.ID != person.ID {
			continue
		}
		label := "set deal role of " + m.refName(person) + " on " + m.refName(deal)
		return m.recordLinkChange(label, deal, person, func() error {
			_, err := m.links.Update(m.ctx, l.Relation, l.LinkID, model.LinkAttrs{Role: &role})
			return err
		})
	}
	return fmt.Errorf("%s is not on %s: %w", m.refName(person), m.refName(deal), model.ErrValidation)
}
