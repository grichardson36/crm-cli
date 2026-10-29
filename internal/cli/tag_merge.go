package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/spf13/cobra"
)

const tagMergeFields = "source,target,dry_run,moved,duplicates,total"

var tagMergeColumns = []format.ColumnDef{
	{Header: "Change", Field: "change"}, {Header: "Type", Field: "type"}, {Header: "Count", Field: "count"},
}

func tagRenameCmd() *cobra.Command {
	var merge bool
	cmd := &cobra.Command{
		Use:   "rename <tag> <new-name>",
		Short: "Rename a tag (its children follow)",
		Long: "Gives a tag a new name on every record that has it. Children follow: renaming work to job turns work/clients into job/clients. " +
			"A name another tag already has, in any case, is refused and the error says to merge instead ('crm tag merge'). " +
			"With --merge the tag is merged into that tag, children with the same names merging too. Changing only the case of the tag's own name is allowed.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			oldName, newName := repo.NormalizeTagName(args[0]), repo.NormalizeTagName(args[1])
			res, err := repo.NewTagRepo(db).RenameTree(cmd.Context(), oldName, newName, merge)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Renamed tag %q to %q (%d renamed, %d merged)\n", oldName, newName, res.Renamed, res.Merged)
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{{"old": oldName, "name": newName}},
				[]format.ColumnDef{{Header: "Old", Field: "old"}, {Header: "Name", Field: "name"}}, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&merge, "merge", false, "when the new name exists, merge into that tag instead of refusing")
	return cmd
}

func tagMergeCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "merge <source> <target>",
		Short: "Merge a tag into another (preview unless --apply)",
		Long: "Moves every record tagged <source> to <target>, counts the ones that already had <target> once, and removes <source>. " +
			"Children follow: the children of <source> become children of <target> and children with the same names merge. " +
			"Without --apply this only shows what would change.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			tr := repo.NewTagRepo(db)
			// A tag with children, or a parent that is no tag, merges as a tree: the children follow.
			if virtual, _ := tr.IsVirtual(cmd.Context(), args[0]); virtual {
				return mergeTreeOutput(cmd, tr, args[0], args[1], apply)
			}
			if st, err := tr.SubtreeStat(cmd.Context(), args[0]); err == nil && st.Tags > 1 {
				return mergeTreeOutput(cmd, tr, args[0], args[1], apply)
			}
			var res *repo.TagMergeResult
			if apply {
				res, err = tr.Merge(cmd.Context(), args[0], args[1])
			} else {
				res, err = tr.MergeDryRun(cmd.Context(), args[0], args[1])
			}
			if err != nil {
				return err
			}
			if apply {
				fmt.Fprintf(os.Stderr, "Merged tag %q into %q (%d records moved)\n", res.Source, res.Target, res.Total)
			} else {
				fmt.Fprintf(os.Stderr, "Preview only: add --apply to merge %q into %q (%d records move)\n", res.Source, res.Target, res.Total)
			}
			if resolveFormat() == format.FormatJSON {
				return json.NewEncoder(os.Stdout).Encode(res)
			}
			var rows []map[string]any
			for _, t := range repo.SortedTypes(res.Moved) {
				rows = append(rows, map[string]any{"change": "moved", "type": t, "count": res.Moved[t]})
			}
			for _, t := range repo.SortedTypes(res.Duplicates) {
				rows = append(rows, map[string]any{"change": "already tagged", "type": t, "count": res.Duplicates[t]})
			}
			return format.Output(os.Stdout, resolveFormat(), rows, tagMergeColumns, flagQuiet)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the merge (without it the merge is only previewed)")
	return cmd
}

