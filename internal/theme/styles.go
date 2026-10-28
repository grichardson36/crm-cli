package theme

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
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

	// Kind holds one style per object kind (person, organization, deal, task, interaction, tag). Use
	// KindStyle to read it.
	Kind map[string]lipgloss.Style
}

// KindStyle is the style of an object kind. A kind without a color is drawn plain. Without color
// support every kind is plain, so the short kind prefix (p:, o:, #) carries the meaning.
func (s Styles) KindStyle(kind string) lipgloss.Style {
	if st, ok := s.Kind[kind]; ok {
		return st
	}
	return lipgloss.NewStyle()
}

// NewStyles builds the styles of t. p is the color profile of the terminal: Bubble Tea v2 reports
// it with a tea.ColorProfileMsg. Styles always carry the exact theme colors and Bubble Tea
// downsamples them to the terminal (256 or 16 colors) when it writes the frame, so the profile only
// decides one thing here: whether colors exist at all.
//
// With no color support (NO_COLOR, --no-color, a dumb terminal) the colors would be dropped and the
// selected row would vanish. NO_COLOR only asks for no color, so in that case the styles use text
// attributes alone: reverse for the selection and the active tab, bold for names and headings,
// faint for secondary text.
func NewStyles(t Theme, p colorprofile.Profile) Styles {
	if p <= colorprofile.Ascii {
		return monoStyles(t)
	}
	ns := lipgloss.NewStyle
	col := lipgloss.Color
	s := Styles{Theme: t}
	s.Dim = ns().Foreground(col(t.Dim))
	s.Name = ns().Bold(true).Foreground(col(t.Accent))
	s.Head = ns().Bold(true).Foreground(col(t.Fg))
	s.Err = ns().Foreground(col(t.Danger))
	s.Ok = ns().Foreground(col(t.Good))
	s.Warn = ns().Foreground(col(t.Warn))
	s.Mark = ns().Bold(true).Foreground(col(t.Mark))
	s.TabOn = ns().Bold(true).Foreground(col(t.TabFg)).Background(col(t.TabBg)).Padding(0, 1)
	s.TabOff = ns().Foreground(col(t.Dim)).Padding(0, 1)
	s.Off = ns().Underline(true)
	s.Kind = map[string]lipgloss.Style{}
	for kind := range KindRoles {
		if k := t.KindColor(kind); k != "" {
			s.Kind[kind] = ns().Foreground(col(k))
		}
	}
	if t.SelFg != "" && t.SelBg != "" {
		s.Sel = ns().Foreground(col(t.SelFg)).Background(col(t.SelBg))
	} else {
		s.Sel = ns().Reverse(true)
	}
	return s
}

// monoStyles sets no color, so only attributes are written.
func monoStyles(t Theme) Styles {
	ns := lipgloss.NewStyle
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
		Kind:   map[string]lipgloss.Style{},
	}
}

// Swatch draws a small block in the given color, for `crm theme show`. It carries the exact color.
// Print it through a colorprofile.Writer so it degrades with the terminal and prints as plain
// blocks without color.
func Swatch(color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("██")
}
