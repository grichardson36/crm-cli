package cli

import (
	"os"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

// crm note is the note object: add, show, list, edit and delete here, link, unlink and restore from
// addObjectVerbs (link.go), the same verbs every object has (patterns/objects-first, one-verb-set).
// A note is an interaction. crm log keeps the older names (log add, log edit, log rm) as aliases.

func registerNoteCommands(rootCmd *cobra.Command) {
	noteCmd := &cobra.Command{
		Use:   "note",
		Short: "Notes (call, email, meeting, note, message or your own types): add, edit, delete, restore, link, unlink",
	}
	noteCmd.AddCommand(noteAddCmd(), noteShowCmd(), logListCmd(), logEditCmd(), noteDeleteCmd())
	rootCmd.AddCommand(noteCmd)
}

func noteAddCmd() *cobra.Command {
	var typeName, subject, direction, at string
	var personIDs, orgIDs, dealIDs []int64
	cmd := &cobra.Command{
		Use:   "add [text]",
		Short: "Add a note. Attach it with --person, --org and --deal, or later with crm note link",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := model.CreateInteractionInput{Type: typeName, Subject: nilIfEmpty(subject), Direction: nilIfEmpty(direction),
				PersonIDs: personIDs, OrgIDs: orgIDs, DealIDs: dealIDs}
			if len(args) == 1 {
				in.Content = nilIfEmpty(args[0])
			}
			if in.Content == nil && in.Subject == nil {
				return model.NewExitError(model.ErrValidation, "a note needs text or --subject")
			}
			if direction != "" && !model.ValidInteractionDirection(direction) {
				return model.NewExitError(model.ErrValidation, "invalid direction: %s (must be one of: %s)", direction, strings.Join(model.InteractionDirections, ", "))
			}
			if at != "" {
				d, err := normalizeOccurredAt(at)
				if err != nil {
					return err
				}
				in.OccurredAt = &d
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			n, err := repo.NewInteractionRepo(db).Create(cmd.Context(), in)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{interactionToMap(n)}, interactionColumns, flagQuiet)
		},
	}
	f := cmd.Flags()
	f.StringVar(&typeName, "type", "note", "note type (see 'crm log types')")
	f.StringVar(&subject, "subject", "", "subject/title")
	f.StringVar(&direction, "direction", "", "inbound or outbound")
	f.StringVar(&at, "at", "", "when it happened (YYYY-MM-DD or ISO 8601)")
	f.Int64SliceVar(&personIDs, "person", nil, "attach to this person (repeatable)")
	f.Int64SliceVar(&orgIDs, "org", nil, "attach to this organization (repeatable)")
	f.Int64SliceVar(&dealIDs, "deal", nil, "attach to this deal (repeatable)")
	return cmd
}

func noteShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show a note with its links",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseInteractionID(args[0])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			n, err := repo.NewInteractionRepo(db).FindByID(cmd.Context(), id)
			if err != nil {
				return err
			}
			return showWithLinks(cmd, db, repo.Ref{Kind: model.NoteKind, ID: id}, interactionToMap(n), interactionColumns)
		},
	}
}

func noteDeleteCmd() *cobra.Command {
	c := logRmCmd()
	c.Use = "delete <id>"
	c.Short = "Delete a note (soft, crm note restore brings it back)"
	return c
}

// noteMeta describes the note commands for `crm schema`, --help and --dry-run.
func noteMeta() map[string]cmdMeta {
	return map[string]cmdMeta{
		"note add": {mutates: true, noDryRun: true, fields: logFields, examples: []string{
			`crm note add "Talked about the offsite" --person 1`, `crm note add "Intro" --type call --org 2 --deal 3`}},
		"note show":   {fields: logFields + ",links", examples: []string{`crm note show 1 -f json`}},
		"note list":   {fields: logFields, examples: []string{`crm note list --person 1 --limit 5 -f json`}},
		"note edit":   {mutates: true, fields: logFields, target: model.NoteKind, targetArgs: "first", examples: []string{`crm note edit 1 --content "Corrected text"`}},
		"note delete": {mutates: true, target: model.NoteKind, targetArgs: "first", examples: []string{`crm note delete 1`}},
	}
}
