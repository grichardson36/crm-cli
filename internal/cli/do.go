package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/quick"
	"github.com/spf13/cobra"
)

func registerDoCommand(root *cobra.Command) {
	root.AddCommand(doCmd())
}

func doCmd() *cobra.Command {
	var personID int64
	var dryRun, noSyntax bool

	cmd := &cobra.Command{
		Use:   "do <text>",
		Short: "Run one line of text: note, task, log, set, tag or snooze",
		Long: "Run one line of text, the same line the TUI command bar (ctrl+k) takes.\n" +
			"Verbs: n or note, t or task, log <type>, set, tag, snooze. A name after the verb picks the person, or use --person.\n" +
			"Short syntax inside the text: #tag @Name !high ~30m for 30m by fri every 2 weeks remind fri, and in tasks and logs plain dates such as fri 3pm. See 'crm help short-syntax'.\n" +
			"A name that matches several people, or none, is an error that lists the candidates.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := strings.Join(args, " ")
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			if cmd.Flags().Changed("person") {
				if _, err := repo.LookupRecord(cmd.Context(), db, "person", personID); err != nil {
					return err
				}
			} else {
				personID = 0
			}
			env, err := quick.LoadEnv(cmd.Context(), db, time.Now(), personID)
			if err != nil {
				return err
			}
			env.NoSyntax = noSyntax
			plan := quick.Parse(text, env)
			if plan.Verb == "" {
				return model.NewExitError(model.ErrUsage, "the text must start with a verb: n, note, t, task, log, set, tag or snooze (see 'crm help short-syntax')")
			}
			if len(plan.Problems) > 0 {
				_ = printDo(quick.OutputOf(plan, nil), true)
				return doProblemError(plan.Problems[0])
			}
			if !plan.Ready() {
				return model.NewExitError(model.ErrValidation, "nothing to do: %s", plan.Preview)
			}
			if dryRun {
				return printDo(quick.OutputOf(plan, nil), false)
			}
			res, err := quick.Apply(cmd.Context(), quick.NewRepos(db), plan)
			if err != nil {
				return err
			}
			return printDo(quick.OutputOf(plan, res), false)
		},
	}

	cmd.Flags().Int64Var(&personID, "person", 0, "the person the line is about, so it needs no name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what the line would do, and write nothing")
	cmd.Flags().BoolVar(&noSyntax, "no-syntax", false, "keep #, @, !, ~, *, + and ^ as plain text")
	return cmd
}

// doProblemError turns a plan problem into an error with the right exit code.
func doProblemError(p quick.Problem) error {
	msg := p.Message
	if len(p.Candidates) > 0 {
		var parts []string
		for _, c := range p.Candidates {
			parts = append(parts, fmt.Sprintf("%s #%d", c.Name, c.ID))
		}
		msg += " (" + strings.Join(parts, ", ") + "; use --person ID or the full name)"
	}
	return model.NewExitError(quick.ErrorFor(p.Code), "%s", msg)
}

// printDo writes the result of crm do. JSON is one object. Other formats print the preview and one
// line per change. toStderr sends the problem plan to stderr when the line cannot run.
func printDo(o quick.Output, problem bool) error {
	if resolveFormat() == format.FormatJSON {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(o)
	}
	w := os.Stdout
	if problem {
		w = os.Stderr
	}
	if o.DryRun {
		fmt.Fprintf(w, "dry run: %s\n", o.Preview)
	} else {
		fmt.Fprintln(w, o.Preview)
	}
	for _, c := range o.Changes {
		fmt.Fprintf(w, "  %s\n", c.Describe())
	}
	for _, warn := range o.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", warn)
	}
	if o.DryRun && !problem {
		fmt.Fprintln(os.Stderr, "nothing was written")
	}
	return nil
}
