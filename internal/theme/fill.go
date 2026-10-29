package theme

import (
	"fmt"
	"math"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// Fill says whether the theme paints its own background over the whole screen. Without it a light
// theme on a dark terminal shows light text boxes on a dark screen. Painting needs the theme's bg
// role, and is skipped without color support (NO_COLOR).
type Fill string

const (
	// FillAuto paints for a light theme (a person who picks a light theme wants a light screen) and
	// for a dark theme only on a light terminal. The auto and anthropocene themes follow the
	// terminal, so they never need it.
	FillAuto Fill = "auto"
	FillOn   Fill = "on"
	FillOff  Fill = "off"
)

// FillKey is the key in config.toml, next to `theme`.
const FillKey = "fill_background"

// ParseFill reads a setting value ("" is auto).
func ParseFill(s string) (Fill, error) {
	switch f := Fill(strings.ToLower(strings.TrimSpace(s))); f {
	case "":
		return FillAuto, nil
	case FillAuto, FillOn, FillOff:
		return f, nil
	}
	return FillAuto, fmt.Errorf("fill_background %q is not valid, use auto, on or off", s)
}

// ChosenFill is the remembered fill choice of this device. A missing or invalid value is auto.
func ChosenFill(dataDir string) Fill {
	f, err := ParseFill(readConfig(dataDir)[FillKey])
	if err != nil {
		return FillAuto
	}
	return f
}

// SaveFill remembers the fill choice for this device. Other settings in the file are kept.
func SaveFill(dataDir string, f Fill) error {
	if _, err := ParseFill(string(f)); err != nil {
		return err
	}
	return saveKey(dataDir, FillKey, string(f))
}

// IsDark reports whether the theme's background is dark. known is false for a theme with neither a
// declared variant nor a bg color.
func (t Theme) IsDark() (dark, known bool) {
	switch t.Variant {
	case "dark":
		return true, true
	case "light":
		return false, true
	}
	if t.Bg != "" {
		if l, ok := relLuminance(t.Bg); ok {
			return l < 0.18, true
		}
	}
	return false, false
}

// Fills reports whether the screen is painted with the theme for this fill choice. terminalDark is
// what the terminal reported about its own background. A theme without bg never paints.
func (t Theme) Fills(mode Fill, terminalDark bool) bool {
	if t.Bg == "" {
		return false
	}
	switch mode {
	case FillOn:
		return true
	case FillOff:
		return false
	}
	if t.Follows() {
		return false
	}
	dark, known := t.IsDark()
	if !known {
		return false
	}
	return !dark || !terminalDark
}

// DefaultColors returns the default foreground and background colors of the theme, for the terminal's
// default-color sequences (OSC 10 and 11). Nil when the theme has no bg.
func (t Theme) DefaultColors() (fg, bg color.Color) {
	if t.Bg == "" {
		return nil, nil
	}
	return lipgloss.Color(t.Fg), lipgloss.Color(t.Bg)
}

// Paint gives every cell of a w x h frame the theme background, and the theme foreground to every
// cell that has no foreground, so no cell is left to the terminal's own colors. Styled cells keep
// their own colors. The result is again a styled string of w columns and the same number of lines
// (at least h). A frame is returned unchanged when the theme has no bg or the size is unknown.
func (t Theme) Paint(frame string, w, h int) string {
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

// relLuminance is the WCAG relative luminance of "#rrggbb" or "#rgb". ANSI numbers are unknown.
func relLuminance(hex string) (float64, bool) {
	if !hexColor.MatchString(hex) {
		return 0, false
	}
	h := hex[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	var ch [3]float64
	for i := range ch {
		var v int
		fmt.Sscanf(h[i*2:i*2+2], "%x", &v)
		f := float64(v) / 255
		if f <= 0.03928 {
			ch[i] = f / 12.92
		} else {
			ch[i] = math.Pow((f+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*ch[0] + 0.7152*ch[1] + 0.0722*ch[2], true
}
