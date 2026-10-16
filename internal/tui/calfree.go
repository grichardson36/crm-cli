package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/schedule"
)

// Find a time: free slots inside working hours, clear of busy time.

func (m *Model) startFindTime() tea.Cmd {
	if m.view != viewCalendar {
		m.calGoto(m.calToday())
	}
	m.startInput("cal-free", "find a time (length | days, e.g. 45m | 7): ", "30m | 7")
	return m.input.Focus()
}

// calFreeRun lists open slots and starts the add prompt for the one picked.
func (m *Model) calFreeRun(value string) {
	parts := strings.Split(value, "|")
	length, err := when.ParseLength(strings.TrimSpace(parts[0]))
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	p := schedule.FindParams{Length: length, Now: m.now(), Limit: 40}
	if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
		days, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || days < 1 {
			m.setStatus("days must be a number", true)
			return
		}
		p.To = m.now().AddDate(0, 0, days)
	}
	res, err := schedule.FindTime(m.ctx, m.db, p)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if len(res.Slots) == 0 {
		m.setStatus("no free "+lengthText(length)+" slot in that window", false)
		return
	}
	items := make([]overlayItem, 0, len(res.Slots))
	for _, s := range res.Slots {
		st, _ := tz.ParseInstant(s.Start)
		en, _ := tz.ParseInstant(s.End)
		ls, le := st.In(m.loc), en.In(m.loc)
		items = append(items, overlayItem{id: s.Start, label: ls.Format("Mon 2 Jan  15:04") + "-" + le.Format("15:04"), value: ls.Format("2006-01-02 15:04") + " for " + lengthText(length)})
	}
	m.openPicker("Free "+lengthText(length)+" slots (pick one to add an event)", items, func(m *Model, it overlayItem) {
		m.startInput("cal-add", "event (Lunch @Sam fri 12:30 for 1h at Cafe Rio every week): ", " "+it.value)
		m.input.SetCursor(0)
		m.pendingCmd = m.input.Focus()
	})
}
