package mcp_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	crmmcp "github.com/jdanielnd/crm-cli/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoteTypesAndArchiveToolsRegistered(t *testing.T) {
	names := toolNames(setupTestServer(t).ListTools())
	for _, n := range []string{"crm_note_types", "crm_person_archive", "crm_person_restore", "crm_person_purge", "crm_person_delete"} {
		assert.Contains(t, names, n)
	}
}

func TestNoteTypesAndInteractionLog(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	s := crmmcp.NewServer(d, "test")
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &p)

	var types []map[string]any
	callJSON(t, s, "crm_note_types", map[string]any{}, &types)
	assert.Len(t, types, 5)

	text, isErr := call(t, s, "crm_interaction_log", map[string]any{"type": "standup", "person_ids": []any{p["id"]}})
	assert.True(t, isErr, text)
	assert.Contains(t, text, "validation error")

	// Defining a type makes the tool accept it, and it shows up in the list.
	_, err = repo.NewNoteTypeRepo(d).Add(context.Background(), "standup", true)
	require.NoError(t, err)
	var logged map[string]any
	callJSON(t, s, "crm_interaction_log", map[string]any{"type": "STANDUP", "person_ids": []any{p["id"]}}, &logged)
	assert.Equal(t, "standup", logged["type"])
	callJSON(t, s, "crm_note_types", map[string]any{}, &types)
	assert.Len(t, types, 6)

	// The old values still work.
	callJSON(t, s, "crm_interaction_log", map[string]any{"type": "call", "person_ids": []any{p["id"]}}, &logged)
	assert.Equal(t, "call", logged["type"])

	require.NoError(t, repo.NewNoteTypeRepo(d).Archive(context.Background(), "standup"))
	callJSON(t, s, "crm_note_types", map[string]any{}, &types)
	assert.Len(t, types, 5)
	callJSON(t, s, "crm_note_types", map[string]any{"include_archived": true}, &types)
	assert.Len(t, types, 6)
	text, isErr = call(t, s, "crm_interaction_log", map[string]any{"type": "standup", "person_ids": []any{p["id"]}})
	assert.True(t, isErr, text)
}

func TestPersonArchiveRestorePurgeTools(t *testing.T) {
	s := setupTestServer(t)
	var p map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &p)
	id := p["id"]

	text, isErr := call(t, s, "crm_person_purge", map[string]any{"id": id, "confirm": true})
	assert.True(t, isErr, "purge needs an archived person: %s", text)
	assert.Contains(t, text, "conflict")

	text, isErr = call(t, s, "crm_person_archive", map[string]any{"id": id})
	require.False(t, isErr, text)
	text, isErr = call(t, s, "crm_person_get", map[string]any{"id": id})
	assert.True(t, isErr, "archived people are hidden: %s", text)

	text, isErr = call(t, s, "crm_person_restore", map[string]any{"id": id})
	require.False(t, isErr, text)
	text, isErr = call(t, s, "crm_person_get", map[string]any{"id": id})
	assert.False(t, isErr, text)

	call(t, s, "crm_person_archive", map[string]any{"id": id})
	text, isErr = call(t, s, "crm_person_purge", map[string]any{"id": id})
	assert.True(t, isErr, "purge without confirm is refused")
	assert.Contains(t, text, "confirm")
	text, isErr = call(t, s, "crm_person_purge", map[string]any{"id": id, "confirm": false})
	assert.True(t, isErr, text)

	text, isErr = call(t, s, "crm_person_purge", map[string]any{"id": id, "confirm": true})
	require.False(t, isErr, text)
	text, isErr = call(t, s, "crm_person_restore", map[string]any{"id": id})
	assert.True(t, isErr, "a purged person cannot be restored: %s", text)
}
