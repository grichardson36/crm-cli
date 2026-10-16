package theme

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Styles are the lipgloss styles a theme produces. Every view draws with these and nothing else.
type Styles struct {
	Theme Theme

	Dim    lipgloss.Style // secondary text
	Name   lipgloss.Style // bold accent: names, key hints
	Head   lipgloss.Style // bold headings
	Err    lipgloss.Style
	Ok     lipgloss.Style
	Warn   lipgloss.Style
	Mark   lipgloss.Style // marked count
	TabOn  lipgloss.Style
	TabOff lipgloss.Style
	Sel    lipgloss.Style // selected row
	Off    lipgloss.Style // cursor row of the pane that is not focused
}

// NewStyles builds the styles of t. r decides the color profile: pass nil for the process
// default, which honors NO_COLOR and degrades truecolor to 256 or 16 colors on older terminals.
//
// With no color support (NO_COLOR, --no-color, a dumb terminal) lipgloss would also drop bold and
// reverse video, which would hide the selected row. NO_COLOR only asks for no color, so in that
// case the styles use text attributes alone: reverse for the selection and the active tab, bold
// for names and headings, faint for secondary text.
func NewStyles(t Theme, r *lipgloss.Renderer) Styles {
	var profile termenv.Profile
	if r != nil {
		profile = r.ColorProfile()
	} else {
		profile = lipgloss.ColorProfile()
	}
	if profile == termenv.Ascii {
		return monoStyles(t)
	}
	ns := lipgloss.NewStyle
	if r != nil {
		ns = r.NewStyle
	}
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }

	s := Styles{Theme: t}
	s.Dim = ns().Foreground(c(t.Dim))
	s.Name = ns().Bold(true).Foreground(c(t.Accent))
	s.Head = ns().Bold(true).Foreground(c(t.Fg))
	s.Err = ns().Foreground(c(t.Danger))
	s.Ok = ns().Foreground(c(t.Good))
	s.Warn = ns().Foreground(c(t.Warn))
	s.Mark = ns().Bold(true).Foreground(c(t.Mark))
	s.TabOn = ns().Bold(true).Foreground(c(t.TabFg)).Background(c(t.TabBg)).Padding(0, 1)
	s.TabOff = ns().Foreground(c(t.Dim)).Padding(0, 1)
	s.Off = ns().Underline(true)
	if t.SelFg != "" && t.SelBg != "" {
		s.Sel = ns().Foreground(c(t.SelFg)).Background(c(t.SelBg))
	} else {
		s.Sel = ns().Reverse(true)
	}
	return s
}

// monoStyles uses a private renderer fixed to 16 colors and sets no color on it, so only
// attributes are written.
func monoStyles(t Theme) Styles {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	ns := r.NewStyle
	return Styles{
		Theme:  t,
		Dim:    ns().Faint(true),
		Name:   ns().Bold(true),
		Head:   ns().Bold(true),
		Err:    ns().Bold(true),
		Ok:     ns(),
		Warn:   ns().Bold(true),
		Mark:   ns().Bold(true),
		TabOn:  ns().Bold(true).Reverse(true).Padding(0, 1),
		TabOff: ns().Faint(true).Padding(0, 1),
		Sel:    ns().Reverse(true),
		Off:    ns().Underline(true),
	}
}

// Swatch draws a small block in the given color, for `crm theme show`. Like every style it
// degrades with the terminal and prints as plain blocks without color.
func Swatch(color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("██")
}
