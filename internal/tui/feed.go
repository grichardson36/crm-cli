package tui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// The activity feed of a record page (patterns/notes-feed). Notes and every interaction (email, call,
// meeting, message) of a person, organization or deal are drawn here, one component for every page.
// Entries are note rows (rkNote) of the page form, so the universal verbs apply to them unchanged.
//
//	Recent
//	2026-10-27 note   Note text, wrapped to the width of
//	 Acme Corp        the text column, at most 5 lines ...
//	 Bob Smith
//	2026-08-01 email  Email text
//
// The left column holds the date (once per day), the type and the linked objects in their kind color.
// The right column holds the text wrapped to the pane. An entry longer than feedMaxLines ends with a
// dim "..." and enter expands it in place, enter again collapses it. An expanded entry may be taller
// than the pane: the arrows and pgup/pgdn then scroll inside it before the cursor leaves it.

const (
	feedMaxLines = 5  // text lines of a collapsed entry
	feedDateW    = 10 // 2006-01-02
	feedGap      = 2  // between the left and the right column
	feedMinText  = 20 // narrower than this the entry stacks its text under the heading
	feedTypeMax  = 9  // widest type column
	feedLimit    = 50 // entries a page loads
)

// feedMarker is the dim sign of text that is not shown.
const feedMarker = "..."

// feedLink is an object linked to an entry, shown in the left column.
type feedLink struct {
	kind  string
	label string
}

// feedData is what an entry row carries besides its text.
type feedData struct {
	subject string
	links   []feedLink
}

// feedState is the state of the feed of the open page: the expanded entries and the selected ones.
type feedState struct {
	open  map[int64]bool
	marks map[int64]bool
}

func isFeedRow(r formRow) bool { return r.kind == rkNote && !r.empty }

// whenFeedEntry is true when the detail cursor is on an entry of a feed.
func whenFeedEntry(m *Model) bool {
	if m.focus != focusDetail || m.fcur < 0 || m.fcur >= len(m.form) {
		return false
	}
	return isFeedRow(m.form[m.fcur])
}

// --- wrapping ---

var feedBullet = regexp.MustCompile(`^(\s*(?:[-*+]|\d+[.)])\s+)`)

// cleanFeedText drops what would break the layout: escape sequences, carriage returns and control
// characters, and turns tabs into spaces.
func cleanFeedText(s string) string {
	s = ansi.Strip(s)
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ", " ", "\n", " ", "\n").Replace(s)
	return strings.Map(func(r rune) rune {
		if r == '\n' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, s)
}

// wrapText wraps text to lines of at most w cells. A line break of the text is kept (markdown-ish
// notes are written line by line), a trailing backslash is dropped, blank lines collapse to one and
// the text never starts or ends with a blank line. A list marker gives its continuation lines a
// hanging indent. A word wider than w is cut at w, wide characters are never split.
func wrapText(s string, w int) []string {
	w = max(w, 2)
	var out []string
	blank := false
	for _, raw := range strings.Split(cleanFeedText(s), "\n") {
		line := strings.TrimRight(raw, "  ")
		line = strings.TrimSuffix(line, "\\")
		line = strings.TrimRight(line, " ")
		if strings.TrimSpace(line) == "" {
			blank = len(out) > 0
			continue
		}
		if blank {
			out = append(out, "")
			blank = false
		}
		out = append(out, wrapLine(line, w)...)
	}
	return out
}

// wrapLine wraps one line of text.
func wrapLine(line string, w int) []string {
	lead, hang := "", ""
	if m := feedBullet.FindString(line); m != "" && ansi.StringWidth(m) <= w/2 {
		lead, hang = m, strings.Repeat(" ", ansi.StringWidth(m))
		line = line[len(m):]
	} else {
		trimmed := strings.TrimLeft(line, " ")
		if n := len(line) - len(trimmed); n > 0 && n <= w/2 {
			lead, hang = line[:n], line[:n]
		}
		line = trimmed
	}
	var out []string
	cur, curW := "", 0
	flush := func() {
		pre := hang
		if len(out) == 0 {
			pre = lead
		}
		out = append(out, pre+cur)
		cur, curW = "", 0
	}
	avail := func() int { return w - ansi.StringWidth(hang) }
	for _, word := range strings.Fields(line) {
		ww := ansi.StringWidth(word)
		switch {
		case ww > avail():
			if curW > 0 {
				flush()
			}
			chunks := strings.Split(ansi.Hardwrap(word, avail(), true), "\n")
			for _, c := range chunks[:len(chunks)-1] {
				cur, curW = c, ansi.StringWidth(c)
				flush()
			}
			cur, curW = chunks[len(chunks)-1], ansi.StringWidth(chunks[len(chunks)-1])
		case curW == 0:
			cur, curW = word, ww
		case curW+1+ww <= avail():
			cur += " " + word
			curW += 1 + ww
		default:
			flush()
			cur, curW = word, ww
		}
	}
	if curW > 0 || len(out) == 0 {
		flush()
	}
	return out
}

// --- layout ---

// feedLayout is the columns of an entry in a pane of width w.
type feedLayout struct {
	w, typeW int
	stacked  bool
	textW    int // width of the right column, the whole width less an indent when stacked
}

// gutterW is the width of the left column: the mark column, the date, a space and the type.
func (l feedLayout) gutterW() int { return 1 + feedDateW + 1 + l.typeW }

func (m *Model) feedLayoutFor(w int) feedLayout {
	l := feedLayout{w: w}
	for _, r := range m.form {
		if isFeedRow(r) {
			l.typeW = max(l.typeW, ansi.StringWidth(r.typ))
		}
	}
	l.typeW = min(max(l.typeW, 4), feedTypeMax)
	l.textW = w - l.gutterW() - feedGap
	if l.textW < feedMinText {
		l.stacked = true
		l.textW = max(2, w-2)
	}
	return l
}

// feedTextWidth is the width of the text column on the current pane, for the editor of an entry.
func (m *Model) feedTextWidth() int { return m.feedLayoutFor(m.detail.Width()).textW }

// feedText is the wrapped text of an entry, the subject first. Every line is plain text, bold marks
// the subject lines: the first n lines.
func feedText(r formRow, textW int) (lines []string, subjectLines int) {
	if r.feed != nil && strings.TrimSpace(r.feed.subject) != "" {
		lines = wrapText(r.feed.subject, textW)
		subjectLines = len(lines)
	}
	body := wrapText(r.value, textW)
	lines = append(lines, body...)
	return lines, subjectLines
}

// feedShown says how many text lines of an entry show and whether the marker is needed.
func (m *Model) feedShown(r formRow, total int) (n int, marker bool) {
	if m.feed.open[r.id] || total <= feedMaxLines {
		return total, false
	}
	return feedMaxLines, true
}

// feedTruncated says whether the entry hides text at the current width when collapsed.
func (m *Model) feedTruncated(r formRow) bool {
	lines, _ := feedText(r, m.feedLayoutFor(m.detail.Width()).textW)
	return len(lines) > feedMaxLines
}

// feedToggle is enter on an entry that hides text, or that is expanded: it expands or collapses the
// entry in place. It reports false when the entry shows everything, so enter edits it.
func (m *Model) feedToggle(r formRow) bool {
	if m.feed.open[r.id] {
		delete(m.feed.open, r.id)
		m.syncDetail()
		return true
	}
	if !m.feedTruncated(r) {
		return false
	}
	if m.feed.open == nil {
		m.feed.open = map[int64]bool{}
	}
	m.feed.open[r.id] = true
	m.syncDetail()
	return true
}

// --- drawing ---

// feedLinkText is a linked object as shown in the left column. Without color the kind prefix (p:, o:,
// d:, #) names the kind, the same prefixes as the related segment.
func (m *Model) feedLinkText(l feedLink, width int) string {
	st, colored := m.st.Kind[l.kind]
	label := cleanCell(l.label)
	if !colored {
		label = kindPrefix(l.kind) + label
		return ansi.Truncate(label, max(1, width), "…")
	}
	return st.Render(ansi.Truncate(label, max(1, width), "…"))
}

// feedFirstOfDay is true when the entry opens a date group: the previous row is not an entry of the same day.
func (m *Model) feedFirstOfDay(i int) bool {
	if i == 0 || !isFeedRow(m.form[i-1]) {
		return true
	}
	return m.form[i-1].label != m.form[i].label
}

// renderFeedEntry returns the screen lines of one entry, each at most w cells wide.
func (m *Model) renderFeedEntry(i int, r formRow, w int) []string {
	l := m.feedLayoutFor(w)
	selected := m.focus == focusDetail && i == m.fcur
	cellSel := selected && m.fcol > 0
	editing := m.mode == modeEdit && m.edit != nil && m.edit.row == i && m.edit.typed == nil && m.edit.ta == nil

	rowSel := selected && !cellSel && !editing

	// Right column.
	var text []string
	subjectLines := 0
	hidden := false
	switch {
	case editing:
		text = []string{strings.TrimRight(m.rowInput.View(), " "), m.st.Dim.Render(ansi.Truncate("enter saves, alt+enter new line, esc cancels", max(1, l.textW), "…"))}
	default:
		lines, sl := feedText(r, l.textW)
		subjectLines = sl
		if len(lines) == 0 {
			lines = []string{"(empty)"}
		}
		n, marker := m.feedShown(r, len(lines))
		text, hidden = lines[:n], marker
	}
	styled := make([]string, len(text))
	for k, t := range text {
		switch {
		case editing:
			styled[k] = t
		case k < subjectLines:
			styled[k] = m.st.Head.Render(t)
		default:
			styled[k] = t
		}
	}
	if hidden && !editing {
		last := text[len(text)-1]
		if k := len(text) - 1; k < subjectLines {
			last = ansi.Truncate(last, max(1, l.textW-ansi.StringWidth(feedMarker)-1), "")
			styled[k] = m.st.Head.Render(last) + " " + m.st.Dim.Render(feedMarker)
		} else {
			last = ansi.Truncate(last, max(1, l.textW-ansi.StringWidth(feedMarker)-1), "")
			styled[k] = last + " " + m.st.Dim.Render(feedMarker)
		}
	}

	// Left column: the date and type, then the linked objects.
	date := strings.Repeat(" ", feedDateW)
	if m.feedFirstOfDay(i) {
		date = fit(r.label, feedDateW)
	}
	mark := " "
	if m.feed.marks[r.id] {
		mark = m.st.Mark.Render("*")
	}
	typ := fit(ansi.Truncate(r.typ, l.typeW, "…"), l.typeW)
	typeCell := m.st.Dim.Render(typ)
	if cellSel {
		typeCell = m.st.Sel.Render(typ)
	}
	head := mark + m.st.Head.Render(date) + " " + typeCell
	if !m.feedFirstOfDay(i) {
		head = mark + date + " " + typeCell
	}

	links := m.feedGutterLinks(r, l, len(styled), m.feed.open[r.id] || editing)

	if l.stacked {
		out := []string{head}
		for _, lk := range links {
			out = append(out, " "+lk)
		}
		for _, t := range styled {
			out = append(out, "  "+t)
		}
		return m.feedFinish(out, w, rowSel, selected)
	}

	g := l.gutterW()
	n := max(len(styled), 1+len(links))
	out := make([]string, n)
	for k := 0; k < n; k++ {
		left := ""
		switch {
		case k == 0:
			left = head
		case k-1 < len(links):
			left = " " + links[k-1]
		}
		right := ""
		if k < len(styled) {
			right = styled[k]
		}
		out[k] = left + strings.Repeat(" ", max(0, g-ansi.StringWidth(left))) + strings.Repeat(" ", feedGap) + right
	}
	return m.feedFinish(out, w, rowSel, selected)
}

// feedGutterLinks are the linked objects of the left column, one per line, in the width of the
// column. A collapsed entry shows at most the lines of its text, the rest is counted.
func (m *Model) feedGutterLinks(r formRow, l feedLayout, textLines int, all bool) []string {
	if r.feed == nil || len(r.feed.links) == 0 {
		return nil
	}
	width := l.gutterW() - 2
	if l.stacked {
		width = l.w - 2
	}
	links := r.feed.links
	room := max(textLines, feedMaxLines) - 1
	if l.stacked {
		room = feedMaxLines - 1
	}
	if all || len(links) <= room {
		out := make([]string, len(links))
		for k, lk := range links {
			out[k] = m.feedLinkText(lk, width)
		}
		return out
	}
	out := make([]string, 0, room)
	for _, lk := range links[:room-1] {
		out = append(out, m.feedLinkText(lk, width))
	}
	out = append(out, m.st.Dim.Render(fmt.Sprintf("+%d more", len(links)-(room-1))))
	return out
}

// feedFinish clips every line to w and paints the selected entry.
func (m *Model) feedFinish(lines []string, w int, rowSel, selected bool) []string {
	for k, ln := range lines {
		if rowSel {
			// the selection style replaces inner styles so the block reads as one highlight
			lines[k] = m.st.Sel.Render(fit(ansi.Strip(ln), w))
			continue
		}
		lines[k] = lipgloss.NewStyle().MaxWidth(w).Render(ln)
	}
	return lines
}

// --- the entries of a page ---

// timelineRef is the object whose page the filter is for.
func timelineRef(f model.InteractionFilters) repo.Ref {
	switch {
	case f.PersonID != nil:
		return repo.Ref{Kind: "person", ID: *f.PersonID}
	case f.OrgID != nil:
		return repo.Ref{Kind: "organization", ID: *f.OrgID}
	case f.DealID != nil:
		return repo.Ref{Kind: "deal", ID: *f.DealID}
	}
	return repo.Ref{}
}

// feedLinksOf lists the objects an interaction is linked to, in the stable order of kinds, the page's
// own object left out because every entry of the page is linked to it.
func (m *Model) feedLinksOf(it *model.Interaction, own repo.Ref, names map[repo.Ref]string) []feedLink {
	var out []feedLink
	add := func(kind string, ids []int64) {
		for _, id := range ids {
			ref := repo.Ref{Kind: kind, ID: id}
			if ref == own {
				continue
			}
			name, ok := names[ref]
			if !ok {
				name = m.refName(ref)
				names[ref] = name
			}
			out = append(out, feedLink{kind: kind, label: name})
		}
	}
	add("person", it.PersonIDs)
	add("organization", it.OrgIDs)
	add("deal", it.DealIDs)
	if tags, err := m.tagr.GetForEntity(m.ctx, model.NoteKind, it.ID); err == nil {
		for _, t := range tags {
			out = append(out, feedLink{kind: tagKind, label: t.Name})
		}
	}
	return out
}

// pruneFeed drops the expanded and selected entries that are not on the page any more.
func (m *Model) pruneFeed() {
	live := map[int64]bool{}
	for _, r := range m.form {
		if isFeedRow(r) {
			live[r.id] = true
		}
	}
	for id := range m.feed.marks {
		if !live[id] {
			delete(m.feed.marks, id)
		}
	}
	for id := range m.feed.open {
		if !live[id] {
			delete(m.feed.open, id)
		}
	}
}

// --- selection ---

// toggleFeedMark is space on an entry: select or unselect it and move down.
func (m *Model) toggleFeedMark() {
	if !whenFeedEntry(m) {
		return
	}
	id := m.form[m.fcur].id
	if m.feed.marks == nil {
		m.feed.marks = map[int64]bool{}
	}
	if m.feed.marks[id] {
		delete(m.feed.marks, id)
	} else {
		m.feed.marks[id] = true
	}
	m.step(1)
	m.syncDetail()
}

// feedMarkedRefs are the selected entries in page order.
func (m *Model) feedMarkedRefs() []repo.Ref {
	var out []repo.Ref
	for _, r := range m.form {
		if isFeedRow(r) && m.feed.marks[r.id] {
			out = append(out, repo.Ref{Kind: model.NoteKind, ID: r.id})
		}
	}
	return out
}

// --- moving ---

// feedJump is ctrl+up and ctrl+down on an entry: the previous or next entry. It reports false when
// the cursor is not on an entry or there is none in that direction, so the page jump applies.
func (m *Model) feedJump(dir int) bool {
	if !whenFeedEntry(m) {
		return false
	}
	for i := m.fcur + dir; i >= 0 && i < len(m.form); i += dir {
		if isFeedRow(m.form[i]) {
			m.fcur, m.fcol = i, 0
			m.syncDetail()
			return true
		}
	}
	return false
}

// feedScroll moves inside a row taller than the pane before the cursor leaves it. It reports whether
// it consumed the step.
func (m *Model) feedScroll(delta int) bool {
	if m.fcur < 0 || m.fcur >= len(m.rowStart) || delta == 0 {
		return false
	}
	top, bottom := m.rowStart[m.fcur], m.rowEnd[m.fcur]
	h := m.detail.Height()
	if bottom-top+1 <= h {
		return false
	}
	y := m.detail.YOffset()
	limit := bottom - h + 1
	switch {
	case delta > 0 && y < limit:
		m.detail.SetYOffset(min(y+delta, limit))
		return true
	case delta < 0 && y > top:
		m.detail.SetYOffset(max(y+delta, top))
		return true
	}
	return false
}
