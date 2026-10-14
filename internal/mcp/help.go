package mcp

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/help"
	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Option configures NewServer.
type Option func(*options)

type options struct {
	schema func() any
}

// WithSchema supplies the function behind the crm_schema tool. The command tree lives in the cli
// package, which imports this one, so the caller passes it in.
func WithSchema(f func() any) Option { return func(o *options) { o.schema = f } }

// HelpResourceURI is the URI of the help topic resource, such as crm://help/duplicates.
func HelpResourceURI(topic string) string { return "crm://help/" + topic }

// ToolNames lists the names of every tool NewServer registers. It builds a server without a
// database and never starts a transport.
func ToolNames() []string {
	s := NewServer((*sql.DB)(nil), "")
	tools := s.ListTools()
	names := make([]string, 0, len(tools))
	for n := range tools {
		names = append(names, n)
	}
	return names
}

// registerHelpTools adds crm_help, crm_schema and one resource per help topic.
func registerHelpTools(s *server.MCPServer, o options) {
	s.AddTool(
		gomcp.NewTool("crm_help",
			gomcp.WithDescription("Built-in documentation. With no arguments, list the topics. With topic, return the guide (Markdown). With search, find a word across all topics."),
			gomcp.WithString("topic", gomcp.Description("Topic name, such as overview, duplicates or agents")),
			gomcp.WithString("search", gomcp.Description("Case-insensitive text to find in all topics")),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			if q := req.GetString("search", ""); q != "" {
				matches := help.Search(q)
				if matches == nil {
					matches = []help.Match{}
				}
				return jsonResult(matches)
			}
			if name := req.GetString("topic", ""); name != "" {
				t, ok := help.Get(name)
				if !ok {
					return gomcp.NewToolResultError("not found: no help topic " + name + " (call crm_help with no arguments for the list)"), nil
				}
				return jsonResult(t)
			}
			type item struct {
				Name    string `json:"name"`
				Title   string `json:"title"`
				Summary string `json:"summary"`
			}
			var items []item
			for _, t := range help.List() {
				items = append(items, item{t.Name, t.Title, t.Summary})
			}
			return jsonResult(items)
		},
	)

	s.AddTool(
		gomcp.NewTool("crm_schema",
			gomcp.WithDescription("Machine-readable manifest of the crm CLI: every command with flags, examples, whether it changes data, output fields, exit codes, the JSON error format and the MCP tool names."),
		),
		func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
			if o.schema == nil {
				return gomcp.NewToolResultError("schema is not available in this server"), nil
			}
			return jsonResult(o.schema())
		},
	)

	for _, t := range help.List() {
		topic := t
		s.AddResource(
			gomcp.NewResource(HelpResourceURI(topic.Name), "Help: "+topic.Title,
				gomcp.WithResourceDescription(topic.Summary),
				gomcp.WithMIMEType("text/markdown")),
			func(ctx context.Context, req gomcp.ReadResourceRequest) ([]gomcp.ResourceContents, error) {
				return []gomcp.ResourceContents{gomcp.TextResourceContents{
					URI: HelpResourceURI(topic.Name), MIMEType: "text/markdown", Text: topic.Body,
				}}, nil
			},
		)
	}
}
