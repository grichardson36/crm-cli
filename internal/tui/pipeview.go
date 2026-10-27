package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// Drawing of the pipe line feedback (patterns/autocomplete-component, patterns/show-syntax-inline-as-you-type).
// The prompt sits on the last lines of the screen, so the feedback is drawn over the bottom of the
// body, right above the prompt, each line starting under the segment it speaks about:
//
//	! E_DATE_INVALID due: ...        the error of a segment that was closed with a pipe (dim error style)
//	 fri        Fri Nov 7            the popup, at most six rows, anchored under the segment being typed
//	 next friday -> Fri Nov 7        the soft indicator of the segment being typed
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
	return runewidth.StringWidth(m.input.Prompt)
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
		return x0 + runewidth.StringWidth(string(rs[:clampInt(runePos, 0, len(rs))]))
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
