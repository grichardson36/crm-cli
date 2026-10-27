package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
)

// autoOrgForCompany applies the company-text rule to a person: the text creates or reuses an
// organization and the person's primary membership points to it. Junk strings (crm org ignore) and
// an empty text do nothing. What it did goes to stderr.
func autoOrgForCompany(ctx context.Context, db *sql.DB, personID int64, company string) error {
	ch, err := repo.NewOrgAutoRepo(db).ApplyCompany(ctx, personID, company, repo.ModePrimary)
	if err != nil {
		return err
	}
	printCompanyChange(ch)
	return nil
}

func printCompanyChange(ch repo.CompanyChange) {
	if ch.OrgCreated {
		fmt.Fprintf(os.Stderr, "Created organization %q (id %d) from the company text\n", ch.OrgName, ch.OrgID)
		for _, s := range ch.Similar {
			fmt.Fprintf(os.Stderr, "  similar to %q (id %d), merge with: crm org merge %d %d\n", s.Name, s.OrgID, s.OrgID, ch.OrgID)
		}
	}
}
