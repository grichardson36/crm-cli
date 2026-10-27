package cli

import (
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/spf13/cobra"
)

// reportHints prints the warnings and suggestions of a validation result to stderr. A hint never
// fails the command: the errors are returned by the repository and reported by ReportError.
func reportHints(cmd *cobra.Command, res validate.Result) {
	w := cmd.ErrOrStderr()
	for _, i := range res.Warnings() {
		fmt.Fprintf(w, "crm: warning: %s %s: %s, %s\n", i.Code, i.Field, i.Expected, i.Fix)
	}
	for _, s := range res.Suggestions {
		fmt.Fprintf(w, "crm: hint: %s %s: %s (%q)\n", s.Code, s.Field, s.Message, s.Value)
	}
}
