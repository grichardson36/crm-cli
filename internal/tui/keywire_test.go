package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeWire runs bytes through the same decoder Bubble Tea v2 reads the terminal with and returns
// the key names the model would see. It shows what arrives for the sequences a terminal sends:
// xterm style sequences (Windows Terminal, ssh, tmux) and the Windows win32 input mode records.
func decodeWire(t *testing.T, raw string) []string {
	t.Helper()
	var d uv.EventDecoder
	var names []string
	buf := []byte(raw)
	for len(buf) > 0 {
		n, ev := d.Decode(buf)
		require.Greater(t, n, 0, "undecodable input %q", raw)
		buf = buf[n:]
		if kp, ok := ev.(uv.KeyPressEvent); ok {
			names = append(names, keyName(tea.KeyPressMsg(kp)))
		}
	}
	return names
}

func TestKeyNamesFromTerminalSequences(t *testing.T) {
	cases := map[string]string{
		"\x1b[1;5A":  "ctrl+up",
		"\x1b[1;5B":  "ctrl+down",
		"\x1b[1;5C":  "ctrl+right",
		"\x1b[1;5D":  "ctrl+left",
		"\x1b[1;6A":  "ctrl+shift+up",
		"\x1b[1;6B":  "ctrl+shift+down",
		"\x1b[1;2A":  "shift+up",
		"\x1b[1;2B":  "shift+down",
		"\x1b[Z":     "shift+tab",
		"\x1b\r":     "alt+enter",
		"\x1b[6~":    "pgdown",
		"\x1b[5~":    "pgup",
		"\x1b[H":     "home",
		"\x1b[F":     "end",
		"\x1b[3~":    "delete",
		"\x1bd":      "alt+d",
		"\x06":       "ctrl+f",
		"\r":         "enter",
		"\t":         "tab",
		"\x1b":       "esc",
		" ":          " ",
		"A":          "A",
		"\x7f":       "backspace",
		"\x1b[13;2u": "shift+enter",
		"\x1b[13;3u": "alt+enter",
	}
	for raw, want := range cases {
		assert.Equal(t, []string{want}, decodeWire(t, raw), "%q", raw)
	}
}

// Windows consoles send key records. Bubble Tea turns them into the same key presses.
func TestKeyNamesFromWindowsKeyRecords(t *testing.T) {
	const (
		leftCtrl = 0x8
		shift    = 0x10
		leftAlt  = 0x2
		enhanced = 0x100
	)
	rec := func(vk, sc, uc, cs int) string {
		return "\x1b[" + itoa(vk) + ";" + itoa(sc) + ";" + itoa(uc) + ";1;" + itoa(cs) + ";1_"
	}
	cases := []struct {
		raw  string
		want string
	}{
		{rec(0x26, 72, 0, leftCtrl|shift|enhanced), "ctrl+shift+up"},
		{rec(0x28, 80, 0, leftCtrl|shift|enhanced), "ctrl+shift+down"},
		{rec(0x26, 72, 0, leftCtrl|enhanced), "ctrl+up"},
		{rec(0x27, 77, 0, leftCtrl|enhanced), "ctrl+right"},
		{rec(0x25, 75, 0, leftCtrl|enhanced), "ctrl+left"},
		{rec(0x0D, 28, 13, leftAlt), "alt+enter"},
		{rec(0x09, 15, 9, shift), "shift+tab"},
		{rec(0x22, 81, 0, enhanced), "pgdown"},
		{rec(0x24, 71, 0, enhanced), "home"},
		{rec(0x20, 57, 32, 0), " "},
		{rec(0x41, 30, 'a', 0), "a"},
	}
	for _, c := range cases {
		assert.Equal(t, []string{c.want}, decodeWire(t, c.raw), "%q", c.raw)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func TestPasteStartAndMouseFromTerminalSequences(t *testing.T) {
	var d uv.EventDecoder
	// Bubble Tea assembles tea.PasteMsg from the decoder's paste start, text and paste end events.
	_, ev := d.Decode([]byte("\x1b[200~"))
	_, ok := ev.(uv.PasteStartEvent)
	require.True(t, ok, "%T", ev)

	_, ev = d.Decode([]byte("\x1b[<0;10;5M"))
	click, ok := ev.(uv.MouseClickEvent)
	require.True(t, ok, "%T", ev)
	assert.Equal(t, 9, click.X)
	assert.Equal(t, 4, click.Y)
	assert.Equal(t, tea.MouseLeft, tea.MouseButton(click.Button))

	_, ev = d.Decode([]byte("\x1b[<64;10;5M"))
	wheel, ok := ev.(uv.MouseWheelEvent)
	require.True(t, ok, "%T", ev)
	assert.Equal(t, tea.MouseWheelUp, tea.MouseButton(wheel.Button))

	_, ev = d.Decode([]byte("\x1b[<0;10;5m"))
	_, ok = ev.(uv.MouseReleaseEvent)
	assert.True(t, ok, "a release is its own event: %T", ev)
}
