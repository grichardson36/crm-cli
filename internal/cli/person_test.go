package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build binary once for all integration tests
	tmp, err := os.MkdirTemp("", "crm-test-bin-")
	if err != nil {
		panic(err)
	}
	binaryPath = filepath.Join(tmp, "crm")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	// Find project root (where go.mod lives)
	projectRoot, err := findProjectRoot()
	if err != nil {
		panic("cannot find project root: " + err.Error())
	}
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/crm")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("build failed: " + string(out))
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func crm(t *testing.T, dbPath string, args ...string) (string, string, int) {
	t.Helper()
	fullArgs := append([]string{"--db", dbPath}, args...)
	cmd := exec.Command(binaryPath, fullArgs...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return stdout.String(), stderr.String(), exitCode
}

func TestVersion(t *testing.T) {
	cmd := exec.Command(binaryPath, "--version")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "crm version")
}

func TestHelp(t *testing.T) {
	stdout, _, code := crm(t, ":memory:", "--help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "local-first personal CRM")
	assert.Contains(t, stdout, "person")
}

func TestPersonAdd(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	stdout, _, code := crm(t, dbPath, "person", "add", "Jane Smith", "--email", "jane@example.com", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Len(t, data, 1)
	assert.Equal(t, "Jane", data[0]["first_name"])
	assert.Equal(t, "Smith", data[0]["last_name"])
	assert.Equal(t, "jane@example.com", data[0]["email"])
}

func TestPersonList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith", "-f", "json")
	crm(t, dbPath, "person", "add", "Bob Jones", "-f", "json")

	stdout, _, code := crm(t, dbPath, "person", "list", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Len(t, data, 2)
}

func TestPersonShow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith", "-f", "json")

	stdout, _, code := crm(t, dbPath, "person", "show", "1", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Equal(t, "Jane", data[0]["first_name"])
}

func TestPersonShow_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith", "-f", "json")

	_, stderr, code := crm(t, dbPath, "person", "show", "999")
	assert.Equal(t, 3, code)
	assert.Contains(t, stderr, "not found")
}

func TestPersonEdit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith", "-f", "json")

	stdout, _, code := crm(t, dbPath, "person", "edit", "1", "--email", "new@example.com", "-f", "json")
	assert.Equal(t, 0, code)

	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Equal(t, "new@example.com", data[0]["email"])
}

func TestPersonDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith", "-f", "json")

	_, stderr, code := crm(t, dbPath, "person", "delete", "1")
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "Deleted person #1")

	stdout, _, _ := crm(t, dbPath, "person", "list", "-f", "json")
	var data []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &data))
	assert.Len(t, data, 0)
}

func TestPersonDelete_NotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	_, _, code := crm(t, dbPath, "person", "delete", "999")
	assert.Equal(t, 3, code)
}

func TestPersonQuietMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith", "-f", "json")
	crm(t, dbPath, "person", "add", "Bob Jones", "-f", "json")

	stdout, _, code := crm(t, dbPath, "person", "list", "-q")
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.Len(t, lines, 2)
}

func TestPersonCSVFormat(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	crm(t, dbPath, "person", "add", "Jane Smith", "--email", "jane@example.com", "-f", "json")

	stdout, _, code := crm(t, dbPath, "person", "list", "-f", "csv")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "jane@example.com")
	assert.Contains(t, stdout, "id,name,email")
}

func TestPersonInvalidID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crm.db")
	_, stderr, code := crm(t, dbPath, "person", "show", "abc")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "invalid person ID")
}

func TestAliasCommands(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "alias.db")
	_, _, code := crm(t, dbPath, "person", "add", "Zachary Lee")
	require.Equal(t, 0, code)

	out, _, code := crm(t, dbPath, "search", "Zach", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "Zachary")

	out, _, code = crm(t, dbPath, "search", "Zeke", "-f", "json")
	require.Equal(t, 0, code)
	assert.NotContains(t, out, "Zachary")

	_, _, code = crm(t, dbPath, "alias", "add", "zeke", "zachary")
	require.Equal(t, 0, code)
	out, _, _ = crm(t, dbPath, "search", "Zeke", "-f", "json")
	assert.Contains(t, out, "Zachary")

	out, _, code = crm(t, dbPath, "alias", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, `"zeke"`)
	assert.Contains(t, out, `"built-in"`)

	_, _, code = crm(t, dbPath, "alias", "add", "big bob", "robert")
	assert.Equal(t, 2, code)
	_, _, code = crm(t, dbPath, "alias", "remove", "zeke")
	require.Equal(t, 0, code)
	_, _, code = crm(t, dbPath, "alias", "remove", "zeke")
	assert.Equal(t, 3, code)
}

func TestPurgeArchivedCommand(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "purge.db")
	for _, n := range []string{"Ann", "Bea", "Cy"} {
		_, _, code := crm(t, dbPath, "person", "add", n)
		require.Equal(t, 0, code)
	}
	_, _, code := crm(t, dbPath, "person", "archive", "1", "2")
	require.Equal(t, 0, code)

	_, _, code = crm(t, dbPath, "purge")
	assert.Equal(t, 2, code, "--archived is required")
	_, _, code = crm(t, dbPath, "purge", "--archived", "--type", "task")
	assert.Equal(t, 2, code)

	out, _, code := crm(t, dbPath, "purge", "--archived", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "would purge")
	out, _, _ = crm(t, dbPath, "person", "list", "--archived", "-f", "json")
	assert.Contains(t, out, "Ann", "dry run deletes nothing")

	_, _, code = crm(t, dbPath, "purge", "--archived", "--ids", "1", "--apply")
	require.Equal(t, 0, code)
	_, _, code = crm(t, dbPath, "person", "show", "1")
	assert.Equal(t, 3, code)
	_, _, code = crm(t, dbPath, "person", "show", "2")
	assert.Equal(t, 3, code, "archived person 2 still hidden")
	out, _, code = crm(t, dbPath, "purge", "--archived", "--apply", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "purged")
	out, _, _ = crm(t, dbPath, "purge", "--archived", "-f", "json")
	assert.NotContains(t, out, "would purge", "nothing archived remains")
	_, _, code = crm(t, dbPath, "person", "show", "3")
	assert.Equal(t, 0, code, "live people are untouched")
}
