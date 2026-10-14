package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/agentdoc"
	"github.com/spf13/cobra"
)

// registerAgentCommand adds `crm agent` and `crm agent install-prompt`. Both print plain text
// from the binary, so an agent needs no repository checkout and no vendor-specific file.
func registerAgentCommand(rootCmd *cobra.Command) {
	agent := &cobra.Command{
		Use:   "agent",
		Short: "Print the quick start for AI agents (the same text as AGENTS.md)",
		Long: "Print AGENTS.md: what crm is, where data lives, the commands an agent needs first and the safety rules.\n" +
			"The text is embedded in the binary, so it always matches the installed version.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printText(agentdoc.AgentsMD(), "agents")
		},
	}
	agent.AddCommand(&cobra.Command{
		Use:   "install-prompt",
		Short: "Print the prompt to paste into an agent to install and set up crm",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printText(agentdoc.InstallPrompt()+"\n", "install-prompt")
		},
	})
	rootCmd.AddCommand(agent)
}

// printText writes text to stdout, or {"name","body"} with -f json.
func printText(text, name string) error {
	if wantsJSON() {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"name": name, "body": text})
	}
	_, err := fmt.Fprint(os.Stdout, text)
	return err
}

// wantsJSON is true only when -f json was given. Text commands stay text when piped.
func wantsJSON() bool { return flagFormat == "json" || flagFormat == "JSON" }
