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

var (
	accent    = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#9D9BFF"}
	dim       = lipgloss.AdaptiveColor{Light: "#777777", Dark: "#888888"}
	danger    = lipgloss.AdaptiveColor{Light: "#C0392B", Dark: "#FF7B72"}
	good      = lipgloss.AdaptiveColor{Light: "#1E7E34", Dark: "#7EE787"}
	tabOn     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#5A56E0")).Padding(0, 1)
	tabOff    = lipgloss.NewStyle().Foreground(dim).Padding(0, 1)
	selStyle  = lipgloss.NewStyle().Reverse(true)
	offStyle  = lipgloss.NewStyle().Underline(true)
	dimStyle  = lipgloss.NewStyle().Foreground(dim)
	nameStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	headStyle = lipgloss.NewStyle().Bold(true)
	errStyle  = lipgloss.NewStyle().Foreground(danger)
	okStyle   = lipgloss.NewStyle().Foreground(good)
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
	sepStyle := dimStyle
	if m.focus == focusDetail {
		sepStyle = nameStyle
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
			tabs = append(tabs, tabOn.Render(seg.label))
		} else {
			tabs = append(tabs, tabOff.Render(seg.label))
		}
	}
	line := lipgloss.NewStyle().MaxWidth(m.width).Render(strings.Join(tabs, ""))
	count := dimStyle.Render(m.countText())
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
			return dimStyle.Render("No matches.")
		case m.view == viewDue:
			return dimStyle.Render("Nobody is due. Set a cadence with: crm person edit <id> --keep-in-touch \"3 mons\"")
		case m.view == viewBirthdays:
			return dimStyle.Render("No birthdays in the next 90 days.")
		case m.view == viewTasks:
			return dimStyle.Render("No open tasks. Press t on a person to add one.")
		case m.view == viewMerge:
			return dimStyle.Render("No duplicates at " + m.minLevel.String() + " confidence or above. m lowers the level.")
		case m.view == viewArchived:
			return dimStyle.Render("Nothing archived. # archives the selected person.")
		case m.view == viewSync:
			return dimStyle.Render("No sync sources connected yet.")
		}
		return dimStyle.Render("Nothing here yet.")
	}
	cursorStyle := selStyle
	if m.focus == focusDetail {
		cursorStyle = offStyle
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
			b.WriteString(nameStyle.Render(fit(title, w-metaW-1)) + dimStyle.Render(" "+meta))
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
			b.WriteString(name + dimStyle.Render(rest+" "+meta))
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
		status = errStyle.Render(m.confirm.prompt)
	default:
		switch {
		case m.busy != "":
			status = m.spin.View() + " " + dimStyle.Render(m.busy+"...")
		case m.status != "" && m.statusIsErr:
			status = errStyle.Render(m.status)
		case m.status != "":
			status = okStyle.Render(m.status)
		case m.mode == modeEdit:
			status = dimStyle.Render("editing: enter saves, esc cancels")
		case m.query != "":
			status = dimStyle.Render("filter: " + m.query + "  (esc clears)")
		}
	}
	if n := len(m.markedIDs()); n > 0 && (m.mode == modeNormal) {
		tag := nameStyle.Render(fmt.Sprintf("%d marked", n))
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
		return dimStyle.Render(runewidth.Truncate(h, m.width, "…"))
	case modeConfirm:
		return dimStyle.Render("y yes  n no")
	case modeSearch, modePrompt:
		if m.mode == modePrompt && m.promptKind == "note" {
			return dimStyle.Render("enter save  tab type  esc cancel")
		}
		return dimStyle.Render("enter confirm  esc cancel")
	}
	var parts []string
	if m.readOnly {
		parts = append(parts, errStyle.Render("READ-ONLY"))
	}
	for _, s := range m.footerSegs() {
		parts = append(parts, nameStyle.Render(s.hint)+" "+dimStyle.Render(s.label))
	}
	return strings.Join(parts, "  ")
}

func (m *Model) helpView() string {
	title := headStyle.Render("Keys") + dimStyle.Render("   generated from the same action list as the palette (ctrl+k)")
	foot := dimStyle.Render("j/k scroll  any other key closes  ctrl+k for every action")
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
