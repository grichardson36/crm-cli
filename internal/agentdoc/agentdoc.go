// Package agentdoc embeds the agent-facing entry documents so the binary can print them.
// AGENTS.md here is a copy of the repository root file, kept in sync by a test.
package agentdoc

import (
	_ "embed"
	"strings"
)

//go:embed AGENTS.md
var agentsMD string

//go:embed install-prompt.txt
var installPrompt string

// AgentsMD returns the content of AGENTS.md.
func AgentsMD() string { return strings.ReplaceAll(agentsMD, "\r\n", "\n") }

// InstallPrompt returns the prompt a person pastes into their agent to install and set up crm.
func InstallPrompt() string {
	return strings.TrimSpace(strings.ReplaceAll(installPrompt, "\r\n", "\n"))
}
