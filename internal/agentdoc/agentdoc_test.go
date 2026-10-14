package agentdoc_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/agentdoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repoFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", name))
	require.NoError(t, err)
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// The embedded copy exists because go:embed cannot reach the repository root. If this fails, run:
// cp AGENTS.md internal/agentdoc/AGENTS.md
func TestEmbeddedAgentsMDMatchesRepoRoot(t *testing.T) {
	assert.Equal(t, repoFile(t, "AGENTS.md"), agentdoc.AgentsMD())
}

func TestInstallPromptIsInREADMEAndINSTALL(t *testing.T) {
	prompt := agentdoc.InstallPrompt()
	require.NotEmpty(t, prompt)
	assert.NotContains(t, prompt, "\n", "the prompt is one paragraph so it pastes cleanly")
	for _, f := range []string{"README.md", "INSTALL.md"} {
		assert.Contains(t, repoFile(t, f), prompt, "%s must contain the install prompt from `crm agent install-prompt`", f)
	}
}

func TestAgentsMDNamesTheFirstCommands(t *testing.T) {
	md := agentdoc.AgentsMD()
	for _, c := range []string{"crm where", "crm help", "crm schema", "crm agent", "crm doctor", "crm init", "crm person list -f json", "crm search", "Never touch the database file directly"} {
		assert.Contains(t, md, c)
	}
}

func TestNoVendorSpecificFiles(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, f := range []string{".claude", ".codex", ".cursorrules", ".mcp.json", "SKILL.md"} {
		if _, err := os.Stat(filepath.Join(root, f)); os.IsNotExist(err) {
			continue
		}
		// The path exists locally (agents may create scratch dirs such as
		// .claude/worktrees). Only a tracked file would ship in the repository.
		cmd := exec.Command("git", "ls-files", "--", f)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Skipf("git unavailable to check whether %s is tracked: %v", f, err)
		}
		assert.Empty(t, strings.TrimSpace(string(out)), "%s must not ship in the repository root", f)
	}
}
