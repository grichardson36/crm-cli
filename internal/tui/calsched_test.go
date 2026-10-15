package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func schedTask(t *testing.T, m *Model, title string) *model.Task {
	t.Helper()
	task, err := m.tr.Create(context.Background(), model.CreateTaskInput{Title: title, Priority: "medium", PersonID: ip(2)})
	require.NoError(t, err)
	require.NoError(t, m.reload())
	return task
}

func ip(n int64) *int64 { return &n }

func TestTaskSchedulingFromTheTasksView(t *testing.T) {
	m := calModel(t)
	task := schedTask(t, m, "Write report")
	press(m, "4")
	require.Equal(t, viewTasks, m.view)
	require.True(t, m.actionByID("task-schedule").enabled(m))

	press(m, "S")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.input.Prompt, "Write report")
	m.input.SetValue("2h | fri 14:00 | 2026-10-30")
	press(m, "enter")
	assert.Equal(t, "task schedule saved", m.status)
	got, err := m.tr.FindByID(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, 120, *got.DurationMin)
	assert.Equal(t, "2026-10-16T19:00:00Z", *got.ScheduledStart)
	assert.True(t, got.ScheduledLocked, "placing by hand locks")
	assert.Equal(t, "2026-10-30", *got.DeadlineAt)

	// The line is prefilled with the current values, and "-" clears.
	press(m, "S")
	assert.Equal(t, "2h | 2026-10-16 14:00 | 2026-10-30", m.input.Value())
	m.input.SetValue("- | - | -")
	press(m, "enter")
	got, _ = m.tr.FindByID(context.Background(), task.ID)
	assert.Nil(t, got.DurationMin)
	assert.Nil(t, got.ScheduledStart)
	assert.Nil(t, got.DeadlineAt)

	press(m, "S")
	m.input.SetValue("soon")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
}

func TestAutoScheduleToggle(t *testing.T) {
	m := calModel(t)
	task := schedTask(t, m, "Plan trip")
	press(m, "4", "A")
	got, err := m.tr.FindByID(context.Background(), task.ID)
	require.NoError(t, err)
	assert.True(t, got.AutoSchedule)
	assert.Contains(t, m.status, "planned as 30 min")
	press(m, "A")
	got, _ = m.tr.FindByID(context.Background(), task.ID)
	assert.False(t, got.AutoSchedule)
	assert.Equal(t, "auto-schedule off", m.status)

	_, err = m.tr.Update(context.Background(), task.ID, model.UpdateTaskInput{Schedule: model.TaskSchedule{DurationMin: intPtr(30)}})
	require.NoError(t, err)
	require.NoError(t, m.reload())
	press(m, "A")
	assert.Equal(t, "auto-schedule on", m.status)
}

func intPtr(n int) *int { return &n }

func TestScheduledTasksAreBlocksInTheCalendar(t *testing.T) {
	m := calModel(t)
	task := schedTask(t, m, "Write report")
	_, err := m.tr.Update(context.Background(), task.ID, model.UpdateTaskInput{
		DueAt:    sp("2026-10-30"),
		Schedule: model.TaskSchedule{DurationMin: intPtr(90), ScheduledStart: sp("2026-10-14T19:00:00Z")},
	})
	require.NoError(t, err)
	require.NoError(t, m.reload())
	press(m, "d", "tab")
	require.Equal(t, "Wed 14 Oct 2026", m.calCountText())
	var found *row
	for i := range m.rows {
		if m.rows[i].taskID == task.ID {
			found = &m.rows[i]
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, "14:00 [ ] Write report", found.title, "a block at its scheduled time")
	assert.Contains(t, found.sub, "1h30m")
	assert.Equal(t, rankTimed, found.cal.rank)

	// The due date does not add a second entry on another day inside the window.
	press(m, "w")
	count := 0
	for _, r := range m.rows {
		if r.taskID == task.ID {
			count++
		}
	}
	assert.Equal(t, 1, count)
	assert.Contains(t, m.View(), "14:00 [ ] Wr")

	// Selecting the block lets S edit its schedule.
	for i, r := range m.rows {
		if r.taskID == task.ID {
			m.cursor = i
		}
	}
	m.refreshDetail(true)
	assert.True(t, m.actionByID("task-schedule").enabled(m))
}

func TestWorkingHoursActions(t *testing.T) {
	m := calModel(t)
	m.runAction(m.actionByID("cal-hours"))
	require.NotNil(t, m.tv)
	body := m.tv.vp.View()
	assert.Contains(t, body, "rule: work")
	assert.Contains(t, body, "09:00-17:00")
	assert.Contains(t, body, "buffer 10 min, step 15 min, horizon 14 days")
	press(m, "esc")
	assert.Nil(t, m.tv)

	m.runAction(m.actionByID("cal-hours-set"))
	require.Equal(t, modePrompt, m.mode)
	m.input.SetValue("sat 10:00-14:00")
	press(m, "enter")
	assert.Equal(t, "working hours saved", m.status)
	h, err := repo.NewAvailabilityRepo(m.db).Get(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, []model.TimeRange{{Start: "10:00", End: "14:00"}}, h.Weekly["sat"])
	assert.Equal(t, tz.Name(tz.Device()), h.TZ)

	m.runAction(m.actionByID("cal-hours-set"))
	m.input.SetValue("mon")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	m.runAction(m.actionByID("cal-hours-set"))
	m.input.SetValue("funday 09:00-10:00")
	press(m, "enter")
	assert.True(t, m.statusIsErr)

	m.readOnly = true
	m.runAction(m.actionByID("cal-hours-set"))
	assert.Contains(t, m.status, "read-only")
	assert.True(t, strings.Contains(m.status, "read-only"))
}
