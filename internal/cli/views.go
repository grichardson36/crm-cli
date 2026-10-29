package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/query"
	"github.com/spf13/cobra"
)

const viewFields = "id,uuid,name,kind,tabs,example,position,filter,order,deleted,created_at,updated_at"

var viewColumns = []format.ColumnDef{
	{Header: "ID", Field: "id"}, {Header: "Name", Field: "name"}, {Header: "Kind", Field: "kind"},
	{Header: "Tabs", Field: "tabs"}, {Header: "Filter", Field: "filter"}, {Header: "Order", Field: "order"},
}

func runColumns(q query.Query) []format.ColumnDef {
	cols := []format.ColumnDef{{Header: "ID", Field: "id"}, {Header: "Name", Field: "name"}, {Header: "Detail", Field: "detail"}, {Header: "Meta", Field: "meta"}}
	if q.GroupBy != "" {
		cols = append([]format.ColumnDef{{Header: "Group", Field: "group"}}, cols...)
	}
	for _, c := range q.Columns {
		cols = append(cols, format.ColumnDef{Header: c, Field: c})
	}
	return cols
}

// viewFlags are the flags that build or change the query of a view.
type viewFlags struct {
	kind, name, groupBy, bucket, search, queryJSON string
	where, addWhere, sorts, columns                []string
	clearWhere, clearSort, clearColumns, tabs      bool
}

func (f *viewFlags) add(cmd *cobra.Command, edit bool) {
	fl := cmd.Flags()
	if edit {
		fl.StringVar(&f.name, "name", "", "new name")
		fl.StringVar(&f.kind, "kind", "", "object kind: person, organization, deal, task, note, tag")
		fl.StringArrayVar(&f.where, "where", nil, "replace the filter with these conditions (repeat the flag)")
		fl.StringArrayVar(&f.addWhere, "add-where", nil, "add a condition to the filter")
		fl.BoolVar(&f.clearWhere, "clear-where", false, "remove every condition")
		fl.BoolVar(&f.clearSort, "clear-sort", false, "remove the sort")
		fl.BoolVar(&f.clearColumns, "clear-columns", false, "remove the optional columns")
	} else {
		fl.StringVar(&f.kind, "kind", "", "object kind: person, organization, deal, task, note, tag (required)")
		fl.StringArrayVar(&f.where, "where", nil, `a condition "FIELD OPERATOR VALUE|VALUE", repeat for more, all must hold (see: crm views fields KIND)`)
	}
	fl.StringArrayVar(&f.sorts, "sort", nil, "sort key FIELD or FIELD:desc, at most two")
	fl.StringVar(&f.groupBy, "group-by", "", "group the rows by a field (an empty value removes the group on edit)")
	fl.StringVar(&f.bucket, "bucket", "", "date group size: day, week or month")
	fl.StringArrayVar(&f.columns, "column", nil, "extra column to print in run, repeat for more")
	fl.StringVar(&f.search, "search", "", "words every row must contain")
	fl.StringVar(&f.queryJSON, "query-json", "", "the whole query as JSON, instead of --where, --sort and the other query flags")
	fl.BoolVar(&f.tabs, "tabs", false, "show the view as a tab with a number key in the terminal UI (--tabs=false hides it)")
}

// build reads the query flags into a query. base is the query to change on an edit.
func (f *viewFlags) build(cmd *cobra.Command, base query.Query, edit bool) (query.Query, bool, error) {
	changed := false
	q := base.Clone()
	if cmd.Flags().Changed("query-json") {
		parsed, err := query.Parse(f.queryJSON)
		if err != nil {
			return q, false, model.NewExitError(model.ErrValidation, "%v", err)
		}
		return parsed, true, nil
	}
	parse := func(list []string) ([]query.Condition, error) {
		var out []query.Condition
		for _, w := range list {
			c, err := query.ParseWhere(w)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		return out, nil
	}
	if f.clearWhere {
		q.Filter, changed = nil, true
	}
	if cmd.Flags().Changed("where") {
		c, err := parse(f.where)
		if err != nil {
			return q, false, err
		}
		q.Filter, changed = c, true
	}
	if cmd.Flags().Changed("add-where") {
		c, err := parse(f.addWhere)
		if err != nil {
			return q, false, err
		}
		q.Filter, changed = append(q.Filter, c...), true
	}
	if f.clearSort {
		q.Sort, changed = nil, true
	}
	if cmd.Flags().Changed("sort") {
		q.Sort = nil
		for _, s := range f.sorts {
			parsed, err := query.ParseSort(s)
			if err != nil {
				return q, false, err
			}
			q.Sort = append(q.Sort, parsed)
		}
		changed = true
	}
	if cmd.Flags().Changed("group-by") {
		q.GroupBy, changed = strings.TrimSpace(f.groupBy), true
		if q.GroupBy == "" {
			q.Bucket = ""
		}
	}
	if cmd.Flags().Changed("bucket") {
		q.Bucket, changed = f.bucket, true
	}
	if f.clearColumns {
		q.Columns, changed = nil, true
	}
	if cmd.Flags().Changed("column") {
		q.Columns, changed = f.columns, true
	}
	if cmd.Flags().Changed("search") {
		q.Search, changed = strings.TrimSpace(f.search), true
	}
	_ = edit
	return q, changed, nil
}

func withViewRepo(cmd *cobra.Command, fn func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error) error {
	d, err := openDB()
	if err != nil {
		return err
	}
	defer d.Close()
	return fn(cmd.Context(), d, repo.NewSavedViewRepo(d))
}

func registerViewsCommands(rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "views",
		Short: "Saved views: a named filter, sort and group over one kind of object",
		Long: "A saved view is a name, an object kind (person, organization, deal, task, note, tag), a filter, a sort, a group and optional columns. " +
			"It shows up in the terminal UI as a view like any other (Views screen, v). The filter is a list of conditions, all must hold, with any-of inside a condition: " +
			`"last_contacted before 90 days ago", "tag has client|lead", "value between 1000|5000". No SQL.` + "\n" +
			"Three example views ship with the program, off by default. Delete is soft and `restore` brings a view back. See 'crm help views'.",
	}
	cmd.AddCommand(viewsListCmd(), viewsShowCmd(), viewsRunCmd(), viewsSaveCmd(), viewsEditCmd(), viewsRenameCmd(),
		viewsDeleteCmd(), viewsRestoreCmd(), viewsMoveCmd(), viewsExportCmd(), viewsImportCmd(), viewsFieldsCmd())
	rootCmd.AddCommand(cmd)
}

func viewsListCmd() *cobra.Command {
	var deleted bool
	var kind string
	cmd := &cobra.Command{
		Use: "list", Short: "List the saved views in order", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				list, err := r.List(ctx, deleted)
				if err != nil {
					return err
				}
				if kind != "" {
					var keep []repo.SavedView
					for _, v := range list {
						if v.Kind == kind {
							keep = append(keep, v)
						}
					}
					list = keep
				}
				return format.Output(os.Stdout, resolveFormat(), r.Rows(ctx, list), viewColumns, flagQuiet)
			})
		},
	}
	cmd.Flags().BoolVar(&deleted, "deleted", false, "list the deleted views instead")
	cmd.Flags().StringVar(&kind, "kind", "", "only views of this kind")
	return cmd
}

func viewsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use: "show <view>", Short: "Show one saved view with its query", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				v, err := r.Find(ctx, args[0])
				if err != nil {
					return err
				}
				row := r.Row(ctx, v, true)
				if resolveFormat() == format.FormatJSON {
					return format.Output(os.Stdout, format.FormatJSON, []map[string]any{row}, nil, false)
				}
				q, _ := json.MarshalIndent(v.Query, "", "  ")
				fmt.Fprintf(os.Stdout, "%s (%s) id %d%s\nfilter: %s\n", v.Name, v.Kind, v.ID, map[bool]string{true: ", example", false: ""}[v.Example], row["filter"])
				if o := row["order"].(string); o != "" {
					fmt.Fprintln(os.Stdout, o)
				}
				fmt.Fprintf(os.Stdout, "in the tabs: %v\nquery: %s\n", v.ShowInTabs, q)
				return nil
			})
		},
	}
}

func viewsRunCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use: "run <view>", Short: "Print the rows a saved view matches now", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				v, err := r.Find(ctx, args[0])
				if err != nil {
					return err
				}
				if v.Deleted {
					return model.NewExitError(model.ErrConflict, "saved view %q is deleted, restore it first", v.Name)
				}
				rows, err := r.Run(ctx, v, time.Now())
				if err != nil {
					return err
				}
				if limit > 0 && len(rows) > limit {
					rows = rows[:limit]
				}
				return format.Output(os.Stdout, resolveFormat(), repo.RunRows(v.Query, rows), runColumns(v.Query), flagQuiet)
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "most rows to print")
	return cmd
}

func viewsSaveCmd() *cobra.Command {
	var f viewFlags
	cmd := &cobra.Command{
		Use: "save <name>", Short: "Save a new view", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, _, err := f.build(cmd, query.Query{}, false)
			if err != nil {
				return err
			}
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				v, err := r.Create(ctx, repo.SavedViewInput{Name: args[0], Kind: f.kind, Query: q, ShowInTabs: f.tabs})
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Saved view %q (%s)\n", v.Name, v.Kind)
				return format.Output(os.Stdout, resolveFormat(), []map[string]any{r.Row(ctx, v, true)}, viewColumns, flagQuiet)
			})
		},
	}
	f.add(cmd, false)
	return cmd
}

func viewsEditCmd() *cobra.Command {
	var f viewFlags
	cmd := &cobra.Command{
		Use: "edit <view>", Short: "Change a saved view: its name, kind, filter, sort, group, columns or tab", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				v, err := r.Find(ctx, args[0])
				if err != nil {
					return err
				}
				q, changed, err := f.build(cmd, v.Query, true)
				if err != nil {
					return err
				}
				p := repo.SavedViewPatch{}
				if changed {
					p.Query = &q
				}
				if cmd.Flags().Changed("name") {
					p.Name = &f.name
				}
				if cmd.Flags().Changed("kind") {
					p.Kind = &f.kind
				}
				if cmd.Flags().Changed("tabs") {
					p.ShowInTabs = &f.tabs
				}
				if p.Query == nil && p.Name == nil && p.Kind == nil && p.ShowInTabs == nil {
					return model.NewExitError(model.ErrUsage, "nothing to change: give --name, --kind, --where, --sort, --group-by, --tabs or another flag")
				}
				nv, err := r.Update(ctx, v.ID, p)
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Updated view %q\n", nv.Name)
				return format.Output(os.Stdout, resolveFormat(), []map[string]any{r.Row(ctx, nv, true)}, viewColumns, flagQuiet)
			})
		},
	}
	f.add(cmd, true)
	return cmd
}

func viewsRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rename <view> <new name>", Short: "Rename a saved view", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				v, err := r.Find(ctx, args[0])
				if err != nil {
					return err
				}
				nv, err := r.Update(ctx, v.ID, repo.SavedViewPatch{Name: &args[1]})
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Renamed %q to %q\n", v.Name, nv.Name)
				return format.Output(os.Stdout, resolveFormat(), []map[string]any{r.Row(ctx, nv, false)}, viewColumns, flagQuiet)
			})
		},
	}
}

func viewsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "delete <view>...", Short: "Delete saved views (soft, restore brings them back)", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				var out []map[string]any
				for _, a := range args {
					v, err := r.Find(ctx, a)
					if err != nil {
						return err
					}
					if err := r.Delete(ctx, v.ID); err != nil {
						return err
					}
					fmt.Fprintf(os.Stderr, "Deleted view %q (crm views restore %d brings it back)\n", v.Name, v.ID)
					out = append(out, map[string]any{"id": v.ID, "name": v.Name, "deleted": true})
				}
				return format.Output(os.Stdout, resolveFormat(), out, []format.ColumnDef{{Header: "ID", Field: "id"}, {Header: "Name", Field: "name"}}, flagQuiet)
			})
		},
	}
}

func viewsRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use: "restore <view>...", Short: "Bring deleted saved views back", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				var out []map[string]any
				for _, a := range args {
					v, err := r.Find(ctx, a)
					if err != nil {
						return err
					}
					nv, err := r.Restore(ctx, v.ID)
					if err != nil {
						return err
					}
					fmt.Fprintf(os.Stderr, "Restored view %q\n", nv.Name)
					out = append(out, r.Row(ctx, nv, false))
				}
				return format.Output(os.Stdout, resolveFormat(), out, viewColumns, flagQuiet)
			})
		},
	}
}

func viewsMoveCmd() *cobra.Command {
	return &cobra.Command{
		Use: "move <view> up|down", Short: "Move a saved view one place up or down in the order of the tabs", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			delta := 0
			switch strings.ToLower(args[1]) {
			case "up":
				delta = -1
			case "down":
				delta = 1
			default:
				return model.NewExitError(model.ErrUsage, "use up or down, got %q", args[1])
			}
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				v, err := r.Find(ctx, args[0])
				if err != nil {
					return err
				}
				if _, err := r.Move(ctx, v.ID, delta); err != nil {
					return err
				}
				list, err := r.List(ctx, false)
				if err != nil {
					return err
				}
				return format.Output(os.Stdout, resolveFormat(), r.Rows(ctx, list), viewColumns, flagQuiet)
			})
		},
	}
}

func viewsExportCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use: "export <view>", Short: "Write a saved view as a small TOML file to share", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				v, err := r.Find(ctx, args[0])
				if err != nil {
					return err
				}
				text, err := r.ExportFile(ctx, v).Encode()
				if err != nil {
					return err
				}
				if out == "" || out == "-" {
					_, err = fmt.Fprint(os.Stdout, text)
					return err
				}
				if err := os.WriteFile(out, []byte(text), 0o644); err != nil {
					return fmt.Errorf("write %s: %w", out, err)
				}
				fmt.Fprintf(os.Stderr, "Wrote view %q to %s\n", v.Name, out)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "file to write, default is standard output")
	return cmd
}

func viewsImportCmd() *cobra.Command {
	var name string
	var tabs bool
	cmd := &cobra.Command{
		Use: "import <file>", Short: "Create a saved view from a TOML file made by export", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return model.NewExitError(model.ErrUsage, "read %s: %v", args[0], err)
			}
			return withViewRepo(cmd, func(ctx context.Context, d *sql.DB, r *repo.SavedViewRepo) error {
				v, err := r.ImportFile(ctx, data, name, tabs)
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Imported view %q (%s)\n", v.Name, v.Kind)
				return format.Output(os.Stdout, resolveFormat(), []map[string]any{r.Row(ctx, v, true)}, viewColumns, flagQuiet)
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "save under this name instead of the name in the file")
	cmd.Flags().BoolVar(&tabs, "tabs", false, "show the view as a tab")
	return cmd
}

// viewsFieldsCmd lists what a query can name for a kind, so a person or an agent writes valid conditions.
func viewsFieldsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "fields <kind>", Short: "List the fields and operators a saved view of a kind can use", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			infos, ok := query.FieldInfos(args[0])
			if !ok {
				return model.NewExitError(model.ErrValidation, "E_VIEW_KIND_UNKNOWN: kind must be one of %s (got %q)", strings.Join(query.Kinds, ", "), args[0])
			}
			rows := make([]map[string]any, len(infos))
			for i, f := range infos {
				rows[i] = map[string]any{"field": f.Field, "label": f.Label, "type": f.Type, "operators": strings.Join(f.Operators, " "),
					"options": strings.Join(f.Options, ", "), "sortable": f.Sortable, "groupable": f.Groupable}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{
				{Header: "Field", Field: "field"}, {Header: "Type", Field: "type"}, {Header: "Operators", Field: "operators"}, {Header: "Options", Field: "options"}}, flagQuiet)
		},
	}
}
