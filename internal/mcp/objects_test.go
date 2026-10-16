package mcp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectToolsAreRegistered(t *testing.T) {
	names := toolNames(setupTestServer(t).ListTools())
	for _, n := range []string{
		"crm_org_create", "crm_org_update", "crm_org_delete", "crm_org_restore",
		"crm_deal_delete", "crm_deal_restore", "crm_task_restore", "crm_tag_remove",
		"crm_person_link", "crm_person_unlink", "crm_org_link", "crm_org_unlink",
		"crm_deal_link", "crm_deal_unlink", "crm_task_link", "crm_task_unlink",
	} {
		assert.Contains(t, names, n)
	}
}

func TestOrgAndDealDeleteRestore(t *testing.T) {
	s := setupTestServer(t)
	var org, deal map[string]any
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Acme", "domain": "acme.com"}, &org)
	callJSON(t, s, "crm_deal_create", map[string]any{"title": "Site"}, &deal)

	callJSON(t, s, "crm_org_update", map[string]any{"id": org["id"], "domain": ""}, &org)
	assert.Empty(t, org["domain"], "an empty string empties the field")

	for _, c := range []struct {
		kind string
		id   any
	}{{"org", org["id"]}, {"deal", deal["id"]}} {
		_, isErr := call(t, s, "crm_"+c.kind+"_delete", map[string]any{"id": c.id})
		require.False(t, isErr)
		_, isErr = call(t, s, "crm_"+c.kind+"_delete", map[string]any{"id": c.id})
		assert.True(t, isErr, "deleting twice is not found")
		text, isErr := call(t, s, "crm_"+c.kind+"_restore", map[string]any{"id": c.id})
		require.False(t, isErr, text)
		_, isErr = call(t, s, "crm_"+c.kind+"_restore", map[string]any{"id": c.id})
		assert.True(t, isErr, "restoring a live %s is not found", c.kind)
	}

	var task map[string]any
	callJSON(t, s, "crm_task_create", map[string]any{"title": "Call"}, &task)
	_, isErr := call(t, s, "crm_task_delete", map[string]any{"id": task["id"]})
	require.False(t, isErr)
	text, isErr := call(t, s, "crm_task_restore", map[string]any{"id": task["id"]})
	require.False(t, isErr, text)
}

func TestLinkToolsWorkFromEitherSide(t *testing.T) {
	s := setupTestServer(t)
	var person, org, deal, task map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &person)
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Acme"}, &org)
	callJSON(t, s, "crm_deal_create", map[string]any{"title": "Site"}, &deal)
	callJSON(t, s, "crm_task_create", map[string]any{"title": "Call"}, &task)

	cases := []struct {
		a    string
		aid  any
		b    string
		bid  any
		flip bool
	}{
		{"person", person["id"], "org", org["id"], false},
		{"deal", deal["id"], "org", org["id"], true},
		{"deal", deal["id"], "person", person["id"], false},
		{"task", task["id"], "deal", deal["id"], true},
		{"task", task["id"], "person", person["id"], false},
	}
	for _, c := range cases {
		from, fromID, to, toID := c.a, c.aid, c.b, c.bid
		if c.flip {
			from, fromID, to, toID = to, toID, from, fromID
		}
		var out map[string]any
		callJSON(t, s, "crm_"+from+"_link", map[string]any{"id": fromID, "other": to, "other_id": toID}, &out)
		assert.Equal(t, true, out["changed"], "%s link %s", from, to)
		callJSON(t, s, "crm_"+to+"_link", map[string]any{"id": toID, "other": from, "other_id": fromID}, &out)
		assert.Equal(t, false, out["changed"], "the other side is the same link")
		_, isErr := call(t, s, "crm_"+to+"_unlink", map[string]any{"id": toID, "other": from, "other_id": fromID})
		require.False(t, isErr)
		_, isErr = call(t, s, "crm_"+from+"_unlink", map[string]any{"id": fromID, "other": to, "other_id": toID})
		assert.True(t, isErr, "not linked any more")
	}

	_, isErr := call(t, s, "crm_org_link", map[string]any{"id": org["id"], "other": "task", "other_id": task["id"]})
	assert.True(t, isErr, "an organization has no link to a task")

	var tagged map[string]any
	text, isErr := call(t, s, "crm_tag_apply", map[string]any{"entity_type": "deal", "entity_id": deal["id"], "tag": "hot"})
	require.False(t, isErr, text)
	text, isErr = call(t, s, "crm_tag_remove", map[string]any{"entity_type": "deal", "entity_id": deal["id"], "tag": "hot"})
	require.False(t, isErr, text)
	_ = tagged
}
