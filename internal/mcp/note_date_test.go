package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoteUpdateOccurredAtIsValidated(t *testing.T) {
	s := setupTestServer(t)
	var note map[string]any
	callJSON(t, s, "crm_note_create", map[string]any{"content": "dated"}, &note)
	callJSON(t, s, "crm_note_update", map[string]any{"id": note["id"], "occurred_at": "2026-09-15"}, &note)
	assert.Equal(t, "2026-09-15 00:00:00", note["occurred_at"])
	_, isErr := call(t, s, "crm_note_update", map[string]any{"id": note["id"], "occurred_at": "garbage"})
	require.True(t, isErr, "an invalid date is refused like the CLI")
}
