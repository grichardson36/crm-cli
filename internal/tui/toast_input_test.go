package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rowOf(lines []string, sub string) int {
	for i, l := range lines {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

func TestToastNeverCoversTheBottomInput(t *testing.T) {
	m, _ := newTestModel(t)
	m.startInput("snooze", "snooze days: ", "")
	m.input.Focus()
	typeText(m, "typed42")
	m.showToast("toast one", false)
	lines := strings.Split(plain(m.screen()), "\n")
	in, ts := rowOf(lines, "typed42"), rowOf(lines, "toast one")
	require.GreaterOrEqual(t, in, 0, "typed text stays on the input row")
	assert.Contains(t, lines[in], "snooze days: ")
	require.GreaterOrEqual(t, ts, 0)
	assert.Less(t, ts, in, "the toast is above the input row")

	// a new toast while the prompt is open still shows above it
	m.showToast("second toast", true)
	lines = strings.Split(plain(m.screen()), "\n")
	assert.Less(t, rowOf(lines, "second toast"), rowOf(lines, "typed42"))
	assert.Equal(t, len(lines)-2, rowOf(lines, "typed42"), "the input stays the status row above the hints")
}

func TestToastDoesNotCoverConfirmOrStatusLine(t *testing.T) {
	m, _ := newTestModel(t)
	m.mode = modeConfirm
	m.confirm = &confirmState{prompt: "delete it? (y/n)"}
	m.showToast("hello toast", false)
	lines := strings.Split(plain(m.screen()), "\n")
	require.GreaterOrEqual(t, rowOf(lines, "delete it?"), 0)
	assert.Less(t, rowOf(lines, "hello toast"), rowOf(lines, "delete it?"))

	m.mode = modeNormal
	m.status, m.statusIsErr = "status text", false
	lines = strings.Split(plain(m.screen()), "\n")
	require.GreaterOrEqual(t, rowOf(lines, "status text"), 0)
	assert.Less(t, rowOf(lines, "hello toast"), rowOf(lines, "status text"))
}
