package cli

import (
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var dealStageColumns = []format.ColumnDef{
	{Header: "#", Field: "position"},
	{Header: "Stage", Field: "name"},
	{Header: "Outcome", Field: "outcome"},
	{Header: "In use", Field: "in_use"},
	{Header: "Removed", Field: "removed"},
}

func stageRows(list []*model.VocabEntry) []map[string]any {
	rows := make([]map[string]any, len(list))
	for i, e := range list {
		rows[i] = map[string]any{"position": e.Position, "name": e.Name, "outcome": e.Outcome, "in_use": e.InUse, "removed": e.Removed}
	}
	return rows
}

// completeStages completes a --stage flag from the live deal stage list. It is best effort: any
// failure gives no completions.
func completeStages(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	db, err := openDB()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer db.Close()
	names, err := repo.NewVocabRepo(db).Names(cmd.Context(), model.DealStageKind)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// stageFlagHelp is the help of every --stage flag. The list is the user's, so the help names the
// command that shows it, not the stages.
func stageFlagHelp(extra string) string {
	return "deal stage from your list (crm deal stages list)" + extra
}

// dealStagesCmd is `crm deal stages list|add|rename|remove|reorder|outcome`: the deal stage
// vocabulary (patterns/deal-stages), kept in the vocabulary table next to the deal roles. Each stage
// has an outcome (open, won or lost) that decides what closes a deal.
func dealStagesCmd() *cobra.Command {
	const kind = model.DealStageKind
	cmd := &cobra.Command{
		Use:   "stages",
		Short: "Manage the deal stages: lead, prospect, proposal, negotiation, won, lost ...",
		Long: "The stages a deal can be in come from a list you manage. Add, rename, reorder and remove them here. " +
			"Each stage has an outcome: open (the deal is in the pipeline), won or lost (the deal is closed). The outcome decides the close date and the won and lost counts, the names do not.\n" +
			"A rename rewrites the deals in the stage. Removing a stage that deals use needs --replace-with STAGE. " +
			"At least one stage must stay open and at least one must be won and one lost (E_STAGE_NO_OPEN, E_STAGE_NO_WON, E_STAGE_NO_LOST). See `crm help deal-stages`.",
	}

	list := &cobra.Command{
		Use: "list", Short: "List the deal stages in pipeline order with outcome and the number of deals in each", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			l, err := repo.NewVocabRepo(db).List(cmd.Context(), kind, all)
			if err != nil {
				return err
			}
			return format.Output(os.Stdout, resolveFormat(), stageRows(l), dealStageColumns, flagQuiet)
		},
	}
	list.Flags().Bool("all", false, "include removed stages")

	add := &cobra.Command{
		Use: "add <stage>", Short: "Add an open deal stage at the end of the list (set its outcome with: crm deal stages outcome)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			e, err := repo.NewVocabRepo(db).Add(cmd.Context(), kind, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Added deal stage %q at position %d (outcome %s)\n", e.Name, e.Position, e.Outcome)
			return format.Output(os.Stdout, resolveFormat(), stageRows([]*model.VocabEntry{e}), dealStageColumns, flagQuiet)
		},
	}

	rename := &cobra.Command{
		Use: "rename <stage> <new name>", Short: "Rename a deal stage and every deal in it", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := repo.NewVocabRepo(db).Rename(cmd.Context(), kind, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Renamed deal stage %q to %q (%d deal(s) updated)\n", res.Old, res.New, res.Rows)
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{{"old": res.Old, "new": res.New, "rows": res.Rows}},
				[]format.ColumnDef{{Header: "Old", Field: "old"}, {Header: "New", Field: "new"}, {Header: "Deals updated", Field: "rows"}}, flagQuiet)
		},
	}

	var replaceWith string
	remove := &cobra.Command{
		Use: "remove <stage>", Short: "Remove a deal stage from the list (deals in it need --replace-with)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := repo.NewVocabRepo(db).Remove(cmd.Context(), kind, args[0], replaceWith, false)
			if err != nil {
				return err
			}
			if res.Moved > 0 {
				fmt.Fprintf(os.Stderr, "Removed deal stage %q, %d deal(s) moved to %q\n", res.Name, res.Moved, res.ReplacedBy)
			} else {
				fmt.Fprintf(os.Stderr, "Removed deal stage %q\n", res.Name)
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{{"name": res.Name, "moved": res.Moved, "replaced_by": res.ReplacedBy}},
				[]format.ColumnDef{{Header: "Stage", Field: "name"}, {Header: "Moved", Field: "moved"}, {Header: "Replaced by", Field: "replaced_by"}}, flagQuiet)
		},
	}
	remove.Flags().StringVar(&replaceWith, "replace-with", "", "move the deals in this stage to another listed stage (required when deals use it)")
	_ = remove.RegisterFlagCompletionFunc("replace-with", completeStages)

	var position int
	reorder := &cobra.Command{
		Use: "reorder <stage>...", Short: "Put the named stages first in the given order, or one stage at --position N", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			vr := repo.NewVocabRepo(db)
			if cmd.Flags().Changed("position") {
				if len(args) != 1 {
					return model.NewExitError(model.ErrUsage, "--position takes exactly one stage")
				}
				err = vr.Move(cmd.Context(), kind, args[0], position)
			} else {
				err = vr.Reorder(cmd.Context(), kind, args)
			}
			if err != nil {
				return err
			}
			l, err := vr.List(cmd.Context(), kind, false)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Reordered the deal stages")
			return format.Output(os.Stdout, resolveFormat(), stageRows(l), dealStageColumns, flagQuiet)
		},
	}
	reorder.Flags().IntVar(&position, "position", 0, "move the one named stage to this 1-based position")

	outcome := &cobra.Command{
		Use: "outcome <stage> <open|won|lost>", Short: "Set what a deal stage means: open, won or lost", Args: cobra.ExactArgs(2),
		Long: "The outcome decides what closes a deal. A deal moved into a won or lost stage gets a close date (stamped again on every move into won or lost), a deal moved into an open stage loses it, " +
			"and the open deal counts and the won and lost reports follow the outcome. Deals already in a stage that becomes won or lost get a close date when they have none. " +
			"At least one stage must stay open and at least one must be won and one lost.",
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return completeStages(cmd, args, toComplete)
			case 1:
				return model.StageOutcomes, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := repo.NewVocabRepo(db).SetOutcome(cmd.Context(), kind, args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Deal stage %q is now %s (was %s, %d deal(s) got a close date)\n", res.Name, res.New, res.Old, res.Closed)
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{{"name": res.Name, "old": res.Old, "new": res.New, "closed": res.Closed}},
				[]format.ColumnDef{{Header: "Stage", Field: "name"}, {Header: "Was", Field: "old"}, {Header: "Now", Field: "new"}, {Header: "Got close date", Field: "closed"}}, flagQuiet)
		},
	}

	cmd.AddCommand(list, add, rename, remove, reorder, outcome)
	return cmd
}
