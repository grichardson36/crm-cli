// Package help holds the built-in documentation topics. The text is Markdown, embedded in the
// binary, so `crm help <topic>` works offline and always matches the installed version.
package help

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed topics/*.md
var topicsFS embed.FS

// order is the reading order of the topic list. Topics not listed here follow alphabetically.
var order = []string{
	"overview", "people", "orgs", "contact-methods", "keep-in-touch", "birthdays", "tasks", "notes", "note-types", "short-syntax", "tags",
	"duplicates", "archive", "domains", "import", "sync", "sync-inbox", "dav", "dav-server", "calendar", "calendar-ics", "calendar-plan", "caldav", "book", "reminders", "output", "errors", "agents",
	"tui", "tui-keys", "theme", "install", "backup",
}

// Topic is one help page.
type Topic struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
}

// Match is one line of a topic that matches a search.
type Match struct {
	Topic string `json:"topic"`
	Line  int    `json:"line"`
	Text  string `json:"text"`
}

// Example is a runnable command from a topic's example block.
type Example struct {
	Topic   string
	Command string
	Skip    bool
}

// List returns every topic in reading order.
func List() []Topic {
	entries, err := topicsFS.ReadDir("topics")
	if err != nil {
		return nil
	}
	byName := map[string]Topic{}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		raw, err := topicsFS.ReadFile("topics/" + e.Name())
		if err != nil {
			continue
		}
		byName[name] = parse(name, string(raw))
	}
	var out []Topic
	seen := map[string]bool{}
	for _, n := range order {
		if t, ok := byName[n]; ok {
			out = append(out, t)
			seen[n] = true
		}
	}
	var rest []string
	for n := range byName {
		if !seen[n] {
			rest = append(rest, n)
		}
	}
	sort.Strings(rest)
	for _, n := range rest {
		out = append(out, byName[n])
	}
	return out
}

// Names returns the topic names in reading order.
func Names() []string {
	topics := List()
	names := make([]string, len(topics))
	for i, t := range topics {
		names[i] = t.Name
	}
	return names
}

// Get returns the topic with the given name.
func Get(name string) (Topic, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, t := range List() {
		if t.Name == name {
			return t, true
		}
	}
	return Topic{}, false
}

// Search finds lines containing text (case-insensitive) in every topic.
func Search(text string) []Match {
	needle := strings.ToLower(strings.TrimSpace(text))
	if needle == "" {
		return nil
	}
	var out []Match
	for _, t := range List() {
		for i, line := range strings.Split(t.Body, "\n") {
			if strings.Contains(strings.ToLower(line), needle) {
				out = append(out, Match{Topic: t.Name, Line: i + 1, Text: strings.TrimSpace(line)})
			}
		}
	}
	return out
}

// Examples returns every command in a fenced block tagged `sh crm-example` or `sh crm-example-skip`.
func Examples() []Example {
	var out []Example
	for _, t := range List() {
		inBlock, skip := false, false
		for _, line := range strings.Split(t.Body, "\n") {
			trim := strings.TrimSpace(line)
			switch {
			case !inBlock && (trim == "```sh crm-example" || trim == "```sh crm-example-skip"):
				inBlock, skip = true, strings.HasSuffix(trim, "-skip")
			case inBlock && strings.HasPrefix(trim, "```"):
				inBlock = false
			case inBlock && trim != "" && !strings.HasPrefix(trim, "#"):
				out = append(out, Example{Topic: t.Name, Command: trim, Skip: skip})
			}
		}
	}
	return out
}

// SplitArgs splits an example command line into arguments the way a shell does for simple
// cases: whitespace separates, and single or double quotes group. No expansion, no pipes.
func SplitArgs(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	quote := rune(0)
	started := false
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, started = r, true
		case r == ' ' || r == '\t':
			if started || cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote in %q", line)
	}
	if started || cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args, nil
}

func parse(name, raw string) Topic {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	t := Topic{Name: name, Title: name, Body: strings.TrimSpace(raw) + "\n"}
	lines := strings.Split(t.Body, "\n")
	rest := lines
	if len(lines) > 0 && strings.HasPrefix(lines[0], "# ") {
		t.Title = strings.TrimSpace(strings.TrimPrefix(lines[0], "# "))
		rest = lines[1:]
	}
	var para []string
	for _, l := range rest {
		if strings.TrimSpace(l) == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, strings.TrimSpace(l))
	}
	t.Summary = firstSentence(strings.Join(para, " "))
	return t
}

// firstSentence returns s up to and including the first ". ", or all of s.
func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

// FormatList renders a topic list for people: name, then summary.
func FormatList(topics []Topic) string {
	width := 0
	for _, t := range topics {
		if len(t.Name) > width {
			width = len(t.Name)
		}
	}
	var b strings.Builder
	for _, t := range topics {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, t.Name, t.Summary)
	}
	return b.String()
}
