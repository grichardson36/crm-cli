package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Owner decisions 2026-10-27: merge is on the undo stack and in the persistent log (X9), "not a
// duplicate" for organization candidates is a palette action (D7), tag delete is one undo step (X26).

func archivedOf(t *testing.T, m *Model, id int64) int {
	t.Helper()
	var n int
	require.NoError(t, m.db.QueryRow(`SELECT archived FROM people WHERE id = ?`, id).Scan(&n))
	return n
}

func TestPersonMergeIsOnTheUndoStackAndInTheLog(t *testing.T) {
	m, _ := dupeModel(t)
	m.openCompare([]int64{2, 4}, 0, []string{"test"}, true)
	require.NotNil(t, m.cmp)
	press(m, "m")
	require.Nil(t, m.cmp)
	require.Equal(t, 1, archivedOf(t, m, 4))

	logs, err := m.ml.List(m.ctx, "person", 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "merged", logs[0].Status)

	press(m, "ctrl+z")
	assert.Equal(t, 0, archivedOf(t, m, 4), "ctrl+z brings the merged person back")
	logs, _ = m.ml.List(m.ctx, "person", 0)
	assert.Equal(t, "undone", logs[0].Status, "the log follows the stack")

	press(m, "ctrl+y")
	assert.Equal(t, 1, archivedOf(t, m, 4))
	logs, _ = m.ml.List(m.ctx, "person", 0)
	assert.Equal(t, "merged", logs[0].Status)

	// Undone from the CLI or MCP first, the stack refuses instead of guessing.
	_, err = m.ml.Undo(m.ctx, logs[0].ID)
	require.NoError(t, err)
	press(m, "ctrl+z")
	assert.Contains(t, m.toast.text, "Cannot undo")
}

func TestOrganizationMergeUndoGoesThroughTheLog(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	var ids []int64
	for _, n := range []string{"Blue Origin", "BLUE ORIGIN"} {
		o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: n, AllowDuplicate: true})
		require.NoError(t, err)
		ids = append(ids, o.ID)
	}
	m.confirmOrgMerge(ids[0], ids[1:])
	require.Equal(t, modeConfirm, m.mode)
	press(m, "y")
	logs, err := m.ml.List(ctx, "organization", 0)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "merged", logs[0].Status)
	press(m, "ctrl+z")
	logs, _ = m.ml.List(ctx, "organization", 0)
	assert.Equal(t, "undone", logs[0].Status)
	assert.Len(t, openOrgs(t, m), 2)
}

func TestNotADuplicateIsAPaletteActionWithNoKey(t *testing.T) {
	var found *action
	for _, a := range buildActions() {
		a := a
		if a.id == "org-not-duplicate" || a.id == "org-undismiss" {
			assert.Empty(t, a.keys, "%s has no key", a.id)
			assert.Empty(t, a.hint, "%s has no key hint", a.id)
			assert.True(t, a.write)
		}
		if a.id == "org-not-duplicate" {
			found = &a
		}
	}
	require.NotNil(t, found)

	m, _ := newTestModel(t)
	ctx := context.Background()
	for _, n := range []string{"Blue Origin", "BLUE ORIGIN", "Other Co"} {
		_, err := m.orr.Create(ctx, model.CreateOrgInput{Name: n, AllowDuplicate: true})
		require.NoError(t, err)
	}
	runPalette(m, "organizations view")

	// Put the cursor on Blue Origin, which has a near-duplicate.
	m.cursorOn(func(r row) bool { return r.orgID == 1 })
	require.NotEmpty(t, m.similarOrgs()[1])

	runPalette(m, "not a duplicate")
	cands, err := m.orr.FindOrgCandidates(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, cands, "the dismissed pair is no longer a candidate")
	assert.Empty(t, m.similarOrgs()[1])
	var n int
	require.NoError(t, m.db.QueryRow(`SELECT COUNT(*) FROM org_dupe_dismissals WHERE deleted_at IS NULL`).Scan(&n))
	assert.Equal(t, 1, n, "stored as data")

	press(m, "ctrl+z")
	cands, _ = m.orr.FindOrgCandidates(ctx, 0)
	assert.Len(t, cands, 1, "ctrl+z makes it a candidate again")
	press(m, "ctrl+y")
	cands, _ = m.orr.FindOrgCandidates(ctx, 0)
	assert.Empty(t, cands)

	// The palette lists dismissed pairs and brings one back.
	runPalette(m, "dismissed organization pairs")
	require.NotNil(t, m.ov)
	press(m, "enter")
	cands, _ = m.orr.FindOrgCandidates(ctx, 0)
	assert.Len(t, cands, 1)
}

func TestTagDeleteIsOneUndoStepOnSoftDeletedTaggings(t *testing.T) {
	m, _ := newTestModel(t) // Bob (person 2) has the tag "friend"
	for _, id := range []int64{1, 3} {
		require.NoError(t, m.tagr.Apply(m.ctx, "person", id, "friend"))
	}
	var uuids []string
	rows, err := m.db.Query(`SELECT uuid FROM taggings ORDER BY id`)
	require.NoError(t, err)
	for rows.Next() {
		var u string
		require.NoError(t, rows.Scan(&u))
		uuids = append(uuids, u)
	}
	require.NoError(t, rows.Close())
	require.Len(t, uuids, 3)

	m.deleteTag("friend")
	tags, _ := m.tagr.FindAll(m.ctx)
	assert.Empty(t, tags, "a tag nothing carries is not listed")
	var dead int
	require.NoError(t, m.db.QueryRow(`SELECT COUNT(*) FROM taggings WHERE deleted_at IS NOT NULL AND updated_at <> ''`).Scan(&dead))
	assert.Equal(t, 3, dead, "the taggings are soft deleted, none is removed for good")

	press(m, "ctrl+z")
	ids, _ := m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.ElementsMatch(t, []int64{1, 2, 3}, ids, "one undo step restores every tagging")
	var same int
	require.NoError(t, m.db.QueryRow(`SELECT COUNT(*) FROM taggings WHERE deleted_at IS NULL AND uuid IN (?, ?, ?)`, uuids[0], uuids[1], uuids[2]).Scan(&same))
	assert.Equal(t, 3, same, "the same rows come back")
	press(m, "ctrl+y")
	ids, _ = m.tagr.GetEntities(m.ctx, "friend", "person")
	assert.Empty(t, ids)
}
