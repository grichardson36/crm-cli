package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeOverlay && m.ov != nil {
		return m.handleOverlayMouse(msg)
	}
	if m.tv != nil {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.tv.vp.LineUp(3)
		case tea.MouseButtonWheelDown:
			m.tv.vp.LineDown(3)
		}
		return m, nil
	}
	if m.help {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.helpVP.LineUp(3)
		case tea.MouseButtonWheelDown:
			m.helpVP.LineDown(3)
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
	case tea.MouseButtonWheelUp:
		if inList {
			m.move(-3)
		} else {
			m.detail.LineUp(3)
		}
	case tea.MouseButtonWheelDown:
		if inList {
			m.move(3)
		} else {
			m.detail.LineDown(3)
		}
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
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
			line := msg.Y - 1 + m.detail.YOffset
			if line < 0 || line >= len(m.lineRow) || m.lineRow[line] < 0 || msg.Y-1 >= m.bodyHeight() {
				return m, nil
			}
			idx := m.lineRow[line]
			again := m.focus == focusDetail && m.fcur == idx
			m.focus = focusDetail
			m.fcur = idx
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
func (m *Model) compareMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.handleCompareMouse(msg.X, msg.Y, -1)
	case tea.MouseButtonWheelDown:
		m.handleCompareMouse(msg.X, msg.Y, 1)
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
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
