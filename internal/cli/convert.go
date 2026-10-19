package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

var convertColumns = []format.ColumnDef{
	{Header: "Key", Field: "key"},
	{Header: "Text", Field: "display"},
	{Header: "People", Field: "people"},
	{Header: "Match", Field: "match_text"},
	{Header: "Domain", Field: "domain"},
	{Header: "Domain match", Field: "domain_match_text"},
	{Header: "Review", Field: "review"},
}

var convertPlanColumns = []format.ColumnDef{
	{Header: "Key", Field: "key"},
	{Header: "Action", Field: "action"},
	{Header: "People", Field: "people"},
	{Header: "Organization", Field: "org_name"},
	{Header: "Domain", Field: "domain"},
	{Header: "Applied", Field: "applied"},
	{Header: "Note", Field: "note"},
}

const (
	convertFields     = "key,display,people,variants,review,skipped,match,match_text,domain,domain_match,domain_match_text,people_list"
	convertPlanFields = "key,action,people,org_id,org_name,domain,note,applied"
)

// parseDecisionFlags reads --link KEY=ORGID, --create KEY[=NAME], --skip KEY and --unskip KEY.
func parseDecisionFlags(links, creates, skips, unskips []string) ([]repo.ConvertDecision, error) {
	var out []repo.ConvertDecision
	for _, s := range links {
		i := strings.LastIndex(s, "=")
		if i < 0 {
			return nil, model.NewExitError(model.ErrUsage, "--link wants KEY=ORG_ID, got %q", s)
		}
		id, err := strconv.ParseInt(strings.TrimSpace(s[i+1:]), 10, 64)
		if err != nil {
			return nil, model.NewExitError(model.ErrUsage, "--link %q: the organization id is not a number", s)
		}
		out = append(out, repo.ConvertDecision{Key: strings.TrimSpace(s[:i]), Action: repo.ConvertLink, OrgID: id})
	}
	for _, s := range creates {
		key, name, _ := strings.Cut(s, "=")
		out = append(out, repo.ConvertDecision{Key: strings.TrimSpace(key), Action: repo.ConvertCreate, Name: strings.TrimSpace(name)})
	}
	for _, k := range skips {
		out = append(out, repo.ConvertDecision{Key: strings.TrimSpace(k), Action: repo.ConvertSkip})
	}
	for _, k := range unskips {
		out = append(out, repo.ConvertDecision{Key: strings.TrimSpace(k), Action: repo.ConvertUnskip})
	}
	return out, nil
}

func orgConvertCmd() *cobra.Command {
	var limit, minPeople int
	var all, withPeople, statsOnly, apply, acceptExact, reviewOnly bool
	var file string
	var links, creates, skips, unskips []string

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Review company text and turn it into organizations, in passes",
		Long: "Lists the people's company text grouped by normalized name (case, spacing and Inc/LLC/Ltd/Corp/Co endings ignored for grouping only), with the existing organization each group matches.\n" +
			"Nothing is created by looking. Give a decision per group with --create KEY[=NAME], --link KEY=ORG_ID, --skip KEY, or a --decisions file (a JSON array of {key, action, org_id, name, domain}), and --accept-exact to link every group that matches an organization exactly. Without --apply the decisions are only previewed.\n" +
			"A group also shows the dominant email domain of its people (free-mail providers ignored) and the organization with that domain, a suggestion that --accept-exact never takes. Create sets the new organization's domain to it unless the decision has one.\n" +
			"Create makes the organization and gives each person a membership (primary when they have none), the person's title as the role. people.company stays as it is. A converted group does not come back, a skipped group stays away until its text changes. See 'crm help convert'.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			cr := repo.NewConvertRepo(db)
			ctx := cmd.Context()

			decisions, err := parseDecisionFlags(links, creates, skips, unskips)
			if err != nil {
				return err
			}
			if file != "" {
				raw, err := os.ReadFile(file)
				if err != nil {
					return model.NewExitError(model.ErrUsage, "read %s: %v", file, err)
				}
				var fromFile []repo.ConvertDecision
				if err := json.Unmarshal(raw, &fromFile); err != nil {
					return model.NewExitError(model.ErrUsage, "%s is not a JSON array of decisions: %v", file, err)
				}
				decisions = append(decisions, fromFile...)
			}
			groups, stats, err := cr.Groups(ctx, all)
			if err != nil {
				return err
			}
			if acceptExact {
				decisions = append(decisions, repo.ExactDecisions(groups)...)
			}

			if len(decisions) > 0 {
				plan, err := cr.Plan(ctx, decisions)
				if err != nil {
					return err
				}
				rows := make([]map[string]any, len(plan))
				for i, p := range plan {
					rows[i] = map[string]any{"key": p.Key, "action": p.Action, "people": p.People, "org_id": p.OrgID, "org_name": p.OrgName, "domain": p.Domain, "note": p.Note, "applied": false}
				}
				if apply {
					ch, err := cr.Apply(ctx, decisions)
					if err != nil {
						return err
					}
					for _, r := range rows {
						r["applied"] = true
					}
					fmt.Fprintf(os.Stderr, "Converted %d people in %d groups, created %d organizations\n", ch.PeopleDone, ch.GroupsDone, len(ch.OrgsCreated))
				} else {
					fmt.Fprintln(os.Stderr, "Preview only: add --apply to write these decisions")
				}
				return format.Output(os.Stdout, resolveFormat(), rows, convertPlanColumns, flagQuiet)
			}
			if apply {
				return model.NewExitError(model.ErrUsage, "--apply needs decisions: --create, --link, --skip, --decisions or --accept-exact")
			}

			if statsOnly {
				return format.Output(os.Stdout, resolveFormat(), []map[string]any{{
					"people": stats.People, "groups": stats.Groups, "matched": stats.Matched, "fuzzy": stats.Fuzzy,
					"domain_matches": stats.DomainMatches, "domains": stats.Domains, "skipped_groups": stats.Skipped, "review_people": stats.Review,
				}}, []format.ColumnDef{{Header: "People", Field: "people"}, {Header: "Groups", Field: "groups"}, {Header: "Matched", Field: "matched"}, {Header: "Fuzzy", Field: "fuzzy"}, {Header: "Domain matches", Field: "domain_matches"}, {Header: "Skipped", Field: "skipped_groups"}}, flagQuiet)
			}
			var rows []map[string]any
			for _, g := range groups {
				if g.Count < minPeople || (reviewOnly && g.Review == 0) {
					continue
				}
				rows = append(rows, g.Record(withPeople))
				if limit > 0 && len(rows) >= limit {
					break
				}
			}
			fmt.Fprintln(os.Stderr, stats.Line())
			return format.Output(os.Stdout, resolveFormat(), rows, convertColumns, flagQuiet)
		},
	}
	f := cmd.Flags()
	f.IntVar(&limit, "limit", 0, "show only the first N groups (biggest first)")
	f.IntVar(&minPeople, "min", 0, "show only groups with at least N people")
	f.BoolVar(&all, "all", false, "include groups that were skipped")
	f.BoolVar(&reviewOnly, "review", false, "only groups with people tagged org-review (phone ORG that matched nothing)")
	f.BoolVar(&withPeople, "people", false, "list the people of each group")
	f.BoolVar(&statsOnly, "stats", false, "print only the counts")
	f.BoolVar(&apply, "apply", false, "write the decisions (without it they are previewed)")
	f.BoolVar(&acceptExact, "accept-exact", false, "link every group that matches an existing organization exactly")
	f.StringVar(&file, "decisions", "", "JSON file with decisions: [{\"key\":\"acme\",\"action\":\"create\",\"name\":\"Acme\"}]")
	f.StringArrayVar(&links, "link", nil, "link a group to an organization: KEY=ORG_ID (repeatable)")
	f.StringArrayVar(&creates, "create", nil, "create an organization for a group: KEY or KEY=NAME (repeatable)")
	f.StringArrayVar(&skips, "skip", nil, "skip a group until its text changes: KEY (repeatable)")
	f.StringArrayVar(&unskips, "unskip", nil, "bring a skipped group back: KEY (repeatable)")
	return cmd
}
