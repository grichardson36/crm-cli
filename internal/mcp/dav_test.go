package mcp_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	crmmcp "github.com/jdanielnd/crm-cli/internal/mcp"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDavTools(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	s := crmmcp.NewServer(d, "test")
	names := toolNames(s.ListTools())
	for _, n := range []string{"crm_dav_info", "crm_dav_conflicts", "crm_dav_conflict_undo", "crm_dav_status"} {
		assert.Contains(t, names, n)
	}

	var info map[string]any
	callJSON(t, s, "crm_dav_info", map[string]any{"base_url": "https://c.example.com", "user": "grant"}, &info)
	assert.Equal(t, "https://c.example.com/dav/grant/contacts/", info["address_book_url"])
	assert.NotEmpty(t, info["steps"])

	ctx := context.Background()
	company := "Acme"
	p, err := repo.NewPersonRepo(d).Create(ctx, model.CreatePersonInput{FirstName: "Jane", Company: &company})
	require.NoError(t, err)

	var st map[string]any
	callJSON(t, s, "crm_dav_status", map[string]any{}, &st)
	assert.EqualValues(t, 1, st["contacts_exposed"])

	var list []map[string]any
	callJSON(t, s, "crm_dav_conflicts", map[string]any{}, &list)
	assert.Empty(t, list)

	// The phone changed the company to Beta and won. Undo puts Acme back.
	beta := "Beta"
	_, err = repo.NewPersonRepo(d).Update(ctx, p.ID, model.UpdatePersonInput{Company: &beta})
	require.NoError(t, err)
	acme := "Acme"
	c, err := repo.NewDavRepo(d).AddConflict(ctx, p.ID, "company", &acme, &beta, "device")
	require.NoError(t, err)

	callJSON(t, s, "crm_dav_conflicts", map[string]any{}, &list)
	require.Len(t, list, 1)
	assert.Equal(t, "company", list[0]["field"])

	var undone map[string]any
	callJSON(t, s, "crm_dav_conflict_undo", map[string]any{"id": c.ID}, &undone)
	assert.Equal(t, true, undone["undone"])
	got, err := repo.NewPersonRepo(d).FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Acme", *got.Company)

	callJSON(t, s, "crm_dav_conflicts", map[string]any{}, &list)
	assert.Empty(t, list)
	callJSON(t, s, "crm_dav_conflicts", map[string]any{"include_undone": true}, &list)
	assert.Len(t, list, 1)

	text, isErr := call(t, s, "crm_dav_conflict_undo", map[string]any{"id": c.ID})
	assert.True(t, isErr)
	assert.Contains(t, text, "conflict")
	text, isErr = call(t, s, "crm_dav_conflict_undo", map[string]any{"id": 999})
	assert.True(t, isErr)
	assert.Equal(t, "not found", text)
}
