package validate_test

import (
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
)

func TestTagNamePaths(t *testing.T) {
	tests := []struct {
		name, in, want, code string
	}{
		{"plain", "work", "work", ""},
		{"nested", "work/clients/acme", "work/clients/acme", ""},
		{"spaces around segments are trimmed", " work / clients ", "work/clients", ""},
		{"eight levels is the limit", "a/b/c/d/e/f/g/h", "a/b/c/d/e/f/g/h", ""},
		{"nine levels", "a/b/c/d/e/f/g/h/i", "", validate.CodePathTooDeep},
		{"empty segment", "work//clients", "", validate.CodePathEmptySegment},
		{"blank segment", "work/ /clients", "", validate.CodePathEmptySegment},
		{"leading slash", "/work", "", validate.CodePathEdgeSlash},
		{"trailing slash", "work/", "", validate.CodePathEdgeSlash},
		{"just a slash", "/", "", validate.CodePathEdgeSlash},
		{"too long", strings.Repeat("x", 201), "", validate.CodePathTooLong},
		{"longest", strings.Repeat("x", 200), strings.Repeat("x", 200), ""},
		{"empty", "", "", validate.CodeFieldRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := validate.TagName(tt.in)
			if tt.code == "" {
				assert.True(t, res.OK(), res.Issues)
				assert.Equal(t, tt.want, res.Value)
				return
			}
			if assert.False(t, res.OK()) {
				assert.Equal(t, tt.code, res.Errors()[0].Code)
			}
		})
	}
}

func TestPathMove(t *testing.T) {
	tests := []struct {
		name, from, to string
		bad            bool
	}{
		{"into itself", "a", "a", true},
		{"into a child", "a", "a/b", true},
		{"a sibling with the same prefix is fine", "a", "ab/c", false},
		{"up", "a/b", "a", false},
		{"elsewhere", "a", "b/a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.bad, !validate.PathMove("tag", tt.from, tt.to).OK())
		})
	}
}
