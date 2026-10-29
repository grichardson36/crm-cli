package validate

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxSearchTemplate is the longest web search address a setting may hold.
const MaxSearchTemplate = 500

// QueryPlaceholder is where the search text goes in a web search address.
const QueryPlaceholder = "{query}"

var hexColorRe = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// SearchTemplate checks a web search address typed in Settings (Behavior > Web search > Custom). It
// must be an http or https address with a host, hold {query} exactly once and have no spaces. The
// cleaned address is in Result.Value. An empty value is fine and means "not set".
func SearchTemplate(raw string) Result {
	var r Result
	v, issues, sugg := Sanitize("Web search address", raw, false)
	r.Issues = append(r.Issues, issues...)
	r.Suggestions = append(r.Suggestions, sugg...)
	if v == "" {
		return r
	}
	bad := func(received, fix string) Result {
		r.add(Issue{Code: CodeSearchTemplate, Field: "Web search address", Expected: "an http or https address with " + QueryPlaceholder + " where the text goes", Received: received, Fix: fix})
		return r
	}
	if utf8.RuneCountInString(v) > MaxSearchTemplate {
		r.add(Issue{Code: CodeFieldTooLong, Field: "Web search address", Expected: "at most 500 characters", Fix: "shorten it"})
		return r
	}
	if strings.ContainsAny(v, " \t") {
		return bad(v, "remove the spaces, the search text is put in place of "+QueryPlaceholder)
	}
	if n := strings.Count(v, QueryPlaceholder); n != 1 {
		return bad(v, "write "+QueryPlaceholder+" once, for example https://example.com/search?q="+QueryPlaceholder)
	}
	u, err := url.Parse(strings.ReplaceAll(v, QueryPlaceholder, "x"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || !strings.Contains(u.Hostname(), ".") {
		return bad(v, "start with https:// and a site name, for example https://example.com/search?q="+QueryPlaceholder)
	}
	r.Value = v
	return r
}

// HexColor checks a custom color typed in Settings > Colors: #rgb or #rrggbb. The cleaned value is
// lower case, in Result.Value.
func HexColor(raw string) Result {
	var r Result
	v, issues, _ := Sanitize("Color", raw, false)
	r.Issues = append(r.Issues, issues...)
	if !hexColorRe.MatchString(v) {
		r.add(Issue{Code: CodeColorInvalid, Field: "Color", Expected: "a hex color like #3b82f6 or #38f", Received: v, Fix: "start with # and use 3 or 6 hex digits"})
		return r
	}
	r.Value = strings.ToLower(v)
	return r
}
