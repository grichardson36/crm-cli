package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// objectNoun is the tool-name word of each object kind, the same words the CLI uses.
var objectNoun = map[string]string{"person": "person", "organization": "org", "deal": "deal", "task": "task"}

// registerObjectTools adds the verbs every object shares, built once from the object list and
// model.Relations: restore, and link and unlink for each relation, from either end. It also fills
// the gaps in org and deal (create, update, delete) so every door has the same verbs.
func registerObjectTools(s *server.MCPServer, db *sql.DB) {
	or := repo.NewOrgRepo(db)
	links := repo.NewLinkRepo(db)
	tagr := repo.NewTagRepo(db)

	s.AddTool(
		gomcp.NewTool("crm_tag_remove",
			gomcp.WithDescription("Remove a tag from an entity (the opposite of crm_tag_apply)"),
			gomcp.WithString("entity_type", gomcp.Required(), gomcp.Description("Entity type: person, organization, deal, interaction, task")),
			gomcp.WithNumber("entity_id", gomcp.Required(), gomcp.Description("Entity ID")),
			gomcp.WithString("tag", gomcp.Required(), gomcp.Description("Tag name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "entity_id")
			if errResult != nil {
				return errResult, nil
			}
			entity, tag := req.GetString("entity_type", ""), req.GetString("tag", "")
			if err := tagr.Remove(ctx, entity, id, tag); err != nil {
				return mcpError(err)
			}
			return gomcp.NewToolResultText(fmt.Sprintf("Removed tag %q from %s #%d", tag, entity, id)), nil
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_org_create",
			gomcp.WithDescription("Create an organization"),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Organization name")),
			gomcp.WithString("domain", gomcp.Description("Domain name")),
			gomcp.WithString("industry", gomcp.Description("Industry")),
			gomcp.WithString("notes", gomcp.Description("Notes")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			name := strings.TrimSpace(req.GetString("name", ""))
			if name == "" {
				return gomcp.NewToolResultError("validation error: name is required"), nil
			}
			o, err := or.Create(ctx, model.CreateOrgInput{Name: name, Domain: optString(req, "domain"), Industry: optString(req, "industry"), Notes: optString(req, "notes")})
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(o)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_org_update",
			gomcp.WithDescription("Update organization fields. A field you leave out is unchanged"),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Organization ID")),
			gomcp.WithString("name", gomcp.Description("Organization name")),
			gomcp.WithString("domain", gomcp.Description("Domain name")),
			gomcp.WithString("industry", gomcp.Description("Industry")),
			gomcp.WithString("notes", gomcp.Description("Notes")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			in := model.UpdateOrgInput{Name: optString(req, "name"), Domain: optString(req, "domain"), Industry: optString(req, "industry"), Notes: optString(req, "notes")}
			o, err := or.Update(ctx, id, in)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(o)
		},
	)

	for _, kind := range []string{"organization", "deal", "task", "person"} {
		kind := kind
		noun := objectNoun[kind]
		if kind != "person" {
			// Person delete and restore exist as crm_person_archive and crm_person_restore.
			if kind != "task" { // crm_task_delete exists
				s.AddTool(
					gomcp.NewTool(fmt.Sprintf("crm_%s_delete", noun),
						gomcp.WithDescription(fmt.Sprintf("Delete (archive) a %s. It is soft and crm_%s_restore brings it back", noun, noun)),
						gomcp.WithNumber("id", gomcp.Required(), gomcp.Description(fmt.Sprintf("%s ID", noun))),
					),
					func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
						id, errResult := requireID(req, "id")
						if errResult != nil {
							return errResult, nil
						}
						if err := repo.ArchiveObject(ctx, db, kind, id); err != nil {
							return mcpError(err)
						}
						return gomcp.NewToolResultText(fmt.Sprintf("%s #%d deleted", strings.ToUpper(noun[:1])+noun[1:], id)), nil
					},
				)
			}
			s.AddTool(
				gomcp.NewTool(fmt.Sprintf("crm_%s_restore", noun),
					gomcp.WithDescription(fmt.Sprintf("Restore a deleted %s", noun)),
					gomcp.WithNumber("id", gomcp.Required(), gomcp.Description(fmt.Sprintf("%s ID", noun))),
				),
				func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
					id, errResult := requireID(req, "id")
					if errResult != nil {
						return errResult, nil
					}
					if err := repo.RestoreObject(ctx, db, kind, id); err != nil {
						return mcpError(err)
					}
					return gomcp.NewToolResultText(fmt.Sprintf("%s #%d restored", strings.ToUpper(noun[:1])+noun[1:], id)), nil
				},
			)
		}

		kinds := model.RelatedKinds(kind)
		if len(kinds) == 0 {
			continue
		}
		enum := make([]string, len(kinds))
		for i, k := range kinds {
			enum[i] = objectNoun[k]
		}
		for _, remove := range []bool{false, true} {
			remove := remove
			verb, desc := "link", fmt.Sprintf("Link a %s to another object. Linking what is linked changes nothing. A %s with a single value for that link (person to org, deal to org or person, task to deal) is moved to the new object", noun, noun)
			if remove {
				verb, desc = "unlink", fmt.Sprintf("Remove the link between a %s and another object", noun)
			}
			s.AddTool(
				gomcp.NewTool(fmt.Sprintf("crm_%s_%s", noun, verb),
					gomcp.WithDescription(desc+". The same link can be made from the other object's tool"),
					gomcp.WithNumber("id", gomcp.Required(), gomcp.Description(fmt.Sprintf("%s ID", noun))),
					gomcp.WithString("other", gomcp.Required(), gomcp.Enum(enum...), gomcp.Description("Kind of the other object")),
					gomcp.WithNumber("other_id", gomcp.Required(), gomcp.Description("ID of the other object")),
				),
				func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
					id, errResult := requireID(req, "id")
					if errResult != nil {
						return errResult, nil
					}
					otherID, errResult := requireID(req, "other_id")
					if errResult != nil {
						return errResult, nil
					}
					other, ok := kindOfNoun(req.GetString("other", ""))
					if !ok {
						return gomcp.NewToolResultError("validation error: other must be one of " + strings.Join(enum, ", ")), nil
					}
					a, b := repo.Ref{Kind: kind, ID: id}, repo.Ref{Kind: other, ID: otherID}
					out := map[string]any{"action": verb, "from": kind, "from_id": id, "to": other, "to_id": otherID, "changed": true}
					if remove {
						if err := links.Unlink(ctx, a, b); err != nil {
							return mcpError(err)
						}
						return jsonResult(out)
					}
					res, err := links.Link(ctx, a, b)
					if err != nil {
						return mcpError(err)
					}
					out["changed"] = res.Changed
					if res.Replaced != nil {
						out["replaced_id"] = res.Replaced.ID
					}
					return jsonResult(out)
				},
			)
		}
	}
}

// kindOfNoun turns a tool word (org) or a kind (organization) into an object kind.
func kindOfNoun(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for kind, noun := range objectNoun {
		if s == noun || s == kind {
			return kind, true
		}
	}
	return "", false
}

// optString returns a pointer to the argument when the caller passed it, even if empty.
func optString(req gomcp.CallToolRequest, key string) *string {
	if _, ok := req.GetArguments()[key]; !ok {
		return nil
	}
	v := req.GetString(key, "")
	return &v
}
