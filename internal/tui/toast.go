package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// A toast is a one-line message on the bottom line of the screen that goes away by itself. It is
// drawn over whatever screen is showing (lists, palette, help, text pages), works in read-only
// mode and is cut to the terminal width. showToast returns the command that clears it.

const toastTime = 4 * time.Second

type toast struct {
	text  string
	isErr bool
	seq   int
}

type toastExpireMsg struct{ seq int }

// showToast shows text for a few seconds. Return the result from an Update path so the timer runs.
func (m *Model) showToast(text string, isErr bool) tea.Cmd {
	m.toastSeq++
	seq := m.toastSeq
	m.toast = &toast{text: strings.Join(strings.Fields(text), " "), isErr: isErr, seq: seq}
	return tea.Tick(toastTime, func(time.Time) tea.Msg { return toastExpireMsg{seq: seq} })
}

// View implements tea.Model: the current screen with the toast on its last line.
func (m *Model) View() string {
	out := m.baseView()
	if m.toast == nil || m.width == 0 {
		return out
	}
	lines := strings.Split(out, "\n")
	at := len(lines) - 1
	// On the main screen keep the key hints and replace the status line above them.
	if at > 0 && m.mode != modeOverlay && m.tv == nil && !m.help && m.cmp == nil {
		at--
	}
	lines[at] = m.toastLine()
	return strings.Join(lines, "\n")
}

func (m *Model) toastLine() string {
	text := " " + m.toast.text + " "
	text = runewidth.Truncate(text, m.width, "…")
	if pad := m.width - runewidth.StringWidth(text); pad > 0 {
		text += strings.Repeat(" ", pad)
	}
	bg := lipgloss.Color("#1E7E34")
	if m.toast.isErr {
		bg = lipgloss.Color("#C0392B")
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(bg).Render(text)
}
