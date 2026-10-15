// Package ics reads and writes iCalendar (RFC 5545) files: the content-line syntax in this file,
// and the mapping of VEVENT and VTODO to typed values in event.go.
package ics

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Property is one content line: NAME;PARAM=value:VALUE.
type Property struct {
	Name   string // upper case
	Params map[string][]string
	Value  string // raw, still escaped
}

// Param returns the first value of a parameter, or "".
func (p Property) Param(name string) string {
	if v := p.Params[strings.ToUpper(name)]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// Component is BEGIN:NAME ... END:NAME with its properties and sub-components.
type Component struct {
	Name     string
	Props    []Property
	Children []*Component
}

// First returns the first property with the name.
func (c *Component) First(name string) (Property, bool) {
	for _, p := range c.Props {
		if p.Name == name {
			return p, true
		}
	}
	return Property{}, false
}

// All returns every property with the name.
func (c *Component) All(name string) []Property {
	var out []Property
	for _, p := range c.Props {
		if p.Name == name {
			out = append(out, p)
		}
	}
	return out
}

// Text returns the unescaped value of the first property with the name.
func (c *Component) Text(name string) string {
	if p, ok := c.First(name); ok {
		return Unescape(p.Value)
	}
	return ""
}

// unfold joins continuation lines (a line that starts with a space or tab continues the previous).
func unfold(data string) []string {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	var lines []string
	for _, l := range strings.Split(data, "\n") {
		if len(l) > 0 && (l[0] == ' ' || l[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
			continue
		}
		lines = append(lines, l)
	}
	return lines
}

// parseLine splits a content line. The name ends at the first ; or :, parameters follow, and the
// value starts after the first : that is not inside quotes.
func parseLine(line string) (Property, error) {
	inQuote := false
	colon := -1
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case ':':
			if !inQuote {
				colon = i
			}
		}
		if colon >= 0 {
			break
		}
	}
	if colon <= 0 {
		return Property{}, fmt.Errorf("not a content line: %q", truncate(line))
	}
	head, value := line[:colon], line[colon+1:]
	parts := splitUnquoted(head, ';')
	p := Property{Name: strings.ToUpper(strings.TrimSpace(parts[0])), Value: value}
	for _, raw := range parts[1:] {
		k, v, ok := strings.Cut(raw, "=")
		if !ok {
			continue
		}
		if p.Params == nil {
			p.Params = map[string][]string{}
		}
		key := strings.ToUpper(strings.TrimSpace(k))
		for _, one := range splitUnquoted(v, ',') {
			p.Params[key] = append(p.Params[key], strings.Trim(one, `"`))
		}
	}
	return p, nil
}

func splitUnquoted(s string, sep byte) []string {
	var out []string
	inQuote := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			inQuote = !inQuote
		case s[i] == sep && !inQuote:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// Parse reads iCalendar text into its top-level components (normally one VCALENDAR).
func Parse(data []byte) ([]*Component, error) {
	var roots []*Component
	var stack []*Component
	for n, line := range unfold(string(data)) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		p, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		switch p.Name {
		case "BEGIN":
			c := &Component{Name: strings.ToUpper(strings.TrimSpace(p.Value))}
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				top.Children = append(top.Children, c)
			} else {
				roots = append(roots, c)
			}
			stack = append(stack, c)
		case "END":
			if len(stack) == 0 {
				return nil, fmt.Errorf("line %d: END without BEGIN", n+1)
			}
			name := strings.ToUpper(strings.TrimSpace(p.Value))
			if top := stack[len(stack)-1]; top.Name != name {
				return nil, fmt.Errorf("line %d: END:%s closes BEGIN:%s", n+1, name, top.Name)
			}
			stack = stack[:len(stack)-1]
		default:
			if len(stack) == 0 {
				return nil, fmt.Errorf("line %d: property %s outside a component", n+1, p.Name)
			}
			top := stack[len(stack)-1]
			top.Props = append(top.Props, p)
		}
	}
	if len(stack) > 0 {
		return nil, fmt.Errorf("component %s is not closed", stack[len(stack)-1].Name)
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no iCalendar data found")
	}
	return roots, nil
}

// Unescape decodes a TEXT value.
func Unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n', 'N':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Escape encodes a TEXT value.
func Escape(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)
	return r.Replace(s)
}

// SplitText splits a TEXT list value (categories) at unescaped commas and unescapes each item.
func SplitText(s string) []string {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			cur.WriteByte(s[i])
			cur.WriteByte(s[i+1])
			i++
			continue
		}
		if s[i] == ',' {
			out = append(out, Unescape(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(s[i])
	}
	return append(out, Unescape(cur.String()))
}

// writer builds folded content lines with CRLF endings.
type writer struct {
	b strings.Builder
}

// line writes NAME;params:value, folding at 75 octets on rune boundaries.
func (w *writer) line(name string, params map[string]string, value string) {
	var head strings.Builder
	head.WriteString(name)
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := params[k]
		if strings.ContainsAny(v, ",;:") {
			v = `"` + v + `"`
		}
		head.WriteString(";" + k + "=" + v)
	}
	w.fold(head.String() + ":" + value)
}

func (w *writer) fold(s string) {
	limit := 75
	for len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		w.b.WriteString(s[:cut])
		w.b.WriteString("\r\n ")
		s = s[cut:]
		limit = 74 // the leading space counts
	}
	w.b.WriteString(s)
	w.b.WriteString("\r\n")
}

func (w *writer) text(name string, params map[string]string, value string) {
	w.line(name, params, Escape(value))
}

func (w *writer) begin(name string) { w.line("BEGIN", nil, name) }
func (w *writer) end(name string)   { w.line("END", nil, name) }
