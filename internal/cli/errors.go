package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// errorBody is the object printed to stderr when -f json is set.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	ExitCode int    `json:"exit_code"`
}

// ErrorCode names the class of an error: not_found, validation, conflict, database, usage or unknown.
func ErrorCode(err error) string {
	switch {
	case errors.Is(err, model.ErrNotFound):
		return "not_found"
	case errors.Is(err, model.ErrValidation):
		return "validation"
	case errors.Is(err, model.ErrConflict):
		return "conflict"
	case errors.Is(err, model.ErrDatabase):
		return "database"
	case errors.Is(err, model.ErrUsage), isCobraUsageError(err):
		return "usage"
	default:
		return "unknown"
	}
}

// isCobraUsageError spots errors Cobra raises itself before a command runs.
func isCobraUsageError(err error) bool {
	msg := err.Error()
	for _, p := range []string{"unknown command", "unknown flag", "unknown shorthand flag", "required flag", "flag needs an argument", "invalid argument", "unknown help topic"} {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}

// isBrokenPipe is true when writing to stdout failed because the reader went away.
func isBrokenPipe(err error) bool {
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) && (errno == 109 || errno == 232) { // Windows: ERROR_BROKEN_PIPE, ERROR_NO_DATA
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "pipe has been ended") || strings.Contains(msg, "pipe is being closed")
}

// ExitCodeFor maps an error to the process exit code, including Cobra usage errors (2).
func ExitCodeFor(err error) int {
	if ErrorCode(err) == "usage" {
		return 2
	}
	return model.ExitCode(err)
}

// WantsJSONErrors reports whether the command line asked for -f json.
func WantsJSONErrors(args []string) bool {
	for i, a := range args {
		switch {
		case a == "--":
			return false
		case a == "-f" || a == "--format":
			if i+1 < len(args) && strings.EqualFold(args[i+1], "json") {
				return true
			}
		case strings.EqualFold(a, "--format=json"), strings.EqualFold(a, "-f=json"), strings.EqualFold(a, "-fjson"):
			return true
		}
	}
	return false
}

// ReportError prints err to w and returns the exit code. With -f json (found in args) the error
// is the JSON object {"error":{"code","message","exit_code"}}, otherwise "crm: error: message".
func ReportError(w io.Writer, err error, args []string) int {
	if isBrokenPipe(err) {
		return 0 // the reader (such as `head`) closed the pipe on purpose
	}
	code := ExitCodeFor(err)
	if WantsJSONErrors(args) {
		b, _ := json.Marshal(errorBody{Error: errorDetail{Code: ErrorCode(err), Message: err.Error(), ExitCode: code}})
		fmt.Fprintln(w, string(b))
		return code
	}
	fmt.Fprintf(w, "crm: error: %s\n", err.Error())
	return code
}
