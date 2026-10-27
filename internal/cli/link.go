package cli

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/format"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/spf13/cobra"
)

// Link, unlink and restore for every object, built once from the object list and model.Relations
// (patterns/objects-first, link-from-either-side). Names read crm <object> link <other> <id> <other id>.

// objectNoun is the CLI word of each object kind.
var objectNoun = map[string]string{"person": "person", "organization": "org", "deal": "deal", "task": "task"}

// kindOfNoun turns a CLI word back into an object kind. "organization" is accepted too.
func kindOfNoun(s string) (string, bool) {
	s = strings.ToLower(s)
	for kind, noun := range objectNoun {
		if s == noun || s == kind {
			return kind, true
		}
	}
	return "", false
}

var linkColumns = []format.ColumnDef{
	{Header: "Action", Field: "action"},
	{Header: "From", Field: "from"},
	{Header: "From ID", Field: "from_id"},
	{Header: "To", Field: "to"},
	{Header: "To ID", Field: "to_id"},
	{Header: "Changed", Field: "changed"},
	{Header: "Link ID", Field: "link_id"},
}

const linkFields = "action,from,from_id,to,to_id,changed,created,updated,link_id,relation,revived,demoted,removed"

// linkMeta describes the generated commands for `crm schema`, --help and --dry-run.
func linkMeta() map[string]cmdMeta {
	meta := map[string]cmdMeta{}
	for _, kind := range model.ObjectKinds {
		noun := objectNoun[kind]
		for _, other := range model.RelatedKinds(kind) {
			on := objectNoun[other]
			meta[noun+" link "+on] = cmdMeta{mutates: true, fields: linkFields, target: kind, target2: other, targetArgs: "pair",
				examples: linkExamples(kind, other, false)}
			meta[noun+" unlink "+on] = cmdMeta{mutates: true, fields: linkFields, target: kind, target2: other, targetArgs: "pair",
				examples: linkExamples(kind, other, true)}
		}
		meta[noun+" link tag"] = cmdMeta{mutates: true, fields: tagFields, target: kind, targetArgs: "first",
			examples: []string{fmt.Sprintf("crm %s link tag 1 vip", noun)}}
		meta[noun+" unlink tag"] = cmdMeta{mutates: true, fields: tagFields, target: kind, targetArgs: "first",
			examples: []string{fmt.Sprintf("crm %s unlink tag 1 vip", noun)}}
		if kind == "organization" || kind == "deal" { // person and task already have restore
			meta[noun+" restore"] = cmdMeta{mutates: true, target: kind, targetArgs: "first",
				examples: []string{fmt.Sprintf("crm %s restore 1", noun)}}
		}
	}
	return meta
}

// linkExamples are the copy-pasteable lines of a generated link command.
func linkExamples(kind, other string, remove bool) []string {
	noun, on := objectNoun[kind], objectNoun[other]
	rel, _ := model.RelationBetween(kind, other)
	if remove {
		out := []string{fmt.Sprintf("crm %s unlink %s 1 1", noun, on)}
		if rel.Kinded {
			out = append(out, fmt.Sprintf("crm %s unlink %s 1 2 --kind friend", noun, on))
		}
		if !rel.Unique && !rel.Activity() {
			out = append(out, fmt.Sprintf("crm %s unlink %s 1 1 --id 4", noun, on))
		}
		return out
	}
	out := []string{fmt.Sprintf("crm %s link %s 1 1", noun, on)}
	switch rel.Name {
	case "membership":
		out = append(out, fmt.Sprintf("crm %s link %s 1 1 --role CTO --from 2019-03 --primary", noun, on),
			fmt.Sprintf("crm %s link %s 1 1 --role Advisor --from 2023 --new", noun, on))
	case "person_relation", "org_relation":
		out = append(out, fmt.Sprintf("crm %s link %s 1 2 --kind parent", noun, on))
	case "deal_person":
		out = append(out, fmt.Sprintf("crm %s link %s 1 1 --role champion", noun, on))
	case "deal_org":
		out = append(out, fmt.Sprintf("crm %s link %s 1 1 --primary", noun, on))
	}
	return out
}

// addObjectVerbs attaches link, unlink and restore to every object command, once.
func addObjectVerbs(root *cobra.Command) {
	for _, kind := range model.ObjectKinds {
		noun := objectNoun[kind]
		var parent *cobra.Command
		for _, c := range root.Commands() {
			if c.Name() == noun {
				parent = c
			}
		}
		if parent == nil {
			continue
		}
		link := &cobra.Command{Use: "link", Short: "Link a " + noun + " to another object, from either side"}
		unlink := &cobra.Command{Use: "unlink", Short: "Remove a link between a " + noun + " and another object"}
		for _, other := range model.RelatedKinds(kind) {
			link.AddCommand(relationCmd(kind, other, false))
			unlink.AddCommand(relationCmd(kind, other, true))
		}
		link.AddCommand(tagLinkCmd(kind, false))
		unlink.AddCommand(tagLinkCmd(kind, true))
		parent.AddCommand(link, unlink)
		if !hasSub(parent, "restore") {
			parent.AddCommand(restoreCmd(kind))
		}
	}
}

func hasSub(c *cobra.Command, name string) bool {
	for _, s := range c.Commands() {
		if s.Name() == name {
			return true
		}
	}
	return false
}

func parseObjectID(what, s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, model.NewExitError(model.ErrValidation, "invalid %s ID: %s", what, s)
	}
	return id, nil
}

// relationCmd is `crm <kind> link|unlink <other> <id> <other id>`. The flags come from the relation
// registry: a membership takes --role --from --to --primary --notes, a person or organization
// relation takes --kind, a deal link takes --role. See `crm help relations`.
func relationCmd(kind, other string, remove bool) *cobra.Command {
	rel, _ := model.RelationBetween(kind, other)
	verb, short := "link", fmt.Sprintf("Link a %s to a %s", objectNoun[kind], objectNoun[other])
	if remove {
		verb, short = "unlink", fmt.Sprintf("Remove the link between a %s and a %s", objectNoun[kind], objectNoun[other])
	}
	var role, from, to, kindName, notes string
	var primary, addNew bool
	var linkID int64
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <%s_id> <%s_id>", objectNoun[other], objectNoun[kind], objectNoun[other]),
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			aid, err := parseObjectID(objectNoun[kind], args[0])
			if err != nil {
				return err
			}
			bid, err := parseObjectID(objectNoun[other], args[1])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			lr := repo.NewLinkRepo(db)
			a, b := repo.Ref{Kind: kind, ID: aid}, repo.Ref{Kind: other, ID: bid}
			row := map[string]any{"action": verb, "from": kind, "from_id": aid, "to": other, "to_id": bid, "changed": true}
			if remove {
				removed, err := lr.Unlink(cmd.Context(), a, b, repo.UnlinkOptions{LinkID: linkID, Kind: kindName})
				if err != nil {
					return err
				}
				ids := make([]int64, len(removed))
				for i, rec := range removed {
					ids[i] = rec.ID
				}
				row["removed"], row["link_id"], row["relation"] = ids, ids[0], rel.Name
				fmt.Fprintf(os.Stderr, "Unlinked %s #%d from %s #%d (%d link(s))\n", kind, aid, other, bid, len(removed))
				return format.Output(os.Stdout, resolveFormat(), []map[string]any{row}, linkColumns, flagQuiet)
			}
			attrs := model.LinkAttrs{}
			flags := cmd.Flags()
			if flags.Changed("role") {
				attrs.Role = &role
			}
			if flags.Changed("from") {
				attrs.StartedOn = &from
			}
			if flags.Changed("to") {
				attrs.EndedOn = &to
			}
			if flags.Changed("primary") {
				attrs.Primary = &primary
			}
			if flags.Changed("kind") {
				attrs.Kind = &kindName
			}
			if flags.Changed("notes") {
				attrs.Notes = &notes
			}
			link := lr.Link
			if addNew {
				link = lr.LinkNew
			}
			res, err := link(cmd.Context(), a, b, attrs)
			if err != nil {
				return err
			}
			row["changed"], row["created"], row["updated"] = res.Changed, res.Created, res.Updated
			row["link_id"], row["relation"] = res.LinkID, res.Relation
			if res.Revived {
				row["revived"] = true
			}
			if len(res.Demoted) > 0 {
				row["demoted"] = res.Demoted
			}
			switch {
			case !res.Changed:
				fmt.Fprintf(os.Stderr, "%s #%d is already linked to %s #%d\n", capitalize(kind), aid, other, bid)
			case res.Revived:
				fmt.Fprintf(os.Stderr, "Linked %s #%d to %s #%d again (the old link came back with its role, dates and notes)\n", kind, aid, other, bid)
			case res.Updated && !res.Created:
				fmt.Fprintf(os.Stderr, "Updated the link of %s #%d and %s #%d\n", kind, aid, other, bid)
			default:
				fmt.Fprintf(os.Stderr, "Linked %s #%d to %s #%d\n", kind, aid, other, bid)
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{row}, linkColumns, flagQuiet)
		},
	}
	f := cmd.Flags()
	if remove {
		f.Int64Var(&linkID, "id", 0, "remove only this link (link_id from `crm "+objectNoun[kind]+" show`) when the pair is linked more than once")
		if rel.Kinded {
			f.StringVar(&kindName, "kind", "", "remove only the link of this kind")
		}
		return cmd
	}
	for _, key := range rel.Attrs {
		switch key {
		case model.AttrRole:
			f.StringVar(&role, "role", "", "free-text role, for example CTO or champion (empty clears)")
		case model.AttrFrom:
			f.StringVar(&from, "from", "", "start date: YYYY, YYYY-MM, YYYY-MM-DD or today (empty clears)")
		case model.AttrTo:
			f.StringVar(&to, "to", "", "end date, same forms. Leave out for a current link (empty clears)")
		case model.AttrPrimary:
			f.BoolVar(&primary, "primary", false, "make this the primary link (--primary=false clears it)")
		case model.AttrKind:
			f.StringVar(&kindName, "kind", "", "link kind, see `crm relation-kind list` (default "+defaultKind(rel)+"). Naming the inverse reverses the ends")
		case model.AttrNotes:
			f.StringVar(&notes, "notes", "", "notes on the link")
		}
	}
	if !rel.Unique {
		f.BoolVar(&addNew, "new", false, "add another link for a pair that is already linked, for example a second stay")
	}
	return cmd
}

func defaultKind(rel model.Relation) string {
	if rel.A == "person" {
		return model.DefaultPersonKind
	}
	return model.DefaultOrgKind
}

// linkLine is one link as text: label, name, role, dates and flags.
func linkLine(li model.LinkedObject) string {
	var parts []string
	if li.Label != "" {
		parts = append(parts, li.Label)
	}
	if li.Role != nil && *li.Role != "" {
		parts = append(parts, *li.Role)
	}
	switch {
	case li.StartedOn != nil && li.EndedOn != nil:
		parts = append(parts, *li.StartedOn+" to "+*li.EndedOn)
	case li.StartedOn != nil:
		parts = append(parts, "since "+*li.StartedOn)
	case li.EndedOn != nil:
		parts = append(parts, "until "+*li.EndedOn)
	}
	if li.Primary {
		parts = append(parts, "primary")
	}
	if !li.Current {
		parts = append(parts, "past")
	}
	line := fmt.Sprintf("%s #%d %s", li.Kind, li.ID, li.Name)
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, ", ") + ")"
	}
	return line + fmt.Sprintf(" [link %d]", li.LinkID)
}

// printLinks writes the links block of a show command in table format. Other formats carry the
// links in the JSON/CSV record.
func printLinks(items []model.LinkedObject) {
	if resolveFormat() != format.FormatTable || len(items) == 0 || flagQuiet {
		return
	}
	fmt.Fprintln(os.Stdout, "\nLinks:")
	for _, li := range items {
		fmt.Fprintln(os.Stdout, "  "+linkLine(li))
	}
}

// showWithLinks prints one object record with its links.
func showWithLinks(cmd *cobra.Command, db *sql.DB, ref repo.Ref, data map[string]any, cols []format.ColumnDef) error {
	items, err := repo.NewLinkRepo(db).All(cmd.Context(), ref)
	if err != nil {
		return err
	}
	if items == nil {
		items = []model.LinkedObject{}
	}
	data["links"] = items
	if err := format.Output(os.Stdout, resolveFormat(), []map[string]any{data}, cols, flagQuiet); err != nil {
		return err
	}
	printLinks(items)
	return nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// tagLinkCmd is `crm <kind> link|unlink tag <id> <tag>...`, the same as crm tag apply|remove.
func tagLinkCmd(kind string, remove bool) *cobra.Command {
	use, short := "tag <id> <tag>...", fmt.Sprintf("Add tags to a %s", objectNoun[kind])
	if remove {
		use, short = "tag <id> <tag>...", fmt.Sprintf("Remove tags from a %s", objectNoun[kind])
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseObjectID(objectNoun[kind], args[0])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			tr := repo.NewTagRepo(db)
			for _, name := range args[1:] {
				name = strings.TrimLeft(name, "#")
				if name == "" {
					continue
				}
				if remove {
					err = tr.Remove(cmd.Context(), kind, id, name)
				} else {
					err = tr.Apply(cmd.Context(), kind, id, name)
				}
				if err != nil {
					return err
				}
			}
			tags, err := tr.GetForEntity(cmd.Context(), kind, id)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, len(tags))
			for i, t := range tags {
				rows[i] = map[string]any{"id": t.ID, "name": t.Name}
			}
			return format.Output(os.Stdout, resolveFormat(), rows, []format.ColumnDef{{Header: "ID", Field: "id"}, {Header: "Tag", Field: "name"}}, flagQuiet)
		},
	}
}

// restoreCmd is `crm <kind> restore <id>`, the one restore for organizations, deals and tasks.
func restoreCmd(kind string) *cobra.Command {
	noun := objectNoun[kind]
	return &cobra.Command{
		Use:   "restore <id>",
		Short: fmt.Sprintf("Restore a deleted %s", noun),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseObjectID(noun, args[0])
			if err != nil {
				return err
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := repo.RestoreObject(cmd.Context(), db, kind, id); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Restored %s #%d\n", noun, id)
			return nil
		},
	}
}
