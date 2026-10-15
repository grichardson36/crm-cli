package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/quick"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

// inlineText is the result of reading short syntax in the text argument of an add command.
type inlineText struct {
	Res shortsyntax.Result
	Env quick.Env
}

// parseInline reads the short syntax in text. With noSyntax the text is left as it is.
func parseInline(ctx context.Context, db *sql.DB, text string, noSyntax bool, skip ...shortsyntax.Kind) (*inlineText, error) {
	now := time.Now()
	env, err := quick.LoadEnv(ctx, db, now, 0)
	if err != nil {
		return nil, err
	}
	res := shortsyntax.Parse(text, shortsyntax.Options{
		Now: now, Disabled: noSyntax, KnownTags: env.Tags, People: env.Index.Names(), Skip: skip,
	})
	return &inlineText{Res: res, Env: env}, nil
}

// people resolves the +Name tokens to person IDs. A name with no match is a not-found error and a
// name with several matches is a validation error that lists them.
func (in *inlineText) people() ([]int64, error) {
	var ids []int64
	seen := map[int64]bool{}
	for _, name := range in.Res.People {
		matches, _ := in.Env.Index.ResolveLoose(name)
		switch {
		case len(matches) == 0:
			return nil, model.NewExitError(model.ErrNotFound, "no person matches +%s", name)
		case len(matches) > 1:
			var parts []string
			for _, m := range matches {
				parts = append(parts, fmt.Sprintf("%s #%d", m.Name, m.ID))
			}
			return nil, model.NewExitError(model.ErrValidation, "%d people match +%s: %s (use the ID with --person)", len(matches), name, strings.Join(parts, ", "))
		}
		if !seen[matches[0].ID] {
			seen[matches[0].ID] = true
			ids = append(ids, matches[0].ID)
		}
	}
	return ids, nil
}

// printInlineDryRun writes what an add command would do. It is JSON in every format, because the point
// is to show the parsed fields.
func printInlineDryRun(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "nothing was written")
	return nil
}
