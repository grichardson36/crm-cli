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
	"github.com/jdanielnd/crm-cli/internal/phone"
	"github.com/spf13/cobra"
)

func registerContextCommand(rootCmd *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "context <person_id_or_name>",
		Short: "Full context briefing for a person",
		Long:  "Returns person profile, organizations, recent interactions, open deals, open tasks, relationships, and tags.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			pr := repo.NewPersonRepo(db)

			// Try to parse as ID first, otherwise search by name
			var person *model.Person
			id, parseErr := parseID(args[0])
			if parseErr == nil {
				person, err = pr.FindByID(cmd.Context(), id)
				if err != nil {
					return err
				}
			} else {
				people, err := pr.Search(cmd.Context(), args[0], 1)
				if err != nil {
					return err
				}
				if len(people) == 0 {
					return model.NewExitError(model.ErrNotFound, "no person found matching %q", args[0])
				}
				person = people[0]
			}

			ctx := cmd.Context()
			result := map[string]any{}

			// Person profile
			if err := pr.LoadDetails(ctx, person); err != nil {
				return err
			}
			result["person"] = personToMap(person)

			// Organizations: the primary one as before, all of them with role and dates
			var org *model.Organization
			lr := repo.NewLinkRepo(db)
			memberships, err := lr.Linked(ctx, repo.Ref{Kind: "person", ID: person.ID}, "organization")
			if err != nil {
				return err
			}
			if len(memberships) > 0 {
				result["organizations"] = memberships
			}
			for _, ms := range memberships {
				if ms.Primary {
					or := repo.NewOrgRepo(db)
					org, err = or.FindByID(ctx, ms.ID)
					if err != nil {
						return err
					}
					result["organization"] = orgToMap(org)
					break
				}
			}

			// Recent interactions
			ir := repo.NewInteractionRepo(db)
			interactions, err := ir.FindAll(ctx, model.InteractionFilters{PersonID: &person.ID, Limit: 10})
			if err != nil {
				return err
			}
			if len(interactions) > 0 {
				result["recent_interactions"] = interactionsToMaps(interactions)
			}

			// Open deals
			dr := repo.NewDealRepo(db)
			deals, err := dr.FindAll(ctx, model.DealFilters{PersonID: &person.ID})
			if err != nil {
				return err
			}
			if len(deals) > 0 {
				result["deals"] = dealsToMaps(deals)
			}

			// Open tasks
			tr := repo.NewTaskRepo(db)
			tasks, err := tr.FindAll(ctx, model.TaskFilters{PersonID: &person.ID})
			if err != nil {
				return err
			}
			if len(tasks) > 0 {
				result["tasks"] = tasksToMaps(tasks)
			}

			// Related people, with the kind of each link
			rels, err := lr.Linked(ctx, repo.Ref{Kind: "person", ID: person.ID}, "person")
			if err != nil {
				return err
			}
			if len(rels) > 0 {
				relMaps := make([]map[string]any, len(rels))
				for i, rel := range rels {
					relMaps[i] = map[string]any{
						"id":                rel.LinkID,
						"person_id":         person.ID,
						"related_person_id": rel.ID,
						"name":              rel.Name,
						"type":              rel.Label,
					}
					if rel.Notes != nil {
						relMaps[i]["notes"] = *rel.Notes
					}
				}
				result["relationships"] = relMaps
			}

			// Tags
			tagRepo := repo.NewTagRepo(db)
			tags, err := tagRepo.GetForEntity(ctx, "person", person.ID)
			if err != nil {
				return err
			}
			if len(tags) > 0 {
				tagNames := make([]string, len(tags))
				for i, t := range tags {
					tagNames[i] = t.Name
				}
				result["tags"] = tagNames
			}

			// JSON/CSV/TSV: output as structured data
			f := resolveFormat()
			if f == format.FormatJSON || f == format.FormatCSV || f == format.FormatTSV {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			// Table/human-readable: custom formatted output
			w := os.Stdout
			name := person.FirstName
			if person.LastName != nil {
				name += " " + *person.LastName
			}
			fmt.Fprintf(w, "# %s (ID: %d)\n", name, person.ID)

			if person.Email != nil {
				fmt.Fprintf(w, "  Email: %s\n", *person.Email)
			}
			if person.Phone != nil {
				fmt.Fprintf(w, "  Phone: %s\n", phone.Display(*person.Phone, ""))
			}
			if person.Title != nil {
				fmt.Fprintf(w, "  Title: %s\n", *person.Title)
			}
			if person.Company != nil {
				fmt.Fprintf(w, "  Company: %s\n", *person.Company)
			}
			if person.Location != nil {
				fmt.Fprintf(w, "  Location: %s\n", *person.Location)
			}
			for _, ms := range memberships {
				fmt.Fprintf(w, "  Organization: %s\n", linkLine(ms))
			}
			if person.Summary != nil {
				fmt.Fprintf(w, "  Summary: %s\n", *person.Summary)
			}

			if tagNames, ok := result["tags"].([]string); ok && len(tagNames) > 0 {
				fmt.Fprintf(w, "\nTags: %s\n", strings.Join(tagNames, ", "))
			}

			if len(interactions) > 0 {
				fmt.Fprintf(w, "\nRecent Interactions (%d):\n", len(interactions))
				for _, i := range interactions {
					subject := ""
					if i.Subject != nil {
						subject = *i.Subject
					}
					dir := ""
					if i.Direction != nil {
						dir = " [" + *i.Direction + "]"
					}
					fmt.Fprintf(w, "  #%-4d %s: %s%s (%s)\n", i.ID, i.Type, subject, dir, i.OccurredAt)
				}
			}

			if len(deals) > 0 {
				fmt.Fprintf(w, "\nDeals (%d):\n", len(deals))
				for _, d := range deals {
					value := ""
					if d.Value != nil {
						value = fmt.Sprintf(" $%.0f", *d.Value)
					}
					fmt.Fprintf(w, "  #%-4d %s [%s]%s\n", d.ID, d.Title, d.Stage, value)
				}
			}

			if len(tasks) > 0 {
				fmt.Fprintf(w, "\nOpen Tasks (%d):\n", len(tasks))
				for _, t := range tasks {
					due := ""
					if t.DueAt != nil {
						due = " (due " + *t.DueAt + ")"
					}
					fmt.Fprintf(w, "  #%-4d %s [%s]%s\n", t.ID, t.Title, t.Priority, due)
				}
			}

			if len(rels) > 0 {
				fmt.Fprintf(w, "\nRelationships (%d):\n", len(rels))
				for _, rel := range rels {
					note := ""
					if rel.Notes != nil {
						note = " — " + *rel.Notes
					}
					fmt.Fprintf(w, "  Person #%d %s (%s)%s\n", rel.ID, rel.Name, rel.Label, note)
				}
			}

			return nil
		},
	}

	rootCmd.AddCommand(cmd)
}

func parseID(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
