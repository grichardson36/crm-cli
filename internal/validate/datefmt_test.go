package validate_test

import (
	"testing"
	"time"

	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/stretchr/testify/assert"
)

// TestDateFormsAtTheValidationLayer checks the typed date forms through validate.Field with the
// default parser and an injected clock (2026-10-15), the path the CLI and MCP take.
func TestDateFormsAtTheValidationLayer(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name, kind, key, in, want string
	}{
		{"due slash mdy", "task", "due", "12/25/2026", "2026-12-25"},
		{"due dash mdy", "task", "due", "12-25-2026", "2026-12-25"},
		{"due two digit year", "task", "due", "12/25/26", "2026-12-25"},
		{"due day first", "task", "due", "25/12/2026", "2026-12-25"},
		{"due month day this year", "task", "due", "12/25", "2026-12-25"},
		{"due month day rolls to next year", "task", "due", "10/14", "2027-10-14"},
		{"due month name", "task", "due", "Dec 25", "2026-12-25"},
		{"due month name with year", "task", "due", "December 25 2026", "2026-12-25"},
		{"due day then month name", "task", "due", "25 Dec 2026", "2026-12-25"},
		{"due day then long month", "task", "due", "25 December", "2026-12-25"},
		{"due iso still works", "task", "due", "2026-12-01", "2026-12-01"},
		{"due impossible day", "task", "due", "2/30/2026", ""},
		{"due both numbers above 12", "task", "due", "25/25/2026", ""},
		{"due month dash day stays an error", "task", "due", "12-25", ""},
		{"due junk", "task", "due", "someday", ""},
		{"birthday slash with year", "person", "birthday", "3/14/1985", "1985-03-14"},
		{"birthday dash with year", "person", "birthday", "03-14-1985", "1985-03-14"},
		{"birthday day first", "person", "birthday", "14/3/1985", "1985-03-14"},
		{"birthday month day has no year", "person", "birthday", "3/14", "--03-14"},
		{"birthday month name has no year", "person", "birthday", "Mar 14", "--03-14"},
		{"birthday month name with year", "person", "birthday", "14 Mar 1985", "1985-03-14"},
		{"birthday keeps the stored forms", "person", "birthday", "--03-14", "--03-14"},
		{"birthday two digit year is not guessed", "person", "birthday", "3/14/85", ""},
		{"birthday impossible", "person", "birthday", "2/30", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := validate.Field(spec(t, tt.kind, tt.key), tt.in, validate.Options{Kind: tt.kind, Now: now})
			if tt.want == "" {
				if assert.False(t, res.OK()) {
					assert.Equal(t, validate.CodeDateInvalid, res.Errors()[0].Code)
				}
				return
			}
			if assert.True(t, res.OK(), res.Err()) {
				assert.Equal(t, tt.want, res.Value)
			}
		})
	}
}
