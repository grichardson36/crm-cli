package tui

import (
	"bytes"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	tea "charm.land/bubbletea/v2"
	"github.com/emersion/go-vcard"
	"github.com/jdanielnd/crm-cli/internal/dav"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/mattn/go-runewidth"
)

// Copy and open (owner decisions 2026-10-28).
//
// ctrl+c copies the value of the focused field when a field is focused (the detail pane). Everywhere
// else it keeps its old job: it quits in a list and cancels in a prompt or a picker. The palette also
// lists "Copy value" and "Copy contact as vCard". The vCard has a key, ctrl+shift+c, but terminals such
// as Windows Terminal take that chord for themselves, so the palette entry is the guaranteed path.
//
// o opens the focused thing outside the app, for every object type. A URL, mail address or phone number
// opens the browser, the mail app or the dialer. Any other focused text, or an object with no link of
// its own, opens a web search for it in the browser. enter stays "open further inside the app". The
// search address is a template in config.toml, with a default:
//
//	[open]
//	search_url = "https://duckduckgo.com/?q={query}"

// defaultSearchURL is the web search used when config.toml has no [open] search_url.
const defaultSearchURL = "https://www.google.com/search?q={query}"

// searchURLFor builds the web search address for a query from the template in config.toml.
func searchURLFor(dataDir, query string) string {
	tmpl := defaultSearchURL
	if dataDir != "" {
		if data, err := os.ReadFile(modules.ConfigFile(dataDir)); err == nil {
			var cfg struct {
				Open struct {
					SearchURL string `toml:"search_url"`
				} `toml:"open"`
			}
			if _, err := toml.Decode(string(data), &cfg); err == nil && strings.TrimSpace(cfg.Open.SearchURL) != "" {
				tmpl = strings.TrimSpace(cfg.Open.SearchURL)
			}
		}
	}
	q := url.QueryEscape(strings.TrimSpace(query))
	if strings.Contains(tmpl, "{query}") {
		return strings.ReplaceAll(tmpl, "{query}", q)
	}
	return tmpl + q
}

var (
	emailValue = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	phoneValue = regexp.MustCompile(`^\+?[\d\s().-]{7,}$`)
	hostValue  = regexp.MustCompile(`(?i)^(www\.)?[a-z0-9][a-z0-9-]*(\.[a-z0-9-]+)+(/\S*)?$`)
)

// urlForValue turns a focused value into something the system opens: a URL, a mailto: or a tel:. It
// returns "" for text that is none of them.
func urlForValue(v string) string {
	v = strings.TrimSpace(v)
	switch {
	case v == "" || strings.ContainsAny(v, "\n"):
		return ""
	case strings.HasPrefix(v, "http://"), strings.HasPrefix(v, "https://"), strings.HasPrefix(v, "mailto:"), strings.HasPrefix(v, "tel:"):
		return v
	case emailValue.MatchString(v):
		return "mailto:" + v
	case phoneValue.MatchString(v) && len(regexp.MustCompile(`\d`).FindAllString(v, -1)) >= 7:
		return "tel:" + v
	case hostValue.MatchString(v):
		return "https://" + v
	}
	return ""
}

// openTarget decides what o opens: the address, and a short description of it for the status line.
func (m *Model) openTarget() (u, what string, ok bool) {
	if m.focus == focusDetail {
		if r, has := m.rowFocused(); has {
			return m.openForRow(r)
		}
		return "", "", false
	}
	if ref, has := m.cursorRef(); has {
		return m.openForObject(ref)
	}
	return "", "", false
}

func (m *Model) searchFor(text string) (string, string, bool) {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if text == "" {
		return "", "", false
	}
	return searchURLFor(m.env.DataDir, text), "web search for " + runewidth.Truncate(text, 40, "…"), true
}

// openForObject is o on an object: its own link when it has one, else a web search for its name.
func (m *Model) openForObject(ref repo.Ref) (string, string, bool) {
	switch ref.Kind {
	case "organization":
		if o, err := m.orr.FindByID(m.ctx, ref.ID); err == nil {
			for _, v := range []*string{o.Website, o.Domain, o.LinkedinURL} {
				if u := urlForValue(str(v)); u != "" {
					return u, u, true
				}
			}
		}
	case "person":
		if p, err := m.pr.FindByID(m.ctx, ref.ID); err == nil && m.pr.LoadDetails(m.ctx, p) == nil {
			for _, kind := range []string{"website", "linkedin"} {
				for _, h := range p.Handles {
					if h.Kind == kind {
						if u := linkFor(formRow{kind: rkHandle, key: h.Kind, value: h.Value}); u != "" {
							return u, u, true
						}
					}
				}
			}
		}
	}
	if o := objectByKind(ref.Kind); o != nil && o.nameOf != nil {
		return m.searchFor(strings.TrimPrefix(o.nameOf(m, ref.ID), "★ "))
	}
	return "", "", false
}

// openForRow is o on a row of the detail pane.
func (m *Model) openForRow(r formRow) (string, string, bool) {
	if c, ok := m.focusedCell(); ok && strings.TrimSpace(c.value) != "" {
		if u := urlForValue(c.value); u != "" {
			return u, u, true
		}
		return m.searchFor(c.value)
	}
	if u := linkFor(r); u != "" {
		return u, u, true
	}
	pageObject := func() (string, string, bool) { return m.openForObject(m.pageRef()) }
	switch r.kind {
	case rkLinked:
		if !r.empty {
			return m.openForObject(repo.Ref{Kind: r.other, ID: r.id})
		}
	case rkTask:
		if !r.empty {
			return m.openForObject(repo.Ref{Kind: "task", ID: r.id})
		}
	case rkTag:
		return m.searchFor(r.value)
	case rkField, rkTyped:
		v := r.value
		if u := urlForValue(v); u != "" {
			return u, u, true
		}
		long := strings.Contains(v, "\n") || len([]rune(v)) > 100 || (r.spec != nil && r.spec.Type == model.FieldLongText) || (r.kind == rkField && r.field == "notes")
		if strings.TrimSpace(v) != "" && !long {
			return m.searchFor(v)
		}
	}
	return pageObject()
}

// openOutside is the o key.
func (m *Model) openOutside() {
	u, what, ok := m.openTarget()
	if !ok {
		m.setStatus("nothing to open here", true)
		return
	}
	if m.open == nil {
		return
	}
	if err := m.open(u); err != nil {
		m.setStatus("open failed: "+err.Error(), true)
		return
	}
	m.setStatus("opened "+runewidth.Truncate(what, 60, "…"), false)
}

func whenOpenable(m *Model) bool {
	if m.focus == focusDetail {
		return len(m.form) > 0
	}
	_, ok := m.cursorRef()
	return ok
}

// --- vCard ---

// vcardPerson is the person Copy contact as vCard acts on: the open record page, else the highlighted one.
func (m *Model) vcardPerson() int64 {
	if m.focus == focusDetail && m.formPerson != 0 && !m.creating {
		return m.formPerson
	}
	if ref, ok := m.cursorRef(); ok && ref.Kind == "person" {
		return ref.ID
	}
	return 0
}

// vcardText renders a person as a vCard 3.0, the same card the phone sync serves.
func (m *Model) vcardText(id int64) (string, error) {
	p, err := m.pr.FindByID(m.ctx, id)
	if err != nil {
		return "", err
	}
	if err := m.pr.LoadDetails(m.ctx, p); err != nil {
		return "", err
	}
	card := dav.Render(p, nil, p.UUID)
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (m *Model) copyVCard() {
	id := m.vcardPerson()
	if id == 0 {
		m.setStatus("select a person to copy as a vCard", true)
		return
	}
	text, err := m.vcardText(id)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if m.clip != nil {
		m.clip(text)
	}
	m.setStatus("copied "+m.personLabel(id)+" as a vCard", false)
}

func copyOpenActions() []action {
	return []action{
		{id: "copy", title: "Copy value", group: "Edit pane", hint: "ctrl+c", keys: []string{"ctrl+c"},
			desc: "copy the value of the focused field (detail pane) to the clipboard, OSC52 so it works over SSH. ctrl+c copies when a field is focused and otherwise keeps its old job: quit in a list, cancel in a prompt or picker",
			foot: 13, footIn: inDetail, footLabel: "copy", when: whenDetail, run: func(m *Model) tea.Cmd { m.copyRow(); return nil }},
		{id: "copy-vcard", title: "Copy contact as vCard", group: "Contact", hint: "ctrl+shift+c", keys: []string{"ctrl+shift+c"},
			desc: "copy the person as a vCard (the card the phone sync serves) to the clipboard. Many terminals take ctrl+shift+c for themselves, so this palette entry always works",
			when: func(m *Model) bool { return m.vcardPerson() != 0 }, run: func(m *Model) tea.Cmd { m.copyVCard(); return nil }},
		{id: "open", title: "Open outside the app", group: "Edit pane", hint: "o", keys: []string{"o"},
			desc:  "open the focused thing outside the app, for any object: a URL, mail address or phone number opens the browser, mail app or dialer, any other text or an object with no link opens a web search for it (search_url under [open] in config.toml). enter opens further inside the app",
			write: true, foot: 14, footIn: inDetail, footLabel: "outside", when: whenOpenable, run: func(m *Model) tea.Cmd { m.openOutside(); return nil }},
	}
}
