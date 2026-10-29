package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/phone"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPhoneTools adds the E.164 tools (crm phone normalize|parse).
func registerPhoneTools(s *server.MCPServer, db *sql.DB) {
	people := repo.NewPersonRepo(db)
	s.AddTool(
		gomcp.NewTool("crm_phone_normalize",
			gomcp.WithDescription("Fill the E.164 form (+14155550100) of existing phone numbers. Dry run by default: returns counts (total, fill, current, unparseable, unlikely, duplicates) and the first changes. Set apply to true to write them in one transaction. Only the E.164 column is written, the typed text never changes. Idempotent. Matching and duplicate detection use E.164, numbers without a country code are read with the default region (config.toml [phone] region, US when unset). Set undo to true to clear every stored E.164 form again"),
			gomcp.WithBoolean("apply", gomcp.Description("Write the changes (default false: dry run)")),
			gomcp.WithBoolean("undo", gomcp.Description("Clear every stored E.164 form instead (the typed text is untouched)")),
			gomcp.WithNumber("limit", gomcp.Description("Max changes listed (default 20)")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			if req.GetBool("undo", false) {
				n, err := people.ClearPhoneE164(ctx)
				if err != nil {
					return mcpError(err)
				}
				return jsonResult(map[string]any{"cleared": n})
			}
			rep, err := people.NormalizePhones(ctx, req.GetBool("apply", false), req.GetInt("limit", 20))
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(map[string]any{"counts": rep, "line": rep.Line()})
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_phone_parse",
			gomcp.WithDescription("Show how a phone number is read: E.164, national and international format, whether it is possible and valid for its region. Read only"),
			gomcp.WithString("number", gomcp.Required(), gomcp.Description("The number as typed")),
			gomcp.WithString("region", gomcp.Description("Region for a number without a country code, for example GB (default: the configured region)")),
		),
		func(_ context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			typed, err := req.RequireString("number")
			if err != nil {
				return mcpError(err)
			}
			region := req.GetString("region", "")
			n := phone.ParseRegion(typed, region)
			return jsonResult(map[string]any{"typed": typed, "region": phone.DefaultRegionOr(region), "e164": n.E164, "key": n.Key(),
				"national": n.National(), "international": n.International(), "number_region": n.Region,
				"parsed": n.Parsed, "possible": n.Possible, "valid": n.Valid})
		},
	)
}
