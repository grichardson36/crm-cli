package schedule

import (
	"database/sql"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func autoTask(t *testing.T, d *sql.DB, title string, mins int, mod func(*model.CreateTaskInput)) *model.Task {
	t.Helper()
	yes := true
	in := model.CreateTaskInput{Title: title, Priority: "medium", Schedule: model.TaskSchedule{DurationMin: &mins, AutoSchedule: &yes}}
	if mod != nil {
		mod(&in)
	}
	task, err := repo.NewTaskRepo(d).Create(ctx, in)
	require.NoError(t, err)
	return task
}

func plan(t *testing.T, d *sql.DB, only ...int64) *PlanResult {
	t.Helper()
	res, err := PlanTasks(ctx, d, PlanOptions{Now: planNow, Only: only})
	require.NoError(t, err)
	return res
}

func TestPlanTasksReadsButNeverWrites(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	a := autoTask(t, d, "Write report", 120, nil)
	autoTask(t, d, "Not automatic", 60, func(in *model.CreateTaskInput) { no := false; in.Schedule.AutoSchedule = &no })
	autoTask(t, d, "No length", 0, func(in *model.CreateTaskInput) { in.Schedule.DurationMin = nil })

	var before string
	require.NoError(t, d.QueryRow(`SELECT group_concat(updated_at || coalesce(scheduled_start, '-')) FROM tasks`).Scan(&before))
	res := plan(t, d)
	require.Len(t, res.Decisions, 2, "the manual task is not planned")
	assert.Equal(t, 1, res.Summary.Placed)
	assert.Equal(t, 1, res.Summary.Skipped)
	placed := byTitle(*res)["Write report"]
	assert.Equal(t, "2026-10-12T14:00:00Z", placed.Blocks[0].Start)
	var after string
	require.NoError(t, d.QueryRow(`SELECT group_concat(updated_at || coalesce(scheduled_start, '-')) FROM tasks`).Scan(&after))
	assert.Equal(t, before, after, "a proposal writes nothing")

	only := plan(t, d, a.ID)
	assert.Len(t, only.Decisions, 1)
}

func TestPlanUsesDueDateAsSoftDeadlineAndEventsAsBusy(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	event(t, d, model.CreateEventInput{Title: "Meeting", Start: utc(2026, 10, 12, 14, 0), End: utc(2026, 10, 12, 16, 0)}) // 09:00-11:00
	autoTask(t, d, "Due Monday", 60, func(in *model.CreateTaskInput) { in.DueAt = ptr("2026-10-12") })
	autoTask(t, d, "Whenever", 60, nil)
	res := plan(t, d)
	m := byTitle(*res)
	assert.Equal(t, "Mon 11:15-12:15", span(t, m["Due Monday"]), "after the meeting and its buffer")
	assert.Contains(t, m["Due Monday"].Why[0], "due date in")
	assert.Equal(t, "2026-10-13T05:00:00Z", m["Due Monday"].Deadline, "the due date means the end of that local day")
	assert.Contains(t, m["Whenever"].Why[0], "no deadline")
}

func TestApplyWritesOnlyPlacementAndUndoRestores(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	tr := repo.NewTaskRepo(d)
	a := autoTask(t, d, "Report", 120, func(in *model.CreateTaskInput) { in.Description = ptr("keep me"); in.DueAt = ptr("2026-10-14") })
	big := autoTask(t, d, "Split me", 300, func(in *model.CreateTaskInput) { mc := 60; in.Schedule.MinChunkMin = &mc })
	event(t, d, model.CreateEventInput{Title: "Block", Start: utc(2026, 10, 12, 14, 0), End: utc(2026, 10, 12, 20, 0)}) // 09:00-15:00

	res := plan(t, d)
	j, err := Apply(ctx, d, res, ApplyOptions{})
	require.NoError(t, err)
	assert.Contains(t, j.Note, "2 placed")

	got, err := tr.FindByID(ctx, a.ID)
	require.NoError(t, err)
	require.NotNil(t, got.ScheduledStart)
	assert.False(t, got.ScheduledLocked, "planned times stay movable")
	assert.Equal(t, "keep me", *got.Description)
	assert.Equal(t, "2026-10-14", *got.DueAt)
	assert.Equal(t, 120, *got.DurationMin)
	assert.True(t, got.AutoSchedule)

	chunks, err := repo.NewPlanRepo(d).Chunks(ctx)
	require.NoError(t, err)
	assert.Len(t, chunks[big.ID], 2, "a split task keeps its blocks")
	split, _ := tr.FindByID(ctx, big.ID)
	assert.Equal(t, chunks[big.ID][0].Start, *split.ScheduledStart, "scheduled_start is the first block")

	// Applying the same plan again changes nothing.
	again := plan(t, d)
	_, err = Apply(ctx, d, again, ApplyOptions{})
	assert.ErrorIs(t, err, model.ErrConflict)

	// Split blocks are busy time for find-a-time once locked (here: busy tasks counted).
	busy, err := Busy(ctx, d, planNow, planNow.AddDate(0, 0, 7), chicagoLoc(t), true)
	require.NoError(t, err)
	var minutes time.Duration
	for _, b := range busy {
		minutes += b.Len()
	}
	assert.GreaterOrEqual(t, int(minutes/time.Minute), 6*60+120+300)

	// Undo puts everything back.
	undone, err := repo.NewPlanRepo(d).Undo(ctx)
	require.NoError(t, err)
	assert.True(t, undone.Undone)
	got, _ = tr.FindByID(ctx, a.ID)
	assert.Nil(t, got.ScheduledStart)
	chunks, _ = repo.NewPlanRepo(d).Chunks(ctx)
	assert.Empty(t, chunks)
	_, err = repo.NewPlanRepo(d).Undo(ctx)
	assert.ErrorIs(t, err, model.ErrNotFound, "one plan, one undo")

	hist, err := repo.NewPlanRepo(d).History(ctx, 5)
	require.NoError(t, err)
	require.Len(t, hist, 1)
	assert.True(t, hist[0].Undone)
}

func TestApplyRejectLockAndUnplace(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	tr := repo.NewTaskRepo(d)
	a := autoTask(t, d, "Keep", 60, nil)
	b := autoTask(t, d, "Skip", 60, nil)
	c := autoTask(t, d, "Pin", 60, nil)
	res := plan(t, d)
	_, err := Apply(ctx, d, res, ApplyOptions{Reject: map[int64]bool{b.ID: true}, Lock: map[int64]bool{c.ID: true}})
	require.NoError(t, err)
	ga, _ := tr.FindByID(ctx, a.ID)
	gb, _ := tr.FindByID(ctx, b.ID)
	gc, _ := tr.FindByID(ctx, c.ID)
	assert.NotNil(t, ga.ScheduledStart)
	assert.Nil(t, gb.ScheduledStart, "a rejected row is left alone")
	assert.NotNil(t, gc.ScheduledStart)
	assert.True(t, gc.ScheduledLocked, "locked at the proposed time")

	// The locked task is now a wall for the next plan and is never moved.
	next := plan(t, d)
	assert.NotContains(t, byTitle(*next), "Pin")
	for _, d := range next.Decisions {
		for _, bl := range d.Blocks {
			assert.NotEqual(t, *gc.ScheduledStart, bl.Start)
		}
	}

	// A task that no longer fits is unplaced by the next apply.
	long := 9 * 60
	_, err = tr.Update(ctx, a.ID, model.UpdateTaskInput{Schedule: model.TaskSchedule{DurationMin: &long}})
	require.NoError(t, err)
	res = plan(t, d)
	assert.Equal(t, Unfit, byTitle(*res)["Keep"].Decision)
	_, err = Apply(ctx, d, res, ApplyOptions{})
	require.NoError(t, err)
	ga, _ = tr.FindByID(ctx, a.ID)
	assert.Nil(t, ga.ScheduledStart)
	_, err = repo.NewPlanRepo(d).Undo(ctx)
	require.NoError(t, err)
	ga, _ = tr.FindByID(ctx, a.ID)
	assert.NotNil(t, ga.ScheduledStart, "undo brings the earlier placement back")
}

func TestPlanMovesWhenEventsChange(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	autoTask(t, d, "Report", 60, nil)
	_, err := Apply(ctx, d, plan(t, d), ApplyOptions{})
	require.NoError(t, err)
	event(t, d, model.CreateEventInput{Title: "New meeting", Start: utc(2026, 10, 12, 14, 0), End: utc(2026, 10, 12, 16, 0)})
	res := plan(t, d)
	m := byTitle(*res)["Report"]
	assert.Equal(t, "Mon 11:15-12:15", span(t, m))
	assert.Equal(t, "2026-10-12T14:00:00Z", m.MovedFrom, "the proposal says where the task was")
}

func TestPlanDeterministicAgainstDatabase(t *testing.T) {
	d := openDB(t)
	seedHours(t, d)
	for i := 0; i < 6; i++ {
		autoTask(t, d, string(rune('a'+i)), 45+i*15, nil)
	}
	assert.Equal(t, plan(t, d), plan(t, d))
}
