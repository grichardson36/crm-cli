package cli

import (
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
}

const linkFields = "action,from,from_id,to,to_id,changed,replaced_id"

// linkMeta describes the generated commands for `crm schema`, --help and --dry-run.
func linkMeta() map[string]cmdMeta {
	meta := map[string]cmdMeta{}
	for _, kind := range model.ObjectKinds {
		noun := objectNoun[kind]
		for _, other := range model.RelatedKinds(kind) {
			on := objectNoun[other]
			meta[noun+" link "+on] = cmdMeta{mutates: true, fields: linkFields, target: kind, target2: other, targetArgs: "pair",
				examples: []string{fmt.Sprintf("crm %s link %s 1 1", noun, on)}}
			meta[noun+" unlink "+on] = cmdMeta{mutates: true, fields: linkFields, target: kind, target2: other, targetArgs: "pair",
				examples: []string{fmt.Sprintf("crm %s unlink %s 1 1", noun, on)}}
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

// relationCmd is `crm <kind> link|unlink <other> <id> <other id>`.
func relationCmd(kind, other string, remove bool) *cobra.Command {
	verb, short := "link", fmt.Sprintf("Link a %s to a %s", objectNoun[kind], objectNoun[other])
	if remove {
		verb, short = "unlink", fmt.Sprintf("Remove the link between a %s and a %s", objectNoun[kind], objectNoun[other])
	}
	return &cobra.Command{
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
				if err := lr.Unlink(cmd.Context(), a, b); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "Unlinked %s #%d from %s #%d\n", kind, aid, other, bid)
			} else {
				res, err := lr.Link(cmd.Context(), a, b)
				if err != nil {
					return err
				}
				row["changed"] = res.Changed
				if res.Replaced != nil {
					row["replaced_id"] = res.Replaced.ID
				}
				switch {
				case !res.Changed:
					fmt.Fprintf(os.Stderr, "%s #%d is already linked to %s #%d\n", capitalize(kind), aid, other, bid)
				case res.Replaced != nil:
					fmt.Fprintf(os.Stderr, "Linked %s #%d to %s #%d (replaced %s #%d)\n", kind, aid, other, bid, other, res.Replaced.ID)
				default:
					fmt.Fprintf(os.Stderr, "Linked %s #%d to %s #%d\n", kind, aid, other, bid)
				}
			}
			return format.Output(os.Stdout, resolveFormat(), []map[string]any{row}, linkColumns, flagQuiet)
		},
	}
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
