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
			gomcp.WithDescription("Create an organization. Refuses a duplicate (same name ignoring case, punctuation, and/&, the, Inc/LLC, or same domain) unless allow_duplicate is true"),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Organization name")),
			gomcp.WithString("domain", gomcp.Description("Domain name")),
			gomcp.WithString("industry", gomcp.Description("Industry")),
			gomcp.WithString("notes", gomcp.Description("Notes")),
			gomcp.WithString("website", gomcp.Description("Website URL")),
			gomcp.WithString("size", gomcp.Description("Company size, free text. Suggested: 1-10, 11-50, 51-200, 201-500, 501-1000, 1001-5000, 5000+")),
			gomcp.WithString("city", gomcp.Description("City")),
			gomcp.WithString("region", gomcp.Description("State or region")),
			gomcp.WithString("country", gomcp.Description("Country")),
			gomcp.WithString("linkedin_url", gomcp.Description("LinkedIn company URL")),
			gomcp.WithString("description", gomcp.Description("What the company does")),
			gomcp.WithString("logo_path", gomcp.Description("Path to a logo file (a path only, nothing draws it yet)")),
			gomcp.WithBoolean("allow_duplicate", gomcp.Description("Create even when a live organization has the same normalized name or domain (otherwise this is refused with a conflict naming the existing one)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			name := strings.TrimSpace(req.GetString("name", ""))
			if name == "" {
				return gomcp.NewToolResultError("validation error: name is required"), nil
			}
			o, err := or.Create(ctx, model.CreateOrgInput{Name: name, Domain: optString(req, "domain"), Industry: optString(req, "industry"), Notes: optString(req, "notes"),
				OrgNativeFields: orgNativeFromRequest(req), AllowDuplicate: req.GetBool("allow_duplicate", false)})
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
			gomcp.WithString("website", gomcp.Description("Website URL")),
			gomcp.WithString("size", gomcp.Description("Company size, free text. Suggested: 1-10, 11-50, 51-200, 201-500, 501-1000, 1001-5000, 5000+")),
			gomcp.WithString("city", gomcp.Description("City")),
			gomcp.WithString("region", gomcp.Description("State or region")),
			gomcp.WithString("country", gomcp.Description("Country")),
			gomcp.WithString("linkedin_url", gomcp.Description("LinkedIn company URL")),
			gomcp.WithString("description", gomcp.Description("What the company does")),
			gomcp.WithString("logo_path", gomcp.Description("Path to a logo file (a path only)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			in := model.UpdateOrgInput{Name: optString(req, "name"), Domain: optString(req, "domain"), Industry: optString(req, "industry"), Notes: optString(req, "notes"),
				OrgNativeFields: orgNativeFromRequest(req)}
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
			verb, desc := "link", fmt.Sprintf("Link a %s to another object, with the link's attributes. Linking what is linked changes nothing, and attributes you pass change on the existing link. A person and organization can be linked more than once (new=true) for a second stay", noun)
			if remove {
				verb, desc = "unlink", fmt.Sprintf("Remove the link between a %s and another object. A removed link is hidden, not erased", noun)
			}
			opts := []gomcp.ToolOption{
				gomcp.WithDescription(desc + ". The same link can be made from the other object's tool. Attributes: " + attrHelp(kind)),
				gomcp.WithNumber("id", gomcp.Required(), gomcp.Description(fmt.Sprintf("%s ID", noun))),
				gomcp.WithString("other", gomcp.Required(), gomcp.Enum(enum...), gomcp.Description("Kind of the other object")),
				gomcp.WithNumber("other_id", gomcp.Required(), gomcp.Description("ID of the other object")),
				gomcp.WithString("kind", gomcp.Description("Relation kind for person-person and org-org links (see crm_relation_kind_list). Default: related for people, parent for organizations. Naming the inverse (child) reverses the two ends")),
			}
			if !remove {
				opts = append(opts,
					gomcp.WithString("role", gomcp.Description("Free-text role, for example CTO or champion. Empty clears it")),
					gomcp.WithString("from", gomcp.Description("Start date: YYYY, YYYY-MM, YYYY-MM-DD or today. Empty clears it")),
					gomcp.WithString("to", gomcp.Description("End date, same forms. Empty means current")),
					gomcp.WithBoolean("primary", gomcp.Description("Make this the primary organization of the person or deal")),
					gomcp.WithString("notes", gomcp.Description("Notes on the link")),
					gomcp.WithBoolean("new", gomcp.Description("Add another link even when the pair is already linked (memberships only)")),
				)
			} else {
				opts = append(opts, gomcp.WithNumber("link_id", gomcp.Description("Remove only this link when the pair is linked more than once")))
			}
			s.AddTool(
				gomcp.NewTool(fmt.Sprintf("crm_%s_%s", noun, verb), opts...),
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
						uo := repo.UnlinkOptions{Kind: req.GetString("kind", "")}
						if lid := int64(req.GetInt("link_id", 0)); lid > 0 {
							uo.LinkID = lid
						}
						removed, err := links.Unlink(ctx, a, b, uo)
						if err != nil {
							return mcpError(err)
						}
						out["removed"] = removed
						return jsonResult(out)
					}
					attrs := LinkAttrsFromRequest(req)
					link := links.Link
					if req.GetBool("new", false) {
						link = links.LinkNew
					}
					res, err := link(ctx, a, b, attrs)
					if err != nil {
						return mcpError(err)
					}
					out["changed"], out["created"], out["updated"] = res.Changed, res.Created, res.Updated
					out["link_id"], out["relation"] = res.LinkID, res.Relation
					if len(res.Demoted) > 0 {
						out["demoted"] = res.Demoted
					}
					return jsonResult(out)
				},
			)
		}
	}
}

// attrHelp names the attributes the links of a kind carry, for tool descriptions.
func attrHelp(kind string) string {
	var parts []string
	for _, rel := range model.RelationsOf(kind) {
		if rel.Activity() && rel.A == "interaction" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s-%s: %s", rel.A, rel.B, strings.Join(rel.Attrs, ", ")))
	}
	return strings.Join(parts, "; ")
}

// LinkAttrsFromRequest reads the link attributes an MCP call carries. A value you leave out is not
// given. An empty string clears.
func LinkAttrsFromRequest(req gomcp.CallToolRequest) model.LinkAttrs {
	a := model.LinkAttrs{
		Role: optString(req, "role"), StartedOn: optString(req, "from"), EndedOn: optString(req, "to"),
		Kind: optString(req, "kind"), Notes: optString(req, "notes"),
	}
	if _, ok := req.GetArguments()["primary"]; ok {
		v := req.GetBool("primary", false)
		a.Primary = &v
	}
	return a
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

// orgNativeFromRequest reads the native organization fields. A field left out is nil.
func orgNativeFromRequest(req gomcp.CallToolRequest) model.OrgNativeFields {
	return model.OrgNativeFields{
		Website: optString(req, "website"), Size: optString(req, "size"), City: optString(req, "city"), Region: optString(req, "region"),
		Country: optString(req, "country"), LinkedinURL: optString(req, "linkedin_url"), Description: optString(req, "description"),
		LogoPath: optString(req, "logo_path"),
	}
}
