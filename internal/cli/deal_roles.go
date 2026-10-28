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

var dealRoleColumns = []format.ColumnDef{
	{Header: "#", Field: "position"},
	{Header: "Role", Field: "name"},
	{Header: "In use", Field: "in_use"},
	{Header: "Removed", Field: "removed"},
}

func vocabRows(list []*model.VocabEntry) []map[string]any {
	rows := make([]map[string]any, len(list))
	for i, e := range list {
		rows[i] = map[string]any{"position": e.Position, "name": e.Name, "in_use": e.InUse, "removed": e.Removed}
	}
	return rows
}

// dealRolesCmd is `crm deal roles list|add|rename|remove|reorder|backfill`: the deal role vocabulary
// (patterns/deal-roles), kept in the vocabulary table. A person's role on a deal is a sales role
// (champion, decision maker, ...), different from the free-text role at a company (a membership).
func dealRolesCmd() *cobra.Command {
	const kind = model.DealRoleKind
	cmd := &cobra.Command{
		Use:   "roles",
		Short: "Manage the deal roles: champion, decision maker, economic buyer ...",
		Long: "The roles a person can have on a deal come from a list you manage. Add, rename, reorder and remove them here. " +
			"The role at a company (a membership) is different and stays free text.\n" +
			"A rename rewrites the deal links that use the role. Removing a role that links still use needs --replace-with ROLE (they move) or --keep (they keep the text as plain text). " +
			"Linking a person to a deal with a role that is not on the list is refused with E_VOCAB_UNKNOWN unless you pass --create. " +
			"'backfill' adds the free-text roles already on your deal links to the list. See `crm help deal-roles`.",
	}

	list := &cobra.Command{
		Use: "list", Short: "List the deal roles in pick order with the number of links that use each", Args: cobra.NoArgs,
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
			return format.Output(os.Stdout, resolveFormat(), vocabRows(l), dealRoleColumns, flagQuiet)
		},
	}
	list.Flags().Bool("all", false, "include removed roles (links may still show their text)")

	add := &cobra.Command{
		Use: "add <role>", Short: "Add a deal role at the end of the list", Args: cobra.ExactArgs(1),
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
			fmt.Fprintf(os.Stderr, "Added deal role %q at position %d\n", e.Name, e.Position)
			return format.Output(os.Stdout, resolveFormat(), vocabRows([]*model.VocabEntry{e}), dealRoleColumns, flagQuiet)
		},
	}

	rename := &cobra.Command{
		Use: "rename <role> <new name>", Short: "Rename a deal role and every deal link that uses it", Args: cobra.ExactArgs(2),
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
			fmt.Fprintf(os.Stderr, "Renamed deal role %q to %q (%d link(s) updated)\n", res.Old, res.New, res.Rows)
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{{"old": res.Old, "new": res.New, "rows": res.Rows}},
				[]format.ColumnDef{{Header: "Old", Field: "old"}, {Header: "New", Field: "new"}, {Header: "Links updated", Field: "rows"}}, flagQuiet)
		},
	}

	var replaceWith string
	var keep bool
	remove := &cobra.Command{
		Use: "remove <role>", Short: "Remove a deal role from the list (links that use it need --replace-with or --keep)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			res, err := repo.NewVocabRepo(db).Remove(cmd.Context(), kind, args[0], replaceWith, keep)
			if err != nil {
				return err
			}
			switch {
			case res.Moved > 0:
				fmt.Fprintf(os.Stderr, "Removed deal role %q, %d link(s) moved to %q\n", res.Name, res.Moved, res.ReplacedBy)
			case res.Kept > 0:
				fmt.Fprintf(os.Stderr, "Removed deal role %q, %d link(s) keep it as plain text\n", res.Name, res.Kept)
			default:
				fmt.Fprintf(os.Stderr, "Removed deal role %q\n", res.Name)
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{{"name": res.Name, "moved": res.Moved, "kept": res.Kept, "replaced_by": res.ReplacedBy}},
				[]format.ColumnDef{{Header: "Role", Field: "name"}, {Header: "Moved", Field: "moved"}, {Header: "Kept", Field: "kept"}, {Header: "Replaced by", Field: "replaced_by"}}, flagQuiet)
		},
	}
	remove.Flags().StringVar(&replaceWith, "replace-with", "", "move the links that use this role to another listed role")
	remove.Flags().BoolVar(&keep, "keep", false, "leave the links as they are: the text stays, shown as plain text")

	var position int
	reorder := &cobra.Command{
		Use: "reorder <role>...", Short: "Put the named roles first in the given order, or one role at --position N", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			vr := repo.NewVocabRepo(db)
			if cmd.Flags().Changed("position") {
				if len(args) != 1 {
					return model.NewExitError(model.ErrUsage, "--position takes exactly one role")
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
			fmt.Fprintln(os.Stderr, "Reordered the deal roles")
			return format.Output(os.Stdout, resolveFormat(), vocabRows(l), dealRoleColumns, flagQuiet)
		},
	}
	reorder.Flags().IntVar(&position, "position", 0, "move the one named role to this 1-based position")

	var apply bool
	backfill := &cobra.Command{
		Use:   "backfill",
		Short: "Add the free-text roles already on deal links to the list (dry run unless --apply)",
		Long: "Reads the roles already on live deal links and says how many are on the list, how many distinct values are not, and which would be added " +
			"(case ignored, the commonest spelling wins). Roles you removed on purpose are skipped and values that cannot be list names (a | or over 80 characters) are counted. " +
			"Without --apply nothing is written. With --apply the values are added to the end of the list in one step, deal links are not changed, and running it again adds nothing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			vr := repo.NewVocabRepo(db)
			var rep repo.VocabBackfillReport
			if apply {
				r, _, err := vr.BackfillApply(cmd.Context(), kind)
				if err != nil {
					return err
				}
				rep = r
				fmt.Fprintf(os.Stderr, "Applied: added %d deal role(s)\n", len(rep.ToAdd))
			} else {
				r, err := vr.BackfillPlan(cmd.Context(), kind)
				if err != nil {
					return err
				}
				rep = r
				fmt.Fprintln(os.Stderr, "Dry run: add --apply to write this")
			}
			fmt.Fprintln(os.Stderr, rep.Line())
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{"applied": apply, "counts": map[string]any{
					"rows": rep.Rows, "distinct": rep.Distinct, "already_listed": rep.Listed, "removed_skipped": rep.Removed, "invalid_skipped": rep.Invalid, "to_add": len(rep.ToAdd)},
					"roles": rep.ToAdd})
			}
			rows := make([]map[string]any, len(rep.ToAdd))
			for i, c := range rep.ToAdd {
				rows[i] = map[string]any{"name": c.Name, "links": strconv.Itoa(c.Rows)}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{{Header: "Role", Field: "name"}, {Header: "Links", Field: "links"}}, flagQuiet)
		},
	}
	backfill.Flags().BoolVar(&apply, "apply", false, "write the changes (without it the backfill is only previewed)")

	cmd.AddCommand(list, add, rename, remove, reorder, backfill)
	return cmd
}
