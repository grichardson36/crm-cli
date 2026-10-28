package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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

// View implements tea.Model. The alternate screen and cell-motion mouse reporting are properties of
// the view in Bubble Tea v2, so they are on for the whole session.
func (m *Model) View() tea.View {
	v := tea.NewView(m.screen())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// screen is the current screen as text with the toast on its last line.
func (m *Model) screen() string {
	out := m.baseView()
	if m.toast == nil || m.width == 0 {
		return out
	}
	lines := strings.Split(out, "\n")
	at := len(lines) - 1
	// On the main screen keep the key hints and replace the status line above them.
	if at > 0 && m.mode != modeOverlay && m.mode != modeNames && m.tv == nil && !m.help && m.cmp == nil {
		at--
	}
	if len(lines) < m.height && at == len(lines)-1 {
		lines = append(lines, "") // a short screen: add a line instead of covering one
		at = len(lines) - 1
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
	st := m.st.Ok
	if m.toast.isErr {
		st = m.st.Err
	}
	return st.Bold(true).Reverse(true).Render(text)
}
