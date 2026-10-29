package validate

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Hierarchy paths (patterns/nested-tags, patterns/tree-view-module). A field with
// FieldSpec.Hierarchy == HierarchyPath holds a path such as work/clients/acme. The rules live here once,
// so the CLI, MCP, the TUI, imports and sync all refuse the same paths with the same codes.

// checkPath trims the space around every segment and enforces the shape: no leading or trailing
// separator, no empty segment, at most model.MaxPathDepth segments and the length of the field.
// It returns the cleaned path, or the issue.
func checkPath(spec model.FieldSpec, v string) (string, *Issue) {
	label := spec.Label
	if strings.HasPrefix(v, model.PathSep) || strings.HasSuffix(v, model.PathSep) {
		return "", &Issue{Code: CodePathEdgeSlash, Field: label, Expected: "no / at the start or end", Received: v,
			Fix: "write work/clients, not /work/clients or work/clients/"}
	}
	segs := strings.Split(v, model.PathSep)
	for i, s := range segs {
		s = strings.TrimSpace(s)
		if s == "" {
			return "", &Issue{Code: CodePathEmptySegment, Field: label, Expected: "a name between every pair of /", Received: v,
				Fix: "remove the extra /"}
		}
		segs[i] = s
	}
	if len(segs) > model.MaxPathDepth {
		return "", &Issue{Code: CodePathTooDeep, Field: label, Expected: fmt.Sprintf("at most %d levels", model.MaxPathDepth),
			Received: fmt.Sprintf("%d levels", len(segs)), Fix: "use fewer levels"}
	}
	clean := strings.Join(segs, model.PathSep)
	max := spec.Max
	if max == 0 {
		max = model.MaxPathLen
	}
	if n := utf8.RuneCountInString(clean); n > max {
		return "", &Issue{Code: CodePathTooLong, Field: label, Expected: fmt.Sprintf("at most %d characters", max),
			Received: fmt.Sprintf("%d characters", n), Fix: "shorten it"}
	}
	return clean, nil
}

// Path checks a value typed for a hierarchy field of a kind that declares one and returns the cleaned
// path in Result.Value.
func Path(kind, raw string) Result {
	spec, ok := model.HierarchyOf(kind)
	if !ok {
		return Result{Value: raw}
	}
	return Field(spec, raw, Options{Kind: kind})
}

// TagName checks the name of a tag: a hierarchy path (work/clients/acme). The cleaned name is in Value.
func TagName(raw string) Result { return Path("tag", raw) }

// PathMove checks that moving the path from to under itself would not happen: to may not be from or
// inside it. It is the cycle rule of every hierarchy move (rename, nest, merge).
func PathMove(label, from, to string) Result {
	var r Result
	if to == from || strings.HasPrefix(to, from+model.PathSep) {
		r.add(Issue{Code: CodePathCycle, Field: label, Expected: "a place outside " + from, Received: to,
			Fix: "pick a different parent"})
	}
	return r
}
