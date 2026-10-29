package tui

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// Web search (patterns/web-search-engines). `o` on plain text searches the web. The user picks a
// default engine from a catalog and may mark more engines to offer when o is pressed. Everything is
// per device, in config.toml:
//
//	[open]
//	engine     = "duckduckgo"                 # the default engine id, google when missing
//	custom_url = "https://x.org/?q={query}"   # used when engine = "custom"
//	offer      = ["google", "bing"]           # engines offered besides the default
//
// An older file with [open] search_url is read as before and rewritten into this form the first time
// the program starts (migrateSearchConfig).

type searchEngine struct {
	id, name, tmpl string
}

// engineCustom is the engine id whose address the user types.
const engineCustom = "custom"

// searchEngines is the catalog, in the order of the choice list.
var searchEngines = []searchEngine{
	{"google", "Google", "https://www.google.com/search?q={query}"},
	{"bing", "Bing", "https://www.bing.com/search?q={query}"},
	{"duckduckgo", "DuckDuckGo", "https://duckduckgo.com/?q={query}"},
	{"brave", "Brave", "https://search.brave.com/search?q={query}"},
	{"ecosia", "Ecosia", "https://www.ecosia.org/search?q={query}"},
	{"linkedin", "LinkedIn", "https://www.linkedin.com/search/results/all/?keywords={query}"},
	{"facebook", "Facebook", "https://www.facebook.com/search/top?q={query}"},
	{"instagram", "Instagram", "https://www.instagram.com/explore/search/?q={query}"},
	{"tiktok", "TikTok", "https://www.tiktok.com/search?q={query}"},
	{"youtube", "YouTube", "https://www.youtube.com/results?search_query={query}"},
	{"x", "X", "https://x.com/search?q={query}"},
	{"reddit", "Reddit", "https://www.reddit.com/search/?q={query}"},
	{"github", "GitHub", "https://github.com/search?q={query}"},
	{"wikipedia", "Wikipedia", "https://en.wikipedia.org/w/index.php?search={query}"},
	{"google-maps", "Google Maps", "https://www.google.com/maps/search/?api=1&query={query}"},
	{"google-images", "Google Images", "https://www.google.com/search?tbm=isch&q={query}"},
	{"amazon", "Amazon", "https://www.amazon.com/s?k={query}"},
	{engineCustom, "Custom", ""},
}

// defaultEngine is the engine used when config.toml sets none.
const defaultEngine = "google"

// defaultSearchURL is the address of the default engine.
const defaultSearchURL = "https://www.google.com/search?q={query}"

func engineByID(id string) searchEngine {
	for _, e := range searchEngines {
		if e.id == id {
			return e
		}
	}
	return searchEngine{}
}

// searchConfig is the web search part of config.toml.
type searchConfig struct {
	Engine string   // default engine id
	Custom string   // the custom address
	Offer  []string // engines offered besides the default
}

// strList reads a TOML list of strings.
func strList(v any) []string {
	var out []string
	switch l := v.(type) {
	case []string:
		out = append(out, l...)
	case []any:
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func tomlStr(t map[string]any, key string) string {
	s, _ := t[key].(string)
	return strings.TrimSpace(s)
}

// loadSearchConfig reads [open]. An old search_url is read as the matching engine, or as the custom
// address, so a file that was never migrated still works.
func loadSearchConfig(dataDir string) searchConfig {
	t := modules.Table(dataDir, "open")
	c := searchConfig{Engine: tomlStr(t, "engine"), Custom: tomlStr(t, "custom_url"), Offer: strList(t["offer"])}
	if c.Engine == "" {
		if legacy := tomlStr(t, "search_url"); legacy != "" {
			c.Engine, c.Custom = legacySearch(legacy)
		}
	}
	if engineByID(c.Engine).id == "" {
		c.Engine = defaultEngine
	}
	if c.Engine == engineCustom && c.Custom == "" {
		c.Engine = defaultEngine
	}
	return c
}

// legacySearch maps an old [open] search_url to an engine id and a custom address.
func legacySearch(tmpl string) (engine, custom string) {
	for _, e := range searchEngines {
		if e.tmpl != "" && e.tmpl == tmpl {
			return e.id, ""
		}
	}
	if !strings.Contains(tmpl, validate.QueryPlaceholder) {
		tmpl += validate.QueryPlaceholder // the old rule: a template without {query} is a prefix
	}
	return engineCustom, tmpl
}

// save writes [open] and drops the old search_url.
func (c searchConfig) save(dataDir string) error {
	if dataDir == "" {
		return fmt.Errorf("no data directory in this session, so the setting cannot be saved: %w", model.ErrValidation)
	}
	t := map[string]any{}
	for k, v := range modules.Table(dataDir, "open") { // keep unrelated keys of [open]
		switch k {
		case "search_url", "engine", "custom_url", "offer":
		default:
			t[k] = v
		}
	}
	if c.Engine != "" && c.Engine != defaultEngine {
		t["engine"] = c.Engine
	}
	if c.Custom != "" {
		t["custom_url"] = c.Custom
	}
	if len(c.Offer) > 0 {
		t["offer"] = c.Offer
	}
	return modules.SetTable(dataDir, "open", t)
}

// templateOf is the address template of an engine id.
func (c searchConfig) templateOf(id string) string {
	if id == engineCustom {
		return c.Custom
	}
	return engineByID(id).tmpl
}

// nameOf is the display name of an engine id.
func (c searchConfig) nameOf(id string) string {
	if e := engineByID(id); e.id != "" {
		return e.name
	}
	return id
}

// offered lists the engines `o` offers: the default first, then the marked ones in catalog order.
// An engine that cannot be used (custom without an address, an unknown id) is left out.
func (c searchConfig) offered() []string {
	out := []string{c.Engine}
	marked := map[string]bool{}
	for _, id := range c.Offer {
		marked[id] = true
	}
	for _, e := range searchEngines {
		if e.id != c.Engine && marked[e.id] && c.templateOf(e.id) != "" {
			out = append(out, e.id)
		}
	}
	return out
}

// isOffered says whether the engine is in the offer list (the default always is).
func (c searchConfig) isOffered(id string) bool {
	for _, o := range c.offered() {
		if o == id {
			return true
		}
	}
	return false
}

// toggleOffer marks or unmarks an engine. The default engine stays offered.
func (c *searchConfig) toggleOffer(id string) {
	if id == c.Engine {
		return
	}
	var out []string
	found := false
	for _, o := range c.Offer {
		if o == id {
			found = true
			continue
		}
		out = append(out, o)
	}
	if !found {
		out = append(out, id)
	}
	sort.SliceStable(out, func(i, j int) bool { return engineIndex(out[i]) < engineIndex(out[j]) })
	c.Offer = out
}

func engineIndex(id string) int {
	for i, e := range searchEngines {
		if e.id == id {
			return i
		}
	}
	return len(searchEngines)
}

// summary is the value shown on the Settings row of the offer list.
func (c searchConfig) summary() string {
	ids := c.offered()
	if len(ids) == 1 {
		return c.nameOf(ids[0]) + " only"
	}
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = c.nameOf(id)
	}
	return strings.Join(names, ", ")
}

// searchAddress fills a template with the query. A template without {query} is a prefix.
func searchAddress(tmpl, query string) string {
	q := url.QueryEscape(strings.TrimSpace(query))
	if strings.Contains(tmpl, validate.QueryPlaceholder) {
		return strings.ReplaceAll(tmpl, validate.QueryPlaceholder, q)
	}
	return tmpl + q
}

// searchURLFor is the web search address of the default engine for a query.
func searchURLFor(dataDir, query string) string {
	c := loadSearchConfig(dataDir)
	return searchAddress(c.templateOf(c.Engine), query)
}

// migrateSearchConfig rewrites an old [open] search_url into engine and custom_url. It reports
// whether it changed the file. A file without search_url is not touched.
func migrateSearchConfig(dataDir string) (bool, error) {
	if modules.Table(dataDir, "open")["search_url"] == nil {
		return false, nil
	}
	return true, loadSearchConfig(dataDir).save(dataDir)
}

// openOfferPicker lists every engine with a mark for the ones o offers. enter on an engine marks or
// unmarks it and the picker stays open on the same row. The default engine is always marked.
func (m *Model) openOfferPicker(at int) {
	cfg := loadSearchConfig(m.env.DataDir)
	var items []overlayItem
	for _, e := range searchEngines {
		it := overlayItem{id: e.id, label: e.name}
		switch {
		case e.id == cfg.Engine:
			it.hint, it.desc = "*", "default, always offered"
		case e.id == engineCustom && cfg.Custom == "":
			it.disabled = "set the custom address first"
		case cfg.isOffered(e.id):
			it.hint = "*"
		}
		items = append(items, it)
	}
	m.openPicker("Web search: engines offered on o", items, func(m *Model, it overlayItem) {
		if it.disabled != "" {
			m.setStatus(it.disabled, true)
			return
		}
		cfg := loadSearchConfig(m.env.DataDir)
		cfg.toggleOffer(it.id)
		if err := cfg.save(m.env.DataDir); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		at := 0
		for i, e := range searchEngines {
			if e.id == it.id {
				at = i
			}
		}
		m.openOfferPicker(at)
		_ = m.rebuild(true)
		m.refreshDetail(true)
	})
	if m.ov != nil && at < len(m.ov.shown) {
		m.ov.cursor = at
	}
}
