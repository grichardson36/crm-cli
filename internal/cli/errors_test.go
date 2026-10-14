package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"syscall"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorCodeAndExitCode(t *testing.T) {
	cases := []struct {
		err      error
		wantCode string
		wantExit int
	}{
		{fmt.Errorf("person 9: %w", model.ErrNotFound), "not_found", 3},
		{model.NewExitError(model.ErrValidation, "bad value"), "validation", 2},
		{model.NewExitError(model.ErrConflict, "exists"), "conflict", 4},
		{fmt.Errorf("open: %w", model.ErrDatabase), "database", 10},
		{model.NewExitError(model.ErrUsage, "accepts 1 arg(s)"), "usage", 2},
		{errors.New(`unknown command "x" for "crm"`), "usage", 2},
		{errors.New("unknown flag: --bogus"), "usage", 2},
		{errors.New("required flag(s) \"contacts\" not set"), "usage", 2},
		{errors.New("something odd"), "unknown", 1},
	}
	for _, tc := range cases {
		t.Run(tc.wantCode+"/"+tc.err.Error(), func(t *testing.T) {
			assert.Equal(t, tc.wantCode, ErrorCode(tc.err))
			assert.Equal(t, tc.wantExit, ExitCodeFor(tc.err))
		})
	}
}

func TestReportError(t *testing.T) {
	err := fmt.Errorf("person 9: %w", model.ErrNotFound)

	var human bytes.Buffer
	assert.Equal(t, 3, ReportError(&human, err, []string{"person", "show", "9"}))
	assert.Equal(t, "crm: error: person 9: not found\n", human.String())

	for _, args := range [][]string{{"person", "show", "9", "-f", "json"}, {"--format=json", "x"}, {"-f", "JSON"}, {"-fjson"}} {
		var js bytes.Buffer
		assert.Equal(t, 3, ReportError(&js, err, args), args)
		var body errorBody
		require.NoError(t, json.Unmarshal(js.Bytes(), &body), js.String())
		assert.Equal(t, errorDetail{Code: "not_found", Message: "person 9: not found", ExitCode: 3}, body.Error)
	}

	var csv bytes.Buffer
	ReportError(&csv, err, []string{"-f", "csv"})
	assert.Contains(t, csv.String(), "crm: error:")
}

func TestBrokenPipeIsQuiet(t *testing.T) {
	var out bytes.Buffer
	assert.Equal(t, 0, ReportError(&out, fmt.Errorf("write: %w", syscall.EPIPE), []string{"-f", "json"}))
	assert.Equal(t, 0, ReportError(&out, errors.New("write /dev/stdout: The pipe has been ended."), nil))
	assert.Empty(t, out.String())
}
