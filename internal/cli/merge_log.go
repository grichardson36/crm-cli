package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

// crm merge log|undo|redo: the persistent merge log (open discrepancy X9). Person and organization
// merges write one entry each, undo works in any later session.

var mergeLogColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"},
	{Header: "Kind", Field: "kind"},
	{Header: "Survivor", Field: "survivor"},
	{Header: "Merged in", Field: "losers"},
	{Header: "Status", Field: "status"},
	{Header: "When", Field: "created_at"},
}

func registerMergeLogCommands(rootCmd *cobra.Command) {
	mergeCmd := &cobra.Command{
		Use:   "merge",
		Short: "List, undo and redo logged merges of people and organizations",
		Long: `Every applied person merge ('crm person merge') and organization merge ('crm org merge --apply') is written to a merge log.
'crm merge log' lists it, 'crm merge undo ID' reverses one in any later session: the merged records come back, their links move back, the alias is removed and fields the merge overwrote get their old values.
A value someone edited after the merge is left as it is. 'crm merge redo ID' repeats an undone merge.`,
	}
	mergeCmd.AddCommand(mergeLogListCmd(), mergeReplayCmd(false), mergeReplayCmd(true))
	rootCmd.AddCommand(mergeCmd)
}

func mergeLogListCmd() *cobra.Command {
	var kind string
	var limit int
	cmd := &cobra.Command{
		Use:   "log",
		Short: "List logged merges, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if kind != "" && kind != "person" && kind != "organization" {
				return model.NewExitError(model.ErrValidation, "--kind must be person or organization")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			list, err := repo.NewMergeLogRepo(db).List(cmd.Context(), kind, limit)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(list))
			for i, e := range list {
				row := map[string]any{"id": e.ID, "kind": e.Kind, "survivor_id": e.SurvivorID, "survivor_name": e.SurvivorName,
					"loser_ids": e.LoserIDs, "loser_names": e.LoserNames, "summary": e.Summary, "status": e.Status, "created_at": e.CreatedAt}
				if e.UndoneAt != nil {
					row["undone_at"] = *e.UndoneAt
				}
				if resolveFormat() != format.FormatJSON {
					row["survivor"] = fmt.Sprintf("#%d %s", e.SurvivorID, e.SurvivorName)
					row["losers"] = strings.Join(e.LoserNames, ", ")
				}
				rows[i] = row
			}
			return format.Output(os.Stdout, resolveFormat(), rows, mergeLogColumns, flagQuiet)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "only person or organization merges")
	cmd.Flags().IntVar(&limit, "limit", 0, "show only the newest N entries")
	return cmd
}

func mergeReplayCmd(redo bool) *cobra.Command {
	use, short := "undo <id>", "Undo a logged merge (the id is in 'crm merge log')"
	if redo {
		use, short = "redo <id>", "Repeat a merge that was undone"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return model.NewExitError(model.ErrValidation, "invalid merge log id: %s", args[0])
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			lr := repo.NewMergeLogRepo(db)
			var res *repo.MergeReplayResult
			verb := "Undid"
			if redo {
				res, err = lr.Redo(cmd.Context(), id)
				verb = "Redid"
			} else {
				res, err = lr.Undo(cmd.Context(), id)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%s %s\n", verb, res.Entry.Label())
			if res.Skipped > 0 {
				fmt.Fprintf(os.Stderr, "%d value(s) were changed after the merge and were left as they are\n", res.Skipped)
			}
			row := map[string]any{"id": res.Entry.ID, "kind": res.Entry.Kind, "survivor_id": res.Entry.SurvivorID,
				"loser_ids": res.Entry.LoserIDs, "status": res.Entry.Status, "skipped": res.Skipped}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{row}, []format.ColumnDef{
				{Header: "ID", Field: "id"}, {Header: "Kind", Field: "kind"}, {Header: "Survivor", Field: "survivor_id"},
				{Header: "Status", Field: "status"}, {Header: "Skipped", Field: "skipped"},
			}, flagQuiet)
		},
	}
}
