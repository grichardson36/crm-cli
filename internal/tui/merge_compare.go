package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/mattn/go-runewidth"
)

// The compare screen: [candidate 1] | [candidate 2] | [final contact]. Rows are aligned across the
// panes. A single-value row (name, title, ...) takes the value of one candidate. A list item row
// (email, phone, link, tag, custom field) is in the result or not. The result pane is computed
// from the same state that becomes the merge call, so what you see is what is saved.

const (
	cmpWide    = 110 // at this width and above all three panes show
	cmpMid     = 64  // from here two panes show, below it one
	cmpSepW    = 3   // " | " between panes
	cmpGutterW = 14  // marker and label column
)

// cmpMember is one person in the group with everything the panes show.
type cmpMember struct {
	id           int64
	name         string
	updated      string
	p            *model.Person
	tags         []string
	fields       []model.PersonField
	notes        int // interactions of type note
	interactions int // all other interactions
	tasks        int
	orgs         int // organizations, past ones too
	related      int // related people
	deals        int
}

type cmpKind int

const (
	rowSingle cmpKind = iota // one value, the cursor picks which candidate it comes from
	rowItem                  // one list item, in the result or not
	rowInfo                  // counts, always combined
)

// cmpRow is one aligned row across the panes.
type cmpRow struct {
	kind  cmpKind
	label string

	// single: merge choice keys the row sets, the value of each candidate and the picked
	// candidate (-1 means the default). newest compares the values themselves (dates).
	keys   []string
	vals   [2]string
	pick   int
	newest bool

	// item: group is email, phone, tag, handle or custom_field. excl names the alternatives a
	// single value slot allows (a link kind, a custom field name). has says which candidate has
	// the item and inc is -1 (default), 0 (out) or 1 (in).
	group string
	excl  string
	text  string
	has   [2]bool
	inc   int

	// info: the count of each candidate.
	nums [2]int
}

// compareState is the open compare screen.
type compareState struct {
	members []cmpMember // the whole group
	a, b    int         // the pair on screen, indexes into members
	s       int         // survivor within the pair: 0 or 1
	rows    []cmpRow
	cur     int
	top     int  // first visible row
	page    int  // narrow screens: which pane (set) shows
	confirm bool // the final summary is showing

	level   dedupe.Level // 0 for a manual merge
	reasons []string
	manual  bool // opened from marked people instead of a duplicate group

	preview    *model.MergeResult
	previewErr string
	st         theme.Styles
}

func (c *compareState) pair(j int) *cmpMember {
	if j == 0 {
		return &c.members[c.a]
	}
	return &c.members[c.b]
}

// --- building ---

func (m *Model) openCompare(ids []int64, level dedupe.Level, reasons []string, manual bool) {
	sorted := append([]int64(nil), ids...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	c := &compareState{level: level, reasons: reasons, manual: manual, b: 1, st: m.st}
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
		mb := cmpMember{id: id, name: fullName(p), updated: p.UpdatedAt, p: p}
		if tags, err := m.tagr.GetForEntity(m.ctx, "person", id); err == nil {
			for _, t := range tags {
				mb.tags = append(mb.tags, t.Name)
			}
		}
		mb.fields, _ = m.pr.CustomFields(m.ctx, id)
		if tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{PersonID: &pid}); err == nil {
			mb.tasks = len(tasks)
		}
		ref := repo.Ref{Kind: "person", ID: pid}
		if l, err := m.links.Linked(m.ctx, ref, "organization"); err == nil {
			mb.orgs = len(l)
		}
		if l, err := m.links.Linked(m.ctx, ref, "person"); err == nil {
			mb.related = len(l)
		}
		if l, err := m.links.Linked(m.ctx, ref, "deal"); err == nil {
			mb.deals = len(l)
		}
		if ints, err := m.ir.FindAll(m.ctx, model.InteractionFilters{PersonID: &pid}); err == nil {
			for _, it := range ints {
				if it.Type == "note" {
					mb.notes++
				} else {
					mb.interactions++
				}
			}
		}
		c.members = append(c.members, mb)
	}
	if len(c.members) < 2 {
		m.setStatus("a merge needs two people", true)
		return
	}
	c.buildRows()
	m.cmp = c
	m.cmpPreview()
	m.setStatus("", false)
}

type itemSrc struct{ id, text, excl string }

// buildRows lists everything the two candidates have, one aligned row per field or list item.
func (c *compareState) buildRows() {
	pa, pb := c.pair(0).p, c.pair(1).p
	var rows []cmpRow
	single := func(label string, keys []string, newest, always bool, get func(*model.Person) string) {
		v := [2]string{strings.TrimSpace(get(pa)), strings.TrimSpace(get(pb))}
		if !always && v[0] == "" && v[1] == "" {
			return
		}
		rows = append(rows, cmpRow{kind: rowSingle, label: label, keys: keys, vals: v, pick: -1, newest: newest, inc: -1})
	}
	items := func(label, group string, la, lb []itemSrc) {
		idx := map[string]int{}
		var out []cmpRow
		for j, list := range [2][]itemSrc{la, lb} {
			for _, it := range list {
				if i, ok := idx[it.id]; ok {
					out[i].has[j] = true
					continue
				}
				idx[it.id] = len(out)
				r := cmpRow{kind: rowItem, label: label, group: group, excl: it.excl, text: it.text, inc: -1, pick: -1}
				r.has[j] = true
				out = append(out, r)
			}
		}
		rows = append(rows, out...)
	}
	lower := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	emails := func(p *model.Person) []itemSrc {
		var out []itemSrc
		for _, e := range p.Emails {
			id := dedupe.EmailKey(e.Email)
			if id == "" {
				id = lower(e.Email)
			}
			out = append(out, itemSrc{id: id, text: e.Email})
		}
		return out
	}
	phones := func(p *model.Person) []itemSrc {
		var out []itemSrc
		for _, ph := range p.Phones {
			id := dedupe.PhoneKey(ph.PhoneNumber)
			if id == "" {
				id = lower(ph.PhoneNumber)
			}
			out = append(out, itemSrc{id: id, text: ph.PhoneNumber})
		}
		return out
	}

	single("name", []string{"first_name", "last_name"}, false, true, fullName)
	single("title", []string{"title"}, false, false, func(p *model.Person) string { return str(p.Title) })
	single("company", []string{"company"}, false, false, func(p *model.Person) string { return str(p.Company) })
	single("location", []string{"location"}, false, false, func(p *model.Person) string { return str(p.Location) })
	single("birthday", []string{"birthday"}, false, false, formatBirthdayParts)

	items("email", "email", emails(pa), emails(pb))
	items("phone", "phone", phones(pa), phones(pb))
	handles := func(p *model.Person) []itemSrc {
		var out []itemSrc
		for _, h := range p.Handles {
			out = append(out, itemSrc{id: h.Kind + "\x00" + lower(h.Value), text: h.Value, excl: h.Kind})
		}
		return out
	}
	// A link row is labeled with its kind, so build the rows per kind.
	kinds := map[string]bool{}
	var kindOrder []string
	for _, p := range []*model.Person{pa, pb} {
		for _, h := range p.Handles {
			if !kinds[h.Kind] {
				kinds[h.Kind] = true
				kindOrder = append(kindOrder, h.Kind)
			}
		}
	}
	for _, k := range kindOrder {
		only := func(list []itemSrc) []itemSrc {
			var out []itemSrc
			for _, it := range list {
				if it.excl == k {
					out = append(out, it)
				}
			}
			return out
		}
		items(k, "handle", only(handles(pa)), only(handles(pb)))
	}

	single("keep in touch", []string{"keep_in_touch"}, false, false, func(p *model.Person) string { return str(p.KeepInTouch) })
	single("priority", []string{"priority_tier"}, false, false, func(p *model.Person) string { return tierText(p.PriorityTier) })
	single("favorite", []string{"is_favorite"}, false, false, func(p *model.Person) string {
		if p.IsFavorite {
			return "yes"
		}
		return ""
	})
	single("last contact", []string{"last_contacted_at"}, true, false, func(p *model.Person) string { return dateOnly(str(p.LastContactedAt)) })
	single("notes", []string{"notes"}, false, false, func(p *model.Person) string { return str(p.Notes) })
	single("summary", []string{"summary"}, false, false, func(p *model.Person) string { return str(p.Summary) })

	tags := func(mb *cmpMember) []itemSrc {
		var out []itemSrc
		for _, t := range mb.tags {
			out = append(out, itemSrc{id: lower(t), text: t})
		}
		return out
	}
	items("tag", "tag", tags(c.pair(0)), tags(c.pair(1)))
	fields := func(mb *cmpMember) []itemSrc {
		var out []itemSrc
		for _, f := range mb.fields {
			out = append(out, itemSrc{id: lower(f.Name) + "\x00" + lower(f.Value), text: f.Name + ": " + f.Value, excl: f.Name})
		}
		return out
	}
	items("field", "custom_field", fields(c.pair(0)), fields(c.pair(1)))

	info := func(label string, get func(*cmpMember) int) {
		rows = append(rows, cmpRow{kind: rowInfo, label: label, nums: [2]int{get(c.pair(0)), get(c.pair(1))}, pick: -1, inc: -1})
	}
	info("notes", func(mb *cmpMember) int { return mb.notes })
	info("interactions", func(mb *cmpMember) int { return mb.interactions })
	info("tasks", func(mb *cmpMember) int { return mb.tasks })
	info("organizations", func(mb *cmpMember) int { return mb.orgs })
	info("related people", func(mb *cmpMember) int { return mb.related })
	info("deals", func(mb *cmpMember) int { return mb.deals })

	c.rows = rows
	c.top = 0
	c.cur = 0
	for i := range c.rows {
		if c.rows[i].kind != rowInfo && c.rows[i].differs() {
			c.cur = i
			return
		}
	}
	for i := range c.rows {
		if c.rows[i].kind != rowInfo {
			c.cur = i
			return
		}
	}
}

func (r *cmpRow) differs() bool {
	switch r.kind {
	case rowSingle:
		return r.vals[0] != r.vals[1]
	case rowItem:
		return r.has[0] != r.has[1]
	}
	return false
}

// conflict says both candidates have a different value for the same slot.
func (c *compareState) conflict(i int) bool {
	r := &c.rows[i]
	switch r.kind {
	case rowSingle:
		return r.vals[0] != "" && r.vals[1] != "" && r.vals[0] != r.vals[1]
	case rowItem:
		if r.excl == "" {
			return false
		}
		for k := range c.rows {
			if k != i && c.rows[k].kind == rowItem && c.rows[k].group == r.group && c.rows[k].excl == r.excl {
				return true
			}
		}
	}
	return false
}

// --- choices ---

// newerSide is the candidate whose record changed last, or the survivor on a tie.
func (c *compareState) newerSide() int {
	switch {
	case c.pair(0).updated > c.pair(1).updated:
		return 0
	case c.pair(1).updated > c.pair(0).updated:
		return 1
	}
	return c.s
}

// defaultSide is the default winner for a pair of values: a non-empty value beats an empty one,
// then the newer record wins (the later date for dates), then the survivor.
func (c *compareState) defaultSide(v [2]string, newest bool) int {
	switch {
	case v[0] == v[1]:
		return c.s
	case v[0] == "":
		return 1
	case v[1] == "":
		return 0
	case newest:
		if v[0] > v[1] {
			return 0
		}
		return 1
	}
	return c.newerSide()
}

// resolved is the candidate whose value row i takes (single rows).
func (c *compareState) resolved(i int) int {
	r := &c.rows[i]
	if r.pick >= 0 {
		return r.pick
	}
	return c.defaultSide(r.vals, r.newest)
}

// included says whether list item i is in the result. Items that are alternatives for one slot
// (two different linkedin links) default to the side that wins the slot.
func (c *compareState) included(i int) bool {
	r := &c.rows[i]
	if r.inc >= 0 {
		return r.inc == 1
	}
	if r.excl == "" || (r.has[0] && r.has[1]) {
		return true
	}
	var has [2]bool
	for k := range c.rows {
		o := &c.rows[k]
		if o.kind == rowItem && o.group == r.group && o.excl == r.excl {
			has[0] = has[0] || o.has[0]
			has[1] = has[1] || o.has[1]
		}
	}
	w := c.newerSide()
	if has[0] != has[1] {
		w = 0
		if has[1] {
			w = 1
		}
	}
	return r.has[w]
}

func (c *compareState) setInclude(i int, on bool) {
	r := &c.rows[i]
	r.inc = 0
	if on {
		r.inc = 1
		if r.excl != "" {
			for k := range c.rows {
				o := &c.rows[k]
				if k != i && o.kind == rowItem && o.group == r.group && o.excl == r.excl {
					o.inc = 0
				}
			}
		}
	}
}

// move puts the cursor on the next selectable row in the direction of delta.
func (c *compareState) move(delta int) {
	if len(c.rows) == 0 || delta == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	cur := c.cur
	for n := abs(delta); n > 0; n-- {
		next := cur + step
		for next >= 0 && next < len(c.rows) && c.rows[next].kind == rowInfo {
			next += step
		}
		if next < 0 || next >= len(c.rows) {
			break
		}
		cur = next
	}
	c.cur = cur
}


// take sets the current row from candidate j: a single row takes its value, a list item goes in.
// It reports false when the candidate has nothing for the row.
func (c *compareState) take(j int) bool {
	if c.cur < 0 || c.cur >= len(c.rows) {
		return false
	}
	r := &c.rows[c.cur]
	switch r.kind {
	case rowSingle:
		r.pick = j
		return true
	case rowItem:
		if !r.has[j] {
			return false
		}
		c.setInclude(c.cur, true)
		return true
	}
	return false
}

// toggle flips the current row: the other candidate for a single row, in or out for a list item.
func (c *compareState) toggle() {
	if c.cur < 0 || c.cur >= len(c.rows) {
		return
	}
	switch c.rows[c.cur].kind {
	case rowSingle:
		c.rows[c.cur].pick = 1 - c.resolved(c.cur)
	case rowItem:
		c.setInclude(c.cur, !c.included(c.cur))
	}
}

func (c *compareState) nextSurvivor() { c.s = 1 - c.s }

// nextPair moves to the next pair of the group. Groups of two have only one.
func (c *compareState) nextPair() {
	n := len(c.members)
	if n <= 2 {
		return
	}
	a, b := c.a, c.b+1
	if b >= n {
		a++
		b = a + 1
	}
	if b >= n {
		a, b = 0, 1
	}
	c.a, c.b, c.s, c.page, c.confirm = a, b, 0, 0, false
	c.buildRows()
}

// nextPage cycles the visible panes on screens too narrow for all three.
func (c *compareState) nextPage() { c.page++ }

// --- the result ---

// finalText is what the result pane shows for row i. Empty means nothing is kept.
func (c *compareState) finalText(i int) string {
	r := &c.rows[i]
	switch r.kind {
	case rowSingle:
		return r.vals[c.resolved(i)]
	case rowItem:
		if c.included(i) {
			return r.text
		}
	case rowInfo:
		return fmt.Sprint(r.nums[0] + r.nums[1])
	}
	return ""
}

// plan returns the survivor, the dropped person and the choices that make the merge match the screen.
func (c *compareState) plan() (int64, []int64, map[string]int64, model.MergeOptions) {
	keep, drop := c.pair(c.s), c.pair(1-c.s)
	choices := map[string]int64{}
	opts := model.MergeOptions{Exclude: map[string][]string{}, CustomFrom: map[string]int64{}}
	type slot struct {
		group, name string
		in          bool
		owner       int64
	}
	slots := map[string]*slot{}
	var order []string
	for i := range c.rows {
		r := &c.rows[i]
		switch r.kind {
		case rowSingle:
			id := c.pair(c.resolved(i)).id
			for _, k := range r.keys {
				choices[k] = id
			}
		case rowItem:
			in := c.included(i)
			if r.excl == "" {
				if !in {
					opts.Exclude[r.group] = append(opts.Exclude[r.group], r.text)
				}
				continue
			}
			k := r.group + "\x00" + r.excl
			sl, ok := slots[k]
			if !ok {
				sl = &slot{group: r.group, name: r.excl}
				slots[k] = sl
				order = append(order, k)
			}
			if in {
				sl.in = true
				sl.owner = keep.id
				if !r.has[c.s] {
					sl.owner = drop.id
				}
			}
		}
	}
	for _, k := range order {
		sl := slots[k]
		switch {
		case sl.group == "handle" && sl.in:
			choices[sl.name] = sl.owner
		case sl.group == "handle":
			opts.Exclude["handle"] = append(opts.Exclude["handle"], sl.name)
		case sl.in:
			opts.CustomFrom[sl.name] = sl.owner
		default:
			opts.Exclude["custom_field"] = append(opts.Exclude["custom_field"], sl.name)
		}
	}
	return keep.id, []int64{drop.id}, choices, opts
}

// cmpTake sets the current row from candidate j and refreshes the dry run.
func (m *Model) cmpTake(j int) {
	if !m.cmp.take(j) {
		m.setStatus("nothing to take from that side for this row", true)
		return
	}
	m.setStatus("", false)
	m.cmpPreview()
}

// cmpPreview refreshes the dry run shown on the screen and in the final summary.
func (m *Model) cmpPreview() {
	c := m.cmp
	if c == nil {
		return
	}
	keep, drops, choices, opts := c.plan()
	res, err := m.pr.MergeDryRunWith(m.ctx, keep, drops, choices, opts)
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
		if n := res.Removed[name]; n > 0 {
			parts = append(parts, fmt.Sprintf("-%d %s", n, strings.ReplaceAll(name, "_", " ")))
		}
	}
	var moved []string
	for _, name := range []string{"interactions", "tasks", "deals", "relationships", "memberships"} {
		if n := res.Moved[name]; n > 0 {
			moved = append(moved, fmt.Sprintf("%d %s", n, strings.ReplaceAll(name, "_", " ")))
		}
	}
	if n := res.Moved["duplicates_dropped"]; n > 0 {
		moved = append(moved, fmt.Sprintf("%d duplicate links dropped", n))
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
	keep, drops, choices, opts := c.plan()
	res, err := m.pr.MergeWith(m.ctx, keep, drops, choices, opts)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	name := c.pair(c.s).name
	m.cmp = nil
	if c.manual {
		m.marked = map[int64]bool{}
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("merged %d into #%d %s: %d fields, %s", len(drops), keep, name, len(res.Fields), summarize(res)), false)
}

// --- layout and rendering ---

// cmpLayout says which panes show and where they start.
type cmpLayout struct {
	panes  []int // 0 and 1 are the candidates, 2 is the result
	gutter int
	pw     int
	x0     []int
}

// layout picks the panes for a screen width: three from 110 columns, two from 64 (candidates, or
// the result with the r key), one below that (candidate 1, candidate 2, result with the r key).
func (c *compareState) layout(width int) cmpLayout {
	var l cmpLayout
	switch {
	case width >= cmpWide:
		l.panes = []int{0, 1, 2}
	case width >= cmpMid:
		if c.page%2 == 0 {
			l.panes = []int{0, 1}
		} else {
			l.panes = []int{2}
		}
	default:
		l.panes = []int{c.page % 3}
	}
	l.gutter = cmpGutterW
	if width < cmpMid {
		l.gutter = min(cmpGutterW, max(3, width/4))
	}
	k := len(l.panes)
	l.pw = max(1, (width-l.gutter-cmpSepW*k)/k)
	for i := range l.panes {
		l.x0 = append(l.x0, l.gutter+cmpSepW+i*(l.pw+cmpSepW))
	}
	return l
}

func (l cmpLayout) paneAt(x int) int {
	for i, x0 := range l.x0 {
		if x >= x0 && x < x0+l.pw {
			return l.panes[i]
		}
	}
	return -1
}

// cmpLine is the first line of a possibly multi-line value, cleaned for a cell.
func cmpLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return cleanCell(s)
}

// cell renders row i for a pane at exactly w columns.
func (c *compareState) cell(i, pane, w int) string {
	r := &c.rows[i]
	diff := r.differs()
	pad := func(s string) string { return fit(s, w) }
	switch r.kind {
	case rowInfo:
		if pane == 2 {
			return c.st.Head.Render(pad(fmt.Sprintf("%d (all moved)", r.nums[0]+r.nums[1])))
		}
		return c.st.Dim.Render(pad(fmt.Sprint(r.nums[pane])))
	case rowSingle:
		if pane == 2 {
			v := cmpLine(c.finalText(i))
			if v == "" {
				return c.st.Dim.Render(pad("-"))
			}
			if diff {
				return c.st.Name.Render(pad("  " + v))
			}
			return c.st.Dim.Render(pad("  " + v))
		}
		v := cmpLine(r.vals[pane])
		if v == "" {
			return c.st.Dim.Render(pad("  -"))
		}
		chosen := c.resolved(i) == pane
		switch {
		case !diff:
			return c.st.Dim.Render(pad("  " + v))
		case chosen:
			return c.st.Name.Render(pad("✓ " + v))
		}
		return pad("  " + v)
	}
	// list item
	in := c.included(i)
	txt := cmpLine(r.text)
	if pane == 2 {
		switch {
		case !in:
			return ""
		case diff:
			return c.st.Name.Render(pad("  " + txt))
		}
		return c.st.Dim.Render(pad("  " + txt))
	}
	if !r.has[pane] {
		return pad("")
	}
	switch {
	case !in:
		return c.st.Dim.Render(pad("✗ " + txt))
	case diff:
		return c.st.Name.Render(pad("✓ " + txt))
	}
	return c.st.Dim.Render(pad("✓ " + txt))
}

// gutterCell is the marker and label of row i. A bang marks a conflict, a tilde a difference.
func (c *compareState) gutterCell(i, w int) string {
	r := &c.rows[i]
	mark, style := " ", c.st.Dim
	switch {
	case r.kind == rowInfo:
	case c.conflict(i):
		mark, style = "!", c.st.Err
	case r.differs():
		mark, style = "~", c.st.Head
	}
	label := fit(r.label, max(0, w-1))
	if i == c.cur {
		return c.st.Sel.Render(mark + label)
	}
	return style.Render(mark + label)
}

func (c *compareState) paneTitle(pane int) (string, string) {
	if pane == 2 {
		keep := c.pair(c.s)
		return fmt.Sprintf("Final #%d %s", keep.id, cleanCell(keep.name)), "saved to #" + fmt.Sprint(keep.id)
	}
	mb := c.pair(pane)
	if pane == c.s {
		return fmt.Sprintf("#%d %s", mb.id, cleanCell(mb.name)), "survivor, keeps its id"
	}
	return fmt.Sprintf("#%d %s", mb.id, cleanCell(mb.name)), "archived after merge"
}

// render returns exactly h lines for the body and, for each line, the row shown there (or -1).
func (c *compareState) render(width, h int) ([]string, []int) {
	l := c.layout(width)
	sep := c.st.Dim.Render(" │ ")
	blank := strings.Repeat(" ", l.gutter)

	head1, head2 := blank, blank
	for _, p := range l.panes {
		t1, t2 := c.paneTitle(p)
		style := c.st.Head
		if p == c.s || p == 2 {
			style = c.st.Name
		}
		head1 += sep + style.Render(fit(t1, l.pw))
		head2 += sep + c.st.Dim.Render(fit(t2, l.pw))
	}

	// Summary line under the table, then room for the rows. Tiny screens drop the extras.
	showHead2, showSummary := true, true
	room := h - 2 - 1
	if room < 1 {
		showSummary = false
		room = h - 2
	}
	if room < 1 {
		showHead2 = false
		room = h - 1
	}
	if room < 1 {
		room = 1
	}
	if c.cur < c.top {
		c.top = c.cur
	}
	if c.cur >= c.top+room {
		c.top = c.cur - room + 1
	}
	c.top = clampInt(c.top, 0, max(0, len(c.rows)-room))

	lines := []string{head1}
	at := []int{-1}
	if showHead2 {
		lines = append(lines, head2)
		at = append(at, -1)
	}
	for i := c.top; i < len(c.rows) && len(lines) < h-btoi(showSummary); i++ {
		line := c.gutterCell(i, l.gutter)
		for _, p := range l.panes {
			line += sep + c.cell(i, p, l.pw)
		}
		lines = append(lines, line)
		at = append(at, i)
	}
	for len(lines) < h-btoi(showSummary) {
		lines = append(lines, "")
		at = append(at, -1)
	}
	if showSummary {
		switch {
		case c.previewErr != "":
			lines = append(lines, c.st.Err.Render(c.previewErr))
		case c.preview != nil:
			lines = append(lines, c.st.Dim.Render("result: "+summarize(c.preview)))
		default:
			lines = append(lines, "")
		}
		at = append(at, -1)
	}
	return lines[:h], at[:h]
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// confirmLines is the final summary: what is kept, archived and moved. It returns exactly h lines.
func (c *compareState) confirmLines(h int) []string {
	keep, drop := c.pair(c.s), c.pair(1-c.s)
	var out []string
	add := func(s string) { out = append(out, s) }
	label := func(k string) string { return c.st.Dim.Render(fit(k, 10)) }
	add(c.st.Head.Render(fmt.Sprintf("Merge #%d %s into #%d %s", drop.id, cleanCell(drop.name), keep.id, cleanCell(keep.name))))
	add("")
	add(label("kept") + c.st.Name.Render(fmt.Sprintf("#%d %s", keep.id, cleanCell(keep.name))) + c.st.Dim.Render("  keeps its id"))
	add(label("archived") + fmt.Sprintf("#%d %s", drop.id, cleanCell(drop.name)) + c.st.Dim.Render(fmt.Sprintf("  merged into #%d, restorable from Archived", keep.id)))
	add(label("notes") + fmt.Sprintf("%d move to #%d", drop.notes, keep.id))
	add(label("interactions") + fmt.Sprintf("%d move to #%d", drop.interactions, keep.id))
	add(label("tasks") + fmt.Sprintf("%d move to #%d", drop.tasks, keep.id))
	if c.previewErr != "" {
		add(c.st.Err.Render(c.previewErr))
	} else if c.preview != nil {
		res := c.preview
		for _, name := range []string{"memberships", "deals", "relationships"} {
			if n := res.Moved[name]; n > 0 {
				add(label(name) + fmt.Sprintf("%d move to #%d", n, keep.id))
			}
		}
		if n := res.Moved["duplicates_dropped"]; n > 0 {
			add(label("duplicate links") + fmt.Sprintf("%d dropped, only rows identical to the survivor's", n))
		}
		add(label("lists") + summarize(res))
		for _, f := range res.Fields {
			add(label(f.Field) + c.st.Dim.Render(fmt.Sprintf("%v", orDash(f.Old))) + " -> " + c.st.Name.Render(fmt.Sprintf("%v", orDash(f.New))))
		}
	}
	add("")
	add(c.st.Dim.Render("enter merges, esc goes back to the fields"))
	for len(out) < h {
		out = append(out, "")
	}
	return out[:h]
}

func orDash(v any) any {
	if v == nil {
		return "-"
	}
	return v
}

func (m *Model) compareView() string {
	c := m.cmp
	c.st = m.st
	title := "Merge"
	if c.level != 0 {
		title = "Duplicates " + levelBadge(c.level) + " confidence"
	}
	if len(c.reasons) > 0 {
		title += ": " + strings.Join(c.reasons, "; ")
	}
	if len(c.members) > 2 {
		title += fmt.Sprintf("  (group of %d, showing #%d and #%d, p changes the pair)", len(c.members), c.pair(0).id, c.pair(1).id)
	}
	cut := lipgloss.NewStyle().MaxWidth(m.width)
	var lines []string
	if c.confirm {
		lines = c.confirmLines(m.bodyHeight())
	} else {
		lines, _ = c.render(m.width, m.bodyHeight())
	}
	for i := range lines {
		lines[i] = cut.Render(lines[i])
	}
	return cut.Render(c.st.Head.Render(runewidth.Truncate(title, m.width, "…"))) + "\n" + strings.Join(lines, "\n") + "\n" + m.footerView()
}

// --- mouse ---

func (m *Model) handleCompareMouse(x, y int, wheel int) {
	c := m.cmp
	c.st = m.st
	if c.confirm {
		return
	}
	if wheel != 0 {
		c.move(wheel)
		return
	}
	_, rowAt := c.render(m.width, m.bodyHeight())
	line := y - 1
	if line < 0 || line >= len(rowAt) || rowAt[line] < 0 {
		return
	}
	c.cur = rowAt[line]
	pane := c.layout(m.width).paneAt(x)
	if pane < 0 || pane > 1 {
		return
	}
	if r := &c.rows[c.cur]; r.kind == rowItem && r.has[pane] {
		c.setInclude(c.cur, !c.included(c.cur))
	} else {
		c.take(pane)
	}
	m.cmpPreview()
}
