package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/agentdoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// crmEnv runs the binary without a --db flag and with HOME pointing at a temp dir, so a test can
// never reach the real ~/.crm. extra is KEY=VALUE pairs. CRM_DB is cleared unless given.
func crmEnv(t *testing.T, home string, extra []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	env := []string{}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "CRM_DB=") || strings.HasPrefix(e, "HOME=") || strings.HasPrefix(e, "USERPROFILE=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home)
	cmd.Env = append(env, extra...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run: %v", err)
		}
		code = ee.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

func jsonOut(t *testing.T, s string, v any) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(s), v), s)
}

// --- help ---

func TestHelpListsTopics(t *testing.T) {
	stdout, _, code := crm(t, ":memory:", "help")
	assert.Equal(t, 0, code)
	for _, topic := range []string{"overview", "duplicates", "keep-in-touch", "agents", "install"} {
		assert.Contains(t, stdout, topic)
	}
}

func TestHelpTopicAndJSON(t *testing.T) {
	stdout, _, code := crm(t, ":memory:", "help", "duplicates")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "# Duplicates, merge and dismiss")
	assert.Contains(t, stdout, "crm person merge")

	stdout, _, code = crm(t, ":memory:", "help", "duplicates", "-f", "json")
	assert.Equal(t, 0, code)
	var topic map[string]string
	jsonOut(t, stdout, &topic)
	assert.Equal(t, "duplicates", topic["name"])
	assert.NotEmpty(t, topic["title"])
	assert.NotEmpty(t, topic["summary"])
	assert.Contains(t, topic["body"], "crm dupes")

	stdout, _, code = crm(t, ":memory:", "help", "-f", "json")
	assert.Equal(t, 0, code)
	var list []map[string]string
	jsonOut(t, stdout, &list)
	assert.Len(t, list, 20)
	assert.Equal(t, "overview", list[0]["name"])
}

func TestHelpFallsThroughToCobra(t *testing.T) {
	stdout, _, code := crm(t, ":memory:", "help", "person", "add")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Add a new person")
	assert.Contains(t, stdout, "--email")
	assert.Contains(t, stdout, "Examples:", "examples from the metadata show in --help")

	stdout, _, code = crm(t, ":memory:", "help", "person")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Manage contacts")

	_, stderr, code := crm(t, ":memory:", "help", "nope")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unknown help topic or command")
}

func TestHelpSearch(t *testing.T) {
	stdout, _, code := crm(t, ":memory:", "help", "--search", "MERGE")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "duplicates:")
	assert.Contains(t, strings.ToLower(stdout), "merge")

	stdout, _, code = crm(t, ":memory:", "help", "--search", "merge", "-f", "json")
	assert.Equal(t, 0, code)
	var matches []map[string]any
	jsonOut(t, stdout, &matches)
	require.NotEmpty(t, matches)
	assert.NotEmpty(t, matches[0]["topic"])
	assert.NotZero(t, matches[0]["line"])
}

// --- schema ---

func TestSchemaJSON(t *testing.T) {
	stdout, _, code := crm(t, ":memory:", "schema", "-f", "json")
	require.Equal(t, 0, code)
	var m struct {
		Name     string `json:"name"`
		Commands []struct {
			Path        string   `json:"path"`
			Short       string   `json:"short"`
			Mutates     bool     `json:"mutates"`
			Destructive bool     `json:"destructive"`
			DryRun      bool     `json:"dry_run"`
			Examples    []string `json:"examples"`
			Flags       []struct {
				Name, Type, Default, Usage string
			} `json:"flags"`
		} `json:"commands"`
		ExitCodes []struct {
			Code int `json:"code"`
		} `json:"exit_codes"`
		ErrorFormat struct {
			Codes []string `json:"codes"`
		} `json:"error_format"`
		MCPTools   []string `json:"mcp_tools"`
		HelpTopics []string `json:"help_topics"`
	}
	jsonOut(t, stdout, &m)
	assert.Equal(t, "crm", m.Name)

	byPath := map[string]int{}
	for i, c := range m.Commands {
		byPath[c.Path] = i
		assert.NotEmpty(t, c.Short, c.Path)
	}
	for _, p := range []string{"person add", "person purge", "tag delete", "schema", "where", "init", "doctor", "agent", "docs man", "help", "dupes", "sync run"} {
		require.Contains(t, byPath, p)
	}
	assert.NotContains(t, byPath, "completion")

	add := m.Commands[byPath["person add"]]
	assert.True(t, add.Mutates)
	assert.False(t, add.Destructive)
	assert.True(t, add.DryRun)
	assert.NotEmpty(t, add.Examples)
	var flagNames []string
	for _, f := range add.Flags {
		flagNames = append(flagNames, f.Name)
	}
	assert.Contains(t, flagNames, "email")
	assert.Contains(t, flagNames, "dry-run")

	assert.True(t, m.Commands[byPath["person purge"]].Destructive)
	assert.True(t, m.Commands[byPath["tag delete"]].Destructive)
	assert.False(t, m.Commands[byPath["person list"]].Mutates)
	assert.False(t, m.Commands[byPath["where"]].Mutates)

	var codes []int
	for _, e := range m.ExitCodes {
		codes = append(codes, e.Code)
	}
	assert.Equal(t, []int{0, 1, 2, 3, 4, 10}, codes)
	assert.Contains(t, m.ErrorFormat.Codes, "not_found")
	assert.Contains(t, m.MCPTools, "crm_help")
	assert.Contains(t, m.MCPTools, "crm_schema")
	assert.Contains(t, m.MCPTools, "crm_person_create")
	assert.Contains(t, m.HelpTopics, "agents")
}

func TestSchemaTable(t *testing.T) {
	stdout, _, code := crm(t, ":memory:", "schema", "-f", "table")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "person add")
}

// --- dry-run and --yes ---

func TestDryRunWritesNothing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith", "-f", "json")

	stdout, _, code := crm(t, dbPath, "person", "edit", "1", "--title", "CTO", "--dry-run", "-f", "json")
	require.Equal(t, 0, code)
	var plan struct {
		DryRun  bool           `json:"dry_run"`
		Command string         `json:"command"`
		Args    []string       `json:"args"`
		Flags   map[string]any `json:"flags"`
		Targets []struct {
			Entity string `json:"entity"`
			ID     int64  `json:"id"`
			Label  string `json:"label"`
		} `json:"targets"`
	}
	jsonOut(t, stdout, &plan)
	assert.True(t, plan.DryRun)
	assert.Equal(t, "person edit", plan.Command)
	assert.Equal(t, []string{"1"}, plan.Args)
	assert.Equal(t, "CTO", plan.Flags["title"])
	require.Len(t, plan.Targets, 1)
	assert.Equal(t, "Jane Smith", plan.Targets[0].Label)

	stdout, _, _ = crm(t, dbPath, "person", "show", "1", "-f", "json")
	assert.NotContains(t, stdout, "CTO", "dry run must not write")
}

func TestDryRunOnEveryKindOfMutation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")
	crm(t, dbPath, "org", "add", "Acme")
	crm(t, dbPath, "deal", "add", "Big deal")
	crm(t, dbPath, "task", "add", "Call")
	crm(t, dbPath, "log", "note", "1", "--content", "hi")

	cases := [][]string{
		{"person", "add", "Bob Jones"},
		{"person", "archive", "1"},
		{"person", "restore", "1"},
		{"person", "delete", "1"},
		{"person", "contacted", "1"},
		{"person", "email", "add", "1", "j@x.com"},
		{"org", "add", "Initech"},
		{"org", "edit", "1", "--domain", "x.com"},
		{"org", "delete", "1"},
		{"deal", "add", "Another"},
		{"deal", "edit", "1", "--stage", "won"},
		{"deal", "delete", "1"},
		{"task", "add", "More"},
		{"task", "done", "1"},
		{"task", "delete", "1"},
		{"log", "call", "1", "--subject", "x"},
		{"log", "edit", "1", "--content", "y"},
		{"log", "rm", "1"},
		{"tag", "apply", "person", "1", "vip"},
		{"tag", "remove", "person", "1", "vip"},
		{"tag", "delete", "vip"},
		{"person", "purge", "1"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c[:2], " "), func(t *testing.T) {
			args := append(append([]string{}, c...), "--dry-run", "-f", "json")
			stdout, stderr, code := crm(t, dbPath, args...)
			require.Equal(t, 0, code, stderr)
			var plan map[string]any
			jsonOut(t, stdout, &plan)
			assert.Equal(t, true, plan["dry_run"])
		})
	}

	// Nothing changed.
	stdout, _, _ := crm(t, dbPath, "person", "list", "-f", "json")
	var people []map[string]any
	jsonOut(t, stdout, &people)
	assert.Len(t, people, 1)
	stdout, _, _ = crm(t, dbPath, "org", "list", "-f", "json")
	assert.Contains(t, stdout, "Acme")
	assert.NotContains(t, stdout, "Initech")
}

func TestDryRunMissingTargetIsNotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	_, stderr, code := crm(t, dbPath, "person", "archive", "999", "--dry-run", "-f", "json")
	assert.Equal(t, 3, code)
	assert.Contains(t, stderr, `"code":"not_found"`)
}

func TestDryRunHumanOutput(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")
	stdout, _, code := crm(t, dbPath, "person", "archive", "1", "--dry-run", "-f", "table")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "dry run: would run: crm person archive 1")
	assert.Contains(t, stdout, "Jane Smith")
	assert.Contains(t, stdout, "nothing was written")
}

func TestDestructiveCommandsNeedYes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")
	crm(t, dbPath, "tag", "apply", "person", "1", "vip")

	_, stderr, code := crm(t, dbPath, "tag", "delete", "vip")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "--yes")
	stdout, _, _ := crm(t, dbPath, "tag", "list", "-f", "json")
	assert.Contains(t, stdout, "vip", "tag survives without --yes")

	_, stderr, code = crm(t, dbPath, "person", "purge", "1")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "--yes")

	_, _, code = crm(t, dbPath, "tag", "delete", "vip", "--yes")
	assert.Equal(t, 0, code)
}

// --- JSON errors ---

func TestJSONErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")

	cases := []struct {
		name     string
		args     []string
		wantCode string
		wantExit int
	}{
		{"not found", []string{"person", "show", "999"}, "not_found", 3},
		{"validation", []string{"person", "show", "abc"}, "validation", 2},
		{"usage missing arg", []string{"person", "show"}, "usage", 2},
		{"usage unknown flag", []string{"person", "list", "--bogus"}, "usage", 2},
		{"usage unknown command", []string{"nonsense"}, "usage", 2},
		{"conflict", []string{"backup", dbPath}, "conflict", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := crm(t, dbPath, append(tc.args, "-f", "json")...)
			assert.Equal(t, tc.wantExit, code)
			assert.Empty(t, strings.TrimSpace(stdout), "errors never go to stdout")
			var body struct {
				Error struct {
					Code     string `json:"code"`
					Message  string `json:"message"`
					ExitCode int    `json:"exit_code"`
				} `json:"error"`
			}
			jsonOut(t, strings.TrimSpace(stderr), &body)
			assert.Equal(t, tc.wantCode, body.Error.Code)
			assert.Equal(t, tc.wantExit, body.Error.ExitCode)
			assert.NotEmpty(t, body.Error.Message)
		})
	}
}

func TestHumanErrorsStayText(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	_, stderr, code := crm(t, dbPath, "person", "show", "999", "-f", "table")
	assert.Equal(t, 3, code)
	assert.True(t, strings.HasPrefix(stderr, "crm: error: "), stderr)
	assert.False(t, strings.Contains(stderr, `"error"`))
}

// --- where and domains ---

func TestWhere(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "work.db")
	crm(t, dbPath, "person", "add", "Jane Smith")
	crm(t, dbPath, "person", "add", "Bob Jones")

	type where struct {
		Path          string         `json:"path"`
		Source        string         `json:"source"`
		Domain        string         `json:"domain"`
		Exists        bool           `json:"exists"`
		SchemaVersion *int           `json:"schema_version"`
		Latest        int            `json:"latest_schema_version"`
		DeviceID      string         `json:"device_id"`
		SizeBytes     int64          `json:"size_bytes"`
		Counts        map[string]int `json:"counts"`
	}

	var w where
	stdout, _, code := crmEnv(t, home, nil, "where", "--db", dbPath, "-f", "json")
	require.Equal(t, 0, code)
	jsonOut(t, stdout, &w)
	assert.Equal(t, "flag", w.Source)
	assert.Equal(t, "work", w.Domain)
	assert.True(t, w.Exists)
	require.NotNil(t, w.SchemaVersion)
	assert.Equal(t, w.Latest, *w.SchemaVersion)
	assert.NotEmpty(t, w.DeviceID)
	assert.Positive(t, w.SizeBytes)
	assert.Equal(t, 2, w.Counts["people"])
	assert.Equal(t, strings.ToLower(filepath.Base(dbPath)), strings.ToLower(filepath.Base(w.Path)))

	// Environment variable.
	w = where{}
	stdout, _, _ = crmEnv(t, home, []string{"CRM_DB=" + dbPath}, "where", "-f", "json")
	jsonOut(t, stdout, &w)
	assert.Equal(t, "env", w.Source)

	// The flag beats the environment.
	other := filepath.Join(dir, "other.db")
	w = where{}
	stdout, _, _ = crmEnv(t, home, []string{"CRM_DB=" + dbPath}, "where", "--db", other, "-f", "json")
	jsonOut(t, stdout, &w)
	assert.Equal(t, "flag", w.Source)
	assert.Equal(t, "other", w.Domain)
	assert.False(t, w.Exists)
	_, statErr := os.Stat(other)
	assert.True(t, os.IsNotExist(statErr), "where never creates the database")

	// Default: under HOME/.crm/crm.db.
	w = where{}
	stdout, _, _ = crmEnv(t, home, nil, "where", "-f", "json")
	jsonOut(t, stdout, &w)
	assert.Equal(t, "default", w.Source)
	assert.Equal(t, "crm", w.Domain)
	assert.Equal(t, strings.ToLower(filepath.Join(home, ".crm", "crm.db")), strings.ToLower(w.Path))
	_, statErr = os.Stat(filepath.Join(home, ".crm"))
	assert.True(t, os.IsNotExist(statErr), "where never creates the data directory")
}

func TestWhereTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")
	stdout, _, code := crmEnv(t, t.TempDir(), nil, "where", "--db", dbPath, "-f", "table")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "chosen by:       flag")
	assert.Contains(t, stdout, "people:")
}

func TestDomains(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".crm")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	crm(t, filepath.Join(dir, "crm.db"), "person", "add", "A One")
	crm(t, filepath.Join(dir, "work.db"), "person", "add", "B Two")
	crm(t, filepath.Join(dir, "work.db"), "person", "add", "C Three")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.db"), []byte("not a database, just text to fill the file"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o600))

	stdout, _, code := crmEnv(t, home, nil, "domains", "-f", "json")
	require.Equal(t, 0, code)
	var rows []struct {
		Domain        string `json:"domain"`
		Path          string `json:"path"`
		SchemaVersion int    `json:"schema_version"`
		People        int    `json:"people"`
		Active        bool   `json:"active"`
	}
	jsonOut(t, stdout, &rows)
	require.Len(t, rows, 2, "notes.db is not a CRM database and readme.txt is not a .db file")
	got := map[string]int{}
	active := map[string]bool{}
	for _, r := range rows {
		got[r.Domain] = r.People
		active[r.Domain] = r.Active
		assert.Positive(t, r.SchemaVersion)
	}
	assert.Equal(t, map[string]int{"crm": 1, "work": 2}, got)
	assert.True(t, active["crm"], "the default database is active")
	assert.False(t, active["work"])

	// CRM_DB moves the active marker.
	stdout, _, _ = crmEnv(t, home, []string{"CRM_DB=" + filepath.Join(dir, "work.db")}, "domains", "-f", "json")
	jsonOut(t, stdout, &rows)
	for _, r := range rows {
		assert.Equal(t, r.Domain == "work", r.Active, r.Domain)
	}

	// --dir and a missing directory.
	stdout, _, code = crmEnv(t, home, nil, "domains", "--dir", filepath.Join(home, "nowhere"), "-f", "json")
	assert.Equal(t, 0, code)
	assert.Equal(t, "[]", strings.TrimSpace(stdout))
}

// --- init ---

func TestInitCreatesAndRefusesToOverwrite(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "sub", "t.db")

	stdout, stderr, code := crmEnv(t, home, nil, "init", "--domain", "t", "--path", target, "--yes", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var res struct {
		Domain        string   `json:"domain"`
		Path          string   `json:"path"`
		Created       bool     `json:"created"`
		SchemaVersion int      `json:"schema_version"`
		NextSteps     []string `json:"next_steps"`
	}
	jsonOut(t, stdout, &res)
	assert.True(t, res.Created)
	assert.Equal(t, "t", res.Domain)
	assert.Positive(t, res.SchemaVersion)
	joined := strings.Join(res.NextSteps, "\n")
	assert.Contains(t, joined, "crm help overview")
	assert.Contains(t, joined, "crm doctor")
	_, err := os.Stat(target)
	require.NoError(t, err)

	// Second run refuses and leaves the file alone.
	before, _ := os.Stat(target)
	_, stderr, code = crmEnv(t, home, nil, "init", "--path", target, "--yes", "-f", "json")
	assert.Equal(t, 4, code)
	assert.Contains(t, stderr, `"code":"conflict"`)
	after, _ := os.Stat(target)
	assert.Equal(t, before.ModTime(), after.ModTime())
}

func TestInitNeedsYesWithoutTerminal(t *testing.T) {
	target := filepath.Join(t.TempDir(), "t.db")
	_, stderr, code := crmEnv(t, t.TempDir(), nil, "init", "--path", target)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "--yes")
	_, err := os.Stat(target)
	assert.True(t, os.IsNotExist(err))
}

func TestInitDomainAndDefaultLocations(t *testing.T) {
	home := t.TempDir()
	_, stderr, code := crmEnv(t, home, nil, "init", "--domain", "work", "--yes", "-f", "json")
	require.Equal(t, 0, code, stderr)
	_, err := os.Stat(filepath.Join(home, ".crm", "work.db"))
	require.NoError(t, err)

	// No flags: the default database every other command uses.
	stdout, stderr, code := crmEnv(t, home, nil, "init", "--yes", "-f", "json")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stdout, `"domain":"crm"`)
	_, err = os.Stat(filepath.Join(home, ".crm", "crm.db"))
	require.NoError(t, err)

	_, stderr, code = crmEnv(t, home, nil, "init", "--domain", "bad name", "--yes")
	assert.Equal(t, 2, code, stderr)
}

func TestInitDryRun(t *testing.T) {
	target := filepath.Join(t.TempDir(), "t.db")
	stdout, _, code := crmEnv(t, t.TempDir(), nil, "init", "--path", target, "--dry-run", "-f", "json")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, `"dry_run":true`)
	_, err := os.Stat(target)
	assert.True(t, os.IsNotExist(err))
}

func TestInitImportsDex(t *testing.T) {
	dir := t.TempDir()
	contacts := filepath.Join(dir, "contacts.csv")
	notes := filepath.Join(dir, "notes.csv")
	require.NoError(t, os.WriteFile(contacts, []byte("full_name,first_name,last_name,dex_email,dex_emails,id\nJane Doe,Jane,Doe,jane@acme.com,jane@acme.com,11111111-1111-1111-1111-111111111111\n"), 0o600))
	require.NoError(t, os.WriteFile(notes, []byte("id,event_time,contact_names,contact_emails,note_type,basic_note,note,contact_ids\nn1,2026-03-01T10:00:00.000Z,Jane Doe,jane@acme.com,Meeting,Talked,<p>Talked</p>,11111111-1111-1111-1111-111111111111\n"), 0o600))
	target := filepath.Join(dir, "d.db")

	stdout, stderr, code := crmEnv(t, t.TempDir(), nil, "init", "--path", target, "--import-dex-contacts", contacts, "--import-dex-notes", notes, "--yes", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var res struct {
		Imported struct {
			Contacts int `json:"contacts_imported"`
			Notes    int `json:"notes_imported"`
		} `json:"imported"`
	}
	jsonOut(t, stdout, &res)
	assert.Equal(t, 1, res.Imported.Contacts)
	assert.Equal(t, 1, res.Imported.Notes)
	list, _, _ := crm(t, target, "person", "list", "-f", "json")
	assert.Contains(t, list, "Jane")

	// Notes alone are refused, a missing file is not found.
	_, _, code = crmEnv(t, t.TempDir(), nil, "init", "--path", filepath.Join(dir, "e.db"), "--import-dex-notes", notes, "--yes")
	assert.Equal(t, 2, code)
	_, _, code = crmEnv(t, t.TempDir(), nil, "init", "--path", filepath.Join(dir, "f.db"), "--import-dex-contacts", filepath.Join(dir, "missing.csv"), "--yes")
	assert.Equal(t, 3, code)
}

// --- doctor ---

func TestDoctorExitCodes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")

	stdout, _, code := crm(t, dbPath, "doctor", "-f", "json")
	assert.Equal(t, 0, code, "warnings (no backup) do not fail")
	var rep struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name, Status, Message, Fix string
		} `json:"checks"`
	}
	jsonOut(t, stdout, &rep)
	assert.True(t, rep.OK)
	statuses := map[string]string{}
	for _, c := range rep.Checks {
		statuses[c.Name] = c.Status
		assert.Contains(t, []string{"ok", "warn", "fail"}, c.Status)
	}
	assert.Equal(t, "ok", statuses["database"])
	assert.Equal(t, "warn", statuses["backups"])

	// A backup in the conventional folder clears the warning.
	require.NoError(t, os.MkdirAll(filepath.Join(filepath.Dir(dbPath), "backups"), 0o700))
	_, _, code = crm(t, dbPath, "backup", filepath.Join(filepath.Dir(dbPath), "backups", "crm-1.db"))
	require.Equal(t, 0, code)
	stdout, _, _ = crm(t, dbPath, "doctor", "-f", "json")
	jsonOut(t, stdout, &rep)
	for _, c := range rep.Checks {
		if c.Name == "backups" {
			assert.Equal(t, "ok", c.Status)
		}
	}

	// A missing database fails with exit code 1, and doctor does not create it.
	missing := filepath.Join(t.TempDir(), "gone.db")
	stdout, _, code = crm(t, missing, "doctor", "-f", "json")
	assert.Equal(t, 1, code)
	jsonOut(t, stdout, &rep)
	assert.False(t, rep.OK)
	_, err := os.Stat(missing)
	assert.True(t, os.IsNotExist(err))

	// Human output.
	stdout, _, code = crm(t, dbPath, "doctor", "-f", "table")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "integrity")
	assert.Contains(t, stdout, "ok, ")
}

func TestDoctorWarnsInCloudFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "OneDrive", "crm")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	dbPath := filepath.Join(dir, "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith")
	stdout, _, code := crm(t, dbPath, "doctor", "-f", "json")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, `"name":"cloud_folder","status":"warn"`)
}

// --- agent ---

func TestAgentPrintsEmbeddedAgentsMD(t *testing.T) {
	stdout, _, code := crm(t, ":memory:", "agent")
	assert.Equal(t, 0, code)
	assert.Equal(t, agentdoc.AgentsMD(), stdout)

	raw, err := os.ReadFile(filepath.Join(projectRoot(t), "AGENTS.md"))
	require.NoError(t, err)
	assert.Equal(t, strings.ReplaceAll(string(raw), "\r\n", "\n"), stdout, "crm agent prints the repository AGENTS.md")

	stdout, _, code = crm(t, ":memory:", "agent", "install-prompt")
	assert.Equal(t, 0, code)
	assert.Equal(t, agentdoc.InstallPrompt()+"\n", stdout)
	assert.Contains(t, stdout, "INSTALL.md")

	stdout, _, _ = crm(t, ":memory:", "agent", "-f", "json")
	var body map[string]string
	jsonOut(t, stdout, &body)
	assert.Equal(t, "agents", body["name"])
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := findProjectRoot()
	require.NoError(t, err)
	return root
}

// --- docs ---

func TestDocsMarkdownAndMan(t *testing.T) {
	md := filepath.Join(t.TempDir(), "md")
	_, stderr, code := crm(t, ":memory:", "docs", "markdown", md)
	require.Equal(t, 0, code, stderr)
	page, err := os.ReadFile(filepath.Join(md, "crm_person_add.md"))
	require.NoError(t, err)
	assert.Contains(t, string(page), "crm person add")
	assert.NotContains(t, string(page), "Auto generated by spf13/cobra", "output is reproducible")

	man := filepath.Join(t.TempDir(), "man")
	_, stderr, code = crm(t, ":memory:", "docs", "man", man)
	require.Equal(t, 0, code, stderr)
	page, err = os.ReadFile(filepath.Join(man, "crm-person-add.1"))
	require.NoError(t, err)
	assert.Contains(t, string(page), "person add")
}

func TestMCPToolsListedWithoutStartingServer(t *testing.T) {
	stdout, _, code := crm(t, ":memory:", "schema", "-f", "json")
	require.Equal(t, 0, code)
	var m struct {
		MCPTools []string `json:"mcp_tools"`
	}
	jsonOut(t, stdout, &m)
	assert.Greater(t, len(m.MCPTools), 30)
}
