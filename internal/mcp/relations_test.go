package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelationToolsAreRegistered(t *testing.T) {
	names := toolNames(setupTestServer(t).ListTools())
	for _, n := range []string{"crm_relation_kind_list", "crm_relation_kind_add", "crm_relation_kind_remove", "crm_links", "crm_link_roles", "crm_task_update"} {
		assert.Contains(t, names, n)
	}
}

func TestLinkToolsCarryAttributesAndLinksListThem(t *testing.T) {
	s := setupTestServer(t)
	var person, other, org, org2 map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &person)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Bob"}, &other)
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Acme"}, &org)
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Globex"}, &org2)

	var out map[string]any
	callJSON(t, s, "crm_person_link", map[string]any{"id": person["id"], "other": "org", "other_id": org["id"], "role": "CTO", "from": "2019-03"}, &out)
	assert.Equal(t, true, out["created"])
	assert.Equal(t, "membership", out["relation"])
	callJSON(t, s, "crm_person_link", map[string]any{"id": person["id"], "other": "org", "other_id": org2["id"], "role": "Advisor"}, &out)
	callJSON(t, s, "crm_person_link", map[string]any{"id": person["id"], "other": "org", "other_id": org2["id"], "primary": true}, &out)
	assert.Equal(t, true, out["updated"])
	assert.NotEmpty(t, out["demoted"])

	var links []map[string]any
	callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": person["id"]}, &links)
	require.Len(t, links, 2)
	assert.Equal(t, "Globex", links[0]["name"], "the primary comes first")
	assert.Equal(t, true, links[0]["primary"])
	assert.Equal(t, "CTO", links[1]["role"])
	assert.Equal(t, "2019-03", links[1]["from"])

	// The organization lists its members from the other side.
	callJSON(t, s, "crm_links", map[string]any{"kind": "org", "id": org["id"]}, &links)
	require.Len(t, links, 1)
	assert.Equal(t, "person", links[0]["kind"])

	// Person to person with a kind and the inverse label.
	callJSON(t, s, "crm_person_link", map[string]any{"id": person["id"], "other": "person", "other_id": other["id"], "kind": "parent"}, &out)
	callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": other["id"]}, &links)
	var labels []string
	for _, l := range links {
		if l["kind"] == "person" {
			labels = append(labels, l["label"].(string))
		}
	}
	assert.Equal(t, []string{"parent"}, labels)
	_, isErr := call(t, s, "crm_person_link", map[string]any{"id": person["id"], "other": "person", "other_id": other["id"], "kind": "nemesis"})
	assert.True(t, isErr)
	_, isErr = call(t, s, "crm_person_link", map[string]any{"id": person["id"], "other": "org", "other_id": org["id"], "kind": "friend"})
	assert.True(t, isErr, "a membership has no kind")

	// A second stay needs new=true, unlink takes a link id.
	_, isErr = call(t, s, "crm_person_link", map[string]any{"id": person["id"], "other": "org", "other_id": org["id"], "role": "Director", "new": true})
	require.False(t, isErr)
	_, isErr = call(t, s, "crm_person_link", map[string]any{"id": person["id"], "other": "org", "other_id": org["id"], "role": "X"})
	assert.True(t, isErr, "two links join the pair, so the caller must say which")
	callJSON(t, s, "crm_person_unlink", map[string]any{"id": person["id"], "other": "org", "other_id": org["id"], "link_id": 1}, &out)
	callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": person["id"]}, &links)
	assert.Len(t, links, 3, "two organizations and the related person minus the removed link plus the new stay")

	var roles []string
	callJSON(t, s, "crm_link_roles", map[string]any{}, &roles)
	assert.Contains(t, roles, "Advisor")
}

func TestRelationKindTools(t *testing.T) {
	s := setupTestServer(t)
	var kinds []map[string]any
	callJSON(t, s, "crm_relation_kind_list", map[string]any{"object_type": "person"}, &kinds)
	assert.Len(t, kinds, 9)
	var k map[string]any
	callJSON(t, s, "crm_relation_kind_add", map[string]any{"object_type": "person", "name": "godparent", "inverse_name": "godchild"}, &k)
	assert.Equal(t, false, k["symmetric"])
	_, isErr := call(t, s, "crm_relation_kind_add", map[string]any{"object_type": "person", "name": "friend"})
	assert.True(t, isErr)
	_, isErr = call(t, s, "crm_relation_kind_remove", map[string]any{"object_type": "person", "name": "friend"})
	assert.True(t, isErr, "built in")
	_, isErr = call(t, s, "crm_relation_kind_remove", map[string]any{"object_type": "person", "name": "godparent"})
	assert.False(t, isErr)
}

func TestTaskUpdateClearsDueAndDealsCloseWithTheStage(t *testing.T) {
	s := setupTestServer(t)
	var task, deal map[string]any
	callJSON(t, s, "crm_task_create", map[string]any{"title": "Call", "due": "2030-01-01"}, &task)
	assert.Equal(t, "2030-01-01", task["due_at"])
	var out map[string]any
	callJSON(t, s, "crm_task_update", map[string]any{"id": task["id"], "due": ""}, &out)
	assert.Nil(t, out["due_at"])

	callJSON(t, s, "crm_deal_create", map[string]any{"title": "Site"}, &deal)
	callJSON(t, s, "crm_deal_update", map[string]any{"id": deal["id"], "stage": "won"}, &out)
	assert.NotNil(t, out["closed_at"])
	out = nil
	callJSON(t, s, "crm_deal_update", map[string]any{"id": deal["id"], "stage": "lead"}, &out)
	assert.Nil(t, out["closed_at"])
}

func TestNotesAttachToOrganizationsAndDeals(t *testing.T) {
	s := setupTestServer(t)
	var org, deal, person map[string]any
	callJSON(t, s, "crm_org_create", map[string]any{"name": "Acme"}, &org)
	callJSON(t, s, "crm_deal_create", map[string]any{"title": "Site"}, &deal)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Jane"}, &person)
	var note map[string]any
	callJSON(t, s, "crm_interaction_log", map[string]any{"type": "note", "content": "kickoff", "org_ids": []any{org["id"]}, "deal_ids": []any{deal["id"]}}, &note)
	assert.Equal(t, []any{org["id"]}, note["org_ids"])
	var list []map[string]any
	callJSON(t, s, "crm_interaction_list", map[string]any{"org_id": org["id"]}, &list)
	assert.Len(t, list, 1)
	callJSON(t, s, "crm_interaction_list", map[string]any{"deal_id": deal["id"]}, &list)
	assert.Len(t, list, 1)
	callJSON(t, s, "crm_interaction_list", map[string]any{"person_id": person["id"]}, &list)
	assert.Empty(t, list)

	var ctxOut json.RawMessage
	callJSON(t, s, "crm_context", map[string]any{"person_id": person["id"]}, &ctxOut)
	assert.NotEmpty(t, ctxOut)
}

func TestReferredByRendersFromTheViewingSideInMCP(t *testing.T) {
	s := setupTestServer(t)
	var a, b, out map[string]any
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ann"}, &a)
	callJSON(t, s, "crm_person_create", map[string]any{"first_name": "Ben"}, &b)
	callJSON(t, s, "crm_person_link", map[string]any{"id": a["id"], "other": "person", "other_id": b["id"], "kind": "referred-by"}, &out)
	var links []map[string]any
	callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": a["id"]}, &links)
	require.Len(t, links, 1)
	assert.Equal(t, "referred-by", links[0]["label"])
	callJSON(t, s, "crm_links", map[string]any{"kind": "person", "id": b["id"]}, &links)
	assert.Equal(t, "referred", links[0]["label"])
}
