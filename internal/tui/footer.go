package tui

import "strings"

// The footer component (CRM-016). Every screen and mode shows its key hints on the last rows of the
// screen. The hints wrap to a second row when they do not fit the width, up to footerMaxRows, and the
// rest are cut. Every screen asks this file for the hint rows and subtracts their count from its body.

// footerMaxRows is the most hint rows a footer takes.
const footerMaxRows = 2

// footerMax is the most hint rows this screen height allows: the tab bar, the status line and a body of
// three rows stay, so a very short terminal cuts the hints instead of wrapping them.
func (m *Model) footerMax() int { return max(1, min(footerMaxRows, m.height-5)) }

// footerRows is the number of hint rows the footer takes now, one before the model is built.
func (m *Model) footerRows() int {
	if m.hintRowsFn == nil {
		return 1
	}
	return m.hintRowsFn()
}

// pageRows is the number of rows a page that takes over the screen has for its title and body: the
// screen minus the hint rows of the page and one more row (the status line, or for a page without one the title).
func (m *Model) pageRows(hints []hintItem) int { return m.height - 1 - m.hintRowsOf(hints) }

// footerHintRows is the number of hint rows under the status line of the main screen and the modes on it.
func (m *Model) footerHintRows() int { return hintRows(m.hintsLine()) }

// hintPlace is where one hint lands in the footer: its row and its columns [x0, x1).
type hintPlace struct{ row, x0, x1 int }

// placeHints lays out hints of the given widths, in order, two spaces apart. lead is the width already
// used on the first row. A hint is never split: one that does not fit the row moves to the next, and
// layout stops at the first hint that does not fit the last row.
func placeHints(widths []int, lead, width, maxRows int) []hintPlace {
	var out []hintPlace
	row, x := 0, lead
	for _, w := range widths {
		if x > 0 && x+w > width {
			row++
			x = 0
		}
		if row >= maxRows || x+w > width {
			break
		}
		out = append(out, hintPlace{row, x, x + w})
		x += w + 2
	}
	return out
}

// joinHintRows draws rendered hints (parts, as wide as widths) on their places, a row per line. lead is
// the text that opens the first row.
func joinHintRows(parts []string, places []hintPlace, lead string) string {
	rows := make([][]string, 1)
	if lead != "" {
		rows[0] = append(rows[0], lead)
	}
	for i, p := range places {
		for len(rows) <= p.row {
			rows = append(rows, nil)
		}
		rows[p.row] = append(rows[p.row], parts[i])
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = strings.Join(r, "  ")
	}
	return strings.Join(lines, "\n")
}

// hintRows is the number of rows of a rendered hints block, at least one.
func hintRows(block string) int { return strings.Count(block, "\n") + 1 }
