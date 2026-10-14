package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var dupeColumns = []format.ColumnDef{
	{Header: "Level", Field: "level"},
	{Header: "IDs", Field: "ids_text"},
	{Header: "Names", Field: "names_text"},
	{Header: "Reasons", Field: "reasons_text"},
}

var mergeColumns = []format.ColumnDef{
	{Header: "Change", Field: "change"},
	{Header: "Name", Field: "name"},
	{Header: "Detail", Field: "detail"},
}

// registerDupesCommands adds `crm dupes` and `crm person merge`. It must run after registerPersonCommands.
func registerDupesCommands(rootCmd *cobra.Command) {
	rootCmd.AddCommand(dupesCmd())
	for _, c := range rootCmd.Commands() {
		if c.Name() == "person" {
			c.AddCommand(personMergeCmd())
		}
	}
}

func dupesCmd() *cobra.Command {
	var minFlag string
	var limit int

	cmd := &cobra.Command{
		Use:   "dupes",
		Short: "Find probable duplicate people",
		Long: `List groups of probable duplicate people, highest confidence first.

Levels: high (shared email, phone or social handle), medium (same normalized name),
low (nickname match), fuzzy (similar spelling). Default minimum is medium.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			min, err := dedupe.ParseLevel(minFlag)
			if err != nil {
				return model.NewExitError(model.ErrValidation, "%s", err.Error())
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			groups, err := repo.NewDupeRepo(db).Find(cmd.Context(), min)
			if err != nil {
				return err
			}
			if limit > 0 && len(groups) > limit {
				groups = groups[:limit]
			}
			data := make([]map[string]any, 0, len(groups))
			for _, g := range groups {
				ids := g.IDs()
				idText := make([]string, len(ids))
				names := make([]string, len(ids))
				for i, m := range g.Members {
					idText[i] = strconv.FormatInt(m.ID, 10)
					names[i] = m.Name
				}
				row := map[string]any{
					"level":   g.Level.String(),
					"ids":     ids,
					"members": g.Members,
					"reasons": g.Reasons,
				}
				if resolveFormat() != format.FormatJSON {
					row["ids_text"] = strings.Join(idText, ", ")
					row["names_text"] = strings.Join(names, " | ")
					row["reasons_text"] = strings.Join(g.Reasons, "; ")
				}
				data = append(data, row)
			}
			if flagQuiet {
				for _, row := range data {
					fmt.Fprintln(os.Stdout, strings.ReplaceAll(fmt.Sprint(row["ids_text"]), ", ", ","))
				}
				return nil
			}
			return format.Output(os.Stdout, resolveFormat(), data, dupeColumns, false)
		},
	}
	cmd.Flags().StringVar(&minFlag, "min", "medium", "lowest confidence level: high, medium, low, fuzzy")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of groups (0 = all)")

	cmd.AddCommand(dupesDismissCmd(false), dupesDismissCmd(true))
	return cmd
}

func dupesDismissCmd(undo bool) *cobra.Command {
	use, short, done := "dismiss <idA> <idB>", "Mark two people as not duplicates", "Dismissed"
	if undo {
		use, short, done = "undismiss <idA> <idB>", "Make a dismissed pair eligible as duplicates again", "Undismissed"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			b, err := parsePersonID(args[1])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			r := repo.NewDupeRepo(db)
			if undo {
				err = r.Undismiss(cmd.Context(), a, b)
			} else {
				err = r.Dismiss(cmd.Context(), a, b)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%s #%d and #%d\n", done, a, b)
			return nil
		},
	}
}

// parsePrefer parses repeated field=ID flags.
func parsePrefer(prefer []string) (map[string]int64, error) {
	choices := map[string]int64{}
	for _, p := range prefer {
		field, idStr, ok := strings.Cut(p, "=")
		if !ok || field == "" {
			return nil, model.NewExitError(model.ErrValidation, "invalid --prefer %q (want field=ID)", p)
		}
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			return nil, model.NewExitError(model.ErrValidation, "invalid person ID in --prefer %q", p)
		}
		choices[strings.TrimSpace(field)] = id
	}
	return choices, nil
}

func personMergeCmd() *cobra.Command {
	var prefer []string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "merge <keepID> <dropID...>",
		Short: "Merge duplicate people into one",
		Long: `Merge the drop people into the keep person.

Scalar fields keep the survivor's value unless --prefer field=ID names another person.
An empty survivor field takes the first non-empty value from the dropped people.
Emails, phones, tags and custom fields are combined. Interactions, tasks, deals and
relationships move to the survivor. Dropped people are archived with merged_into set.

Fields: ` + strings.Join(repo.MergeFieldKeys(), ", ") + `, or a handle kind such as linkedin.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			keep, err := parsePersonID(args[0])
			if err != nil {
				return err
			}
			drops := make([]int64, 0, len(args)-1)
			for _, a := range args[1:] {
				id, err := parsePersonID(a)
				if err != nil {
					return err
				}
				drops = append(drops, id)
			}
			choices, err := parsePrefer(prefer)
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			pr := repo.NewPersonRepo(db)
			var res *model.MergeResult
			if dryRun {
				res, err = pr.MergeDryRun(cmd.Context(), keep, drops, choices)
			} else {
				res, err = pr.Merge(cmd.Context(), keep, drops, choices)
			}
			if err != nil {
				return err
			}
			if dryRun {
				fmt.Fprintf(os.Stderr, "Dry run: nothing changed. Merging %v into #%d would:\n", drops, keep)
			} else {
				fmt.Fprintf(os.Stderr, "Merged %v into #%d\n", drops, keep)
			}
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(res)
			}
			return format.Output(os.Stdout, resolveFormat(), mergeRows(res), mergeColumns, false)
		},
	}
	cmd.Flags().StringArrayVar(&prefer, "prefer", nil, "take a field from a specific person: field=ID (repeatable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without changing anything")
	return cmd
}

func mergeRows(res *model.MergeResult) []map[string]any {
	var rows []map[string]any
	for _, f := range res.Fields {
		rows = append(rows, map[string]any{
			"change": "field",
			"name":   f.Field,
			"detail": fmt.Sprintf("%v -> %v (from #%d)", f.Old, f.New, f.From),
		})
	}
	for _, kind := range []struct {
		change string
		counts map[string]int
	}{{"added", res.Added}, {"moved", res.Moved}} {
		for _, name := range sortedKeys(kind.counts) {
			if n := kind.counts[name]; n > 0 {
				rows = append(rows, map[string]any{"change": kind.change, "name": name, "detail": n})
			}
		}
	}
	return rows
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
