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

func TestMergeLogUndoRedoAcrossSeparateRuns(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, db string)
		merge []string
		gone  string // what the list shows only while the merge is not applied
		list  []string
	}{
		{"person merge", func(t *testing.T, db string) {
			crm(t, db, "person", "add", "Jane Smith", "--email", "jane@x.com")
			crm(t, db, "person", "add", "Janet Smith", "--email", "janet@x.com")
		}, []string{"person", "merge", "1", "2"}, "Janet", []string{"person", "list", "-f", "json"}},
		{"organization merge", func(t *testing.T, db string) {
			crm(t, db, "org", "add", "Blue Origin")
			crm(t, db, "org", "add", "BLUE ORIGIN", "--allow-duplicate")
		}, []string{"org", "merge", "1", "2", "--apply"}, "BLUE ORIGIN", []string{"org", "list", "-f", "json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "crm.db")
			tc.setup(t, dbPath)
			_, stderr, code := crm(t, dbPath, tc.merge...)
			assert.Equal(t, 0, code, stderr)
			assert.Contains(t, stderr, "crm merge undo 1")
			out, _, _ := crm(t, dbPath, tc.list...)
			assert.NotContains(t, out, tc.gone)

			log, _, code := crm(t, dbPath, "merge", "log", "-f", "json")
			assert.Equal(t, 0, code)
			assert.Contains(t, log, `"status":"merged"`)

			_, stderr, code = crm(t, dbPath, "merge", "undo", "1")
			assert.Equal(t, 0, code, stderr)
			out, _, _ = crm(t, dbPath, tc.list...)
			assert.Contains(t, out, tc.gone, "undo from a later run brings the merged record back")
			log, _, _ = crm(t, dbPath, "merge", "log", "-f", "json")
			assert.Contains(t, log, `"status":"undone"`)

			_, _, code = crm(t, dbPath, "merge", "undo", "1")
			assert.Equal(t, 4, code, "undoing twice is a conflict")
			_, _, code = crm(t, dbPath, "merge", "undo", "99")
			assert.Equal(t, 3, code)

			_, _, code = crm(t, dbPath, "merge", "redo", "1")
			assert.Equal(t, 0, code)
			out, _, _ = crm(t, dbPath, tc.list...)
			assert.NotContains(t, out, tc.gone)
		})
	}
}

func TestOrgDupesDismissAndUndismiss(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "org", "add", "Blue Origin")
	crm(t, dbPath, "org", "add", "BLUE ORIGIN", "--allow-duplicate")
	out, _, _ := crm(t, dbPath, "org", "dupes", "-f", "json")
	assert.Contains(t, out, "Blue Origin")

	_, stderr, code := crm(t, dbPath, "org", "dupes", "dismiss", "2", "1")
	assert.Equal(t, 0, code, stderr)
	out, _, _ = crm(t, dbPath, "org", "dupes", "-f", "json")
	assert.NotContains(t, out, "Blue Origin", "a dismissed pair is remembered across runs")
	out, _, _ = crm(t, dbPath, "org", "dupes", "--dismissed", "-f", "json")
	assert.Contains(t, out, "Blue Origin")

	_, _, code = crm(t, dbPath, "org", "dupes", "undismiss", "1", "2")
	assert.Equal(t, 0, code)
	out, _, _ = crm(t, dbPath, "org", "dupes", "-f", "json")
	assert.Contains(t, out, "Blue Origin")
	_, _, code = crm(t, dbPath, "org", "dupes", "undismiss", "1", "2")
	assert.Equal(t, 3, code)
}

func TestTagDeleteUntagsAndTagLinkRevives(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")
	crm(t, dbPath, "tag", "apply", "person", "1", "vip")
	_, stderr, code := crm(t, dbPath, "tag", "delete", "vip", "--yes")
	assert.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "untagged 1")
	out, _, _ := crm(t, dbPath, "tag", "list", "-f", "json")
	assert.NotContains(t, out, "vip")
	_, _, code = crm(t, dbPath, "tag", "delete", "vip", "--yes")
	assert.Equal(t, 3, code, "a tag nothing carries is gone")
	crm(t, dbPath, "person", "link", "tag", "1", "vip")
	out, _, _ = crm(t, dbPath, "tag", "list", "-f", "json")
	assert.Contains(t, out, "vip")
}
