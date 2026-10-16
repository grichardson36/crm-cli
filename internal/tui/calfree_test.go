package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Today is Tuesday 2026-10-13 07:00 in Chicago.
func TestFindATimePicksASlotAndAddsAnEvent(t *testing.T) {
	m := calModel(t)
	_, err := repo.NewAvailabilityRepo(m.db).SetDays(context.Background(), "", "America/Chicago", model.WeekdayKeys[:5], []model.TimeRange{{Start: "09:00", End: "17:00"}})
	require.NoError(t, err)
	addEvent(t, m, model.CreateEventInput{Title: "Planning", Start: utcT(2026, 10, 13, 15, 0), End: utcT(2026, 10, 13, 16, 0)})

	m.runAction(m.actionByID("cal-free"))
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "30m | 7", m.input.Value())
	m.input.SetValue("45m | 2")
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode)
	assert.Contains(t, m.ov.title, "Free 45m")
	first := m.ov.items[0]
	assert.Equal(t, "Tue 13 Oct  09:00-09:45", first.label)
	second := m.ov.items[1]
	assert.Equal(t, "Tue 13 Oct  11:15-12:00", second.label, "10 minute buffers around the 10:00-11:00 meeting")

	press(m, "enter")
	require.Equal(t, modePrompt, m.mode, "the picked slot starts the add prompt")
	assert.Equal(t, " 2026-10-13 09:00 for 45m", m.input.Value())
	typeText(m, "Coffee")
	press(m, "enter")
	assert.Contains(t, m.status, "event added")
	list, _, err := m.er.List(context.Background(), model.EventFilters{From: utcT(2026, 10, 13, 0, 0), To: utcT(2026, 10, 14, 0, 0), Loc: m.loc})
	require.NoError(t, err)
	var found bool
	for _, l := range list {
		if l.Title == "Coffee" {
			found = true
			assert.Equal(t, "2026-10-13T14:00:00Z", l.Start)
			assert.Equal(t, "2026-10-13T14:45:00Z", l.End)
		}
	}
	assert.True(t, found)
}

func TestFindATimeFromAnyViewAndErrors(t *testing.T) {
	m, _ := newTestModel(t)
	m.runAction(m.actionByID("find-time"))
	assert.Equal(t, viewCalendar, m.view)
	require.Equal(t, modePrompt, m.mode)
	m.input.SetValue("soon")
	press(m, "enter")
	assert.True(t, m.statusIsErr)

	m.runAction(m.actionByID("find-time"))
	m.input.SetValue("30m | many")
	press(m, "enter")
	assert.True(t, m.statusIsErr)

	m.runAction(m.actionByID("find-time"))
	m.input.SetValue("9h | 1")
	press(m, "enter")
	assert.Contains(t, m.status, "no free 9h slot")
}
