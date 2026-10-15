package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fvFields(t *testing.T, f *eventFx, entity string, id int64) map[string]string {
	t.Helper()
	rows, err := f.d.Query(`SELECT field, changed_at FROM field_versions WHERE entity = ? AND local_id = ?`, entity, id)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var field, at string
		require.NoError(t, rows.Scan(&field, &at))
		out[field] = at
	}
	return out
}

func TestEventWritesRegisterFieldVersions(t *testing.T) {
	f := newEventFx(t)
	e, err := f.er.Create(f.ctx, model.CreateEventInput{Title: "Standup", Start: time.Date(2026, 3, 2, 15, 0, 0, 0, time.UTC), TZ: "UTC"})
	require.NoError(t, err)
	assert.Empty(t, fvFields(t, f, "event", e.ID), "a new row has no history")

	_, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("Standup v2"), Location: ptr("Room 4")}, "", "")
	require.NoError(t, err)
	got := fvFields(t, f, "event", e.ID)
	assert.Contains(t, got, "title")
	assert.Contains(t, got, "location")
	assert.NotContains(t, got, "when", "unchanged fields are not touched")
	assert.NotContains(t, got, "notes")

	moved := time.Date(2026, 3, 3, 16, 0, 0, 0, time.UTC)
	_, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Start: &moved}, "", "")
	require.NoError(t, err)
	assert.Contains(t, fvFields(t, f, "event", e.ID), "when", "start, end and zone are one field")

	// Same value written again is not a change.
	_, err = f.d.Exec(`DELETE FROM field_versions`)
	require.NoError(t, err)
	_, err = f.er.Update(f.ctx, e.ID, model.UpdateEventInput{Title: ptr("Standup v2")}, "", "")
	require.NoError(t, err)
	assert.Empty(t, fvFields(t, f, "event", e.ID))

	require.NoError(t, f.er.Archive(f.ctx, e.ID, "", ""))
	_, err = f.d.Exec(`INSERT INTO field_versions (entity, local_id, field, changed_at) VALUES ('event', ?, 'title', datetime('now'))`, e.ID)
	require.NoError(t, err)
	require.NoError(t, f.er.Purge(f.ctx, e.ID))
	assert.Empty(t, fvFields(t, f, "event", e.ID), "purge takes the history with it")
}

func TestTaskWritesRegisterFieldVersions(t *testing.T) {
	d := openRaw(t)
	tr := repo.NewTaskRepo(d)
	ctx := context.Background()
	task, err := tr.Create(ctx, model.CreateTaskInput{Title: "Draft", Priority: "medium"})
	require.NoError(t, err)

	read := func() map[string]bool {
		rows, err := d.Query(`SELECT field FROM field_versions WHERE entity = 'task' AND local_id = ?`, task.ID)
		require.NoError(t, err)
		defer rows.Close()
		out := map[string]bool{}
		for rows.Next() {
			var f string
			require.NoError(t, rows.Scan(&f))
			out[f] = true
		}
		return out
	}
	assert.Empty(t, read())
	mins := 90
	_, err = tr.Update(ctx, task.ID, model.UpdateTaskInput{Title: ptr("Draft v2"), Schedule: model.TaskSchedule{DurationMin: &mins}})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"title": true, "duration_min": true}, read())
	_, err = tr.Complete(ctx, task.ID)
	require.NoError(t, err)
	assert.True(t, read()["completed"])
}
