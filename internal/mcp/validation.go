package mcp

import (
	"encoding/json"

	"github.com/jdanielnd/crm-cli/internal/validate"
	gomcp "github.com/mark3labs/mcp-go/mcp"
)

// validationBody is the isError content of a failed validation: the first failing issue flat
// (code, field, expected, received, fix) plus every issue and the smart-correction suggestions.
type validationBody struct {
	Error       string                `json:"error"`
	Code        string                `json:"code"`
	Field       string                `json:"field,omitempty"`
	Expected    string                `json:"expected,omitempty"`
	Received    string                `json:"received,omitempty"`
	Fix         string                `json:"fix,omitempty"`
	Issues      []validate.Issue      `json:"issues"`
	Suggestions []validate.Suggestion `json:"suggestions,omitempty"`
}

// validationResult builds the tool error for a validate.Error.
func validationResult(ve *validate.Error) *gomcp.CallToolResult {
	first := ve.First()
	b, _ := json.Marshal(validationBody{Error: "validation error: " + ve.Error(), Code: first.Code, Field: first.Field,
		Expected: first.Expected, Received: first.Received, Fix: first.Fix, Issues: ve.Issues, Suggestions: ve.Suggestions})
	return gomcp.NewToolResultError(string(b))
}
