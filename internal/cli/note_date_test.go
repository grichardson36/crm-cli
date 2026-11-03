package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The activity date is edited with the same dates in the CLI, MCP and TUI doors (repo.NormalizeOccurredAt).
func TestNoteEditDate(t *testing.T) {
	dbPath := noteSeed(t)
	_, stderr, code := crm(t, dbPath, "note", "add", "dated", "--type", "call")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, dbPath, "note", "edit", "1", "--date", "2026-09-15")
	require.Equal(t, 0, code, stderr)
	stdout, _, _ := crm(t, dbPath, "note", "show", "1", "-f", "json")
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Equal(t, "2026-09-15 00:00:00", rows[0]["occurred_at"])

	_, _, code = crm(t, dbPath, "note", "edit", "1", "--date", "garbage")
	assert.Equal(t, 2, code, "an invalid date is a validation error")
}
