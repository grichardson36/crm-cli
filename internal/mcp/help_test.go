package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/help"
	crmmcp "github.com/jdanielnd/crm-cli/internal/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelpAndSchemaToolsRegistered(t *testing.T) {
	names := toolNames(setupTestServer(t).ListTools())
	assert.Contains(t, names, "crm_help")
	assert.Contains(t, names, "crm_schema")
}

func TestToolNamesMatchServer(t *testing.T) {
	got := crmmcp.ToolNames()
	want := toolNames(setupTestServer(t).ListTools())
	assert.ElementsMatch(t, want, got)
	assert.Contains(t, got, "crm_help")
}

func TestHelpToolListTopicAndSearch(t *testing.T) {
	s := setupTestServer(t)

	var list []map[string]string
	callJSON(t, s, "crm_help", map[string]any{}, &list)
	require.Len(t, list, len(help.Names()))
	assert.Equal(t, "overview", list[0]["name"])
	assert.NotEmpty(t, list[0]["summary"])

	var topic map[string]string
	callJSON(t, s, "crm_help", map[string]any{"topic": "duplicates"}, &topic)
	assert.Equal(t, "duplicates", topic["name"])
	assert.Contains(t, topic["body"], "crm dupes")

	var matches []map[string]any
	callJSON(t, s, "crm_help", map[string]any{"search": "MERGE"}, &matches)
	require.NotEmpty(t, matches)
	assert.NotEmpty(t, matches[0]["topic"])

	callJSON(t, s, "crm_help", map[string]any{"search": "zzzz-no-such-text"}, &matches)
	assert.Empty(t, matches)

	text, isErr := call(t, s, "crm_help", map[string]any{"topic": "nope"})
	assert.True(t, isErr)
	assert.Contains(t, text, "not found")
}

func TestSchemaToolUsesProvidedFunction(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	without := crmmcp.NewServer(d, "test")
	text, isErr := call(t, without, "crm_schema", nil)
	assert.True(t, isErr, text)

	with := crmmcp.NewServer(d, "test", crmmcp.WithSchema(func() any {
		return map[string]any{"name": "crm", "commands": []string{"person add"}}
	}))
	var m map[string]any
	callJSON(t, with, "crm_schema", nil, &m)
	assert.Equal(t, "crm", m["name"])
}

func rpc(t *testing.T, s *server.MCPServer, id int, method string, params map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	require.NoError(t, err)
	out, err := json.Marshal(s.HandleMessage(context.Background(), raw))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m), string(out))
	return m
}

func TestHelpTopicsAreResources(t *testing.T) {
	s := setupTestServer(t)
	rpc(t, s, 0, "initialize", map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "test", "version": "1.0"},
	})

	list := rpc(t, s, 1, "resources/list", nil)
	res := list["result"].(map[string]any)["resources"].([]any)
	uris := map[string]bool{}
	for _, r := range res {
		uris[r.(map[string]any)["uri"].(string)] = true
	}
	require.Len(t, uris, len(help.Names()))
	for _, name := range help.Names() {
		assert.True(t, uris["crm://help/"+name], name)
	}

	read := rpc(t, s, 2, "resources/read", map[string]any{"uri": "crm://help/agents"})
	contents := read["result"].(map[string]any)["contents"].([]any)
	require.Len(t, contents, 1)
	c := contents[0].(map[string]any)
	assert.Equal(t, "text/markdown", c["mimeType"])
	topic, _ := help.Get("agents")
	assert.Equal(t, topic.Body, c["text"])
}
