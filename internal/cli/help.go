package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/help"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

// helpCommand replaces Cobra's help command. A topic name shows the built-in topic. Anything
// else falls through to Cobra, so `crm help person add` still prints the usage of that command.
func helpCommand() *cobra.Command {
	var search string
	cmd := &cobra.Command{
		Use:   "help [topic | command]",
		Short: "Built-in guides (topics) and command help",
		Long: "Without arguments, list the built-in topics. With a topic name, print the guide. With a command, print its usage\n" +
			"(the same as 'crm <command> --help'). Use --search to find a word across all topics. Add -f json for JSON.\n" +
			"When a topic and a command share a name (sync, import, tui, domains), the topic wins: use 'crm <command> --help' for the command.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if search != "" {
				return helpSearch(search)
			}
			if len(args) == 0 {
				return helpList()
			}
			if len(args) == 1 {
				if t, ok := help.Get(args[0]); ok {
					return helpTopic(t)
				}
			}
			target, _, err := cmd.Root().Find(args)
			if err != nil || target == nil || target == cmd.Root() {
				return model.NewExitError(model.ErrUsage, "unknown help topic or command %q (run 'crm help' for the list)", args[0])
			}
			target.InitDefaultHelpFlag()
			return target.Help()
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "find text (case-insensitive) in all topics")
	return cmd
}

func helpList() error {
	topics := help.List()
	if wantsJSON() {
		type item struct {
			Name    string `json:"name"`
			Title   string `json:"title"`
			Summary string `json:"summary"`
		}
		items := make([]item, len(topics))
		for i, t := range topics {
			items[i] = item{t.Name, t.Title, t.Summary}
		}
		return json.NewEncoder(os.Stdout).Encode(items)
	}
	fmt.Fprint(os.Stdout, "crm help <topic>   built-in guides with examples\n\nTopics:\n")
	fmt.Fprint(os.Stdout, help.FormatList(topics))
	fmt.Fprint(os.Stdout, "\nMore:\n"+
		"  crm help <command>        usage and flags of one command, such as: crm help person add\n"+
		"  crm help --search <text>  find a word across all topics\n"+
		"  crm schema -f json        every command, flag, example and exit code as JSON\n"+
		"  crm agent                 quick start for AI agents\n")
	return nil
}

func helpTopic(t help.Topic) error {
	if wantsJSON() {
		return json.NewEncoder(os.Stdout).Encode(t)
	}
	_, err := fmt.Fprint(os.Stdout, t.Body)
	return err
}

func helpSearch(text string) error {
	matches := help.Search(text)
	if wantsJSON() {
		if matches == nil {
			matches = []help.Match{}
		}
		return json.NewEncoder(os.Stdout).Encode(matches)
	}
	for _, m := range matches {
		fmt.Fprintf(os.Stdout, "%s:%d: %s\n", m.Topic, m.Line, m.Text)
	}
	if len(matches) == 0 {
		fmt.Fprintf(os.Stderr, "no topic mentions %q\n", text)
	}
	return nil
}
