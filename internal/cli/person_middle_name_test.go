package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-019, CLI door: add, edit, show and search carry the middle name, the display name does not.
func TestPersonMiddleName_AddEditShowSearch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")

	out, _, code := crm(t, dbPath, "person", "add", "Ann Smith", "--middle-name", "Marigold", "-f", "json")
	require.Equal(t, 0, code)
	row := jsonRows(t, out)[0]
	assert.Equal(t, "Marigold", row["middle_name"])
	assert.Equal(t, "Ann Smith", row["name"], "the display name stays First Last")

	out, _, code = crm(t, dbPath, "person", "show", "1", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "Marigold", jsonRows(t, out)[0]["middle_name"])

	out, _, code = crm(t, dbPath, "search", "Marigold", "-f", "json")
	require.Equal(t, 0, code)
	found := jsonRows(t, out)
	require.Len(t, found, 1)
	assert.Equal(t, "Ann Smith", found[0]["name"], "search results keep First Last")

	out, _, code = crm(t, dbPath, "person", "edit", "1", "--middle-name", "Quill", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "Quill", jsonRows(t, out)[0]["middle_name"])

	out, _, code = crm(t, dbPath, "search", "Marigold", "-f", "json")
	require.Equal(t, 0, code)
	assert.NotContains(t, out, "Ann", "the old value is no longer found")

	out, _, code = crm(t, dbPath, "person", "edit", "1", "--middle-name", "", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "", jsonRows(t, out)[0]["middle_name"], "an empty value clears it")

	out, _, code = crm(t, dbPath, "person", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "Ann Smith", jsonRows(t, out)[0]["name"])
}

func TestPersonMiddleName_ShowTableAndArchiveRestore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	_, _, code := crm(t, dbPath, "person", "add", "Ann Smith", "--middle-name", "Marigold")
	require.Equal(t, 0, code)

	out, _, code := crm(t, dbPath, "person", "show", "1", "-f", "table")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "Marigold")

	_, _, code = crm(t, dbPath, "person", "delete", "1")
	require.Equal(t, 0, code)
	out, _, _ = crm(t, dbPath, "search", "Marigold", "-f", "json")
	assert.NotContains(t, out, "Ann", "archived people are not found")
	_, _, code = crm(t, dbPath, "person", "restore", "1")
	require.Equal(t, 0, code)
	out, _, code = crm(t, dbPath, "search", "Marigold", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "Ann")
}
