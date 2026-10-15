package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/mattn/go-runewidth"
)

// Merge view (duplicate groups) and the compare screen that merges people.

// countLabels name the list counts shown on the compare screen, in the order of cmpMember.counts.
var countLabels = [...]string{"emails", "phones", "links", "tags", "tasks", "interactions"}

type cmpMember struct {
	id     int64
	name   string
	counts [len(countLabels)]int
}

// cmpRow is one field that differs between the people. vals holds the raw value per member
// (empty means no value). pick is the member whose value wins, or -1 for the default.
type cmpRow struct {
	label string
	keys  []string // merge choice keys the row sets
	vals  []string
	pick  int
}

// compareState is the open compare screen.
type compareState struct {
	members []cmpMember
	rows    []cmpRow
	cur     int
	surv    int
	top     int // first visible field row
	colOff  int // first visible person column

	level   dedupe.Level // 0 for a manual merge
	reasons []string
	manual  bool // opened from marked people instead of a duplicate group

	preview    *model.MergeResult
	previewErr string
}

const cmpLabelW = 14

func whenCompare(m *Model) bool   { return m.cmp != nil }
func whenMergeList(m *Model) bool { return m.view == viewMerge && m.focus == focusList }
func whenMergeRows(m *Model) bool { return whenMergeList(m) && len(m.rows) > 0 }
func whenDismiss(m *Model) bool   { return m.cmp != nil || whenMergeRows(m) }
func whenArchivedRows(m *Model) bool {
	return m.view == viewArchived && m.focus == focusList && len(m.rows) > 0
}

// levelBadge is the short label shown in the list.
func levelBadge(l dedupe.Level) string {
	switch l {
	case dedupe.High:
		return "HIGH"
	case dedupe.Medium:
		return "MED"
	case dedupe.Low:
		return "LOW"
	case dedupe.Fuzzy:
		return "FUZZY"
	}
	return "?"
}

func groupRow(g dedupe.Group) row {
	names := make([]string, len(g.Members))
	for i, mb := range g.Members {
		names[i] = cleanCell(mb.Name)
	}
	r := row{title: fmt.Sprintf("%-5s %s", levelBadge(g.Level), strings.Join(names, " | "))}
	if len(g.Reasons) > 0 {
		r.sub = cleanCell(g.Reasons[0])
	}
	return r
}

func fullName(p *model.Person) string {
	return strings.TrimSpace(p.FirstName + " " + str(p.LastName))
}

func dateOnly(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// nameOf is a person's display name from whatever list knows them.
func (m *Model) nameOf(id int64) string {
	for _, p := range m.people {
		if p.ID == id {
			return p.Name
		}
	}
	if p, ok := m.arch[id]; ok {
		return fullName(p)
	}
	return fmt.Sprintf("#%d", id)
}

// --- list actions ---

func (m *Model) cycleLevel() {
	next := m.minLevel - 1
	if next < dedupe.Fuzzy {
		next = dedupe.High
	}
	m.minLevel = next
	m.cursor, m.offset = 0, 0
	m.report(m.rebuild(false))
	m.setStatus("duplicates: "+m.minLevel.String()+" confidence and above", false)
}

func (m *Model) openGroupCompare() {
	if m.cursor < 0 || m.cursor >= len(m.groups) {
		return
	}
	g := m.groups[m.cursor]
	m.openCompare(g.IDs(), g.Level, g.Reasons, false)
}

func (m *Model) openMarkedCompare() {
	ids := m.markedIDs()
	if len(ids) < 2 {
		m.setStatus("mark at least two people first (space)", true)
		return
	}
	m.openCompare(ids, 0, []string{"marked by you"}, true)
}

// groupIDs are the people a dismiss applies to: the open compare, else the selected group.
func (m *Model) groupIDs() []int64 {
	if m.cmp != nil {
		ids := make([]int64, len(m.cmp.members))
		for i, mb := range m.cmp.members {
			ids[i] = mb.id
		}
		return ids
	}
	if m.cursor >= 0 && m.cursor < len(m.groups) && m.view == viewMerge {
		return m.groups[m.cursor].IDs()
	}
	return nil
}

func (m *Model) dismissGroup() {
	ids := m.groupIDs()
	if len(ids) < 2 {
		return
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if err := m.dr.Dismiss(m.ctx, ids[i], ids[j]); err != nil {
				m.setStatus(err.Error(), true)
				return
			}
		}
	}
	m.dismissed = append(m.dismissed, ids)
	manual := m.cmp != nil && m.cmp.manual
	m.cmp = nil
	if manual {
		m.marked = map[int64]bool{}
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("dismissed %d people as not duplicates (u undoes)", len(ids)), false)
}

func (m *Model) undoDismiss() {
	if len(m.dismissed) == 0 {
		m.setStatus("no dismissal to undo in this session", true)
		return
	}
	ids := m.dismissed[len(m.dismissed)-1]
	m.dismissed = m.dismissed[:len(m.dismissed)-1]
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			if err := m.dr.Undismiss(m.ctx, ids[i], ids[j]); err != nil && !isNotFound(err) {
				m.setStatus(err.Error(), true)
				return
			}
		}
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("restored the group of %d people", len(ids)), false)
}

func isNotFound(err error) bool { return err != nil && strings.Contains(err.Error(), "not found") }

// --- compare screen ---

func (m *Model) openCompare(ids []int64, level dedupe.Level, reasons []string, manual bool) {
	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	c := &compareState{level: level, reasons: reasons, manual: manual}
	people := make([]*model.Person, 0, len(sorted))
	for _, id := range sorted {
		p, err := m.pr.FindByID(m.ctx, id)
		if err == nil {
			err = m.pr.LoadDetails(m.ctx, p)
		}
		if err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		pid := id
		mb := cmpMember{id: id, name: fullName(p)}
		mb.counts[0], mb.counts[1], mb.counts[2] = len(p.Emails), len(p.Phones), len(p.Handles)
		if tags, err := m.tagr.GetForEntity(m.ctx, "person", id); err == nil {
			mb.counts[3] = len(tags)
		}
		if tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{PersonID: &pid}); err == nil {
			mb.counts[4] = len(tasks)
		}
		if ints, err := m.ir.FindAll(m.ctx, model.InteractionFilters{PersonID: &pid}); err == nil {
			mb.counts[5] = len(ints)
		}
		c.members = append(c.members, mb)
		people = append(people, p)
	}
	c.rows = diffRows(people)
	m.cmp = c
	m.cmpPreview()
	m.setStatus("", false)
}

// diffRows lists the fields on which the people disagree.
func diffRows(people []*model.Person) []cmpRow {
	type def struct {
		label string
		keys  []string
		get   func(p *model.Person) string
	}
	defs := []def{
		{"name", []string{"first_name", "last_name"}, fullName},
		{"title", []string{"title"}, func(p *model.Person) string { return strings.TrimSpace(str(p.Title)) }},
		{"company", []string{"company"}, func(p *model.Person) string { return strings.TrimSpace(str(p.Company)) }},
		{"birthday", []string{"birthday"}, func(p *model.Person) string { return formatBirthdayParts(p) }},
		{"location", []string{"location"}, func(p *model.Person) string { return strings.TrimSpace(str(p.Location)) }},
		{"keep in touch", []string{"keep_in_touch"}, func(p *model.Person) string { return str(p.KeepInTouch) }},
		{"priority", []string{"priority_tier"}, func(p *model.Person) string { return tierText(p.PriorityTier) }},
		{"favorite", []string{"is_favorite"}, func(p *model.Person) string {
			if p.IsFavorite {
				return "yes"
			}
			return ""
		}},
		{"notes", []string{"notes"}, func(p *model.Person) string { return strings.TrimSpace(str(p.Notes)) }},
	}
	var rows []cmpRow
	for _, d := range defs {
		vals := make([]string, len(people))
		differ := false
		for i, p := range people {
			vals[i] = d.get(p)
			if vals[i] != vals[0] {
				differ = true
			}
		}
		if differ {
			rows = append(rows, cmpRow{label: d.label, keys: d.keys, vals: vals, pick: -1})
		}
	}
	return rows
}

// resolved is the member whose value wins for row r. Unless picked, it is the survivor, or the
// first person with a value when the survivor has none (the same rule as the merge itself).
func (c *compareState) resolved(r int) int {
	row := c.rows[r]
	if row.pick >= 0 {
		return row.pick
	}
	if row.vals[c.surv] != "" {
		return c.surv
	}
	for i, v := range row.vals {
		if v != "" {
			return i
		}
	}
	return c.surv
}

func (c *compareState) move(delta int) {
	if len(c.rows) == 0 {
		return
	}
	c.cur = clampInt(c.cur+delta, 0, len(c.rows)-1)
}

// pick moves the winner of the current row to the next (or previous) person with a different value.
func (c *compareState) pick(dir int) {
	if len(c.rows) == 0 {
		return
	}
	n := len(c.members)
	cur := c.resolved(c.cur)
	for i := 1; i < n; i++ {
		j := ((cur+dir*i)%n + n) % n
		if c.rows[c.cur].vals[j] != c.rows[c.cur].vals[cur] {
			c.rows[c.cur].pick = j
			return
		}
	}
}

func (c *compareState) nextSurvivor() { c.surv = (c.surv + 1) % len(c.members) }

// plan returns the survivor, the dropped people and the explicit field choices.
func (c *compareState) plan() (int64, []int64, map[string]int64) {
	keep := c.members[c.surv].id
	var drops []int64
	for i, mb := range c.members {
		if i != c.surv {
			drops = append(drops, mb.id)
		}
	}
	choices := map[string]int64{}
	for r, row := range c.rows {
		id := c.members[c.resolved(r)].id
		for _, k := range row.keys {
			choices[k] = id
		}
	}
	return keep, drops, choices
}

// cmpPreview refreshes the dry run shown under the table.
func (m *Model) cmpPreview() {
	c := m.cmp
	if c == nil {
		return
	}
	keep, drops, choices := c.plan()
	res, err := m.pr.MergeDryRun(m.ctx, keep, drops, choices)
	c.preview, c.previewErr = res, ""
	if err != nil {
		c.preview, c.previewErr = nil, err.Error()
	}
}

// summarize lists the non-zero list changes of a merge on one line.
func summarize(res *model.MergeResult) string {
	var parts []string
	for _, name := range []string{"emails", "phones", "handles", "tags", "custom_fields"} {
		if n := res.Added[name]; n > 0 {
			parts = append(parts, fmt.Sprintf("+%d %s", n, strings.ReplaceAll(name, "_", " ")))
		}
	}
	var moved []string
	for _, name := range []string{"interactions", "tasks", "task_people", "deals", "relationships"} {
		if n := res.Moved[name]; n > 0 {
			moved = append(moved, fmt.Sprintf("%d %s", n, strings.ReplaceAll(name, "_", " ")))
		}
	}
	if len(moved) > 0 {
		parts = append(parts, "moved "+strings.Join(moved, ", "))
	}
	if len(parts) == 0 {
		return "no list changes"
	}
	return strings.Join(parts, ", ")
}

func (m *Model) applyMerge() {
	c := m.cmp
	if c == nil {
		return
	}
	keep, drops, choices := c.plan()
	res, err := m.pr.Merge(m.ctx, keep, drops, choices)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	name := c.members[c.surv].name
	m.cmp = nil
	if c.manual {
		m.marked = map[int64]bool{}
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("merged %d into #%d %s: %d fields, %s", len(drops), keep, name, len(res.Fields), summarize(res)), false)
}

// --- compare rendering ---

// geometry returns the column layout for a screen width and keeps the focused column visible.
func (c *compareState) geometry(width int) (colW, vis int) {
	n := len(c.members)
	avail := max(1, width-cmpLabelW)
	vis = clampInt(avail/16, 1, n)
	colW = max(4, avail/vis)
	focus := c.surv
	if len(c.rows) > 0 {
		focus = c.resolved(c.cur)
	}
	if focus < c.colOff {
		c.colOff = focus
	}
	if focus >= c.colOff+vis {
		c.colOff = focus - vis + 1
	}
	c.colOff = clampInt(c.colOff, 0, n-vis)
	return colW, vis
}

// render returns exactly h lines for the body and, for each line, the field row shown there (or -1).
func (c *compareState) render(st *theme.Styles, width, h int) ([]string, []int) {
	colW, vis := c.geometry(width)
	end := min(len(c.members), c.colOff+vis)
	pad := strings.Repeat(" ", cmpLabelW)

	head := pad
	sub := pad
	for j := c.colOff; j < end; j++ {
		mb := c.members[j]
		title := fmt.Sprintf("#%d %s", mb.id, cleanCell(mb.name))
		if j == c.surv {
			head += st.Name.Render(fit(title, colW))
			sub += st.Name.Render(fit("keeps the record", colW))
		} else {
			head += st.Head.Render(fit(title, colW))
			sub += st.Dim.Render(fit("archived after merge", colW))
		}
	}
	if c.colOff > 0 || end < len(c.members) {
		sub = lipgloss.NewStyle().MaxWidth(width).Render(sub)
	}

	var body []string
	var rowAt []int
	push := func(s string, r int) {
		body = append(body, s)
		rowAt = append(rowAt, r)
	}
	if len(c.rows) == 0 {
		push(st.Dim.Render("No field differs. The lists below are combined."), -1)
	}
	for i, row := range c.rows {
		label := fit(row.label, cmpLabelW)
		if i == c.cur {
			label = st.Sel.Render(label)
		} else {
			label = st.Dim.Render(label)
		}
		win := c.resolved(i)
		line := label
		for j := c.colOff; j < end; j++ {
			v := cleanCell(row.vals[j])
			if v == "" {
				v = "-"
			}
			if j == win {
				line += st.Name.Render(fit("✓ "+v, colW))
			} else {
				line += st.Dim.Render(fit("  "+v, colW))
			}
		}
		push(line, i)
	}
	nFields := len(body)
	push("", -1)
	push(st.Head.Render("Combined, always"), -1)
	for k, name := range countLabels {
		line := st.Dim.Render(fit(name, cmpLabelW))
		for j := c.colOff; j < end; j++ {
			line += fit(fmt.Sprint(c.members[j].counts[k]), colW)
		}
		push(line, -1)
	}
	push("", -1)
	switch {
	case c.previewErr != "":
		push(st.Err.Render(c.previewErr), -1)
	case c.preview != nil:
		push(st.Dim.Render("result: "+summarize(c.preview)), -1)
	}

	// The two header lines stay put, the field rows scroll to keep the cursor visible.
	room := max(1, h-2)
	if c.cur < c.top {
		c.top = c.cur
	}
	if c.cur >= c.top+room && nFields > 0 {
		c.top = c.cur - room + 1
	}
	c.top = clampInt(c.top, 0, max(0, len(body)-room))
	lines := []string{head, sub}
	at := []int{-1, -1}
	for i := c.top; i < len(body) && len(lines) < h; i++ {
		lines = append(lines, body[i])
		at = append(at, rowAt[i])
	}
	for len(lines) < h {
		lines = append(lines, "")
		at = append(at, -1)
	}
	return lines[:h], at[:h]
}

func (m *Model) compareView() string {
	c := m.cmp
	title := "Merge " + fmt.Sprint(len(c.members)) + " people"
	if c.level != 0 {
		title += "  " + levelBadge(c.level)
	}
	if len(c.reasons) > 0 {
		title += "  " + strings.Join(c.reasons, "; ")
	}
	cut := lipgloss.NewStyle().MaxWidth(m.width)
	lines, _ := c.render(&m.st, m.width, m.bodyHeight())
	for i := range lines {
		lines[i] = cut.Render(lines[i])
	}
	return cut.Render(m.st.Head.Render(runewidth.Truncate(title, m.width, "…"))) + "\n" + strings.Join(lines, "\n") + "\n" + m.footerView()
}

// --- detail text for views without a form ---

// emptyDetail is the detail pane content when no form is shown.
func (m *Model) emptyDetail() string {
	w := max(1, m.detail.Width)
	line := func(s string) string { return runewidth.Truncate(s, w, "…") }
	r, ok := m.selected()
	if !ok {
		return m.st.Dim.Render("Nothing selected.")
	}
	var b strings.Builder
	switch m.view {
	case viewSync:
		return m.syncItemDetail(r)
	case viewMerge:
		if m.cursor >= len(m.groups) {
			return ""
		}
		g := m.groups[m.cursor]
		b.WriteString(m.st.Name.Render(fmt.Sprintf("%s confidence, %d people", levelBadge(g.Level), len(g.Members))) + "\n")
		for _, reason := range g.Reasons {
			b.WriteString(m.st.Dim.Render(line(reason)) + "\n")
		}
		for _, mb := range g.Members {
			b.WriteString("\n" + m.st.Head.Render(line(fmt.Sprintf("#%d %s", mb.ID, cleanCell(mb.Name)))) + "\n")
			for _, p := range m.people {
				if p.ID != mb.ID {
					continue
				}
				var role []string
				for _, v := range []string{cleanCell(p.Title), cleanCell(p.Company)} {
					if v != "" {
						role = append(role, v)
					}
				}
				if len(role) > 0 {
					b.WriteString(line(strings.Join(role, " at ")) + "\n")
				}
				if p.AllEmails != "" {
					b.WriteString(m.st.Dim.Render(line(p.AllEmails)) + "\n")
				}
				if p.Location != "" {
					b.WriteString(m.st.Dim.Render(line(cleanCell(p.Location))) + "\n")
				}
			}
		}
		b.WriteString("\n" + m.st.Dim.Render("enter compares and merges, D dismisses"))
	case viewArchived:
		p := m.arch[r.personID]
		if p == nil {
			return m.st.Dim.Render("Nothing selected.")
		}
		_ = m.pr.LoadDetails(m.ctx, p)
		b.WriteString(m.st.Name.Render(line(fullName(p))) + m.st.Dim.Render("  archived "+dateOnly(p.UpdatedAt)) + "\n\n")
		for _, kv := range [][2]string{{"title", str(p.Title)}, {"company", str(p.Company)}, {"location", str(p.Location)}, {"birthday", formatBirthdayParts(p)}} {
			if kv[1] != "" {
				b.WriteString(m.st.Dim.Render(fit(kv[0], labelWidth)) + line(kv[1]) + "\n")
			}
		}
		for _, e := range p.Emails {
			b.WriteString(m.st.Dim.Render(fit("email", labelWidth)) + line(e.Email) + "\n")
		}
		for _, ph := range p.Phones {
			b.WriteString(m.st.Dim.Render(fit("phone", labelWidth)) + line(ph.PhoneNumber) + "\n")
		}
		b.WriteString("\n" + m.st.Dim.Render("u restores, X purges forever"))
	default:
		return m.st.Dim.Render("Nothing selected.")
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- mouse ---

func (m *Model) handleCompareMouse(x, y int, wheel int) {
	c := m.cmp
	if wheel != 0 {
		c.move(wheel)
		return
	}
	_, rowAt := c.render(&m.st, m.width, m.bodyHeight())
	line := y - 1
	if line < 0 || line >= len(rowAt) || rowAt[line] < 0 {
		return
	}
	colW, vis := c.geometry(m.width)
	c.cur = rowAt[line]
	if x >= cmpLabelW {
		if col := c.colOff + (x-cmpLabelW)/colW; col < c.colOff+vis && col < len(c.members) {
			c.rows[c.cur].pick = col
			m.cmpPreview()
		}
	}
}
