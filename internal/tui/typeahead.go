package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/alias"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	sg "github.com/jdanielnd/crm-cli/internal/suggest"
)

// The relation typeahead (owner decision 2026-10-28, patterns/pick-or-create). A relation field of a
// record page is edited in place: type and the rows under the field list the existing objects that
// match, up and down move, enter picks the highlighted one, and the last row reads Create 'X' and
// makes the object and links it. tab completes the highlighted name into the field. Names match
// through the shared aliases (Zach finds Zachary) and forgive a typo (Zachery), which plain search
// does not. A linked row is a chip, u unlinks it.

const taMax = 5 // rows of matches shown

// typeahead is the state of an open relation typeahead.
type typeahead struct {
	src       repo.Ref
	other     string
	label     string
	all       []overlayItem
	shown     []overlayItem
	cur       int // index into shown, len(shown) is the Create row
	canCreate bool
	text      string
}

// typeaheadOK says whether a link started now runs as an in-place typeahead: from the link row of a
// record page, for a link of the page's own object. Every other start (a note row, the list, the bar)
// uses the picker.
func (m *Model) typeaheadOK(src repo.Ref, other string) bool {
	if m.focus != focusDetail || len(m.form) == 0 || m.fcur < 0 || m.fcur >= len(m.form) || m.creating || m.mode != modeNormal {
		return false
	}
	r := m.form[m.fcur]
	return r.kind == rkLinked && r.other == other && src == m.pageRef()
}

// beginTypeahead opens the typeahead under the focused row.
func (m *Model) beginTypeahead(src repo.Ref, other string) tea.Cmd {
	label := relationLabel(other)
	ta := &typeahead{src: src, other: other, label: label, all: m.candidateItems(src, other)}
	m.edit = &editState{row: m.fcur, kind: rkLinked, ta: ta}
	m.rowInput.SetValue("")
	m.rowInput.SetWidth(max(5, m.detail.Width()-labelWidth-1))
	m.mode = modeEdit
	m.focus = focusDetail
	m.setStatus("", false)
	m.taRefresh()
	m.syncDetail()
	return m.rowInput.Focus()
}

// taMatches ranks the candidates for the typed text with the matching every autocomplete shares
// (sg.Rank): aliases, word prefixes and a forgiven typo.
func taMatches(all []overlayItem, text string, tbl *alias.Table) []overlayItem {
	return sg.Rank(all, text, func(it overlayItem) string { return it.label }, sg.Options{Aliases: tbl, Limit: taMax})
}

// taRefresh recomputes the match rows for the text in the input.
func (m *Model) taRefresh() {
	ta := m.edit.ta
	ta.text = strings.TrimSpace(m.rowInput.Value())
	ta.shown = taMatches(ta.all, ta.text, m.aliasTable())
	typed := strings.TrimSpace(strings.TrimPrefix(ta.text, "#"))
	ta.canCreate = typed != ""
	for _, it := range ta.shown {
		if strings.EqualFold(strings.TrimPrefix(it.label, "#"), typed) {
			ta.canCreate = false
		}
	}
	rows := len(ta.shown)
	if ta.canCreate {
		rows++
	}
	if ta.cur >= rows {
		ta.cur = max(0, rows-1)
	}
}

// handleTypeaheadKey is a key while a relation typeahead is open.
func (m *Model) handleTypeaheadKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	ta := m.edit.ta
	rows := len(ta.shown)
	if ta.canCreate {
		rows++
	}
	switch keyName(msg) {
	case "esc":
		m.cancelEdit()
		return m, nil
	case "up", "shift+tab":
		if rows > 0 {
			ta.cur = (ta.cur + rows - 1) % rows
		}
		m.syncDetail()
		return m, nil
	case "down":
		if rows > 0 {
			ta.cur = (ta.cur + 1) % rows
		}
		m.syncDetail()
		return m, nil
	case "tab":
		if ta.cur < len(ta.shown) {
			m.rowInput.SetValue(strings.TrimPrefix(ta.shown[ta.cur].label, "#"))
			m.rowInput.CursorEnd()
			m.taRefresh()
		}
		m.syncDetail()
		return m, nil
	case "enter":
		return m, m.taPick()
	}
	var cmd tea.Cmd
	m.rowInput, cmd = m.rowInput.Update(msg)
	ta.cur = 0
	m.taRefresh()
	m.syncDetail()
	return m, cmd
}

// taPick links the highlighted match, or creates and links the typed text.
func (m *Model) taPick() tea.Cmd {
	ta := m.edit.ta
	var it overlayItem
	switch {
	case ta.cur < len(ta.shown):
		it = ta.shown[ta.cur]
	case ta.canCreate:
		it = overlayItem{id: "create", label: fmt.Sprintf("Create '%s'", ta.text), value: strings.TrimSpace(strings.TrimPrefix(ta.text, "#"))}
	default:
		m.cancelEdit()
		return nil
	}
	src, other := ta.src, ta.other
	m.mode = modeNormal
	m.edit = nil
	m.rowInput.Blur()
	m.pendingCmd = nil
	m.doLink(src, other, it)
	m.syncDetail()
	cmd := m.pendingCmd
	m.pendingCmd = nil
	return cmd
}

// typeaheadLines draws the field row with its input and the rows under it.
func (m *Model) typeaheadLines(label string, w int) []string {
	ta := m.edit.ta
	head := m.st.Dim.Render(fit(ta.label, labelWidth)) + strings.TrimRight(m.rowInput.View(), " ")
	if ta.text == "" {
		head += m.st.Dim.Render(cutHint(hintSpacing(m.rowInput.Value())+model.FieldSpec{Type: model.FieldRelation}.DefaultHint(), w-labelWidth-ansi.StringWidth(m.rowInput.Value())-2))
	}
	out := []string{lipgloss.NewStyle().MaxWidth(w).Render(head)}
	pad := strings.Repeat(" ", labelWidth)
	line := func(i int, text, desc string) {
		text = ansi.Truncate(cleanCell(text), max(1, w-labelWidth-2), "…")
		if i == ta.cur {
			out = append(out, m.st.Sel.Render(fit(pad+text+desc, w)))
			return
		}
		out = append(out, m.st.Dim.Render(pad)+text+m.st.Dim.Render(desc))
	}
	for i, it := range ta.shown {
		desc := ""
		if d := cleanCell(it.desc); d != "" {
			desc = "  " + d
		}
		line(i, it.label, ansi.Truncate(desc, max(0, w-labelWidth-ansi.StringWidth(it.label)-2), "…"))
	}
	if ta.canCreate {
		line(len(ta.shown), fmt.Sprintf("Create '%s'", strings.TrimPrefix(ta.text, "#")), "")
	}
	if len(ta.shown) == 0 && !ta.canCreate {
		out = append(out, m.st.Dim.Render(pad+"nothing to link yet"))
	}
	return out
}
