package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Owner decisions 2026-10-27, verbs and undo branch.

func TestPersonDeleteVerbAndHiddenArchiveAlias(t *testing.T) {
	for _, verb := range []string{"delete", "archive"} {
		t.Run(verb, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "crm.db")
			crm(t, dbPath, "person", "add", "Jane Smith")
			_, stderr, code := crm(t, dbPath, "person", verb, "1")
			assert.Equal(t, 0, code, stderr)
			stdout, _, _ := crm(t, dbPath, "person", "list", "-f", "json")
			assert.NotContains(t, stdout, "Jane")
			_, _, code = crm(t, dbPath, "person", "restore", "1")
			assert.Equal(t, 0, code)
		})
	}
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	help, _, _ := crm(t, dbPath, "person", "--help")
	assert.Contains(t, help, "delete")
	assert.NotContains(t, help, "archive ", "archive is a hidden deprecated alias")
}

func TestOldRelateNamesPointToLink(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"relate", []string{"person", "relate", "1", "2", "--type", "friend"}, "crm person link person"},
		{"relationships", []string{"person", "relationships", "1"}, "crm person show"},
		{"unrelate", []string{"person", "unrelate", "1"}, "crm person unlink person"},
	}
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Ann")
	crm(t, dbPath, "person", "add", "Bob")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, _ := crm(t, dbPath, tc.args...)
			assert.Contains(t, stderr, "deprecated")
			assert.Contains(t, stderr, tc.want)
			assert.NotContains(t, stdout, "deprecated", "the pointer goes to stderr only")
		})
	}
}
