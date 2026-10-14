// Package schema builds the machine-readable manifest of the CLI: every command, flag,
// example, exit code and MCP tool, derived from the Cobra command tree.
package schema

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Annotation keys read from cobra.Command.Annotations.
const (
	// AnnMutates is "true" when the command can change data, "false" when it only reads.
	AnnMutates = "mutates"
	// AnnDestructive is "true" when the change cannot be undone with crm. Such commands need --yes.
	AnnDestructive = "destructive"
	// AnnExamples holds copy-pasteable example command lines, one per line.
	AnnExamples = "examples"
	// AnnOutputFields is a comma separated list of the fields in JSON output.
	AnnOutputFields = "output_fields"
)

// Manifest is the full description of the CLI.
type Manifest struct {
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Summary     string     `json:"summary"`
	Commands    []Command  `json:"commands"`
	GlobalFlags []Flag     `json:"global_flags"`
	ExitCodes   []ExitCode `json:"exit_codes"`
	ErrorFormat ErrorDoc   `json:"error_format"`
	MCPTools    []string   `json:"mcp_tools"`
	HelpTopics  []string   `json:"help_topics"`
}

// Command describes one runnable or grouping command.
type Command struct {
	Path         string   `json:"path"`
	Use          string   `json:"use"`
	Args         string   `json:"args,omitempty"`
	Short        string   `json:"short"`
	Long         string   `json:"long,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
	Runnable     bool     `json:"runnable"`
	Hidden       bool     `json:"hidden,omitempty"`
	Mutates      bool     `json:"mutates"`
	Destructive  bool     `json:"destructive"`
	DryRun       bool     `json:"dry_run"`
	Flags        []Flag   `json:"flags"`
	Examples     []string `json:"examples"`
	OutputFields []string `json:"output_fields,omitempty"`
}

// Flag describes one flag.
type Flag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default"`
	Usage     string `json:"usage"`
}

// ExitCode documents one process exit code.
type ExitCode struct {
	Code      int    `json:"code"`
	Name      string `json:"name"`
	Meaning   string `json:"meaning"`
	ErrorCode string `json:"error_code,omitempty"`
}

// ErrorDoc documents the error object printed to stderr when -f json is set.
type ErrorDoc struct {
	Shape string   `json:"shape"`
	Codes []string `json:"codes"`
}

// ExitCodes is the exit code table.
func ExitCodes() []ExitCode {
	return []ExitCode{
		{0, "ok", "success (doctor: all checks ok or warn)", ""},
		{1, "error", "unexpected error (doctor: at least one check failed)", "unknown"},
		{2, "usage", "bad arguments, flags or values", "usage|validation"},
		{3, "not_found", "the record does not exist", "not_found"},
		{4, "conflict", "the change conflicts with existing data", "conflict"},
		{10, "database", "the database could not be opened or written", "database"},
	}
}

// ErrorFormat documents the JSON error object.
func ErrorFormat() ErrorDoc {
	return ErrorDoc{
		Shape: `{"error":{"code":"not_found","message":"...","exit_code":3}}`,
		Codes: []string{"not_found", "validation", "conflict", "database", "usage", "unknown"},
	}
}

// Options carries the values the manifest needs from outside the command tree.
type Options struct {
	Version    string
	MCPTools   []string
	HelpTopics []string
}

// skipped built-ins that carry no CRM behavior.
var skipped = map[string]bool{"completion": true}

// Build walks the command tree below root and returns the manifest.
func Build(root *cobra.Command, opt Options) Manifest {
	m := Manifest{
		Name:        root.Name(),
		Version:     opt.Version,
		Summary:     root.Short,
		ExitCodes:   ExitCodes(),
		ErrorFormat: ErrorFormat(),
		MCPTools:    append([]string{}, opt.MCPTools...),
		HelpTopics:  append([]string{}, opt.HelpTopics...),
		Commands:    []Command{},
		GlobalFlags: flagList(root.PersistentFlags()),
	}
	sort.Strings(m.MCPTools)
	walk(root, root, &m)
	return m
}

func walk(root, cmd *cobra.Command, m *Manifest) {
	subs := cmd.Commands()
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
	for _, c := range subs {
		if cmd == root && skipped[c.Name()] {
			continue
		}
		m.Commands = append(m.Commands, describe(root, c))
		walk(root, c, m)
	}
}

// CommandPath returns the path of cmd below root, such as "person add".
func CommandPath(root, cmd *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), root.Name()))
}

func describe(root, c *cobra.Command) Command {
	d := Command{
		Path:        CommandPath(root, c),
		Use:         c.Use,
		Args:        argsOf(c.Use),
		Short:       c.Short,
		Long:        c.Long,
		Aliases:     c.Aliases,
		Runnable:    c.Runnable(),
		Hidden:      c.Hidden,
		Mutates:     c.Annotations[AnnMutates] == "true",
		Destructive: c.Annotations[AnnDestructive] == "true",
		Flags:       flagList(c.LocalNonPersistentFlags()),
		Examples:    lines(c.Annotations[AnnExamples]),
	}
	d.DryRun = c.Flags().Lookup("dry-run") != nil
	if f := c.Annotations[AnnOutputFields]; f != "" {
		for _, s := range strings.Split(f, ",") {
			if s = strings.TrimSpace(s); s != "" {
				d.OutputFields = append(d.OutputFields, s)
			}
		}
	}
	return d
}

func argsOf(use string) string {
	_, rest, _ := strings.Cut(use, " ")
	return strings.TrimSpace(rest)
}

func lines(s string) []string {
	out := []string{}
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func flagList(fs *pflag.FlagSet) []Flag {
	out := []Flag{}
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		out = append(out, Flag{Name: f.Name, Shorthand: f.Shorthand, Type: f.Value.Type(), Default: f.DefValue, Usage: f.Usage})
	})
	return out
}
