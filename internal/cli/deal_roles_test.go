package cli_test

import (
	"encoding/json"
	"github.com/jdanielnd/crm-cli/internal/db"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func roleNames(t *testing.T, db string, extra ...string) []string {
	t.Helper()
	stdout, stderr, code := crm(t, db, append([]string{"deal", "roles", "list", "-f", "json"}, extra...)...)
	require.Equal(t, 0, code, stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r["name"].(string)
	}
	return out
}

func TestDealRolesList_Seeded(t *testing.T) {
	db := linkSeed(t)
	assert.Equal(t, []string{"champion", "decision maker", "economic buyer", "influencer", "blocker", "end user", "signer", "billing contact"}, roleNames(t, db))
}

func TestDealRolesManage(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
		wantHead []string
	}{
		{"add", []string{"add", "technical evaluator"}, 0, "", nil},
		{"add duplicate is a validation error with a code", []string{"add", "Champion"}, 2, "E_VOCAB_DUPLICATE", nil},
		{"add empty", []string{"add", " "}, 2, "E_VOCAB_INVALID", nil},
		{"rename", []string{"rename", "signer", "contract signer"}, 0, "", nil},
		{"rename to an existing name", []string{"rename", "signer", "blocker"}, 2, "E_VOCAB_DUPLICATE", nil},
		{"rename unknown", []string{"rename", "ghost", "x"}, 3, "", nil},
		{"remove unused", []string{"remove", "blocker"}, 0, "", nil},
		{"remove unknown", []string{"remove", "ghost"}, 3, "", nil},
		{"reorder names", []string{"reorder", "signer", "blocker"}, 0, "", []string{"signer", "blocker", "champion"}},
		{"reorder one role to a position", []string{"reorder", "signer", "--position", "2"}, 0, "", []string{"champion", "signer", "decision maker"}},
		{"reorder unknown", []string{"reorder", "ghost"}, 3, "", nil},
		{"position with two roles", []string{"reorder", "signer", "blocker", "--position", "1"}, 2, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := linkSeed(t)
			_, stderr, code := crm(t, db, append([]string{"deal", "roles"}, tt.args...)...)
			assert.Equal(t, tt.wantCode, code, stderr)
			if tt.wantErr != "" {
				assert.Contains(t, stderr, tt.wantErr)
			}
			if tt.wantHead != nil {
				assert.Equal(t, tt.wantHead, roleNames(t, db)[:len(tt.wantHead)])
			}
		})
	}
}

func TestDealRolesUseOnLinks(t *testing.T) {
	db := linkSeed(t)

	t.Run("an unknown role is refused with a code and a fix", func(t *testing.T) {
		_, stderr, code := crm(t, db, "deal", "link", "person", "1", "1", "--role", "kingmaker")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "E_VOCAB_UNKNOWN")
		assert.Contains(t, stderr, "crm deal roles add")
		assert.Contains(t, stderr, "--create")
		assert.NotContains(t, roleNames(t, db), "kingmaker")
	})
	t.Run("a listed role is stored in the listed spelling", func(t *testing.T) {
		_, stderr, code := crm(t, db, "deal", "link", "person", "1", "1", "--role", "CHAMPION")
		require.Equal(t, 0, code, stderr)
		stdout, _, _ := crm(t, db, "deal", "show", "1", "-f", "json")
		assert.Contains(t, stdout, `"role":"champion"`)
	})
	t.Run("from the person side the same check applies", func(t *testing.T) {
		_, stderr, code := crm(t, db, "person", "link", "deal", "2", "1", "--role", "kingmaker")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "E_VOCAB_UNKNOWN")
	})
	t.Run("--create adds the role", func(t *testing.T) {
		_, stderr, code := crm(t, db, "person", "link", "deal", "2", "1", "--role", "kingmaker", "--create")
		require.Equal(t, 0, code, stderr)
		assert.Contains(t, roleNames(t, db), "kingmaker")
	})
	t.Run("a membership role stays free text and has no --create", func(t *testing.T) {
		_, stderr, code := crm(t, db, "person", "link", "org", "1", "1", "--role", "Chief Whatever Officer")
		assert.Equal(t, 0, code, stderr)
		_, stderr, code = crm(t, db, "person", "link", "org", "1", "2", "--role", "x", "--create")
		assert.Equal(t, 2, code, stderr)
	})
	t.Run("remove of a used role asks for a decision", func(t *testing.T) {
		_, stderr, code := crm(t, db, "deal", "roles", "remove", "champion")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "E_VOCAB_IN_USE")
		assert.Contains(t, stderr, "--replace-with")
		_, stderr, code = crm(t, db, "deal", "roles", "remove", "champion", "--replace-with", "influencer")
		require.Equal(t, 0, code, stderr)
		stdout, _, _ := crm(t, db, "deal", "show", "1", "-f", "json")
		assert.Contains(t, stdout, `"role":"influencer"`)
	})
	t.Run("rename rewrites the links", func(t *testing.T) {
		_, stderr, code := crm(t, db, "deal", "roles", "rename", "influencer", "key influencer")
		require.Equal(t, 0, code, stderr)
		assert.Contains(t, stderr, "1 link(s) updated")
		stdout, _, _ := crm(t, db, "deal", "show", "1", "-f", "json")
		assert.Contains(t, stdout, `"role":"key influencer"`)
	})
	t.Run("keep leaves plain text", func(t *testing.T) {
		_, stderr, code := crm(t, db, "deal", "roles", "remove", "key influencer", "--keep")
		require.Equal(t, 0, code, stderr)
		stdout, _, _ := crm(t, db, "deal", "show", "1", "-f", "json")
		assert.Contains(t, stdout, `"role":"key influencer"`)
		assert.NotContains(t, roleNames(t, db), "key influencer")
		assert.Contains(t, roleNames(t, db, "--all"), "key influencer")
	})
	t.Run("replace-with and keep together are refused", func(t *testing.T) {
		_, _, code := crm(t, db, "deal", "roles", "remove", "signer", "--keep", "--replace-with", "blocker")
		assert.Equal(t, 2, code)
	})
}

func TestDealRolesBackfillCLI(t *testing.T) {
	dbPath := linkSeed(t)
	// Free-text roles from before the list existed are written straight into the table.
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO people (uuid, first_name) VALUES ('p3', 'Kim'), ('p4', 'Lee')`)
	require.NoError(t, err)
	for i, role := range []string{"Board Observer", "board observer", "champion", "Sponsor"} {
		_, err := d.Exec(`INSERT INTO deal_person (uuid, deal_id, person_id, role) VALUES (?, 1, ?, ?)`, "u"+role, i+1, role)
		require.NoError(t, err)
	}
	require.NoError(t, d.Close())

	run := func(args ...string) (map[string]any, string) {
		stdout, stderr, code := crm(t, dbPath, append([]string{"deal", "roles", "backfill", "-f", "json"}, args...)...)
		require.Equal(t, 0, code, stderr)
		var out map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &out), stdout)
		return out, stderr
	}

	before := roleNames(t, dbPath)
	plan, stderr := run()
	assert.Contains(t, stderr, "Dry run")
	assert.Equal(t, false, plan["applied"])
	assert.Equal(t, map[string]any{"rows": 4.0, "distinct": 3.0, "already_listed": 1.0, "removed_skipped": 0.0, "invalid_skipped": 0.0, "to_add": 2.0}, plan["counts"])
	assert.Equal(t, before, roleNames(t, dbPath), "the dry run writes nothing")

	done, stderr := run("--apply")
	assert.Contains(t, stderr, "Applied: added 2")
	assert.Equal(t, true, done["applied"])
	assert.Equal(t, append(append([]string{}, before...), "Board Observer", "Sponsor"), roleNames(t, dbPath))

	again, _ := run("--apply")
	assert.EqualValues(t, 0, again["counts"].(map[string]any)["to_add"], "a second run adds nothing")
}
