package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// runID runs a registry action by id the way the palette does, for actions that have no key. An
// action that does not apply in the current context does nothing, as in the palette ("not here").
func runID(m *Model, id string) tea.Cmd {
	a := m.actionByID(id)
	if a == nil || !a.enabled(m) || (a.keyIf != nil && !a.keyIf(m)) {
		return nil
	}
	return m.runAction(a)
}

// runIDCmd is runID plus the commands it returned, the way pressRun does for keys.
func runIDCmd(m *Model, id string) {
	runCmds(m, runID(m, id))
}

// typeLinked types a line into the add prompt that is open and then accepts the highlighted person in
// its people segment with tab. It is the way the old grayscale link offer was taken: the text first,
// then the link.
func typeLinked(m *Model, text string) {
	pl := m.pipeFor(m.promptKind)
	kind := "person"
	if m.addSugg != nil && m.addSugg.ref.Kind != "" {
		kind = m.addSugg.ref.Kind
	}
	idx := segFor(pl, kind)
	typeText(m, text)
	if more := idx - strings.Count(text, "|"); more > 0 {
		typeText(m, strings.Repeat("|", more))
	}
	press(m, "tab")
}

// submitLinked is typeLinked followed by enter.
func submitLinked(m *Model, line string) {
	typeLinked(m, line)
	press(m, "enter")
}
