package cli

import (
	"sort"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/help"
	"github.com/jdanielnd/crm-cli/internal/schema"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// builtIns are Cobra's own commands. They carry no CRM behavior and need no annotations.
var builtIns = map[string]bool{"help": true, "completion": true}

func isBuiltIn(c *cobra.Command) bool {
	for p := c; p != nil && p.HasParent(); p = p.Parent() {
		if p.Parent().Parent() == nil && builtIns[p.Name()] {
			return true
		}
	}
	return false
}

// A command added without a description, an example or the mutates annotation fails here, so
// `crm schema` can never describe a command incompletely.
func TestEveryCommandIsDescribed(t *testing.T) {
	root := NewRootCmd()
	var missing []string
	for _, c := range allCommands(root) {
		path := schema.CommandPath(root, c)
		if isBuiltIn(c) && path != "help" {
			continue
		}
		if strings.TrimSpace(c.Short) == "" {
			missing = append(missing, path+": no Short description")
		}
		if !c.Runnable() {
			continue // a group of subcommands: its children carry the details
		}
		if strings.TrimSpace(c.Annotations[schema.AnnExamples]) == "" || strings.TrimSpace(c.Example) == "" {
			missing = append(missing, path+": no example")
		}
		switch c.Annotations[schema.AnnMutates] {
		case "true", "false":
		default:
			missing = append(missing, path+": no mutates annotation")
		}
		if c.Annotations[schema.AnnDestructive] != "true" && c.Annotations[schema.AnnDestructive] != "false" {
			missing = append(missing, path+": no destructive annotation")
		}
	}
	assert.Empty(t, missing, "add the command to commandMeta in meta.go")
}

// Every key in commandMeta must be a real command, so a rename cannot leave a stale entry.
func TestCommandMetaHasNoStaleEntries(t *testing.T) {
	root := NewRootCmd()
	paths := map[string]bool{}
	for _, c := range allCommands(root) {
		paths[schema.CommandPath(root, c)] = true
	}
	var stale []string
	for p := range commandMeta {
		if !paths[p] {
			stale = append(stale, p)
		}
	}
	sort.Strings(stale)
	assert.Empty(t, stale)
}

// Every mutating command can be previewed, and every destructive one needs --yes.
func TestMutatingCommandsHaveDryRunAndDestructiveHaveYes(t *testing.T) {
	root := NewRootCmd()
	var problems []string
	for _, c := range allCommands(root) {
		path := schema.CommandPath(root, c)
		meta, ok := commandMeta[path]
		if !ok || !c.Runnable() {
			continue
		}
		if meta.mutates && !meta.noDryRun && c.Flags().Lookup("dry-run") == nil {
			problems = append(problems, path+": mutates but has no --dry-run")
		}
		if meta.destructive && c.Flags().Lookup("yes") == nil {
			problems = append(problems, path+": destructive but has no --yes")
		}
		if meta.destructive && !meta.mutates {
			problems = append(problems, path+": destructive must also mutate")
		}
	}
	assert.Empty(t, problems)
}

// Examples that cannot run in a test (skipped, interactive, or needing files) must still name a
// real command and real flags. Runnable help examples are executed in examples_test.go.
func TestAllExamplesParse(t *testing.T) {
	check := func(where, line string) {
		args, err := help.SplitArgs(line)
		if !assert.NoError(t, err, where) || !assert.NotEmpty(t, args, where) {
			return
		}
		assert.Equal(t, "crm", args[0], where)
		root := NewRootCmd()
		cmd, rest, err := root.Find(args[1:])
		if !assert.NoError(t, err, "%s: %s", where, line) || cmd == root {
			return
		}
		assert.NoError(t, cmd.ParseFlags(rest), "%s: %s", where, line)
	}
	for _, ex := range help.Examples() {
		check("help topic "+ex.Topic, ex.Command)
	}
	for path, meta := range commandMeta {
		for _, ex := range meta.examples {
			check("commandMeta "+path, ex)
		}
	}
}

func TestExamplesStartWithCrm(t *testing.T) {
	for path, meta := range commandMeta {
		for _, ex := range meta.examples {
			assert.True(t, strings.HasPrefix(ex, "crm "), "%s: %q", path, ex)
		}
	}
}
