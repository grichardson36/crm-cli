package tui

import (
	tea "charm.land/bubbletea/v2"
)

// mouseEvent is the part of a Bubble Tea v2 mouse message the screens use: a left click or one wheel
// step at a cell. Bubble Tea v1 sent one message type with an action, v2 sends a click, a wheel
// step, a release and a motion message. Only clicks and wheel steps reach the handlers, as before.
type mouseEvent struct {
	X, Y   int
	Button tea.MouseButton
}

func (m *Model) handleMouse(msg mouseEvent) (tea.Model, tea.Cmd) {
	if m.mode == modeOverlay && m.ov != nil {
		return m.handleOverlayMouse(msg)
	}
	if m.tv != nil {
		switch msg.Button {
		case tea.MouseWheelUp:
			m.tv.vp.ScrollUp(3)
		case tea.MouseWheelDown:
			m.tv.vp.ScrollDown(3)
		}
		return m, nil
	}
	if m.help {
		switch msg.Button {
		case tea.MouseWheelUp:
			m.helpVP.ScrollUp(3)
		case tea.MouseWheelDown:
			m.helpVP.ScrollDown(3)
		}
		return m, nil
	}
	if m.mode != modeNormal {
		return m, nil
	}
	if m.cmp != nil {
		return m.compareMouse(msg)
	}
	listW := m.listWidth()
	inList := msg.X < listW
	switch msg.Button {
	case tea.MouseWheelUp:
		if inList {
			m.move(-3)
		} else {
			m.detail.ScrollUp(3)
		}
	case tea.MouseWheelDown:
		if inList {
			m.move(3)
		} else {
			m.detail.ScrollDown(3)
		}
	case tea.MouseLeft:
		footerY := 1 + m.bodyHeight()
		switch {
		case msg.Y == 0:
			// Click on the tab bar.
			for _, seg := range m.tabLayout() {
				if msg.X >= seg.x0 && msg.X < seg.x1 {
					if seg.more {
						return m, m.openViewSwitcher()
					}
					m.switchView(seg.view)
					break
				}
			}
		case msg.Y == footerY:
			// Tapping the status line opens the palette.
			if a := m.actionByID("palette"); a != nil {
				return m, m.runAction(a)
			}
		case msg.Y == footerY+1:
			for _, s := range m.footerSegs() {
				if msg.X >= s.x0 && msg.X < s.x1 {
					if a := m.actionByID(s.id); a != nil && a.enabled(m) {
						return m, m.runAction(a)
					}
				}
			}
		case inList:
			idx := m.offset + msg.Y - 1
			if idx >= 0 && idx < len(m.rows) && msg.Y-1 < m.bodyHeight() && !m.rows[idx].skip {
				m.focus = focusList
				m.cursor = idx
				m.clamp()
				m.refreshDetail(false)
				m.syncDetail()
			}
		case msg.X > listW:
			line := msg.Y - 1 + m.detail.YOffset()
			if line < 0 || line >= len(m.lineRow) || m.lineRow[line] < 0 || msg.Y-1 >= m.bodyHeight() {
				return m, nil
			}
			idx := m.lineRow[line]
			again := m.focus == focusDetail && m.fcur == idx
			m.focus = focusDetail
			m.fcur = idx
			m.fcol = 0
			m.syncDetail()
			if again {
				// A second click on the selected row edits it.
				if a := m.actionByID("edit"); a != nil && a.enabled(m) {
					return m, m.runAction(a)
				}
			}
		}
	}
	return m, nil
}

// compareMouse handles the mouse on the compare screen: wheel moves, a click picks the clicked
// person's value for the row, and the footer works as usual.
func (m *Model) compareMouse(msg mouseEvent) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseWheelUp:
		m.handleCompareMouse(msg.X, msg.Y, -1)
	case tea.MouseWheelDown:
		m.handleCompareMouse(msg.X, msg.Y, 1)
	case tea.MouseLeft:
		footerY := 1 + m.bodyHeight()
		switch {
		case msg.Y == footerY:
			if a := m.actionByID("palette"); a != nil {
				return m, m.runAction(a)
			}
		case msg.Y == footerY+1:
			for _, s := range m.footerSegs() {
				if msg.X >= s.x0 && msg.X < s.x1 {
					if a := m.actionByID(s.id); a != nil && a.enabled(m) {
						return m, m.runAction(a)
					}
				}
			}
		default:
			m.handleCompareMouse(msg.X, msg.Y, 0)
		}
	}
	return m, nil
}

// handlePaste puts pasted text where typing would go. Bubble Tea v2 sends bracketed paste as its
// own message, v1 sent it as a key message with the Paste flag. The footer prompt, the inline cell
// editors and the overlay filter are text inputs, so the paste goes to the one that has focus.
func (m *Model) handlePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case m.mode == modeOverlay && m.ov != nil:
		if m.ov.cmd != nil {
			return m, nil
		}
		m.ov.filter, cmd = m.ov.filter.Update(msg)
		m.ov.refilter()
	case m.mode == modePrompt:
		m.input, cmd = m.input.Update(msg)
		m.pipeRefresh()
	case m.mode == modeEdit && m.rowInput.Focused():
		m.rowInput, cmd = m.rowInput.Update(msg)
	}
	return m, cmd
}
