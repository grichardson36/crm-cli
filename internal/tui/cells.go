package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/mattn/go-runewidth"
)

// Link attributes and note type as cells (owner decision 2026-10-28, patterns/field-types-drive-editors).
// A relation field shows its linked rows as a small table. The first column is the linked object, the
// other columns are the attributes of the link: role, dates, primary flag, kind, notes, and for a deal
// its stage and value. A note row has its type as a cell. Each cell is a field with a type, edited in
// place with enter or e like any other field. left, right and tab move across the cells of the row,
// and from the first cell left or shift+tab (and from the last tab) return to the list. There is no
// separate "edit link" or "change note type" action.

// rowCell is one attribute cell of a form row.
type rowCell struct {
	key   string          // role, from, to, primary, kind, notes, stage, value, type
	label string          // column title
	value string          // the stored value
	spec  model.FieldSpec // the type decides the editor and the check
}

// cellWidths are the column widths by cell key.
var cellWidths = map[string]int{"role": 16, "from": 10, "to": 10, "primary": 8, "kind": 12, "notes": 16, "stage": 11, "value": 10, "type": 10}

func cellWidth(key string) int {
	if w, ok := cellWidths[key]; ok {
		return w
	}
	return 10
}

const cellGap = 2

func yesNoChoices() []model.FieldChoiceOption {
	return []model.FieldChoiceOption{{Value: "no", Label: "no"}, {Value: "yes", Label: "yes"}}
}

// linkCells builds the cells of one linked row from the relation registry.
func (m *Model) linkCells(li model.LinkedObject, kindNames map[string][]model.FieldChoiceOption) []rowCell {
	rel, ok := model.RelationNamed(li.Relation)
	if !ok {
		return nil
	}
	var cells []rowCell
	text := func(key, label, value, hint string) rowCell {
		return rowCell{key: key, label: label, value: value, spec: model.FieldSpec{Key: key, Label: label, Type: model.FieldText, Hint: hint}}
	}
	if rel.Kinded {
		cells = append(cells, rowCell{key: "kind", label: "kind", value: li.Label, spec: model.FieldSpec{Key: "kind", Label: "kind", Type: model.FieldChoice, Choices: kindNames[rel.A]}})
	}
	for _, a := range rel.AskAttrs() {
		switch a {
		case model.AttrRole:
			if rel.Name == model.DealPersonKind {
				// A role on a deal is a value of the deal role list, edited with the choice editor
				// and pick-or-create. A text from before the list shows as written.
				spec, _ := model.FieldNamed(model.DealPersonKind, model.AttrRole)
				cells = append(cells, rowCell{key: "role", label: "deal role", value: str(li.Role), spec: spec.WithVocabulary(roleNames(kindNames[model.DealRoleKind]))})
				continue
			}
			cells = append(cells, text("role", "role", str(li.Role), "what they do there, for example CTO"))
		case model.AttrFrom:
			cells = append(cells, text("from", "from", str(li.StartedOn), "2021, 2021-03 or 2021-03-15"))
		case model.AttrTo:
			cells = append(cells, text("to", "to", str(li.EndedOn), "2021, 2021-03 or 2021-03-15, empty means current"))
		case model.AttrPrimary:
			v := "no"
			if li.Primary {
				v = "yes"
			}
			cells = append(cells, rowCell{key: "primary", label: "primary", value: v, spec: model.FieldSpec{Key: "primary", Label: "primary", Type: model.FieldChoice, Choices: yesNoChoices()}})
		case model.AttrNotes:
			cells = append(cells, text("notes", "notes", cleanCell(str(li.Notes)), ""))
		}
	}
	if rel.Kinded && rel.HasAttr(model.AttrNotes) {
		cells = append(cells, text("notes", "notes", cleanCell(str(li.Notes)), ""))
	}
	if li.Kind == "deal" {
		if d, err := m.dlr.FindByID(m.ctx, li.ID); err == nil {
			stage, _ := m.fieldSpec("deal", "stage")
			value, _ := model.FieldNamed("deal", "value")
			cv := ""
			if d.Value != nil {
				cv = fmt.Sprintf("%g", *d.Value)
			}
			cells = append(cells, rowCell{key: "stage", label: "stage", value: d.Stage, spec: stage}, rowCell{key: "value", label: "value", value: cv, spec: value})
		}
	}
	return cells
}

// kindChoices loads the relation kind names of the object types that have kinds, once per form build.
func (m *Model) kindChoices() map[string][]model.FieldChoiceOption {
	out := map[string][]model.FieldChoiceOption{}
	kr := repo.NewKindRepo(m.db)
	for _, t := range []string{"person", "organization"} {
		kinds, err := kr.List(m.ctx, t)
		if err != nil {
			continue
		}
		for _, k := range kinds {
			out[t] = append(out[t], model.FieldChoiceOption{Value: k.Name, Label: k.Name})
		}
	}
	if names, err := m.vr.Names(m.ctx, model.DealRoleKind); err == nil {
		out[model.DealRoleKind] = model.ChoicesFromNames(names)
	}
	return out
}

// noteCells is the type cell of a note row.
func (m *Model) noteCells(typ string) []rowCell {
	var choices []model.FieldChoiceOption
	if list, err := m.ntr.List(m.ctx, false); err == nil {
		for _, t := range list {
			choices = append(choices, model.FieldChoiceOption{Value: t.Name, Label: t.Name})
		}
	}
	return []rowCell{{key: "type", label: "type", value: typ, spec: model.FieldSpec{Key: "type", Label: "type", Type: model.FieldChoice, Choices: choices}}}
}

// --- moving across cells ---

func whenCellRow(m *Model) bool {
	r, ok := m.rowFocused()
	return whenDetail(m) && ok && !r.empty && len(r.cells) > 0
}

func whenCellNext(m *Model) bool { return whenCellRow(m) && m.fcol < len(m.form[m.fcur].cells) }
func whenCellPrev(m *Model) bool { return whenCellRow(m) && m.fcol > 0 }

func cellActions() []action {
	return []action{
		{id: "cell-next", title: "Next cell", group: "Edit pane", hint: "right", keys: []string{"right", "tab"}, desc: "move to the next attribute cell of a linked row or note (role, dates, primary, kind, type). From the last cell tab goes back to the list",
			foot: 16, footIn: inDetail, footLabel: "cell", when: whenCellNext, run: func(m *Model) tea.Cmd { m.fcol++; m.syncDetail(); return nil }},
		{id: "cell-prev", title: "Previous cell", group: "Edit pane", hint: "left", keys: []string{"left", "shift+tab"}, desc: "move to the previous cell of a linked row or note. From the first cell left goes back to the list",
			when: whenCellPrev, run: func(m *Model) tea.Cmd { m.fcol--; m.syncDetail(); return nil }},
	}
}

// focusedCell is the cell under the detail cursor, when the cursor is on one.
func (m *Model) focusedCell() (rowCell, bool) {
	r, ok := m.rowFocused()
	if !ok || r.empty || m.fcol < 1 || m.fcol > len(r.cells) {
		return rowCell{}, false
	}
	return r.cells[m.fcol-1], true
}

// beginCell edits the cell under the cursor in place, by the type of its field.
func (m *Model) beginCell() tea.Cmd {
	c, ok := m.focusedCell()
	if !ok {
		return nil
	}
	r, _ := m.rowFocused()
	if m.readOnly {
		m.setStatus("read-only: editing is disabled", true)
		return nil
	}
	if c.spec.Type == model.FieldChoice && c.spec.Vocabulary == "" && len(c.spec.Choices) == 0 {
		m.setStatus("nothing to choose from for "+c.label, true)
		return nil
	}
	ref := m.pageRef()
	return m.startTypedEdit(c.spec, ref, c.value, &cellEdit{cell: c, row: r})
}

// cellEdit says a typed edit is for a cell of a row, not for a field of the record.
type cellEdit struct {
	cell rowCell
	row  formRow
}

// saveCell writes one cell value as one undo step.
func (m *Model) saveCell(ce *cellEdit, src repo.Ref, v string) error {
	c, r := ce.cell, ce.row
	switch c.key {
	case "stage", "value":
		return m.saveTyped(repo.Ref{Kind: "deal", ID: r.li.ID}, c.spec, v)
	case "type":
		_, err := m.hUpdateNote(m.ctx, r.id, model.UpdateInteractionInput{Type: &v})
		return err
	}
	if r.li == nil {
		return fmt.Errorf("this row has no link to edit: %w", model.ErrValidation)
	}
	li := *r.li
	var at model.LinkAttrs
	switch c.key {
	case "role":
		at.Role = &v
	case "from":
		at.StartedOn = &v
	case "to":
		at.EndedOn = &v
	case "notes":
		at.Notes = &v
	case "kind":
		at.Kind = &v
	case "primary":
		p := v == "yes"
		at.Primary = &p
	}
	target := repo.Ref{Kind: li.Kind, ID: li.ID}
	label := "edit " + c.label + " of the link of " + m.refName(src) + " and " + m.refName(target)
	return m.recordLinkChange(label, src, target, func() error {
		_, err := m.links.Update(m.ctx, li.Relation, li.LinkID, at)
		return err
	})
}

// --- drawing ---

// cellLayout decides the columns of a cell row in width w (after the label column): the width of the
// name column, and the first and last cell shown. The cells scroll sideways so the selected one is
// always visible, the name column keeps a readable minimum.
func cellLayout(cells []rowCell, w, sel int) (nameW, first, last int) {
	const minName = 10
	avail := w - labelWidth
	if avail < minName+4 {
		return max(1, avail), 0, -1
	}
	// widths from the first shown cell on
	fits := func(from int) (int, int) {
		used := minName
		to := from - 1
		for i := from; i < len(cells); i++ {
			need := cellGap + cellWidth(cells[i].key)
			if used+need > avail {
				break
			}
			used += need
			to = i
		}
		return used, to
	}
	first = 0
	for first < len(cells) {
		_, to := fits(first)
		if sel < 1 || sel-1 <= to {
			break
		}
		first++
	}
	used, to := fits(first)
	nameW = minName + (avail - used)
	if to >= first && nameW > 40 { // do not stretch the name column beyond reason
		nameW = 40
	}
	return nameW, first, to
}

// cellSection tells whether two rows share a table: same kind of row and the same columns.
func sameTable(a, b formRow) bool {
	if a.kind != b.kind || len(a.cells) != len(b.cells) || a.other != b.other {
		return false
	}
	for i := range a.cells {
		if a.cells[i].key != b.cells[i].key {
			return false
		}
	}
	return true
}

// tableSel is the selected column of the table row i belongs to, 0 when the cursor is elsewhere.
func (m *Model) tableSel(i int) int {
	if m.focus != focusDetail || m.fcur < 0 || m.fcur >= len(m.form) || i < 0 || i >= len(m.form) {
		return 0
	}
	if sameTable(m.form[m.fcur], m.form[i]) {
		return m.fcol
	}
	return 0
}

// cellText is a cell value as shown in its column.
func cellText(c rowCell) string {
	switch c.key {
	case "primary":
		if c.value == "yes" {
			return "primary"
		}
		return "-"
	case "value":
		if c.value == "" {
			return "-"
		}
		return typedDisplay(formRow{value: c.value, spec: &c.spec})
	}
	if c.value == "" {
		return "-"
	}
	return c.value
}

// renderCellRow draws a row with cells: label, the linked object (or the note text) and the cells.
func (m *Model) renderCellRow(i int, r formRow, w int) string {
	sel := m.tableSel(i)
	focusedRow := m.focus == focusDetail && i == m.fcur
	label := fit(r.label, labelWidth)
	if len(r.label) > labelWidth-1 {
		label = fit(runewidth.Truncate(r.label, labelWidth-1, "…"), labelWidth)
	}
	nameW, first, last := cellLayout(r.cells, w, sel)
	name := r.value
	if r.kind == rkNote {
		name = strings.TrimSpace(strings.TrimPrefix(r.key, r.typ))
		name = strings.TrimSpace(strings.TrimPrefix(name, ":"))
		if name == "" {
			name = firstLineOf(r.value)
		}
	}
	if name == "" {
		name = "-"
	}
	name = fit(runewidth.Truncate(name, nameW, "…"), nameW)
	dim := r.dim
	style := func(s string, active bool) string {
		switch {
		case active:
			return m.st.Sel.Render(s)
		case dim:
			return m.st.Dim.Render(s)
		}
		return s
	}
	var b strings.Builder
	b.WriteString(m.st.Dim.Render(label))
	b.WriteString(style(name, focusedRow && sel == 0))
	for c := first; c <= last && c < len(r.cells); c++ {
		cw := cellWidth(r.cells[c].key)
		txt := fit(runewidth.Truncate(cellText(r.cells[c]), cw, "…"), cw)
		b.WriteString("  ")
		if focusedRow && sel == c+1 {
			b.WriteString(m.st.Sel.Render(txt))
		} else {
			b.WriteString(m.st.Dim.Render(txt))
		}
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(b.String())
}

// cellHeader is the line of column titles above the first row of a table.
func (m *Model) cellHeader(i int, r formRow, w int) string {
	sel := m.tableSel(i)
	nameW, first, last := cellLayout(r.cells, w, sel)
	var b strings.Builder
	b.WriteString(m.st.Head.Render(fit(runewidth.Truncate(r.head, labelWidth+nameW-1, "…"), labelWidth+nameW)))
	for c := first; c <= last && c < len(r.cells); c++ {
		cw := cellWidth(r.cells[c].key)
		b.WriteString("  ")
		b.WriteString(m.st.Dim.Render(fit(runewidth.Truncate(r.cells[c].label, cw, "…"), cw)))
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(b.String())
}

func firstLineOf(s string) string {
	l, _ := splitFirst(s)
	return l
}

// roleNames turns the options of a vocabulary back into its values.
func roleNames(opts []model.FieldChoiceOption) []string {
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = o.Value
	}
	return out
}
