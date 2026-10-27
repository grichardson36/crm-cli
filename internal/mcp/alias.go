package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerAliasTools adds the given-name alias tools (crm alias add|list|remove).
func registerAliasTools(s *server.MCPServer, db *sql.DB) {
	ar := repo.NewAliasRepo(db)
	s.AddTool(
		gomcp.NewTool("crm_alias_list",
			gomcp.WithDescription("List the given-name aliases in effect (nickname to full name): the built-in set and the user's additions. A built-in the user turned off shows active=false. Person search, the duplicate finder, the command bar and the relation typeahead read this list"),
		),
		func(ctx context.Context, _ gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			list, err := ar.List(ctx)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(list)
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_alias_add",
			gomcp.WithDescription("Make a word an alias of a given name (zach of zachary). Both are one word of letters. Searching the alias also finds the full name and the duplicate finder treats them as one name. Turns a removed built-in back on"),
			gomcp.WithString("alias", gomcp.Required(), gomcp.Description("The nickname or variant")),
			gomcp.WithString("name", gomcp.Required(), gomcp.Description("The full given name it stands for")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			a, err1 := req.RequireString("alias")
			n, err2 := req.RequireString("name")
			if err1 != nil || err2 != nil {
				return gomcp.NewToolResultError("validation error: alias and name are required: " + model.ErrValidation.Error()), nil
			}
			changed, err := ar.Add(ctx, a, n)
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"alias": a, "name": n, "changed": changed})
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_alias_remove",
			gomcp.WithDescription("Stop treating a word as a name alias: a user addition is deleted, a built-in is turned off"),
			gomcp.WithString("alias", gomcp.Required(), gomcp.Description("The alias word")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			a, err := req.RequireString("alias")
			if err != nil {
				return gomcp.NewToolResultError("validation error: alias is required: " + model.ErrValidation.Error()), nil
			}
			if err := ar.Remove(ctx, a); err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"alias": a, "removed": true})
		},
	)
}
