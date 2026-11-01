package theme

import "charm.land/lipgloss/v2"

// ForegroundStyle is a style with the given color as its foreground. The terminal UI uses it for a
// kind color the user chose (patterns/kind-colors-are-chosen), so color literals stay in this package.
func ForegroundStyle(color string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}
