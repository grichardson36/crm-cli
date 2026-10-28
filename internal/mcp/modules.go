package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func defaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".crm"), nil
}

// registerModuleTools adds the module arrangement tools (M14): list, set and reset. The
// arrangement is per device, in config.toml, and shapes the terminal UI only.
func registerModuleTools(s *server.MCPServer, dataDir func() (string, error)) {
	if dataDir == nil {
		dataDir = defaultDataDir
	}
	load := func() (string, modules.Arrangement, error) {
		dir, err := dataDir()
		if err != nil {
			return "", modules.Default(), err
		}
		a, _ := modules.Load(dir)
		return dir, a, nil
	}
	s.AddTool(
		gomcp.NewTool("crm_modules_list",
			gomcp.WithDescription("List the terminal UI modules (views) in order with on or off, the position among the visible ones and the number key. Per device, from config.toml"),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			_, a, err := load()
			if err != nil {
				return mcpError(err)
			}
			return jsonResult(a.Rows())
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_modules_set",
			gomcp.WithDescription("Turn a module on or off and/or move it. Modules: home, people, follow-up, tasks, organizations, birthdays, merge, archived, sync, deals, settings. Off hides the view from tabs, number keys, [ ] and the palette and lists it under Hidden in the Views screen, its data and links stay. Settings cannot be turned off, Archived and Sync start off. One module must stay on, and Home or People"),
			gomcp.WithString("module", gomcp.Required(), gomcp.Description("Module name")),
			gomcp.WithBoolean("on", gomcp.Description("true turns it on, false turns it off")),
			gomcp.WithNumber("position", gomcp.Description("Place among all modules, 1 is first")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			name := req.GetString("module", "")
			dir, a, err := load()
			if err != nil {
				return mcpError(err)
			}
			args := req.GetArguments()
			if _, ok := args["on"]; !ok {
				if _, ok := args["position"]; !ok {
					return gomcp.NewToolResultError("validation error: give on, position or both"), nil
				}
			}
			if v, ok := args["on"].(bool); ok {
				if a, err = a.With(name, v); err != nil {
					return mcpError(fmt.Errorf("%v: %w", err, model.ErrValidation))
				}
			}
			if _, ok := args["position"]; ok {
				if a, err = a.Moved(name, req.GetInt("position", 0)); err != nil {
					return mcpError(fmt.Errorf("%v: %w", err, model.ErrValidation))
				}
			}
			if err := modules.Save(dir, a); err != nil {
				return mcpError(err)
			}
			return jsonResult(a.Rows())
		},
	)
	s.AddTool(
		gomcp.NewTool("crm_modules_reset",
			gomcp.WithDescription("Put the default module arrangement back"),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			dir, _, err := load()
			if err != nil {
				return mcpError(err)
			}
			if err := modules.Save(dir, modules.Default()); err != nil {
				return mcpError(err)
			}
			return jsonResult(modules.Default().Rows())
		},
	)
}
