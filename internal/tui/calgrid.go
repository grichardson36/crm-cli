package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/mattn/go-runewidth"
)

// calCell is a day's rectangle on screen, relative to the top of the body.
type calCell struct {
	x0, x1, y0, y1 int
	date           time.Time
}

var weekdayHeads = [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// gridText is how an item reads inside a grid cell.
func gridText(it *calItem) string {
	switch it.kind {
	case "event":
		if it.clock != "" {
			return it.clock + " " + it.title
		}
		return "- " + it.title
	case "birthday":
		return "* " + it.title
	case "due":
		return "~ " + it.title
	case "task":
		return "[ ] " + it.title
	case "empty":
		return ""
	}
	return it.title
}

// calGridBody draws the Week or Month grid into w columns and h lines.
func (m *Model) calGridBody(w, h int) string {
	m.cal.cells = m.cal.cells[:0]
	var lines []string
	if m.calEff() == calWeek {
		lines = m.calWeekLines(w, h)
	} else {
		lines = m.calMonthLines(w, h)
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	cut := lipgloss.NewStyle().MaxWidth(w)
	for i := range lines {
		lines[i] = cut.Render(lines[i])
	}
	return strings.Join(lines[:h], "\n")
}

func (m *Model) calColWidth(w int) int { return max(6, (w-6)/7) }

// calStrip is the one-line summary of the selected item under the grid.
func (m *Model) calStrip(w int) string {
	it := m.calSelected()
	var s string
	switch {
	case it == nil:
		s = ""
	case it.kind == "event":
		s = m.calWhen(it.inst) + "  " + it.title
		if it.inst.Location != "" {
			s += "  @ " + cleanCell(it.inst.Location)
		}
	case it.kind == "empty" || it.kind == "day":
		s = it.date.Format("Monday 2 January") + ": nothing planned"
	default:
		s = it.date.Format("Mon 2 Jan") + "  " + it.title + "  " + it.sub
	}
	return dimStyle.Render(runewidth.Truncate(s, w, "…"))
}

func (m *Model) dayStyle(d time.Time, sel, today bool, dimmed bool) lipgloss.Style {
	switch {
	case sel:
		return tabOn
	case today:
		return nameStyle
	case dimmed:
		return dimStyle
	}
	return headStyle
}

func (m *Model) calWeekLines(w, h int) []string {
	colW := m.calColWidth(w)
	sel, today := m.calSelDate(), m.calToday()
	selected := m.calSelected()
	sep := dimStyle.Render("│")
	var hdr, rule []string
	cols := make([][]string, 7)
	bodyH := max(1, h-3)
	for i := 0; i < 7; i++ {
		d := m.cal.from.AddDate(0, 0, i)
		label := weekdayHeads[i] + " " + fmt.Sprint(d.Day())
		hdr = append(hdr, m.dayStyle(d, d.Equal(sel), d.Equal(today), false).Render(fit(label, colW)))
		rule = append(rule, dimStyle.Render(strings.Repeat("─", colW)))
		x0 := i * (colW + 1)
		m.cal.cells = append(m.cal.cells, calCell{x0: x0, x1: x0 + colW, y0: 0, y1: h - 1, date: d})

		items := m.cal.byDay[dayKey(d)]
		var cell []string
		for j, it := range items {
			if len(cell) == bodyH-1 && len(items)-j > 1 {
				cell = append(cell, dimStyle.Render(fit(fmt.Sprintf("+%d more", len(items)-j), colW)))
				break
			}
			text := fit(cleanCell(gridText(it)), colW)
			switch {
			case it == selected:
				text = selStyle.Render(text)
			case it.kind != "event":
				text = dimStyle.Render(text)
			}
			cell = append(cell, text)
		}
		cols[i] = cell
	}
	lines := []string{strings.Join(hdr, sep), strings.Join(rule, dimStyle.Render("┼"))}
	blank := strings.Repeat(" ", colW)
	for r := 0; r < bodyH; r++ {
		parts := make([]string, 7)
		for i := range parts {
			if r < len(cols[i]) {
				parts[i] = cols[i][r]
			} else {
				parts[i] = blank
			}
		}
		lines = append(lines, strings.Join(parts, sep))
	}
	lines = append(lines, m.calStrip(w))
	return lines
}

func (m *Model) calMonthLines(w, h int) []string {
	colW := m.calColWidth(w)
	sel, today := m.calSelDate(), m.calToday()
	sep := dimStyle.Render("│")
	weeks := int(m.cal.to.Sub(m.cal.from).Hours()/24) / 7
	cellH := max(1, (h-2)/max(1, weeks))
	var hdr []string
	for i := 0; i < 7; i++ {
		hdr = append(hdr, headStyle.Render(fit(weekdayHeads[i], colW)))
	}
	lines := []string{strings.Join(hdr, sep)}
	blank := strings.Repeat(" ", colW)
	for wk := 0; wk < weeks; wk++ {
		cells := make([][]string, 7)
		for i := 0; i < 7; i++ {
			d := m.cal.from.AddDate(0, 0, wk*7+i)
			items := m.cal.byDay[dayKey(d)]
			label := fmt.Sprint(d.Day())
			if len(items) > 0 {
				label += fmt.Sprintf(" (%d)", len(items))
			}
			dimmed := d.Month() != m.cal.day.Month()
			cell := []string{m.dayStyle(d, d.Equal(sel), d.Equal(today), dimmed).Render(fit(label, colW))}
			for j, it := range items {
				if len(cell) >= cellH {
					break
				}
				if len(cell) == cellH-1 && len(items)-j > 1 {
					cell = append(cell, dimStyle.Render(fit(fmt.Sprintf("+%d more", len(items)-j), colW)))
					break
				}
				text := fit(cleanCell(gridText(it)), colW)
				if dimmed || it.kind != "event" {
					text = dimStyle.Render(text)
				}
				cell = append(cell, text)
			}
			cells[i] = cell
			x0 := i * (colW + 1)
			y0 := 1 + wk*cellH
			m.cal.cells = append(m.cal.cells, calCell{x0: x0, x1: x0 + colW, y0: y0, y1: y0 + cellH, date: d})
		}
		for r := 0; r < cellH; r++ {
			parts := make([]string, 7)
			for i := range parts {
				if r < len(cells[i]) {
					parts[i] = cells[i][r]
				} else {
					parts[i] = blank
				}
			}
			lines = append(lines, strings.Join(parts, sep))
		}
	}
	for len(lines) < h-1 {
		lines = append(lines, "")
	}
	lines = append(lines[:min(len(lines), h-1)], m.calStrip(w))
	return lines
}

// calClick selects the day under a click. x and y are screen coordinates, the body starts at line 1.
func (m *Model) calClick(x, y int) bool {
	for _, c := range m.cal.cells {
		if x >= c.x0 && x < c.x1+1 && y-1 >= c.y0 && y-1 < c.y1 {
			m.focus = focusList
			if c.date.Before(m.cal.from) || !c.date.Before(m.cal.to) {
				m.calReload(c.date)
			} else {
				m.cal.day = c.date
				m.cursor = m.calFirstOnOrAfter(c.date)
				m.settle()
				m.clamp()
				m.refreshDetail(false)
			}
			return true
		}
	}
	return false
}

var _ = tz.DateLayout
