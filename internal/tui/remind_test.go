package tui

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Today is Tuesday 2026-10-13 07:00 in Chicago.
func TestHomeBannerAndReminderActions(t *testing.T) {
	m := bookModel(t)
	addEvent(t, m, model.CreateEventInput{Title: "Early call", Start: utcT(2026, 10, 13, 12, 10), End: utcT(2026, 10, 13, 12, 30),
		Reminders: []model.EventReminder{{MinutesBefore: 15, Method: "display"}}})

	rows, err := m.homeRows()
	require.NoError(t, err)
	var banner, call bool
	for _, r := range rows {
		if r.skip && r.title == "Reminders due" {
			banner = true
		}
		if r.title == "Early call" {
			call = true
			assert.Equal(t, "in 10m", r.meta)
			assert.Equal(t, "Tue 13 Oct 07:10", r.sub)
		}
	}
	assert.True(t, banner && call, "Home lists the due reminder")

	m.runAction(m.actionByID("remind-due"))
	require.NotNil(t, m.tv)
	assert.Contains(t, m.tv.vp.View(), "Early call")
	m.tv = nil
	rows, _ = m.homeRows()
	var still bool
	for _, r := range rows {
		still = still || r.title == "Reminders due"
	}
	assert.True(t, still, "looking at them does not mark them")
}

func TestMarkingRemindersClearsTheBanner(t *testing.T) {
	m := bookModel(t)
	addEvent(t, m, model.CreateEventInput{Title: "Early call", Start: utcT(2026, 10, 13, 12, 10), End: utcT(2026, 10, 13, 12, 30),
		Reminders: []model.EventReminder{{MinutesBefore: 15, Method: "display"}}})
	m.runAction(m.actionByID("remind-mark"))
	assert.Contains(t, m.status, "marked 1 reminders and 0 booking messages")
	rows, err := m.homeRows()
	require.NoError(t, err)
	for _, r := range rows {
		assert.NotEqual(t, "Reminders due", r.title)
	}
	m.runAction(m.actionByID("remind-due"))
	require.NotNil(t, m.tv)
	assert.Contains(t, m.tv.vp.View(), "Nothing is due.")

	m.readOnly = true
	m.runAction(m.actionByID("remind-mark"))
	assert.Contains(t, m.status, "read-only")
}
