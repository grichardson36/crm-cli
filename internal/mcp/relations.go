package mcp

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerRelationTools adds the tools around links that are not link and unlink themselves: the
// relation kind vocabulary, the list of an object's links, and role suggestions.
func registerRelationTools(s *server.MCPServer, db *sql.DB) {
	kinds := repo.NewKindRepo(db)
	links := repo.NewLinkRepo(db)

	s.AddTool(
		gomcp.NewTool("crm_relation_kind_list",
			gomcp.WithDescription("List the link kinds usable as kind in crm_person_link and crm_org_link. A kind reads: a is <name> b, and seen from a, b is <inverse_name>"),
			gomcp.WithString("object_type", gomcp.Enum("person", "organization"), gomcp.Description("Only kinds for people or organizations")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			list, err := kinds.List(ctx, req.GetString("object_type", ""))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(list)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_relation_kind_add",
			gomcp.WithDescription("Add a link kind. Without inverse_name the kind is symmetric (friend). With one it is directional (godparent / godchild)"),
			gomcp.WithString("object_type", gomcp.Required(), gomcp.Enum("person", "organization"), gomcp.Description("person or organization")),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Kind name")),
			gomcp.WithString("inverse_name", gomcp.Description("Name seen from the other end")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			k, err := kinds.Add(ctx, req.GetString("object_type", ""), req.GetString("name", ""), req.GetString("inverse_name", ""))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(k)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_relation_kind_remove",
			gomcp.WithDescription("Remove a user-defined link kind that no link uses. Built-in kinds stay"),
			gomcp.WithString("object_type", gomcp.Required(), gomcp.Enum("person", "organization"), gomcp.Description("person or organization")),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("Kind name")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			if err := kinds.Remove(ctx, req.GetString("object_type", ""), req.GetString("name", "")); err != nil {
				return mcpError(err)
			}
			return gomcp.NewToolResultText(fmt.Sprintf("Removed %s kind %q", req.GetString("object_type", ""), req.GetString("name", ""))), nil
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_links",
			gomcp.WithDescription("List every link of a person, organization, deal or task with its attributes (role, from, to, primary, kind label). Hidden objects are left out"),
			gomcp.WithString("kind", gomcp.Required(), gomcp.Enum("person", "organization", "org", "deal", "task"), gomcp.Description("Object kind")),
			gomcp.WithNumber("id", gomcp.Required(), gomcp.Description("Object ID")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			id, errResult := requireID(req, "id")
			if errResult != nil {
				return errResult, nil
			}
			kind, ok := kindOfNoun(req.GetString("kind", ""))
			if !ok {
				return gomcp.NewToolResultError("validation error: kind must be person, organization, deal or task"), nil
			}
			items, err := links.All(ctx, repo.Ref{Kind: kind, ID: id})
			if err != nil {
				return mcpError(err)
			}
			if items == nil {
				items = []model.LinkedObject{}
			}
			return jsonResult(items)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_link_roles",
			gomcp.WithDescription("List the roles already used in a relation, most used first. Roles are free text, these are suggestions"),
			gomcp.WithString("relation", gomcp.Enum("membership", "deal_person", "deal_org"), gomcp.Description("Relation (default membership)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			roles, err := links.Roles(ctx, req.GetString("relation", "membership"), 50)
			if err != nil {
				return mcpError(err)
			}
			if roles == nil {
				roles = []string{}
			}
			return jsonResult(roles)
		},
	)
}
