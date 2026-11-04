package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlaceHints(t *testing.T) {
	tests := []struct {
		name              string
		widths            []int
		lead, width, rows int
		want              []hintPlace
	}{
		{"fits one row", []int{5, 5}, 0, 20, 2, []hintPlace{{0, 0, 5}, {0, 7, 12}}},
		{"wraps to a second row", []int{8, 8, 8}, 0, 20, 2, []hintPlace{{0, 0, 8}, {0, 10, 18}, {1, 0, 8}}},
		{"cuts what does not fit the last row", []int{8, 8, 8, 8}, 0, 10, 2, []hintPlace{{0, 0, 8}, {1, 0, 8}}},
		{"a hint is never split", []int{12, 12}, 0, 20, 2, []hintPlace{{0, 0, 12}, {1, 0, 12}}},
		{"lead takes room on the first row only", []int{8, 8}, 12, 20, 2, []hintPlace{{0, 12, 20}, {1, 0, 8}}},
		{"one row only", []int{8, 8}, 0, 10, 1, []hintPlace{{0, 0, 8}}},
		{"a hint wider than the width stops layout", []int{30, 5}, 0, 20, 2, nil},
		{"no hints", nil, 0, 20, 2, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, placeHints(tt.widths, tt.lead, tt.width, tt.rows))
		})
	}
}

// footerRowsOf returns the hint rows of a rendered screen: everything under the status line.
func screenLines(m *Model) []string {
	return strings.Split(ansi.Strip(m.screen()), "\n")
}

func TestMainFooterWrapsToASecondRow(t *testing.T) {
	m := newDefaultModel(t)
	m.switchView(viewPeople)
	m.width = 80
	lines := screenLines(m)
	require.Len(t, lines, m.height, "the screen keeps the terminal height")
	hints := strings.Join(lines[len(lines)-2:], " ")
	assert.Contains(t, hints, "ctrl+o options", "the hint that did not fit on one row shows on the second")
	assert.Contains(t, hints, "? help")
	assert.NotEmpty(t, strings.TrimSpace(lines[len(lines)-2]), "the first hint row is full")
	assert.NotEmpty(t, strings.TrimSpace(lines[len(lines)-1]), "and so is the second")
}

func TestWrappedFooterShortensTheBodyByOneRow(t *testing.T) {
	m := newDefaultModel(t)
	m.switchView(viewPeople)
	m.width = 200
	one := m.bodyHeight()
	m.width = 80
	assert.Equal(t, one-1, m.bodyHeight())
}

func TestFooterIsTwoHintRowsAtMost(t *testing.T) {
	m := newDefaultModel(t)
	m.switchView(viewPeople)
	m.width = 30
	lines := screenLines(m)
	require.Len(t, lines, m.height)
	assert.Equal(t, m.height-4, m.bodyHeight(), "tabs, status and two hint rows around the body")
}

func TestClickOnASecondRowHintRunsItsAction(t *testing.T) {
	m := newDefaultModel(t)
	m.switchView(viewPeople)
	m.width = 80
	var second *footSeg
	for _, s := range m.footerSegs() {
		if s.id == "options" {
			s := s
			second = &s
		}
	}
	require.NotNil(t, second, "the options hint is in the footer")
	require.Equal(t, 1, second.row, "and it wrapped to the second row")
	click(m, second.x0+1, m.height-1)
	assert.Equal(t, modeOverlay, m.mode, "ctrl+o options opened")
}

func TestModeFootersWrapAndKeepTheScreenHeight(t *testing.T) {
	tests := []struct {
		name string
		open func(m *Model)
	}{
		{"help page", func(m *Model) { m.openHelp() }},
		{"text page", func(m *Model) { m.openText("Title", "body", nil) }},
		{"note prompt", func(m *Model) { m.runAction(m.actionByID("note")) }},
		{"names panel", func(m *Model) { m.names = &namesPanel{refs: nil}; m.mode = modeNames }},
		{"view editor", func(m *Model) { m.sve = &svEditor{name: "x", kind: "person", labels: map[string]string{}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newDefaultModel(t)
			m.switchView(viewPeople)
			m.width = 22
			tt.open(m)
			lines := screenLines(m)
			assert.LessOrEqual(t, len(lines), m.height, "the screen fits the terminal")
			assert.LessOrEqual(t, ansi.StringWidth(lines[len(lines)-1]), m.width)
		})
	}
}

func TestShortTerminalCutsTheHintsInsteadOfWrapping(t *testing.T) {
	m := newDefaultModel(t)
	m.switchView(viewPeople)
	m.width = 20
	for _, h := range []int{6, 7, 8} {
		m.height = h
		lines := screenLines(m)
		assert.LessOrEqual(t, len(lines), h, "height %d", h)
		assert.GreaterOrEqual(t, m.bodyHeight(), 3, "height %d keeps a body", h)
	}
}

func TestHelpPageHintsWrapUnderTheBody(t *testing.T) {
	m := newDefaultModel(t)
	m.width = 22
	m.openHelp()
	lines := screenLines(m)
	require.Len(t, lines, m.height, "the page keeps the terminal height")
	assert.Contains(t, lines[len(lines)-2], "any key close")
	assert.Contains(t, lines[len(lines)-1], "↑/↓ scroll")
}

func TestDetailPaneFollowsTheWrappedFooter(t *testing.T) {
	m := newDefaultModel(t)
	m.switchView(viewPeople)
	m.width = 200
	m.screen()
	assert.Equal(t, m.bodyHeight(), m.detail.Height())
	m.width = 80
	m.screen()
	assert.Equal(t, m.bodyHeight(), m.detail.Height(), "the detail pane gives up the row the footer took")
}
