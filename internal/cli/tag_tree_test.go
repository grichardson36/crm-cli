package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// treeDB has Ann tagged work and work/clients/acme, Ben tagged work/clients/beta and home.
func treeDB(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "crm.db")
	for _, s := range [][]string{
		{"person", "add", "Ann"}, {"person", "add", "Ben"},
		{"tag", "apply", "person", "1", "work"}, {"tag", "apply", "person", "1", "work/clients/acme"},
		{"tag", "apply", "person", "2", "work/clients/beta"}, {"tag", "apply", "person", "2", "home"},
	} {
		_, stderr, code := crm(t, db, s...)
		require.Equal(t, 0, code, "%v: %s", s, stderr)
	}
	return db
}

func tagNamesOf(t *testing.T, db string) []string {
	t.Helper()
	out, _, _ := crm(t, db, "tag", "list", "-f", "json")
	var names []string
	for _, r := range jsonRows(t, out) {
		names = append(names, r["name"].(string))
	}
	return names
}

func TestTagTreeCommand(t *testing.T) {
	db := treeDB(t)
	out, stderr, code := crm(t, db, "tag", "tree", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var nodes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &nodes))
	var paths []string
	by := map[string]map[string]any{}
	for _, n := range nodes {
		paths = append(paths, n["path"].(string))
		by[n["path"].(string)] = n
	}
	assert.Equal(t, []string{"home", "work", "work/clients", "work/clients/acme", "work/clients/beta"}, paths)
	assert.Equal(t, true, by["work/clients"]["virtual"])
	assert.EqualValues(t, 2, by["work"]["total"], "Ann and Ben")
	assert.EqualValues(t, 1, by["work"]["own"])
	assert.EqualValues(t, 3, by["work/clients/acme"]["depth"])

	text, _, code := crm(t, db, "tag", "tree", "-f", "table")
	require.Equal(t, 0, code)
	assert.Contains(t, text, "home  1\n")
	assert.Contains(t, text, "work  1 (2)\n")
	assert.Contains(t, text, "  clients  (2)\n", "a virtual parent shows only the total")
	assert.Contains(t, text, "    acme  1\n")
}

func TestTagNestUnnestRenameDeleteCommands(t *testing.T) {
	tests := []struct {
		name string
		args [][]string
		want []string
		code []int
	}{
		{"nest under a parent", [][]string{{"tag", "nest", "home", "work/clients", "--under", "area"}}, []string{"area/clients/acme", "area/clients/beta", "area/home", "work"}, []int{0}},
		{"nest to the top level", [][]string{{"tag", "nest", "work/clients/beta", "--under", ""}}, []string{"beta", "home", "work", "work/clients/acme"}, []int{0}},
		{"nest needs --under", [][]string{{"tag", "nest", "home"}}, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, []int{2}},
		{"nest dry run", [][]string{{"tag", "nest", "home", "--under", "area", "--dry-run"}}, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, []int{0}},
		{"nest into itself", [][]string{{"tag", "nest", "work", "--under", "work/clients"}}, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, []int{2}},
		{"unnest", [][]string{{"tag", "unnest", "work/clients/acme"}}, []string{"home", "work", "work/acme", "work/clients/beta"}, []int{0}},
		{"unnest at the top", [][]string{{"tag", "unnest", "home"}}, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, []int{2}},
		{"rename carries the children", [][]string{{"tag", "rename", "work", "job"}}, []string{"home", "job", "job/clients/acme", "job/clients/beta"}, []int{0}},
		{"rename onto a tag is a conflict, then --merge", [][]string{{"tag", "rename", "work/clients/acme", "home"}, {"tag", "rename", "work/clients/acme", "home", "--merge"}},
			[]string{"home", "work", "work/clients/beta"}, []int{4, 0}},
		{"rename to a bad path", [][]string{{"tag", "rename", "work", "a//b"}}, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, []int{2}},
		{"merge takes the children along", [][]string{{"tag", "merge", "work", "home", "--apply"}}, []string{"home", "home/clients/acme", "home/clients/beta"}, []int{0}},
		{"merge preview changes nothing", [][]string{{"tag", "merge", "work", "home"}}, []string{"home", "work", "work/clients/acme", "work/clients/beta"}, []int{0}},
		{"delete a parent deletes the subtree", [][]string{{"tag", "delete", "work", "--yes"}}, []string{"home"}, []int{0}},
		{"delete only this tag", [][]string{{"tag", "delete", "work", "--only-this", "--yes"}}, []string{"home", "work/clients/acme", "work/clients/beta"}, []int{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := treeDB(t)
			for i, a := range tt.args {
				_, stderr, code := crm(t, db, a...)
				assert.Equal(t, tt.code[i], code, "%v: %s", a, stderr)
			}
			assert.Equal(t, tt.want, tagNamesOf(t, db))
		})
	}
}

func TestTagUnusedRespectsHierarchy(t *testing.T) {
	db := treeDB(t)
	_, _, code := crm(t, db, "tag", "remove", "person", "1", "work")
	require.Equal(t, 0, code)
	out, _, code := crm(t, db, "tag", "unused", "-f", "json")
	require.Equal(t, 0, code)
	var res map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.EqualValues(t, 0, res["count"], "work has no object of its own but children in use")
}
