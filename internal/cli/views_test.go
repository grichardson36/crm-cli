package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func viewList(t *testing.T, s string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &rows), s)
	return rows
}

func viewNames(rows []map[string]any) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r["name"].(string))
	}
	return out
}

func TestViewsCommands(t *testing.T) {
	home := t.TempDir()
	run := func(args ...string) (string, string, int) {
		return keysRun(t, home, append([]string{"views"}, args...)...)
	}
	must := func(args ...string) string {
		t.Helper()
		stdout, stderr, code := run(args...)
		require.Equal(t, 0, code, "crm views %v: %s", args, stderr)
		return stdout
	}

	t.Run("the three examples are listed, off and marked", func(t *testing.T) {
		rows := viewList(t, must("list", "-f", "json"))
		assert.Equal(t, []string{"Overdue follow-up", "Open deals by stage", "Tasks due this week"}, viewNames(rows))
		for _, r := range rows {
			assert.Equal(t, true, r["example"])
			assert.Equal(t, false, r["tabs"])
		}
		assert.Equal(t, "last contacted before 90 days ago", rows[0]["filter"])
	})

	t.Run("save, show and run", func(t *testing.T) {
		for _, a := range [][]string{
			{"person", "add", "Ann Alder", "--company", "Initech"},
			{"person", "add", "Bob Brown", "--company", "Globex"},
			{"person", "add", "Cy Cole"},
			{"person", "contacted", "1", "--at", "2020-01-02"},
			{"person", "contacted", "2", "--at", "2099-01-01"},
		} {
			_, stderr, code := keysRun(t, home, a...)
			require.Equal(t, 0, code, stderr)
		}
		out := must("save", "Old contacts", "--kind", "person", "--where", "last_contacted before 90 days ago", "--sort", "name:desc",
			"--column", "company", "--tabs", "-f", "json")
		saved := viewList(t, out)[0]
		assert.Equal(t, "Old contacts", saved["name"])
		assert.Equal(t, true, saved["tabs"])
		assert.Equal(t, "last contacted before 90 days ago", saved["filter"])

		shown := viewList(t, must("show", "old contacts", "-f", "json"))[0]
		q := shown["query"].(map[string]any)
		assert.Equal(t, "last_contacted", q["filter"].([]any)[0].(map[string]any)["field"])

		rows := viewList(t, must("run", "Old contacts", "-f", "json"))
		require.Len(t, rows, 1)
		assert.Equal(t, "Ann Alder", rows[0]["name"])
		assert.Equal(t, "Initech", rows[0]["company"], "the optional column is printed")

		rows = viewList(t, must("run", "Overdue follow-up", "-f", "json"))
		assert.Equal(t, []string{"Ann Alder"}, viewNames(rows), "the example view runs")

		table, _, _ := run("run", "Old contacts", "-f", "table")
		assert.Contains(t, table, "Ann Alder")
		ids := must("run", "Old contacts", "-q")
		assert.Equal(t, "1", strings.TrimSpace(ids))
	})

	t.Run("edit changes the filter, the group and the tab", func(t *testing.T) {
		out := must("edit", "Old contacts", "--add-where", "company is Initech|Globex", "--group-by", "company", "--tabs=false", "-f", "json")
		v := viewList(t, out)[0]
		assert.Equal(t, false, v["tabs"])
		assert.Equal(t, "last contacted before 90 days ago, company is Initech or Globex", v["filter"])
		assert.Equal(t, "sorted by name desc, grouped by company", v["order"])
		rows := viewList(t, must("run", "Old contacts", "-f", "json"))
		assert.Equal(t, "Initech", rows[0]["group"])
		out = must("edit", "Old contacts", "--clear-where", "--group-by", "", "-f", "json")
		assert.Equal(t, "everything", viewList(t, out)[0]["filter"])
		_, stderr, code := run("edit", "Old contacts")
		assert.Equal(t, 2, code, "nothing to change is a usage error: "+stderr)
		must("edit", "Old contacts", "--where", "last_contacted before 90 days ago")
	})

	t.Run("rename, move, delete and restore", func(t *testing.T) {
		must("rename", "Old contacts", "Cold people")
		rows := viewList(t, must("list", "-f", "json"))
		assert.Contains(t, viewNames(rows), "Cold people")
		must("move", "Cold people", "up")
		rows = viewList(t, must("list", "-f", "json"))
		assert.Equal(t, "Cold people", viewNames(rows)[2])

		must("delete", "Cold people")
		assert.NotContains(t, viewNames(viewList(t, must("list", "-f", "json"))), "Cold people")
		assert.Equal(t, []string{"Cold people"}, viewNames(viewList(t, must("list", "--deleted", "-f", "json"))))
		_, stderr, code := run("run", "Cold people")
		assert.Equal(t, 4, code, "a deleted view does not run: "+stderr)
		_, _, code = run("delete", "Cold people")
		assert.Equal(t, 4, code, "already deleted")
		must("restore", "Cold people")
		assert.Contains(t, viewNames(viewList(t, must("list", "-f", "json"))), "Cold people")
	})

	t.Run("errors carry the validation codes and exit codes", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			code int
			want string
		}{
			{"unknown field", []string{"save", "X", "--kind", "person", "--where", "shoe_size is 9"}, 2, "E_QUERY_FIELD_UNKNOWN"},
			{"unknown operator", []string{"save", "X", "--kind", "person", "--where", "company matches Acme"}, 2, "E_QUERY_OPERATOR_INVALID"},
			{"bad value", []string{"save", "X", "--kind", "deal", "--where", "value > lots"}, 2, "E_QUERY_VALUE_INVALID"},
			{"bad kind", []string{"save", "X", "--kind", "widget"}, 2, "E_VIEW_KIND_UNKNOWN"},
			{"empty name", []string{"save", " ", "--kind", "person"}, 2, "E_VIEW_NAME_INVALID"},
			{"bad sort", []string{"save", "X", "--kind", "person", "--sort", "organization"}, 2, "E_QUERY_SORT_INVALID"},
			{"bad group", []string{"save", "X", "--kind", "person", "--group-by", "email"}, 2, "E_QUERY_GROUP_INVALID"},
			{"hostile where", []string{"save", "X", "--kind", "person", "--where", "company is 1; DROP TABLE people"}, 0, ""},
			{"duplicate name", []string{"save", "overdue follow-up", "--kind", "person"}, 4, ""},
			{"missing view", []string{"show", "nope"}, 3, ""},
			{"no kind", []string{"save", "Y"}, 2, "E_VIEW_KIND_UNKNOWN"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, stderr, code := run(append(tt.args, "-f", "json")...)
				assert.Equal(t, tt.code, code, stderr)
				if tt.want != "" {
					assert.Contains(t, stderr, tt.want)
				}
			})
		}
		stdout, _, code := keysRun(t, home, "person", "list", "-f", "json")
		require.Equal(t, 0, code, "the people table survived the hostile condition")
		assert.Contains(t, stdout, "Ann Alder")
	})

	t.Run("fields lists operators by type", func(t *testing.T) {
		out := must("fields", "deal", "-f", "json")
		byField := map[string]map[string]any{}
		for _, r := range viewList(t, out) {
			byField[r["field"].(string)] = r
		}
		assert.Equal(t, "number", byField["value"]["type"])
		assert.Equal(t, "= < > between", byField["value"]["operators"])
		assert.Contains(t, byField["stage"]["options"], "(your list)")
		_, _, code := run("fields", "widget")
		assert.Equal(t, 2, code)
	})
}

func TestViewsExportImportRoundTrip(t *testing.T) {
	home := t.TempDir()
	run := func(args ...string) (string, string, int) {
		return keysRun(t, home, args...)
	}
	for _, a := range [][]string{
		{"org", "add", "Initech"},
		{"person", "add", "Ann Alder", "--company", "Initech"},
		{"person", "tag", "1", "client"},
	} {
		_, stderr, code := run(a...)
		require.Equal(t, 0, code, "%v: %s", a, stderr)
	}
	_, stderr, code := run("views", "save", "Initech clients", "--kind", "person", "--where", "organization linked_to Initech", "--where", "tag has client|lead",
		"--where", "last_contacted before 90 days ago", "--sort", "name", "--group-by", "tier", "--column", "email", "--search", "ann")
	require.Equal(t, 0, code, stderr)

	file := filepath.Join(home, "initech.toml")
	_, stderr, code = run("views", "export", "Initech clients", "-o", file)
	require.Equal(t, 0, code, stderr)
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, `name = "Initech clients"`)
	assert.Contains(t, text, `kind = "person"`)
	assert.Contains(t, text, `values = ["Initech"]`, "a relation is written as its name, not as a uuid")
	assert.NotContains(t, text, "uuid")
	assert.Regexp(t, `(?m)^version = 1$`, text)

	t.Run("import into another database gives the same view", func(t *testing.T) {
		other := t.TempDir()
		for _, a := range [][]string{{"org", "add", "Initech"}} {
			_, stderr, code := keysRun(t, other, a...)
			require.Equal(t, 0, code, stderr)
		}
		stdout, stderr, code := keysRun(t, other, "views", "import", file, "-f", "json")
		require.Equal(t, 0, code, stderr)
		imported := viewList(t, stdout)[0]
		assert.Equal(t, "Initech clients", imported["name"])
		assert.Equal(t, false, imported["tabs"])
		mine, _, _ := run("views", "show", "Initech clients", "-f", "json")
		orig := viewList(t, mine)[0]
		// Same words, same sort and group. The relation resolved to the other database's own uuid.
		assert.Equal(t, orig["filter"], imported["filter"])
		assert.Equal(t, orig["order"], imported["order"])
		oq := orig["query"].(map[string]any)
		iq := imported["query"].(map[string]any)
		assert.Equal(t, oq["sort"], iq["sort"])
		assert.Equal(t, oq["group_by"], iq["group_by"])
		assert.Equal(t, oq["columns"], iq["columns"])
		assert.Equal(t, oq["search"], iq["search"])
	})
	t.Run("import under a new name, and a taken name is a conflict", func(t *testing.T) {
		_, stderr, code := run("views", "import", file)
		assert.Equal(t, 4, code, stderr)
		stdout, stderr, code := run("views", "import", file, "--name", "Copy", "--tabs", "-f", "json")
		require.Equal(t, 0, code, stderr)
		assert.Equal(t, true, viewList(t, stdout)[0]["tabs"])
	})
	t.Run("a relation that does not exist in the other database is refused", func(t *testing.T) {
		_, stderr, code := keysRun(t, t.TempDir(), "views", "import", file, "-f", "json")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "E_QUERY_VALUE_INVALID")
	})
	t.Run("a file with an unknown key or version is refused", func(t *testing.T) {
		bad := filepath.Join(home, "bad.toml")
		require.NoError(t, os.WriteFile(bad, []byte("version = 1\nname = \"B\"\nkind = \"person\"\nsql = \"select 1\"\n"), 0o644))
		_, stderr, code := run("views", "import", bad, "-f", "json")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "sql")
		require.NoError(t, os.WriteFile(bad, []byte("version = 9\nname = \"B\"\nkind = \"person\"\n"), 0o644))
		_, _, code = run("views", "import", bad)
		assert.Equal(t, 2, code)
		require.NoError(t, os.WriteFile(bad, []byte("not toml ==="), 0o644))
		_, _, code = run("views", "import", bad)
		assert.Equal(t, 2, code)
		_, _, code = run("views", "import", filepath.Join(home, "missing.toml"))
		assert.Equal(t, 2, code)
	})
	t.Run("export to standard output", func(t *testing.T) {
		stdout, _, code := run("views", "export", "Overdue follow-up")
		require.Equal(t, 0, code)
		assert.Contains(t, stdout, `field = "last_contacted"`)
		assert.Contains(t, stdout, `values = ["90 days ago"]`)
	})
}
