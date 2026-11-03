package theme

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// DefaultColors returns the default foreground and background colors of the palette, for the terminal's
// default-color sequences (OSC 10 and 11). Nil when the palette has no bg.
func (t Palette) DefaultColors() (fg, bg color.Color) {
	if t.Bg == "" {
		return nil, nil
	}
	return lipgloss.Color(t.Fg), lipgloss.Color(t.Bg)
}

// Paint gives every cell of a w x h frame the palette background, and the palette foreground to every
// cell that has no foreground, so no cell is left to the terminal's own colors. Styled cells keep
// their own colors. The result is again a styled string of w columns and the same number of lines
// (at least h). A frame is returned unchanged when the palette has no bg or the size is unknown.
func (t Palette) Paint(frame string, w, h int) string {
	if t.Bg == "" || w <= 0 || h <= 0 {
		return frame
	}
	if n := strings.Count(frame, "\n") + 1; n > h {
		h = n
	}
	bg, fg := lipgloss.Color(t.Bg), lipgloss.Color(t.Fg)
	scr := uv.NewScreenBuffer(w, h)
	uv.NewStyledString(frame).Draw(scr, scr.Bounds())
	for y := range scr.Lines {
		for x := range scr.Lines[y] {
			c := &scr.Lines[y][x]
			if c.Width == 0 { // second column of a wide character
				continue
			}
			if c.Style.Bg == nil {
				c.Style.Bg = bg
			}
			if c.Style.Fg == nil {
				c.Style.Fg = fg
			}
		}
	}
	return scr.Buffer.Render()
}
