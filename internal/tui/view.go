package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/mattn/go-runewidth"
)

func tabLabel(i int, name string) string { return fmt.Sprintf("%d %s", i+1, name) }

// cleanCell removes emoji and control characters that break column widths in list cells.
func cleanCell(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 0x1F000, r >= 0xFE00 && r <= 0xFE0F, r == 0x200D, r == 0x20E3:
			continue
		case r >= 0x2600 && r <= 0x27BF && r != '★':
			continue
		case unicode.IsControl(r):
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// fit truncates s to width cells and pads it with spaces.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = runewidth.Truncate(s, width, "…")
	if pad := width - runewidth.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// View implements tea.Model.
func (m *Model) baseView() string {
	if m.width == 0 {
		return "loading..."
	}
	if m.mode == modeOverlay && m.ov != nil {
		return m.overlayView()
	}
	if m.tv != nil {
		return m.textViewRender()
	}
	if m.help {
		return m.helpView()
	}
	if m.cmp != nil {
		return m.compareView()
	}
	listW := m.listWidth()
	bodyH := m.bodyHeight()
	detailW := m.width - listW - 1
	if detailW < 1 {
		detailW = 1
	}

	list := lipgloss.NewStyle().Width(listW).Height(bodyH).MaxHeight(bodyH).Render(m.listView(listW, bodyH))
	sepStyle := m.st.Dim
	if m.focus == focusDetail {
		sepStyle = m.st.Name
	}
	sep := sepStyle.Render(strings.TrimRight(strings.Repeat("│\n", bodyH), "\n"))
	detail := lipgloss.NewStyle().Width(detailW).Height(bodyH).MaxHeight(bodyH).PaddingLeft(1).Render(m.detail.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, sep, detail)

	return m.tabsView() + "\n" + body + "\n" + m.footerView()
}

// tabSeg is one tab in the tab bar with its screen columns [x0, x1). A segment with more set is
// the "More" entry that opens the view switcher. on marks the highlighted tab.
type tabSeg struct {
	label  string
	x0, x1 int
	view   viewKind
	more   bool
	on     bool
}

// mainTabs are the views with their own tab. Every other view is reached through More or v.
var mainTabs = []viewKind{viewHome, viewPeople, viewDue, viewTasks}

func isMainTab(v viewKind) bool {
	for _, t := range mainTabs {
		if t == v {
			return true
		}
	}
	return false
}

// tabLayout places the tabs: the main views plus a "v More" entry. When the current view is not
// a main view, the More entry shows its name. When the full labels do not fit the width, tabs
// other than the current one shrink to their key so the bar still fits.
func (m *Model) tabLayout() []tabSeg {
	type item struct {
		full, short string
		seg         tabSeg
	}
	var items []item
	for _, v := range mainTabs {
		items = append(items, item{full: tabLabel(int(v), viewNames[v]), short: strconv.Itoa(int(v) + 1),
			seg: tabSeg{view: v, on: v == m.view}})
	}
	more := item{full: "v More", short: "v", seg: tabSeg{more: true, view: m.view}}
	if !isMainTab(m.view) {
		more.full = "v " + viewNames[m.view]
		more.seg.on = true
	}
	items = append(items, more)

	total := 0
	for _, it := range items {
		total += runewidth.StringWidth(it.full) + 2
	}
	compact := total > m.width
	var segs []tabSeg
	x := 0
	for _, it := range items {
		label := it.full
		if compact && !it.seg.on {
			label = it.short
		}
		w := runewidth.StringWidth(label) + 2
		seg := it.seg
		seg.label, seg.x0, seg.x1 = label, x, x+w
		segs = append(segs, seg)
		x += w
	}
	return segs
}

// countText is the right side of the tab bar.
func (m *Model) countText() string {
	switch m.view {
	case viewHome:
		return ""
	case viewMerge:
		return fmt.Sprintf("%d groups, %s+", len(m.rows), m.minLevel)
	case viewArchived:
		return fmt.Sprintf("%d archived", len(m.rows))
	case viewSync:
		sources, inbox := 0, 0
		for _, r := range m.rows {
			if r.sync != nil && r.sync.source != nil {
				sources++
			}
			if r.sync != nil && r.sync.entry != nil {
				inbox++
			}
		}
		return fmt.Sprintf("%d sources, %d in inbox", sources, inbox)
	}
	return fmt.Sprintf("%d shown / %d total", len(m.rows), len(m.people))
}

func (m *Model) tabsView() string {
	var tabs []string
	for _, seg := range m.tabLayout() {
		if seg.on {
			tabs = append(tabs, m.st.TabOn.Render(seg.label))
		} else {
			tabs = append(tabs, m.st.TabOff.Render(seg.label))
		}
	}
	line := lipgloss.NewStyle().MaxWidth(m.width).Render(strings.Join(tabs, ""))
	count := m.st.Dim.Render(m.countText())
	gap := m.width - lipgloss.Width(line) - lipgloss.Width(count)
	if gap < 1 {
		return line
	}
	return line + strings.Repeat(" ", gap) + count
}

func (m *Model) listView(w, h int) string {
	if len(m.rows) == 0 {
		switch {
		case m.view == viewPeople && m.query != "":
			return m.st.Dim.Render("No matches.")
		case m.view == viewDue:
			return m.st.Dim.Render("Nobody is due. Set a cadence with: crm person edit <id> --keep-in-touch \"3 mons\"")
		case m.view == viewBirthdays:
			return m.st.Dim.Render("No birthdays in the next 90 days.")
		case m.view == viewTasks:
			return m.st.Dim.Render("No open tasks. Press t on a person to add one.")
		case m.view == viewMerge:
			return m.st.Dim.Render("No duplicates at " + m.minLevel.String() + " confidence or above. m lowers the level.")
		case m.view == viewArchived:
			return m.st.Dim.Render("Nothing archived. # archives the selected person.")
		case m.view == viewSync:
			return m.st.Dim.Render("No sync sources connected yet.")
		}
		return m.st.Dim.Render("Nothing here yet.")
	}
	cursorStyle := m.st.Sel
	if m.focus == focusDetail {
		cursorStyle = m.st.Off
	}
	anyMarks := len(m.marked) > 0
	var b strings.Builder
	end := m.offset + h
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := m.offset; i < end; i++ {
		r := m.rows[i]
		title, sub := cleanCell(r.title), cleanCell(r.sub)
		if r.skip {
			meta := runewidth.Truncate(cleanCell(r.meta), w/2, "…")
			metaW := runewidth.StringWidth(meta)
			b.WriteString(m.st.Name.Render(fit(title, w-metaW-1)) + m.st.Dim.Render(" "+meta))
			if i < end-1 {
				b.WriteString("\n")
			}
			continue
		}
		if anyMarks {
			if m.marked[r.personID] && r.personID != 0 {
				title = "✓ " + title
			} else {
				title = "  " + title
			}
		}
		meta := runewidth.Truncate(cleanCell(r.meta), w/2, "…")
		metaW := runewidth.StringWidth(meta)
		left := title
		if sub != "" {
			left += "  " + sub
		}
		line := fit(left, w-metaW-1) + " " + meta
		line = fit(line, w)
		if i == m.cursor {
			b.WriteString(cursorStyle.Render(line))
		} else {
			// Dim the subtitle by rendering name and rest separately.
			name := runewidth.Truncate(title, w-metaW-1, "…")
			rest := ""
			if sub != "" && runewidth.StringWidth(name) == runewidth.StringWidth(title) {
				rest = fit("  "+sub, w-metaW-1-runewidth.StringWidth(name))
			} else {
				rest = strings.Repeat(" ", max(0, w-metaW-1-runewidth.StringWidth(name)))
			}
			b.WriteString(name + m.st.Dim.Render(rest+" "+meta))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// footSeg is one clickable key hint in the footer.
type footSeg struct {
	hint, label, id string
	x0, x1          int // screen columns [x0, x1)
}

var footShort = map[string]string{
	"down": "move", "search": "search", "focus-detail": "edit", "mark": "mark", "contacted": "contacted",
	"tag": "tag", "cadence": "cadence", "tier": "tier", "note": "note", "new": "new", "task": "task",
	"help": "help", "palette": "menu", "edit": "edit", "add": "add", "delete": "delete", "copy": "copy",
	"open": "open", "back": "back", "compare": "compare", "dupe-level": "level", "dismiss": "dismiss",
	"undo-dismiss": "undo", "merge-marked": "merge", "archive": "archive", "restore": "restore", "purge": "purge",
	"home-open": "open", "sync-now": "sync", "sync-dry": "dry-run", "inbox-accept": "accept", "inbox-attach": "attach",
	"inbox-ignore": "ignore", "goto-view": "views", "note-retype": "type",
	"cmp-pick": "pick", "cmp-survivor": "survivor", "cmp-apply": "merge", "cmp-back": "back",
}

// footerSegs lists the hints for the focused pane, in order, limited to what fits the width.
func (m *Model) footerSegs() []footSeg {
	var acts []*action
	for i := range m.actions {
		a := &m.actions[i]
		if a.foot == 0 || a.footIn&m.focusBit() == 0 || !a.enabled(m) {
			continue
		}
		if a.footIf != nil && !a.footIf(m) {
			continue
		}
		if a.write && m.readOnly {
			continue
		}
		acts = append(acts, a)
	}
	sort.SliceStable(acts, func(i, j int) bool { return acts[i].foot < acts[j].foot })

	x := 0
	if m.readOnly {
		x = len("READ-ONLY") + 2
	}
	var segs []footSeg
	for _, a := range acts {
		hint, label := a.hint, footShort[a.id]
		if a.id == "down" {
			hint = "j/k"
		}
		if a.id == "focus-detail" && m.readOnly {
			label = "detail"
		}
		w := runewidth.StringWidth(hint) + 1 + runewidth.StringWidth(label)
		if x+w > m.width {
			break
		}
		segs = append(segs, footSeg{hint: hint, label: label, id: a.id, x0: x, x1: x + w})
		x += w + 2
	}
	return segs
}

func (m *Model) footerView() string {
	var status string
	switch m.mode {
	case modeSearch, modePrompt:
		status = m.input.View()
	case modeConfirm:
		status = m.st.Err.Render(m.confirm.prompt)
	default:
		switch {
		case m.busy != "":
			status = m.spin.View() + " " + m.st.Dim.Render(m.busy+"...")
		case m.status != "" && m.statusIsErr:
			status = m.st.Err.Render(m.status)
		case m.status != "":
			status = m.st.Ok.Render(m.status)
		case m.mode == modeEdit:
			status = m.st.Dim.Render("editing: enter saves, esc cancels")
		case m.query != "":
			status = m.st.Dim.Render("filter: " + m.query + "  (esc clears)")
		}
	}
	if n := len(m.markedIDs()); n > 0 && (m.mode == modeNormal) {
		tag := m.st.Mark.Render(fmt.Sprintf("%d marked", n))
		room := m.width - lipgloss.Width(tag) - 1
		if room > 0 {
			status = lipgloss.NewStyle().MaxWidth(room).Render(status)
			pad := room - lipgloss.Width(status)
			status += strings.Repeat(" ", max(0, pad)) + " " + tag
		}
	}
	statusLine := lipgloss.NewStyle().MaxWidth(m.width).Render(status)
	if pad := m.width - lipgloss.Width(statusLine); pad > 0 {
		statusLine += strings.Repeat(" ", pad)
	}
	return statusLine + "\n" + m.hintsLine()
}

func (m *Model) hintsLine() string {
	switch m.mode {
	case modeEdit:
		h := "enter save  esc cancel"
		if m.edit != nil && m.edit.kind == rkHandle {
			h += "  tab change kind"
		}
		return m.st.Dim.Render(runewidth.Truncate(h, m.width, "…"))
	case modeConfirm:
		return m.st.Dim.Render("y yes  n no")
	case modeSearch, modePrompt:
		if m.mode == modePrompt && m.promptKind == "note" {
			return m.st.Dim.Render("enter save  tab type  esc cancel")
		}
		return m.st.Dim.Render("enter confirm  esc cancel")
	}
	var parts []string
	if m.readOnly {
		parts = append(parts, m.st.Warn.Render("READ-ONLY"))
	}
	for _, s := range m.footerSegs() {
		parts = append(parts, m.st.Name.Render(s.hint)+" "+m.st.Dim.Render(s.label))
	}
	return strings.Join(parts, "  ")
}

func (m *Model) helpView() string {
	title := m.st.Head.Render("Keys") + m.st.Dim.Render("   generated from the same action list as the palette (ctrl+k)")
	foot := m.st.Dim.Render("j/k scroll  any other key closes  ctrl+k for every action")
	cut := lipgloss.NewStyle().MaxWidth(m.width)
	return cut.Render(title) + "\n" + m.helpVP.View() + "\n" + cut.Render(foot)
}

func formatBirthdayParts(p *model.Person) string {
	if p.BirthdayMonth == nil || p.BirthdayDay == nil {
		return ""
	}
	if p.BirthdayYear != nil {
		return fmt.Sprintf("%04d-%02d-%02d", *p.BirthdayYear, *p.BirthdayMonth, *p.BirthdayDay)
	}
	return fmt.Sprintf("%02d-%02d", *p.BirthdayMonth, *p.BirthdayDay)
}
