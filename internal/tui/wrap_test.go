package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The options of a choice editor flow onto more lines instead of running off the pane, so the chosen
// one is always visible.
func TestChoiceEditorWrapsItsOptionsAndKeepsTheChosenOneVisible(t *testing.T) {
	m, _ := newTestModel(t)
	spec := model.FieldSpec{Key: "x", Label: "x", Type: model.FieldChoice}
	for _, n := range []string{"Google", "Bing", "DuckDuckGo", "Brave", "Ecosia", "LinkedIn", "YouTube", "Maps"} {
		spec.Choices = append(spec.Choices, model.FieldChoiceOption{Value: n, Label: n})
	}
	m.edit = &editState{typed: &typedEdit{spec: spec, choice: 7}}
	lines := m.typedEditLines("web search", 40)
	require.Greater(t, len(lines), 1, "the options need more than one line at this width")
	for _, l := range lines {
		assert.LessOrEqual(t, ansi.StringWidth(l), 40)
	}
	assert.Contains(t, ansi.Strip(strings.Join(lines, "\n")), "Maps", "the chosen option is shown")
}

func TestLongInfoRowsWrapUnderTheirLabel(t *testing.T) {
	m, _ := newTestModel(t)
	r := infoRow("about", strings.Repeat("a long sentence that does not fit ", 4))
	lines := m.renderRow(0, r, 50)
	require.Greater(t, len(lines), 1)
	for _, l := range lines {
		assert.LessOrEqual(t, ansi.StringWidth(l), 50)
	}
}
