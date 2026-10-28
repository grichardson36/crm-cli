package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/mattn/go-runewidth"
)

// The reusable autocomplete component (owner decision 2026-10-29, patterns/autocomplete-component).
// Any small input in the program gets an incremental search popup by implementing Completer: it says
// which rows fit the text before the cursor and what the text becomes when a row is accepted. The
// component owns the part every input shares: the rows, the highlighted row, the keys (up and down
// move, tab accepts, esc closes), and the drawing of a small popup of at most six rows. Matching is
// not here: sources rank their candidates with internal/suggest, the same ranker the command bar
// completions and the relation typeahead use.
//
// Enter is never taken by the popup. It still submits the prompt.

// Suggestion is one row of the popup.
type Suggestion struct {
	// Label is shown and is the text accepting the row puts into the input.
	Label string
	// Desc is dim text after the label (a company, a stage, "highlighted").
	Desc string
	// Ref is the object the row stands for. It is zero for plain text (a date phrase, a choice).
	Ref repo.Ref
	// Tag is set when the row stands for a tag.
	Tag string
	// Kind is the object kind of the row (tagKind for a tag) in a popup that mixes kinds. It gives the row
	// its color and its short prefix (p:, o:, #). Empty for a popup of one kind.
	Kind string
	// Group is the heading drawn above the row. A completer sets it on the first row of each run of rows.
	Group string
}

// Completer is what an input implements to get a popup. Cursor and the returned cursor are rune
// positions in the text.
type Completer interface {
	// Suggest returns the rows for the text, best first. The component shows the first acMaxRows.
	Suggest(text string, cursor int) []Suggestion
	// Accept returns the text and cursor after the user accepted s.
	Accept(text string, cursor int, s Suggestion) (string, int)
}

// acMaxRows is the most rows the popup shows. A completer that groups its rows may ask for more
// through Autocomplete.limit, and the popup then scrolls inside acMaxLines lines.
const acMaxRows = 6

// acMaxLines is the most lines (rows and headings) the popup draws.
const acMaxLines = 10

// Autocomplete is the popup state of one input.
type Autocomplete struct {
	src    Completer
	rows   []Suggestion
	cur    int
	limit  int    // most rows kept, acMaxRows when zero
	closed bool   // esc closed the popup, it opens again when the text changes
	sig    string // text and cursor of the last refresh
}

// NewAutocomplete returns a popup fed by src.
func NewAutocomplete(src Completer) *Autocomplete { return &Autocomplete{src: src} }

// Refresh recomputes the rows when the text or the cursor moved since the last call. The highlight
// returns to the first row and a closed popup opens again.
func (a *Autocomplete) Refresh(text string, cursor int) {
	sig := fmt.Sprintf("%d:%s", cursor, text)
	if sig == a.sig {
		return
	}
	a.sig = sig
	a.rows = a.src.Suggest(text, cursor)
	lim := acMaxRows
	if a.limit > 0 {
		lim = a.limit
	}
	if len(a.rows) > lim {
		a.rows = a.rows[:lim]
	}
	a.cur, a.closed = 0, false
}

// Open reports whether the popup has rows to show.
func (a *Autocomplete) Open() bool { return a != nil && !a.closed && len(a.rows) > 0 }

// Rows are the rows being shown.
func (a *Autocomplete) Rows() []Suggestion {
	if !a.Open() {
		return nil
	}
	return a.rows
}

// Highlighted is the row tab would accept.
func (a *Autocomplete) Highlighted() (Suggestion, bool) {
	if !a.Open() {
		return Suggestion{}, false
	}
	return a.rows[a.cur], true
}

// Close hides the popup until the text changes.
func (a *Autocomplete) Close() { a.closed = true }

// Key handles a key while the input has the focus. consumed says the popup used it. accepted is the
// row tab chose: the caller puts it into the input with Accept. Only up, down, tab and esc are read,
// and only while the popup is open, so every other key (enter included) reaches the input.
func (a *Autocomplete) Key(key string) (consumed bool, accepted *Suggestion) {
	if !a.Open() {
		return false, nil
	}
	switch key {
	case "up":
		a.cur = (a.cur + len(a.rows) - 1) % len(a.rows)
		return true, nil
	case "down":
		a.cur = (a.cur + 1) % len(a.rows)
		return true, nil
	case "esc":
		a.closed = true
		return true, nil
	case "tab":
		s := a.rows[a.cur]
		return true, &s
	}
	return false, nil
}

// Accept puts the row into the input, and the popup reopens for the new text.
func (a *Autocomplete) Accept(in *textinput.Model, s Suggestion) {
	text, cur := a.src.Accept(in.Value(), in.Position(), s)
	in.SetValue(text)
	in.SetCursor(cur)
	a.sig = "" // the text changed: the next Refresh recomputes
}

// rowLabel is the text of a row in the popup: the short kind prefix, then the label.
func rowLabel(r Suggestion) string { return kindPrefix(r.Kind) + cleanCell(r.Label) }

// Lines draws the popup, one string per row, each exactly width cells wide. The highlighted row is
// in the selection style, the description is dim. Rows that carry a Group are drawn under a dim rule
// heading, built with the grouped-list component (groupRows, grouped.go). A row with a Kind draws its
// prefix and label in the color of the kind. At most acMaxLines lines are drawn, scrolled to keep the
// highlighted row in view.
func (a *Autocomplete) Lines(width int, st theme.Styles) []string {
	if !a.Open() || width < 4 {
		return nil
	}
	type idxRow struct {
		i int
		r Suggestion
	}
	var items []idxRow
	for i, r := range a.rows {
		items = append(items, idxRow{i, r})
	}
	// A heading starts wherever a row carries a Group. The key is the row's position so two runs of the
	// same kind would not merge.
	grouped := groupRows(items, func(it idxRow) []groupHead {
		if it.r.Group == "" {
			return nil
		}
		return []groupHead{{key: fmt.Sprintf("%d", it.i), title: it.r.Group, rule: true}}
	}, func(it idxRow) row { return row{title: it.r.Label} })
	var lines []string
	curLine := 0
	idx := 0
	for _, gr := range grouped.rows {
		if gr.skip {
			lines = append(lines, st.Dim.Render(fit(" "+ruleLine(gr.title, "", width-1), width)))
			continue
		}
		r := a.rows[idx]
		if idx == a.cur {
			curLine = len(lines)
		}
		lines = append(lines, a.line(r, idx == a.cur, width, st))
		idx++
	}
	if len(lines) > acMaxLines {
		start := clampInt(curLine-acMaxLines/2, 0, len(lines)-acMaxLines)
		lines = lines[start : start+acMaxLines]
	}
	return lines
}

// line draws one row.
func (a *Autocomplete) line(r Suggestion, selected bool, width int, st theme.Styles) string {
	label := runewidth.Truncate(rowLabel(r), max(1, width-2), "…")
	desc := ""
	if d := cleanCell(r.Desc); d != "" {
		desc = runewidth.Truncate("  "+d, max(0, width-2-runewidth.StringWidth(label)), "…")
	}
	if selected {
		return st.Sel.Render(fit(" "+label+desc, width))
	}
	body := strings.TrimRight(label, " ")
	if r.Kind != "" {
		body = st.KindStyle(r.Kind).Render(body)
	}
	return st.Dim.Render(" ") + body + st.Dim.Render(desc) + strings.Repeat(" ", max(0, width-1-runewidth.StringWidth(label)-runewidth.StringWidth(desc)))
}

// popupWidth is the width the popup needs for its rows, between min and max.
func popupWidth(rows []Suggestion, minW, maxW int) int {
	w := minW
	for _, r := range rows {
		n := 2 + runewidth.StringWidth(rowLabel(r))
		if d := cleanCell(r.Desc); d != "" {
			n += 2 + runewidth.StringWidth(d)
		}
		if g := runewidth.StringWidth(r.Group) + 4; r.Group != "" && g > n {
			n = g
		}
		if n > w {
			w = n
		}
	}
	if w > maxW {
		w = maxW
	}
	return w
}
