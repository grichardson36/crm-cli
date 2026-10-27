package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var orgMergeColumns = []format.ColumnDef{
	{Header: "Change", Field: "change"},
	{Header: "Name", Field: "name"},
	{Header: "Detail", Field: "detail"},
}

func parseOrgIDArg(arg string) (int64, error) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return 0, model.NewExitError(model.ErrValidation, "invalid organization ID: %s", arg)
	}
	return id, nil
}

func orgMergeRows(res *repo.OrgMergeResult) []map[string]any {
	var rows []map[string]any
	for _, f := range res.Fields {
		rows = append(rows, map[string]any{"change": "field filled", "name": f.Field, "detail": fmt.Sprintf("%s (from #%d)", f.Value, f.From)})
	}
	for _, f := range res.Lost {
		rows = append(rows, map[string]any{"change": "field differs", "name": f.Field, "detail": fmt.Sprintf("#%d has %q, the survivor's value stays", f.From, f.Value)})
	}
	for _, name := range sortedKeys(res.Moved) {
		if n := res.Moved[name]; n > 0 {
			rows = append(rows, map[string]any{"change": "moved", "name": name, "detail": n})
		}
	}
	return rows
}

func orgMergeCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "merge <survivor> <duplicate...>",
		Short: "Merge duplicate organizations into one (preview unless --apply)",
		Long: `Merge the duplicate organizations into the survivor. Without --apply this only shows what would change.

The survivor keeps its own fields and an empty one takes the first value from the duplicates (domain, industry, notes, summary).
Memberships all move to the survivor, only a row identical to one the survivor already has (same person, role, start and end) is dropped.
Parent links move, a link that becomes a self link or a duplicate is dropped, and a merge that would make an organization its own parent is refused.
Deal links, task and interaction links and tags move too. Each duplicate is archived and its id and uuid keep resolving to the survivor (merge alias).
Company text keeps working: the duplicate's name now matches the survivor as an alias, so a person typed with the old name joins the survivor. Find merge candidates with 'crm org dupes'.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			keep, err := parseOrgIDArg(args[0])
			if err != nil {
				return err
			}
			drops := make([]int64, 0, len(args)-1)
			for _, a := range args[1:] {
				id, err := parseOrgIDArg(a)
				if err != nil {
					return err
				}
				drops = append(drops, id)
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			or := repo.NewOrgRepo(db)
			var res *repo.OrgMergeResult
			if apply {
				res, err = or.MergeOrgs(cmd.Context(), keep, drops)
			} else {
				res, err = or.MergeOrgsDryRun(cmd.Context(), keep, drops)
			}
			if err != nil {
				return err
			}
			if apply {
				fmt.Fprintf(os.Stderr, "Merged %v into #%d (%s) (undo: crm merge undo %d)\n", drops, keep, res.KeepName, res.LogID)
			} else {
				fmt.Fprintf(os.Stderr, "Preview only: add --apply to merge %v into #%d (%s)\n", drops, keep, res.KeepName)
			}
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(res)
			}
			return format.Output(os.Stdout, resolveFormat(), orgMergeRows(res), orgMergeColumns, false)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the merge (without it the merge is only previewed)")
	return cmd
}
