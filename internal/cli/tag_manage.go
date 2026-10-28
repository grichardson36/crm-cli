package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/spf13/cobra"
)

var tagUnusedColumns = []format.ColumnDef{{Header: "ID", Field: "id"}, {Header: "Name", Field: "name"}}

var tagDuplicateColumns = []format.ColumnDef{
	{Header: "Key", Field: "key"}, {Header: "Tag", Field: "tag"}, {Header: "Count", Field: "count"}, {Header: "Merge into", Field: "keep"},
}

func tagUnusedCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "unused",
		Short: "List tags no object carries (preview unless --apply deletes them)",
		Long: "Lists the tags that no object carries, archived objects included. Without --apply this only lists them. " +
			"With --apply the tag names are deleted. The terminal UI can undo that in the same session.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			tr := repo.NewTagRepo(db)
			res, err := tr.DeleteUnused(cmd.Context(), !apply)
			if err != nil {
				return err
			}
			if apply {
				fmt.Fprintf(os.Stderr, "Deleted %d unused tags\n", res.Count)
			} else {
				fmt.Fprintf(os.Stderr, "%d unused tags (add --apply to delete them)\n", res.Count)
			}
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(res)
			}
			rows := make([]map[string]any, len(res.Tags))
			for i, n := range res.Tags {
				rows[i] = map[string]any{"id": "", "name": n}
			}
			if stats, err := tr.Stats(cmd.Context()); err == nil && !apply {
				ids := map[string]int64{}
				for _, s := range stats {
					ids[s.Name] = s.ID
				}
				for i, n := range res.Tags {
					rows[i]["id"] = ids[n]
				}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, tagUnusedColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "delete the unused tags (without it they are only listed)")
	return cmd
}

func tagDuplicatesCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "duplicates",
		Short: "List tags that differ only by case or separators (preview unless --apply merges them)",
		Long: "Finds tags that differ only by case, spaces, hyphens or underscores (Friend, friend, the-well, The_Well). " +
			"Each group lists the most used spelling to merge the others into. Without --apply this only lists the groups. " +
			"With --apply every group is merged into its most used spelling.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			tr := repo.NewTagRepo(db)
			groups, err := tr.Duplicates(cmd.Context())
			if err != nil {
				return err
			}
			var merged *repo.TagDupMergeResult
			if apply {
				if merged, err = tr.MergeDuplicates(cmd.Context(), false); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Merged %d duplicate tags in %d groups\n", merged.Merged, merged.Groups)
			} else {
				fmt.Fprintf(os.Stderr, "%d groups of duplicate tags (add --apply to merge each into its most used spelling)\n", len(groups))
			}
			if resolveFormat() == format.FormatJSON {
				if groups == nil {
					groups = []repo.TagDupGroup{}
				}
				return json.NewEncoder(os.Stdout).Encode(map[string]any{"groups": groups, "applied": apply})
			}
			var rows []map[string]any
			for _, g := range groups {
				for _, t := range g.Tags {
					rows = append(rows, map[string]any{"key": g.Key, "tag": t.Name, "count": t.Count, "keep": g.Keep})
				}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, tagDuplicateColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "merge every group into its most used spelling (without it they are only listed)")
	return cmd
}
