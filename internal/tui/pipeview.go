package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/theme"
)

// Drawing of the pipe line feedback (patterns/autocomplete-component, patterns/show-syntax-inline-as-you-type).
// The prompt sits on the last lines of the screen, so the feedback is drawn over the bottom of the
// body, right above the prompt, each line starting under the segment it speaks about:
//
//	! E_DATE_INVALID due: ...        the error of a segment that was closed with a pipe (dim error style)
//	 fri        Fri Nov 7            the popup, at most six rows, anchored under the segment being typed
//	 next friday -> Fri Nov 7        the soft indicator of the segment being typed
//	[p:Zed Adams] [p:Pia Lane]  [o:Acme]   the chosen related objects as chips grouped by kind, above the rest
//
// The grayscale syntax hint after the cursor (hint.go) is untouched and still draws beside the input.

// stackLine is one line of feedback with the screen column it starts in.
type stackLine struct {
	x    int
	text string
}

// pipeInputColumn is the screen column where the text of the pipe input starts.
func (m *Model) pipeInputColumn(kind string) int {
	if kind == personAddKind {
		return m.listWidth() + 2 + labelWidth
	}
	return ansi.StringWidth(m.input.Prompt)
}

// pipeStack builds the feedback lines for the pipe input that has the focus, top to bottom.
func (m *Model) pipeStack() []stackLine {
	s := m.pipeRefresh()
	if s == nil {
		return nil
	}
	_, in := m.pipeSessionFor()
	text, cursor := in.Value(), in.Position()
	rs := []rune(text)
	rep := m.pipeReportFor(s, text, cursor)
	x0 := m.pipeInputColumn(s.kind)
	col := func(runePos int) int {
		return x0 + ansi.StringWidth(string(rs[:clampInt(runePos, 0, len(rs))]))
	}
	// Where each segment's text starts, skipping the space after its pipe.
	starts := []int{0}
	for i, r := range rs {
		if r == '|' {
			starts = append(starts, i+1)
		}
	}
	segCol := func(i int) int {
		if i >= len(starts) {
			return x0
		}
		p := starts[i]
		for p < len(rs) && rs[p] == ' ' {
			p++
		}
		return col(p)
	}
	var out []stackLine
	for _, ln := range m.chipLines(s, text, cursor, max(10, m.width-x0)) {
		out = append(out, stackLine{x0, ln})
	}
	if rep.hasErr {
		out = append(out, stackLine{segCol(rep.errSeg), m.st.Err.Faint(true).Render("! " + issueText(rep.errIssue))})
	}
	// The popup starts under the name being typed.
	if s.ac.Open() {
		idx, start, end := segBounds(rs, cursor)
		anchor := segCol(idx)
		if idx < len(s.pl.segs) && s.pl.segs[idx].Link {
			ts, _ := tokenBounds(rs, start, end, cursor)
			anchor = col(ts)
		}
		w := popupWidth(s.ac.Rows(), 18, max(18, min(48, m.width-2)))
		if anchor+w > m.width {
			anchor = max(0, m.width-w)
		}
		for _, ln := range s.ac.Lines(w, m.st) {
			out = append(out, stackLine{anchor, ln})
		}
	}
	switch {
	case rep.soft != "":
		out = append(out, stackLine{segCol(rep.cur), m.st.Dim.Render(rep.soft)})
	case rep.suggest != "":
		out = append(out, stackLine{segCol(rep.cur), m.st.Dim.Render(rep.suggest)})
	}
	// A line that would run off the right edge starts further left instead of being cut.
	for i := range out {
		if w := ansi.StringWidth(out[i].text); w <= m.width && out[i].x+w > m.width {
			out[i].x = m.width - w
		}
	}
	return out
}

// overlayStack draws the feedback lines over the bottom of the body.
func (m *Model) overlayStack(body string) string {
	lines := m.pipeStack()
	if len(lines) == 0 {
		return body
	}
	rows := strings.Split(body, "\n")
	if len(lines) > len(rows) {
		lines = lines[len(lines)-len(rows):]
	}
	for i, ln := range lines {
		r := len(rows) - len(lines) + i
		rows[r] = overlayAt(rows[r], ln.x, ln.text, m.width)
	}
	return strings.Join(rows, "\n")
}

// overlayAt writes text over base starting at screen column x, keeping the rest of base on both sides.
func overlayAt(base string, x int, text string, width int) string {
	x = max(0, x)
	room := width - x
	if room <= 0 {
		return base
	}
	text = ansi.Truncate(text, room, "…")
	w := ansi.StringWidth(text)
	left := ansi.Truncate(base, x, "")
	if pad := x - ansi.StringWidth(left); pad > 0 {
		left += strings.Repeat(" ", pad)
	}
	right := ansi.TruncateLeft(base, x+w, "")
	return left + ansi.ResetStyle + text + ansi.ResetStyle + right
}

// chipLines draws the objects chosen in the link segments of the line as chips, grouped by kind in the
// stable order (people, organizations, deals, tasks, notes, tags). Each group is in the color of its kind
// and each chip carries the short kind prefix, so the kinds read without color. The chips wrap onto more
// lines when they do not fit in width. The name still being typed is not chosen yet. The prompt itself
// stays one line: the chips are feedback drawn above it.
func (m *Model) chipLines(s *pipeSession, text string, cursor, width int) []string {
	rs := []rune(text)
	idx, start, end := segBounds(rs, cursor)
	tok := ""
	if idx < len(s.pl.segs) && s.pl.segs[idx].Link {
		ts, te := tokenBounds(rs, start, end, cursor)
		tok = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(rs[ts:te])), "#"))
	}
	chosen, _ := m.chosenLinks(s, text, tok, idx)
	return renderChips(chipGroups(chosen), width, m.st)
}

// renderChips lays the chips out in lines of at most width cells. A chip is never split. Groups are
// separated by two spaces, chips of a group by one.
func renderChips(groups []chipGroup, width int, st theme.Styles) []string {
	var lines []string
	var cur strings.Builder
	curW := 0
	flush := func() {
		if curW > 0 {
			lines = append(lines, cur.String())
			cur.Reset()
			curW = 0
		}
	}
	for gi, g := range groups {
		for ci, name := range g.names {
			chip := "[" + kindPrefix(g.kind) + cleanCell(name) + "]"
			w := ansi.StringWidth(chip)
			if w > width {
				chip = ansi.Truncate(chip, width-1, "…") + "]"
				w = ansi.StringWidth(chip)
			}
			gap := 0
			if curW > 0 {
				gap = 1
				if ci == 0 && gi > 0 {
					gap = 2
				}
			}
			if curW+gap+w > width {
				flush()
				gap = 0
			}
			cur.WriteString(strings.Repeat(" ", gap))
			cur.WriteString(st.KindStyle(g.kind).Render(chip))
			curW += gap + w
		}
	}
	flush()
	return lines
}
