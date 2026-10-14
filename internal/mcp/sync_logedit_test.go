package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	crmmcp "github.com/jdanielnd/crm-cli/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogUpdateAndDeleteTools(t *testing.T) {
	s := setupTestServer(t)
	names := toolNames(s.ListTools())
	for _, n := range []string{"crm_log_update", "crm_log_delete", "crm_sync_run", "crm_sync_status",
		"crm_sync_inbox_list", "crm_sync_inbox_accept", "crm_sync_inbox_attach", "crm_sync_inbox_ignore"} {
		assert.Contains(t, names, n)
	}

	var p, logged, updated map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &p)
	callJSON(t, s, "crm_interaction_log", map[string]any{"type": "note", "content": "first draft", "person_ids": []any{p["id"]}}, &logged)

	callJSON(t, s, "crm_log_update", map[string]any{"id": logged["id"], "type": "call", "content": "second draft", "occurred_at": "2026-02-03 04:05:06"}, &updated)
	assert.Equal(t, "call", updated["type"])
	assert.Equal(t, "second draft", updated["content"])
	assert.Equal(t, "2026-02-03 04:05:06", updated["occurred_at"])

	text, isErr := call(t, s, "crm_log_update", map[string]any{"id": logged["id"], "type": "bogus"})
	assert.True(t, isErr)
	assert.Contains(t, text, "validation error")
	text, isErr = call(t, s, "crm_log_update", map[string]any{"id": logged["id"]})
	assert.True(t, isErr, text)
	text, isErr = call(t, s, "crm_log_update", map[string]any{"id": 999, "content": "x"})
	assert.True(t, isErr)
	assert.Equal(t, "not found", text)

	text, isErr = call(t, s, "crm_log_delete", map[string]any{"id": logged["id"]})
	assert.False(t, isErr, text)
	var list []map[string]any
	callJSON(t, s, "crm_interaction_list", map[string]any{}, &list)
	assert.Empty(t, list)
	_, isErr = call(t, s, "crm_log_delete", map[string]any{"id": logged["id"]})
	assert.True(t, isErr, "double delete")
	_, isErr = call(t, s, "crm_log_update", map[string]any{"id": logged["id"], "content": "x"})
	assert.True(t, isErr, "archived rows cannot be edited")

	text, isErr = call(t, s, "crm_log_delete", map[string]any{"id": logged["id"], "restore": true})
	assert.False(t, isErr, text)
	callJSON(t, s, "crm_interaction_list", map[string]any{}, &list)
	assert.Len(t, list, 1)
}

func TestSyncTools(t *testing.T) {
	records := filepath.Join(t.TempDir(), "fake.json")
	require.NoError(t, os.WriteFile(records, []byte(`[
	 {"seq":1,"kind":"person","external_id":"g1","person":{"emails":["jane@acme.com"],"title":"CTO"}},
	 {"seq":2,"kind":"person","external_id":"g2","person":{"first_name":"Zed","emails":["zed@x.com"]}},
	 {"seq":3,"kind":"person","external_id":"g3","person":{"first_name":"Ivy","emails":["ivy@x.com"]}}]`), 0o600))
	t.Setenv("CRM_SYNC_FAKE_FILE", records)

	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	s := crmmcp.NewServer(d, "test")

	var jane map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane", "email": "jane@acme.com"}, &jane)

	var res map[string]any
	callJSON(t, s, "crm_sync_run", map[string]any{"source": "fake", "dry_run": true}, &res)
	assert.Equal(t, true, res["dry_run"])
	assert.EqualValues(t, 1, res["updated"])
	assert.EqualValues(t, 2, res["inbox"])

	var status map[string]any
	callJSON(t, s, "crm_sync_status", map[string]any{}, &status)
	assert.EqualValues(t, 0, status["inbox_pending"], "dry run writes nothing")

	callJSON(t, s, "crm_sync_run", map[string]any{"source": "fake"}, &res)
	assert.Equal(t, "ok", res["status"])
	callJSON(t, s, "crm_sync_status", map[string]any{}, &status)
	assert.EqualValues(t, 2, status["inbox_pending"])
	sources := status["sources"].([]any)
	require.Len(t, sources, 1)
	assert.Equal(t, "3", sources[0].(map[string]any)["cursor"])

	text, isErr := call(t, s, "crm_sync_run", map[string]any{"source": "nope"})
	assert.True(t, isErr)
	assert.Equal(t, "not found", text)

	var items []map[string]any
	callJSON(t, s, "crm_sync_inbox_list", map[string]any{}, &items)
	require.Len(t, items, 2)

	var accepted map[string]any
	callJSON(t, s, "crm_sync_inbox_accept", map[string]any{"id": items[0]["id"]}, &accepted)
	assert.Equal(t, "accepted", accepted["status"])
	text, isErr = call(t, s, "crm_sync_inbox_ignore", map[string]any{"id": items[0]["id"]})
	assert.True(t, isErr)
	assert.Contains(t, text, "conflict")

	_, isErr = call(t, s, "crm_sync_inbox_ignore", map[string]any{"id": items[1]["id"]})
	assert.False(t, isErr)
	callJSON(t, s, "crm_sync_inbox_list", map[string]any{}, &items)
	assert.Empty(t, items)
	callJSON(t, s, "crm_sync_inbox_list", map[string]any{"status": "all"}, &items)
	assert.Len(t, items, 2)

	// Attach path with a fresh record.
	require.NoError(t, os.WriteFile(records, []byte(`[{"seq":4,"kind":"person","external_id":"g4","person":{"first_name":"Al","emails":["al@x.com"],"title":"Dir"}}]`), 0o600))
	callJSON(t, s, "crm_sync_run", map[string]any{"source": "fake"}, &res)
	callJSON(t, s, "crm_sync_inbox_list", map[string]any{}, &items)
	require.Len(t, items, 1)
	var pat map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Pat"}, &pat)
	_, isErr = call(t, s, "crm_sync_inbox_attach", map[string]any{"id": items[0]["id"], "person_id": pat["id"]})
	assert.False(t, isErr)
	callJSON(t, s, "crm_person_get", map[string]any{"id": pat["id"]}, &pat)
	assert.Equal(t, "Dir", pat["title"])
}
