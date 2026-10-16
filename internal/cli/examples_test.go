package cli_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/help"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedTemplate builds a small database that every help example can run against:
//
//	people 1 Jane Smith (acme, vip, 14 days), 2 Bob Jones, 3 Janet Smyth (shares an email with 1), 4 Old Contact (archived)
//	organization 1, task 1, deal 1, interaction 1, a linkedin link on person 1, and pair 1/2 dismissed as duplicates
func seedTemplate(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "template.db")
	steps := [][]string{
		{"person", "add", "Jane Smith", "--email", "jane@example.com", "--company", "Acme Corp", "--keep-in-touch", "14 days", "--birthday", "1985-03-14"},
		{"person", "add", "Bob Jones", "--email", "bob@example.com"},
		{"person", "add", "Janet Smyth", "--email", "janet@work.example"},
		{"person", "email", "add", "3", "jane@example.com"},
		{"person", "add", "Old Contact"},
		{"person", "archive", "4"},
		{"org", "add", "Acme Corp", "--domain", "acme.com"},
		{"task", "add", "Send proposal", "--person", "1", "--due", "2026-11-01"},
		{"deal", "add", "Website Redesign", "--value", "15000", "--person", "1", "--stage", "proposal"},
		{"log", "note", "1", "--subject", "Intro", "--content", "Met at ReactConf"},
		{"tag", "apply", "person", "1", "vip"},
		{"person", "handle", "set", "1", "linkedin", "https://linkedin.com/in/janesmith"},
		{"dupes", "dismiss", "1", "2"},
	}
	for _, s := range steps {
		_, stderr, code := crm(t, path, s...)
		require.Equal(t, 0, code, "seed %v: %s", s, stderr)
	}
	return path
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	require.NoError(t, err)
	defer in.Close()
	out, err := os.Create(to)
	require.NoError(t, err)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
	require.NoError(t, out.Close())
}

// Every example in the help topics tagged `sh crm-example` runs against a fresh copy of the seeded
// database and must exit 0, so the documentation cannot rot. Mark an example that cannot run in
// a test (interactive, needs files) as `sh crm-example-skip`.
func TestHelpExamplesRun(t *testing.T) {
	template := seedTemplate(t)
	ran := 0
	for _, ex := range help.Examples() {
		if ex.Skip {
			continue
		}
		ran++
		t.Run(ex.Topic+"/"+ex.Command, func(t *testing.T) {
			t.Parallel()
			args, err := help.SplitArgs(ex.Command)
			require.NoError(t, err)
			require.Equal(t, "crm", args[0])

			dir := t.TempDir()
			dbPath := filepath.Join(dir, "crm.db")
			copyFile(t, template, dbPath)
			home := filepath.Join(dir, "home")
			require.NoError(t, os.MkdirAll(home, 0o700))

			// --db goes first so an example that names its own --db would override it.
			full := append([]string{"--db", dbPath}, args[1:]...)
			stdout, stderr, code := crmEnv(t, home, nil, full...)
			assert.Equal(t, 0, code, "%s\nstdout: %s\nstderr: %s", ex.Command, stdout, stderr)
		})
	}
	assert.Greater(t, ran, 60, "the topics should carry plenty of runnable examples")
}
