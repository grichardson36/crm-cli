package tui

import tea "github.com/charmbracelet/bubbletea"

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
