package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/alias"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/mattn/go-runewidth"
)

// The relation typeahead (owner decision 2026-10-28, patterns/pick-or-create). A relation field of a
// record page is edited in place: type and the rows under the field list the existing objects that
// match, up and down move, enter picks the highlighted one, and the last row reads Create 'X' and
// makes the object and links it. tab completes the highlighted name into the field. Names match
// through the shared aliases (Zach finds Zachary) and forgive a typo (Zachery), which plain search
// does not. A linked row is a chip, u unlinks it.

const (
	taMax        = 5    // rows of matches shown
	taTypoWord   = 0.75 // spelling ratio of a typed word to a word of a name
	taTypoFull   = 0.80 // spelling ratio of the typed text to the whole name
	taMinTypoLen = 3
)

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
	m.rowInput.Width = max(5, m.detail.Width-labelWidth-1)
	m.mode = modeEdit
	m.focus = focusDetail
	m.setStatus("", false)
	m.taRefresh()
	m.syncDetail()
	return m.rowInput.Focus()
}

// taMatches ranks the candidates for the typed text. Direct matches (the text, or an alias of a typed
// word, inside the name) come first. Names that are one typo away follow while there is room.
func taMatches(all []overlayItem, text string, tbl *alias.Table) []overlayItem {
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "#"))
	if text == "" {
		out := append([]overlayItem(nil), all...)
		if len(out) > taMax {
			out = out[:taMax]
		}
		return out
	}
	low := strings.ToLower(text)
	words := strings.Fields(low)
	type scored struct {
		it    overlayItem
		score int
	}
	var direct, typo []scored
	for _, it := range all {
		name := strings.ToLower(strings.TrimPrefix(it.label, "#"))
		sc := taScore(name, low, words, tbl)
		if sc >= 0 {
			direct = append(direct, scored{it, sc})
			continue
		}
		if len([]rune(low)) >= taMinTypoLen && taTypo(name, low, words) {
			typo = append(typo, scored{it, 9})
		}
	}
	sort.SliceStable(direct, func(i, j int) bool { return direct[i].score < direct[j].score })
	var out []overlayItem
	for _, s := range direct {
		out = append(out, s.it)
	}
	for _, s := range typo {
		out = append(out, s.it)
	}
	if len(out) > taMax {
		out = out[:taMax]
	}
	return out
}

// taScore is 0 for an exact name, 1 for a prefix, 2 for a word prefix, 3 for any other substring, and
// -1 for no match. Every word typed must match, each through its aliases.
func taScore(name, low string, words []string, tbl *alias.Table) int {
	if name == low {
		return 0
	}
	if strings.HasPrefix(name, low) {
		return 1
	}
	best := 3
	for _, w := range words {
		ok := false
		for _, n := range tbl.Expand(w) {
			switch {
			case strings.HasPrefix(name, n):
				ok = true
			case strings.Contains(" "+name, " "+n):
				ok = true
				if best > 2 {
					best = 2
				}
			case strings.Contains(name, n):
				ok = true
			}
			if ok {
				break
			}
		}
		if !ok {
			return -1
		}
	}
	return best
}

// taTypo says whether the text is a misspelling of the name: the whole text close to the whole name,
// or each typed word close to a word of the name (existing similarity scoring, dedupe.Ratio).
func taTypo(name, low string, words []string) bool {
	if dedupe.Ratio(low, name) >= taTypoFull {
		return true
	}
	parts := strings.Fields(name)
	for _, w := range words {
		if len([]rune(w)) < taMinTypoLen {
			return false
		}
		hit := false
		for _, p := range parts {
			if dedupe.Ratio(w, p) >= taTypoWord {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return len(words) > 0
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
func (m *Model) handleTypeaheadKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	ta := m.edit.ta
	rows := len(ta.shown)
	if ta.canCreate {
		rows++
	}
	switch msg.String() {
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
		head += m.st.Dim.Render(cutHint(hintSpacing(m.rowInput.Value())+model.FieldSpec{Type: model.FieldRelation}.DefaultHint(), w-labelWidth-runewidth.StringWidth(m.rowInput.Value())-2))
	}
	out := []string{lipgloss.NewStyle().MaxWidth(w).Render(head)}
	pad := strings.Repeat(" ", labelWidth)
	line := func(i int, text, desc string) {
		text = runewidth.Truncate(cleanCell(text), max(1, w-labelWidth-2), "…")
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
		line(i, it.label, runewidth.Truncate(desc, max(0, w-labelWidth-runewidth.StringWidth(it.label)-2), "…"))
	}
	if ta.canCreate {
		line(len(ta.shown), fmt.Sprintf("Create '%s'", strings.TrimPrefix(ta.text, "#")), "")
	}
	if len(ta.shown) == 0 && !ta.canCreate {
		out = append(out, m.st.Dim.Render(pad+"nothing to link yet"))
	}
	return out
}
