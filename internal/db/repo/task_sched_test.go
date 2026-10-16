package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intp(n int) *int      { return &n }
func boolPtr(b bool) *bool { return &b }

func TestTaskSchedulingFieldsRoundTrip(t *testing.T) {
	tr := setupTaskTestDB(t)
	ctx := context.Background()

	created, err := tr.Create(ctx, model.CreateTaskInput{
		Title: "Write report", Priority: "high", DueAt: ptr("2026-11-10"),
		Schedule: model.TaskSchedule{
			DurationMin: intp(120), EarliestStart: ptr("2026-11-02"), DeadlineAt: ptr("2026-11-06T22:00:00Z"),
			DeadlineHard: boolPtr(true), MinChunkMin: intp(30), AutoSchedule: boolPtr(true), ScheduleName: ptr("deep-work"),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 120, *created.DurationMin)
	assert.Equal(t, "2026-11-02", *created.EarliestStart)
	assert.Equal(t, "2026-11-06T22:00:00Z", *created.DeadlineAt)
	assert.True(t, created.DeadlineHard)
	assert.Equal(t, 30, *created.MinChunkMin)
	assert.True(t, created.AutoSchedule)
	assert.Equal(t, "deep-work", created.ScheduleName)
	assert.Nil(t, created.ScheduledStart)
	assert.False(t, created.ScheduledLocked)
	assert.Equal(t, "2026-11-10", *created.DueAt, "the soft due date is untouched")

	got, err := tr.FindByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got)

	// Defaults on a plain task.
	plain, err := tr.Create(ctx, model.CreateTaskInput{Title: "Plain", Priority: "medium"})
	require.NoError(t, err)
	assert.Nil(t, plain.DurationMin)
	assert.False(t, plain.AutoSchedule)
	assert.Equal(t, "work", plain.ScheduleName)
}

func TestTaskPlacingByHandLocksAndClearing(t *testing.T) {
	tr := setupTaskTestDB(t)
	ctx := context.Background()
	task, err := tr.Create(ctx, model.CreateTaskInput{Title: "Call", Priority: "medium", Schedule: model.TaskSchedule{DurationMin: intp(30)}})
	require.NoError(t, err)

	placed, err := tr.Update(ctx, task.ID, model.UpdateTaskInput{Schedule: model.TaskSchedule{ScheduledStart: ptr("2026-11-03T20:00:00Z")}})
	require.NoError(t, err)
	assert.Equal(t, "2026-11-03T20:00:00Z", *placed.ScheduledStart)
	assert.True(t, placed.ScheduledLocked, "a time set by hand is locked")

	unlocked, err := tr.Update(ctx, task.ID, model.UpdateTaskInput{Schedule: model.TaskSchedule{ScheduledLocked: boolPtr(false)}})
	require.NoError(t, err)
	assert.False(t, unlocked.ScheduledLocked)
	assert.NotNil(t, unlocked.ScheduledStart)

	both, err := tr.Update(ctx, task.ID, model.UpdateTaskInput{Schedule: model.TaskSchedule{ScheduledStart: ptr("2026-11-04T20:00:00Z"), ScheduledLocked: boolPtr(false)}})
	require.NoError(t, err)
	assert.False(t, both.ScheduledLocked, "an explicit lock setting wins")

	cleared, err := tr.Update(ctx, task.ID, model.UpdateTaskInput{Schedule: model.TaskSchedule{
		ScheduledStart: ptr(""), DurationMin: intp(0), DeadlineAt: ptr(""), EarliestStart: ptr(""), MinChunkMin: intp(0),
	}})
	require.NoError(t, err)
	assert.Nil(t, cleared.ScheduledStart)
	assert.False(t, cleared.ScheduledLocked)
	assert.Nil(t, cleared.DurationMin)
	assert.Nil(t, cleared.DeadlineAt)
	assert.Nil(t, cleared.MinChunkMin)

	// Fields that are not passed keep their values.
	kept, err := tr.Update(ctx, task.ID, model.UpdateTaskInput{Title: ptr("Call Sam")})
	require.NoError(t, err)
	assert.Equal(t, "Call Sam", kept.Title)
}

func TestTaskSchedulingValidation(t *testing.T) {
	tr := setupTaskTestDB(t)
	ctx := context.Background()
	task, err := tr.Create(ctx, model.CreateTaskInput{Title: "x", Priority: "medium"})
	require.NoError(t, err)
	tests := []struct {
		name string
		s    model.TaskSchedule
	}{
		{"negative duration", model.TaskSchedule{DurationMin: intp(-5)}},
		{"negative chunk", model.TaskSchedule{MinChunkMin: intp(-1)}},
		{"bad earliest start", model.TaskSchedule{EarliestStart: ptr("soon")}},
		{"bad deadline", model.TaskSchedule{DeadlineAt: ptr("11/06/2026")}},
		{"bad placed time", model.TaskSchedule{ScheduledStart: ptr("2026-11-03")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tr.Update(ctx, task.ID, model.UpdateTaskInput{Schedule: tt.s})
			assert.ErrorIs(t, err, model.ErrValidation)
			_, err = tr.Create(ctx, model.CreateTaskInput{Title: "y", Priority: "medium", Schedule: tt.s})
			assert.ErrorIs(t, err, model.ErrValidation)
		})
	}
}
