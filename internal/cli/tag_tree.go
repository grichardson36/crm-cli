package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/tree"
	"github.com/spf13/cobra"
)

// Nested tags on the command line (patterns/nested-tags): crm tag tree, nest and unnest. Rename, merge
// and delete work on a tag with its children (tag.go, tag_merge.go).

// treeNodeJSON is a node of crm tag tree -f json.
type treeNodeJSON struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Depth    int    `json:"depth"`
	Virtual  bool   `json:"virtual"`
	Own      int    `json:"own"`
	Total    int    `json:"total"`
	Children int    `json:"children"`
}

func tagTreeCmd() *cobra.Command {
	var unused bool
	cmd := &cobra.Command{
		Use:   "tree",
		Short: "Show the tags as a tree (nested tags: work/clients is a child of work)",
		Long: "Shows every tag in use as a tree. A name with / in it is a child of the name before the last /, and a parent no tag is named after " +
			"is shown as a virtual parent. Each line has the objects of the tag itself and, in brackets, the objects with everything below it (an object counts once). " +
			"With -f json the nodes come as a flat list in tree order.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			nodes, err := repo.NewTagRepo(db).Tree(cmd.Context(), unused)
			if err != nil {
				return err
			}
			if resolveFormat() == format.FormatJSON {
				out := make([]treeNodeJSON, len(nodes))
				for i, n := range nodes {
					out[i] = treeNodeJSON{Path: n.Path, Name: n.Name, Depth: n.Depth, Virtual: n.Virtual, Own: n.Own, Total: n.Total, Children: n.Children}
				}
				return json.NewEncoder(os.Stdout).Encode(out)
			}
			if flagQuiet {
				for _, n := range nodes {
					fmt.Fprintln(os.Stdout, n.Path)
				}
				return nil
			}
			fmt.Fprint(os.Stdout, renderTagTree(nodes))
			return nil
		},
	}
	cmd.Flags().BoolVar(&unused, "unused", false, "include tags no object carries")
	return cmd
}

// renderTagTree draws the nodes indented, two spaces per level.
func renderTagTree(nodes []tree.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		b.WriteString(strings.Repeat("  ", n.Depth-1))
		b.WriteString(n.Name)
		switch {
		case n.Virtual:
			fmt.Fprintf(&b, "  (%d)", n.Total)
		case n.Children > 0:
			fmt.Fprintf(&b, "  %d (%d)", n.Own, n.Total)
		default:
			fmt.Fprintf(&b, "  %d", n.Own)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func tagNestCmd() *cobra.Command {
	var under string
	var merge, dry bool
	cmd := &cobra.Command{
		Use:   "nest <tag>... --under <parent>",
		Short: "Put tags under a parent tag (a path rename)",
		Long: "Moves each tag under the parent: nesting work under area makes area/work, and the children of work follow. " +
			"The parent is a path and needs no tag of its own. An empty --under moves the tags to the top level. " +
			"A name the parent already holds is refused, or with --merge merged. All or nothing, one undo step in the terminal UI.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("under") {
				return model.NewExitError(model.ErrValidation, "--under <parent> is required (use --under \"\" for the top level)")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			tr := repo.NewTagRepo(db)
			var res *repo.TagTreeResult
			if dry {
				res, err = tr.NestDryRun(cmd.Context(), args, under, merge)
			} else {
				res, err = tr.Nest(cmd.Context(), args, under, merge)
			}
			if err != nil {
				return err
			}
			announceTreeResult(res, "nested under "+under)
			return printTreeResult(res)
		},
	}
	cmd.Flags().StringVar(&under, "under", "", "the parent tag path")
	cmd.Flags().BoolVar(&merge, "merge", false, "merge into a tag that already has the new name instead of refusing")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "show what would change and write nothing")
	return cmd
}

func tagUnnestCmd() *cobra.Command {
	var merge, dry bool
	cmd := &cobra.Command{
		Use:   "unnest <tag>...",
		Short: "Move tags up one level (a/b/c becomes a/c)",
		Long:  "Moves each tag up one level and its children with it. A tag at the top level stays. With --merge a name that exists is merged into instead of refused.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			tr := repo.NewTagRepo(db)
			var res *repo.TagTreeResult
			if dry {
				res, err = tr.UnnestDryRun(cmd.Context(), args, merge)
			} else {
				res, err = tr.Unnest(cmd.Context(), args, merge)
			}
			if err != nil {
				return err
			}
			announceTreeResult(res, "unnested")
			return printTreeResult(res)
		},
	}
	cmd.Flags().BoolVar(&merge, "merge", false, "merge into a tag that already has the new name instead of refusing")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "show what would change and write nothing")
	return cmd
}

func announceTreeResult(res *repo.TagTreeResult, what string) {
	if res.DryRun {
		fmt.Fprintf(os.Stderr, "Dry run, nothing changed: %d tags would be renamed, %d merged\n", res.Renamed, res.Merged)
		return
	}
	fmt.Fprintf(os.Stderr, "Tags %s: %d renamed, %d merged\n", what, res.Renamed, res.Merged)
}

var tagTreeOpColumns = []format.ColumnDef{
	{Header: "Change", Field: "kind"}, {Header: "From", Field: "from"}, {Header: "To", Field: "to"}, {Header: "Records", Field: "records"},
}

// printTreeResult writes the steps of a tree change: JSON, or one row per step.
func printTreeResult(res *repo.TagTreeResult) error {
	if resolveFormat() == format.FormatJSON {
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	rows := make([]map[string]any, len(res.Ops))
	for i, op := range res.Ops {
		rows[i] = map[string]any{"kind": op.Kind, "from": op.From, "to": op.To, "records": op.Records}
	}
	return format.Output(os.Stdout, resolveFormat(), rows, tagTreeOpColumns, flagQuiet)
}

// mergeTreeOutput merges a tag with its children (or only previews it).
func mergeTreeOutput(cmd *cobra.Command, tr *repo.TagRepo, source, target string, apply bool) error {
	var res *repo.TagTreeResult
	var err error
	if apply {
		res, err = tr.MergeTree(cmd.Context(), source, target)
	} else {
		res, err = tr.MergeTreeDryRun(cmd.Context(), source, target)
	}
	if err != nil {
		return err
	}
	if apply {
		fmt.Fprintf(os.Stderr, "Merged tag %q into %q with its children (%d renamed, %d merged, %d records moved)\n", source, target, res.Renamed, res.Merged, res.Records)
	} else {
		fmt.Fprintf(os.Stderr, "Preview only: add --apply to merge %q into %q with its children (%d renamed, %d merged)\n", source, target, res.Renamed, res.Merged)
	}
	return printTreeResult(res)
}
