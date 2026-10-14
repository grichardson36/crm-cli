package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeOverlay && m.ov != nil {
		return m.handleOverlayMouse(msg)
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
			x := 0
			for i, name := range viewNames {
				w := len(tabLabel(i, name)) + 2
				if msg.X >= x && msg.X < x+w {
					m.switchView(viewKind(i))
					break
				}
				x += w
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
			if idx >= 0 && idx < len(m.rows) && msg.Y-1 < m.bodyHeight() {
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
