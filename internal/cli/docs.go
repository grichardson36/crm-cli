package cli

import (
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// registerDocsCommand adds `crm docs markdown|man <dir>`, generated from the command tree.
func registerDocsCommand(rootCmd *cobra.Command) {
	docs := &cobra.Command{
		Use:   "docs",
		Short: "Generate reference documentation from the command tree",
	}

	docs.AddCommand(&cobra.Command{
		Use:   "markdown <dir>",
		Short: "Write one Markdown reference page per command into <dir>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := makeDocDir(args[0]); err != nil {
				return err
			}
			if err := doc.GenMarkdownTree(cmd.Root(), args[0]); err != nil {
				return fmt.Errorf("write markdown: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote Markdown pages to %s\n", args[0])
			return nil
		},
	})

	docs.AddCommand(&cobra.Command{
		Use:   "man <dir>",
		Short: "Write one man page (section 1) per command into <dir>",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := makeDocDir(args[0]); err != nil {
				return err
			}
			header := &doc.GenManHeader{Title: "CRM", Section: "1"}
			if err := doc.GenManTree(cmd.Root(), header, args[0]); err != nil {
				return fmt.Errorf("write man pages: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote man pages to %s\n", args[0])
			return nil
		},
	})

	rootCmd.AddCommand(docs)
}

func makeDocDir(dir string) error {
	if dir == "" {
		return model.NewExitError(model.ErrValidation, "directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return nil
}
