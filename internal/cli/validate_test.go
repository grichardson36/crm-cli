package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The CLI maps framework codes to the existing exit codes and prints code, field and fix on stderr.
func TestCLIUsesTheValidationFramework(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code string
		fld  string
	}{
		{"person email", []string{"person", "add", "Jane", "--email", "nope"}, "E_EMAIL_INVALID", "email"},
		{"org website", []string{"org", "add", "Acme", "--website", "not a url"}, "E_URL_INVALID", "website"},
		{"deal stage", []string{"deal", "add", "D", "--stage", "maybe"}, "E_CHOICE_INVALID", "stage"},
		{"task priority", []string{"task", "add", "T", "--priority", "zzz"}, "E_CHOICE_INVALID", "priority"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "crm.db")
			_, stderr, code := crm(t, dbPath, tt.args...)
			assert.Equal(t, 2, code, stderr)
			assert.Contains(t, stderr, tt.code)
			assert.Contains(t, stderr, tt.fld)
		})
	}
}

func TestCLINoteLengthAndEditUseTheFramework(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")
	_, stderr, code := crm(t, dbPath, "person", "edit", "1", "--email", "jane@")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "E_EMAIL_INVALID")
	_, stderr, code = crm(t, dbPath, "note", "add", "--direction", "sideways", "hello")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "E_CHOICE_INVALID")
}

func TestCLISuggestionsAreHintsOnStderrAndNeverFail(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	stdout, stderr, code := crm(t, dbPath, "person", "add", "Jane Smith", "--company", "+1 (555) 010-0100", "-f", "json")
	assert.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "S_PHONE_IN_TEXT_FIELD")
	assert.Contains(t, stderr, "phone")
	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Equal(t, "+1 (555) 010-0100", data[0]["company"], "the value is kept, only a hint is printed")

	_, stderr, code = crm(t, dbPath, "org", "add", "www.acme.com")
	assert.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "S_URL_IN_TEXT_FIELD")
}

func TestCLIJSONErrorCarriesTheStructuredIssue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	_, stderr, code := crm(t, dbPath, "person", "add", "Jane", "--email", "nope", "-f", "json")
	assert.Equal(t, 2, code)
	var body struct {
		Error struct {
			Code   string `json:"code"`
			Issues []struct {
				Code, Field, Expected, Received, Fix string
			} `json:"issues"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(stderr), &body), stderr)
	assert.Equal(t, "validation", body.Error.Code)
	require.Len(t, body.Error.Issues, 1)
	assert.Equal(t, "E_EMAIL_INVALID", body.Error.Issues[0].Code)
	assert.Equal(t, "nope", body.Error.Issues[0].Received)
	assert.NotEmpty(t, body.Error.Issues[0].Fix)
}
