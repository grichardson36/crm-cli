package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func planModel(t *testing.T) (*Model, *model.Task, *model.Task) {
	t.Helper()
	m := calModel(t)
	_, err := repo.NewAvailabilityRepo(m.db).SetDays(context.Background(), "", "America/Chicago", model.WeekdayKeys[:5], []model.TimeRange{{Start: "09:00", End: "17:00"}})
	require.NoError(t, err)
	yes := true
	mk := func(title string, mins int) *model.Task {
		task, err := m.tr.Create(context.Background(), model.CreateTaskInput{Title: title, Priority: "medium", Schedule: model.TaskSchedule{DurationMin: &mins, AutoSchedule: &yes}})
		require.NoError(t, err)
		return task
	}
	a, b := mk("Write report", 120), mk("Review notes", 60)
	dl := "2026-10-14" // a dated task is planned before an undated one
	_, err = m.tr.Update(context.Background(), a.ID, model.UpdateTaskInput{Schedule: model.TaskSchedule{DeadlineAt: &dl}})
	require.NoError(t, err)
	require.NoError(t, m.reload())
	return m, a, b
}

func TestPlanOverlayApplyRejectLockAndUndo(t *testing.T) {
	m, a, b := planModel(t)
	require.True(t, m.actionByID("cal-plan").enabled(m))
	press(m, "P")
	require.Equal(t, modeOverlay, m.mode)
	assert.Contains(t, m.ov.title, "Plan")
	require.Len(t, m.ov.items, 3)
	assert.Equal(t, "Apply 2 change(s)", m.ov.items[0].label)
	assert.Contains(t, m.ov.items[1].label, "Tue 13 Oct 09:00-11:00")

	// Reject the first task: pick its row, then Reject.
	press(m, "down", "enter")
	require.Equal(t, modeOverlay, m.mode)
	assert.Contains(t, m.ov.title, "Write report")
	press(m, "down", "enter") // Reject
	require.Equal(t, modeOverlay, m.mode, "back to the plan")
	assert.Equal(t, "Apply 1 change(s)", m.ov.items[0].label)
	assert.Contains(t, m.ov.items[1].label, "x Write report")

	// Lock the second.
	press(m, "down", "down", "enter")
	press(m, "down", "down", "enter") // Accept and lock
	assert.Contains(t, m.ov.items[2].label, "L Review notes")
	press(m, "up", "up", "enter") // Apply
	assert.Contains(t, m.status, "plan applied to 1 task")

	ga, _ := m.tr.FindByID(context.Background(), a.ID)
	gb, _ := m.tr.FindByID(context.Background(), b.ID)
	assert.Nil(t, ga.ScheduledStart)
	require.NotNil(t, gb.ScheduledStart)
	assert.True(t, gb.ScheduledLocked)

	// The scheduled block shows in the calendar.
	assert.Contains(t, rowsText(m.rows), "[ ] Review notes")

	m.runAction(m.actionByID("cal-plan-undo"))
	assert.Contains(t, m.status, "undone")
	gb, _ = m.tr.FindByID(context.Background(), b.ID)
	assert.Nil(t, gb.ScheduledStart)
	m.runAction(m.actionByID("cal-plan-undo"))
	assert.True(t, m.statusIsErr)
}

func rowsText(rows []row) string {
	out := ""
	for _, r := range rows {
		out += r.title + "|"
	}
	return out
}

func TestPlanOverlayEmptyAndCancel(t *testing.T) {
	m := calModel(t)
	press(m, "P")
	assert.Equal(t, modeNormal, m.mode)
	assert.Contains(t, m.status, "no auto-scheduled tasks")

	m, a, _ := planModel(t)
	press(m, "P", "esc")
	assert.Equal(t, modeNormal, m.mode)
	ga, _ := m.tr.FindByID(context.Background(), a.ID)
	assert.Nil(t, ga.ScheduledStart, "cancelling writes nothing")

	m.readOnly = true
	press(m, "P")
	assert.Contains(t, m.status, "read-only")
}

func TestSplitTaskShowsAsBlocksInTheCalendar(t *testing.T) {
	m, a, _ := planModel(t)
	chunks := []repo.Chunk{{Start: "2026-10-14T14:00:00Z", End: "2026-10-14T15:00:00Z"}, {Start: "2026-10-14T19:00:00Z", End: "2026-10-14T20:00:00Z"}}
	_, err := repo.NewPlanRepo(m.db).Apply(context.Background(), []repo.PlanEntry{{TaskID: a.ID, Start: &chunks[0].Start, Blocks: chunks}}, "test")
	require.NoError(t, err)
	require.NoError(t, m.reload())
	press(m, "d", "]")
	s := rowsText(m.rows)
	assert.Contains(t, s, "09:00 [ ] Write report")
	assert.Contains(t, s, "14:00 [ ] Write report")
}
