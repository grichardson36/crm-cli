package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/spf13/cobra"
)

var backfillColumns = []format.ColumnDef{
	{Header: "Text", Field: "text"},
	{Header: "People", Field: "people"},
	{Header: "Action", Field: "action"},
	{Header: "Organization", Field: "organization"},
	{Header: "To link", Field: "to_link"},
}

func orgBackfillCmd() *cobra.Command {
	var apply bool
	var limit int
	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Create or reuse organizations for existing people's company text (dry run unless --apply)",
		Long: "Reads every person's company text and says what would happen: how many people have company text, how many distinct groups (case, punctuation and Inc/LLC endings ignored), how many organizations would be created, how many already exist and are reused, how many strings are ignored (see 'crm org ignore list'), and how many people get a membership. Groups of 3 or more people are listed first.\n" +
			"Matching is the normalized name or a merge alias. Close but different names never merge: find them with 'crm org dupes'.\n" +
			"Without --apply nothing is written. With --apply it creates the missing organizations and gives each person a membership (primary when they have none, existing memberships stay, the title becomes the role) as one batch. people.company is not changed. Running it again does nothing.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackfill(cmd, apply, limit)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the changes (without it the backfill is only previewed)")
	cmd.Flags().IntVar(&limit, "limit", 0, "list only the first N groups (0 for all)")
	return cmd
}

func runBackfill(cmd *cobra.Command, apply bool, limit int) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	oa := repo.NewOrgAutoRepo(db)
	var rep repo.BackfillReport
	if apply {
		r, ch, err := oa.BackfillApply(cmd.Context())
		if err != nil {
			return err
		}
		rep = r
		fmt.Fprintf(os.Stderr, "Applied: created %d organizations, linked %d people\n", ch.OrgsCreated, ch.Linked)
	} else {
		r, err := oa.BackfillPlan(cmd.Context())
		if err != nil {
			return err
		}
		rep = r
		fmt.Fprintln(os.Stderr, "Dry run: add --apply to write this")
	}
	fmt.Fprintln(os.Stderr, rep.Line())
	groups := rep.List
	if limit > 0 && len(groups) > limit {
		groups = groups[:limit]
	}
	rows := make([]map[string]any, len(groups))
	for i, g := range groups {
		rows[i] = g.Record()
	}
	if resolveFormat() == format.FormatJSON {
		rep.List = nil
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"applied": apply, "counts": rep, "groups": rows})
	}
	return format.Output(os.Stdout, resolveFormat(), rows, backfillColumns, flagQuiet)
}

// orgConvertCmd is the old company review, kept as a deprecated alias of backfill.
func orgConvertCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:        "convert",
		Short:      "Deprecated: use 'crm org backfill'",
		Long:       "Deprecated alias of 'crm org backfill'. Company text now creates or reuses organizations automatically, there is no review. Per-group decisions are gone, this runs the backfill (dry run unless --apply).",
		Deprecated: "use 'crm org backfill'",
		Args:       cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackfill(cmd, apply, 0)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the changes (without it the backfill is only previewed)")
	return cmd
}

func orgIgnoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ignore",
		Short: "Manage company strings that never create an organization (N/A, none, retired ...)",
		Long:  "The ignore list holds company strings that are junk, so they never create an organization. It has built-in defaults and your additions. Turning off a built-in is a removal. Existing organizations and memberships are not touched.",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the ignored company strings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			list, err := repo.NewIgnoreRepo(db).List(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(list))
			for i, e := range list {
				src := "added"
				if e.Builtin {
					src = "built-in"
				}
				rows[i] = map[string]any{"text": e.Text, "source": src, "active": e.Active}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{{Header: "Text", Field: "text"}, {Header: "Source", Field: "source"}, {Header: "Active", Field: "active"}}, flagQuiet)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "add <text>",
		Short: "Ignore a company string",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			changed, err := repo.NewIgnoreRepo(db).Add(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if changed {
				fmt.Fprintf(os.Stderr, "Ignoring %q\n", args[0])
			} else {
				fmt.Fprintf(os.Stderr, "%q is already ignored\n", args[0])
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "remove <text>",
		Short: "Stop ignoring a company string",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := repo.NewIgnoreRepo(db).Remove(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "No longer ignoring %q\n", args[0])
			return nil
		},
	})
	return cmd
}

func orgDupesCmd() *cobra.Command {
	var limit int
	var dismissed bool
	cmd := &cobra.Command{
		Use:   "dupes",
		Short: "Find organizations that may be the same one",
		Long:  "Lists pairs of organizations with near-duplicate names (same name after normalizing, close spelling, one name starting with the other) or the same domain, best first. Company text never merges close names on its own, this is where they show up. Merge a pair with 'crm org merge <survivor> <duplicate>'. A pair that is not a duplicate is marked with 'crm org dupes dismiss <id> <id>', it stops showing up here and 'crm org dupes undismiss <id> <id>' brings it back. 'crm org dupes --dismissed' lists the dismissed pairs.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			var c []repo.OrgCandidate
			if dismissed {
				c, err = repo.NewOrgRepo(db).DismissedOrgCandidates(cmd.Context())
			} else {
				c, err = repo.NewOrgRepo(db).FindOrgCandidates(cmd.Context(), limit)
			}
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(c))
			for i, x := range c {
				rows[i] = map[string]any{"a_id": x.AID, "a_name": x.AName, "b_id": x.BID, "b_name": x.BName, "reason": x.Reason, "merge": fmt.Sprintf("crm org merge %d %d", x.AID, x.BID)}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{
				{Header: "A", Field: "a_id"}, {Header: "Name A", Field: "a_name"}, {Header: "B", Field: "b_id"}, {Header: "Name B", Field: "b_name"}, {Header: "Reason", Field: "reason"},
			}, flagQuiet)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "show only the first N pairs")
	cmd.Flags().BoolVar(&dismissed, "dismissed", false, "list the pairs marked as not duplicates instead")
	cmd.AddCommand(orgDupesDismissCmd(false), orgDupesDismissCmd(true))
	return cmd
}

func orgDupesDismissCmd(undo bool) *cobra.Command {
	use, short, done := "dismiss <idA> <idB>", "Mark two organizations as not duplicates", "Dismissed"
	if undo {
		use, short, done = "undismiss <idA> <idB>", "Make a dismissed pair a merge candidate again", "Undismissed"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := parseOrgIDArg(args[0])
			if err != nil {
				return err
			}
			b, err := parseOrgIDArg(args[1])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			r := repo.NewOrgRepo(db)
			if undo {
				err = r.UndismissOrgPair(cmd.Context(), a, b)
			} else {
				_, err = r.DismissOrgPair(cmd.Context(), a, b)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%s organizations #%d and #%d\n", done, a, b)
			return nil
		},
	}
}
