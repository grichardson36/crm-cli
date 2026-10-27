package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
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
}

// Completer is what an input implements to get a popup. Cursor and the returned cursor are rune
// positions in the text.
type Completer interface {
	// Suggest returns the rows for the text, best first. The component shows the first acMaxRows.
	Suggest(text string, cursor int) []Suggestion
	// Accept returns the text and cursor after the user accepted s.
	Accept(text string, cursor int, s Suggestion) (string, int)
}

// acMaxRows is the most rows the popup shows.
const acMaxRows = 6

// Autocomplete is the popup state of one input.
type Autocomplete struct {
	src    Completer
	rows   []Suggestion
	cur    int
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
	if len(a.rows) > acMaxRows {
		a.rows = a.rows[:acMaxRows]
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

// Lines draws the popup, one string per row, each exactly width cells wide. The highlighted row is
// in the selection style, the description is dim.
func (a *Autocomplete) Lines(width int, st theme.Styles) []string {
	if !a.Open() || width < 4 {
		return nil
	}
	var out []string
	for i, r := range a.rows {
		label := runewidth.Truncate(cleanCell(r.Label), max(1, width-2), "…")
		desc := ""
		if d := cleanCell(r.Desc); d != "" {
			desc = runewidth.Truncate("  "+d, max(0, width-2-runewidth.StringWidth(label)), "…")
		}
		line := fit(" "+label+desc, width)
		if i == a.cur {
			out = append(out, st.Sel.Render(line))
			continue
		}
		out = append(out, st.Dim.Render(" ")+strings.TrimRight(label, " ")+st.Dim.Render(desc)+strings.Repeat(" ", max(0, width-1-runewidth.StringWidth(label)-runewidth.StringWidth(desc))))
	}
	return out
}

// popupWidth is the width the popup needs for its rows, between min and max.
func popupWidth(rows []Suggestion, minW, maxW int) int {
	w := minW
	for _, r := range rows {
		n := 2 + runewidth.StringWidth(cleanCell(r.Label))
		if d := cleanCell(r.Desc); d != "" {
			n += 2 + runewidth.StringWidth(d)
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
