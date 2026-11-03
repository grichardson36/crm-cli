package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	uv "github.com/charmbracelet/ultraviolet"
)

// The popup is the one floating box every overlay uses (patterns/popup-component): a centered box
// with a border over the dimmed screen, a top row that is either a filter line or a plain title, the
// list, and the key hints of the popup in the bottom row inside the box.
//
//	┌──────────────────────────────┐
//	│ > Select a command…          │   filter line (long lists), or the title
//	├──────────────────────────────┤
//	│ Add person                   │   rows
//	│ Add task                     │
//	├──────────────────────────────┤
//	│ enter run | esc close | ...  │   hints, only keys that work on the highlighted row
//	└──────────────────────────────┘
//
// A popup is declared with an overlay (overlay.go): title, items, onPick, and optionally hints,
// filter, selector rows and the size rule. This file only draws the box. Rows are drawn by
// popupListRows from the same overlay, so the box and the mouse use one geometry.

const (
	popupChrome    = 6  // top border, title or filter, rule, rule, hints, bottom border
	popupMaxRows   = 14 // the list scrolls past this
	popupMinW      = 44 // text width limits, the box adds 4 for border and padding
	popupMaxW      = 76
	popupPaletteW  = 96 // the palette and the command line are wider
	popupFilterMin = 7 // a picker with more items than this gets a filter line
)

// popupBox is where a popup sits on the screen. x, y, w and h are the box with its border, textW
// the width of a row, listY the screen line of the first row and rows the number of list lines.
type popupBox struct {
	x, y, w, h int
	textW      int
	listY      int
	rows       int
}

// popupSpec is everything the box draws.
type popupSpec struct {
	title string   // plain top row, used when input is empty
	input string   // the filter line, drawn instead of the title
	rows  []string // list lines, already styled
	empty string   // shown in place of rows when there are none
	hints string   // the hint row, from renderPopupHints
	box   popupBox
}

// popupPage is how many list rows fit on this screen: one blank line above and below the box.
func (m *Model) popupPage() int { return max(1, min(popupMaxRows, m.height-popupChrome-2)) }

// popupTextW clamps a wanted row width to the screen.
func (m *Model) popupTextW(want int) int {
	return max(1, min(want, m.width-4))
}

// popupBoxAt centers a box for a row width and a number of list rows.
func (m *Model) popupBoxAt(textW, rows int) popupBox {
	rows = max(1, rows)
	b := popupBox{textW: textW, rows: rows, w: textW + 4, h: rows + popupChrome}
	b.x = max(0, (m.width-b.w)/2)
	b.y = max(0, (m.height-b.h)/2)
	b.listY = b.y + 3
	return b
}

// renderPopupHints draws hints as "key action | key action", dropping the ones that do not fit.
func (m *Model) renderPopupHints(items []hintItem, width int) string {
	sep := m.st.Dim.Render(" | ")
	var parts []string
	used := 0
	for _, h := range items {
		k := m.keyText(h)
		if k == "" {
			continue
		}
		w := ansi.StringWidth(k) + 1 + ansi.StringWidth(h.label)
		add := w
		if len(parts) > 0 {
			add += 3
		}
		if used+add > width {
			break
		}
		parts = append(parts, m.st.Name.Render(k)+" "+m.st.Dim.Render(h.label))
		used += add
	}
	return strings.Join(parts, sep)
}

// popupLines is the box as lines of exactly box.w cells.
func (m *Model) popupLines(s popupSpec) []string {
	b := s.box
	bd := m.st.Dim
	row := func(content string) string {
		content = ansi.Truncate(content, b.textW, "…")
		if pad := b.textW - ansi.StringWidth(content); pad > 0 {
			content += strings.Repeat(" ", pad)
		}
		return bd.Render("│") + " " + content + " " + bd.Render("│")
	}
	rule := func(l, r string) string { return bd.Render(l + strings.Repeat("─", b.w-2) + r) }
	top := s.input
	if top == "" {
		top = m.st.Head.Render(s.title)
	}
	lines := []string{rule("┌", "┐"), row(top), rule("├", "┤")}
	body := s.rows
	if len(body) == 0 && s.empty != "" {
		body = []string{m.st.Dim.Render(s.empty)}
	}
	for i := 0; i < b.rows; i++ {
		if i < len(body) {
			lines = append(lines, row(body[i]))
		} else {
			lines = append(lines, row(""))
		}
	}
	lines = append(lines, rule("├", "┤"), row(s.hints), rule("└", "┘"))
	return lines
}

// popupOver draws the popup centered over a screen. The screen behind is dimmed. The result has the
// width and height of the terminal.
func (m *Model) popupOver(under string, s popupSpec) string {
	w, h := m.width, m.height
	if n := strings.Count(under, "\n") + 1; n > h {
		h = n
	}
	scr := uv.NewScreenBuffer(w, h)
	uv.NewStyledString(under).Draw(scr, scr.Bounds())
	for y := range scr.Lines {
		for x := range scr.Lines[y] {
			scr.Lines[y][x].Style.Attrs |= uv.AttrFaint
		}
	}
	box := strings.Join(m.popupLines(s), "\n")
	uv.NewStyledString(box).Draw(scr, uv.Rect(s.box.x, s.box.y, s.box.w, s.box.h))
	return scr.Buffer.Render()
}
