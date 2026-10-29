package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCfg(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(modules.ConfigFile(dir), []byte(body), 0o644))
}

func TestEveryEngineHasAValidTemplate(t *testing.T) {
	for _, e := range searchEngines {
		if e.id == engineCustom {
			assert.Empty(t, e.tmpl)
			continue
		}
		assert.Equal(t, 1, strings.Count(e.tmpl, "{query}"), e.id)
		assert.True(t, strings.HasPrefix(e.tmpl, "https://"), e.id)
	}
	assert.Len(t, searchEngines, 18)
	assert.Equal(t, defaultSearchURL, engineByID(defaultEngine).tmpl)
}

func TestSearchConfigReadsAndMigrates(t *testing.T) {
	tests := []struct {
		name, file         string
		engine, custom     string
		offered            []string
		urlFor             string
		migrateRewritesTo  string // substring the rewritten file must hold ("" = no rewrite)
		migrateMustNotHold string
	}{
		{"no file defaults to Google only", "", "google", "", []string{"google"}, "https://www.google.com/search?q=a+b", "", ""},
		{"engine", "[open]\nengine = \"brave\"\n", "brave", "", []string{"brave"}, "https://search.brave.com/search?q=a+b", "", ""},
		{"unknown engine falls back", "[open]\nengine = \"nope\"\n", "google", "", []string{"google"}, "https://www.google.com/search?q=a+b", "", ""},
		{"custom without an address falls back", "[open]\nengine = \"custom\"\n", "google", "", []string{"google"}, "https://www.google.com/search?q=a+b", "", ""},
		{"custom", "[open]\nengine = \"custom\"\ncustom_url = \"https://x.example.org/f?q={query}\"\n", "custom", "https://x.example.org/f?q={query}", []string{"custom"}, "https://x.example.org/f?q=a+b", "", ""},
		{"offer keeps catalog order and puts the default first", "[open]\nengine = \"bing\"\noffer = [\"youtube\", \"google\", \"bing\"]\n", "bing", "", []string{"bing", "google", "youtube"}, "https://www.bing.com/search?q=a+b", "", ""},
		{"offered custom without address is dropped", "[open]\noffer = [\"custom\", \"brave\"]\n", "google", "", []string{"google", "brave"}, "https://www.google.com/search?q=a+b", "", ""},
		{"legacy known template becomes the engine", "[open]\nsearch_url = \"https://duckduckgo.com/?q={query}\"\n", "duckduckgo", "", []string{"duckduckgo"}, "https://duckduckgo.com/?q=a+b", "engine = \"duckduckgo\"", "search_url"},
		{"legacy other template becomes custom", "[open]\nsearch_url = \"https://example.org/find/{query}&x=1\"\n", "custom", "https://example.org/find/{query}&x=1", []string{"custom"}, "https://example.org/find/a+b&x=1", "custom_url", "search_url"},
		{"legacy prefix template gets the placeholder", "[open]\nsearch_url = \"https://example.org/find/\"\n", "custom", "https://example.org/find/{query}", []string{"custom"}, "https://example.org/find/a+b", "custom_url", "search_url"},
		{"other tables are kept by the migration", "[modules]\norder = [\"people\"]\n[open]\nsearch_url = \"https://www.google.com/search?q={query}\"\n", "google", "", []string{"google"}, "https://www.google.com/search?q=a+b", "[modules]", "search_url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.file != "" {
				writeCfg(t, dir, tt.file)
			}
			c := loadSearchConfig(dir)
			assert.Equal(t, tt.engine, c.Engine)
			assert.Equal(t, tt.custom, c.Custom)
			assert.Equal(t, tt.offered, c.offered())
			assert.Equal(t, tt.urlFor, searchURLFor(dir, "a b"))

			changed, err := migrateSearchConfig(dir)
			require.NoError(t, err)
			assert.Equal(t, tt.migrateRewritesTo != "", changed)
			if changed {
				data, _ := os.ReadFile(modules.ConfigFile(dir))
				assert.Contains(t, string(data), tt.migrateRewritesTo)
				assert.NotContains(t, string(data), tt.migrateMustNotHold)
				assert.Equal(t, tt.urlFor, searchURLFor(dir, "a b"), "the migrated file searches the same")
			}
		})
	}
}

func TestToggleOffer(t *testing.T) {
	c := searchConfig{Engine: "google"}
	c.toggleOffer("youtube")
	c.toggleOffer("bing")
	assert.Equal(t, []string{"bing", "youtube"}, c.Offer, "catalog order")
	c.toggleOffer("google")
	assert.Equal(t, []string{"bing", "youtube"}, c.Offer, "the default cannot be unmarked")
	c.toggleOffer("bing")
	assert.Equal(t, []string{"youtube"}, c.Offer)
	assert.Equal(t, "Google, YouTube", c.summary())
	assert.Equal(t, "Google only", searchConfig{Engine: "google"}.summary())
}

func TestOPicksTheEngineOnlyWhenSeveralAreOffered(t *testing.T) {
	m, _ := newTestModel(t)
	m.env.DataDir = t.TempDir()
	var opened []string
	m.open = func(u string) error { opened = append(opened, u); return nil }

	// Default: Google only, no picker.
	press(m, "o")
	require.Nil(t, m.ov)
	assert.Equal(t, []string{"https://www.google.com/search?q=Zed+Adams"}, opened)

	// Two engines marked: a picker with the default first.
	c := searchConfig{Engine: "duckduckgo", Offer: []string{"google", "github"}}
	require.NoError(t, c.save(m.env.DataDir))
	opened = nil
	press(m, "o")
	require.NotNil(t, m.ov, "several engines open the picker")
	assert.Empty(t, opened)
	var labels []string
	for _, i := range m.ov.shown {
		labels = append(labels, m.ov.items[i].label)
	}
	assert.Equal(t, []string{"DuckDuckGo", "Google", "GitHub"}, labels)
	press(m, "down", "enter")
	assert.Equal(t, []string{"https://www.google.com/search?q=Zed+Adams"}, opened)
	assert.Nil(t, m.ov)

	// esc opens nothing.
	opened = nil
	press(m, "o", "esc")
	assert.Empty(t, opened)
	assert.Nil(t, m.ov)
}

func TestOfferPickerMarksAndUnmarks(t *testing.T) {
	m := settingsModel(t)
	cursorToSetting(t, m, "web-search-offer")
	assert.Contains(t, m.rows[m.cursor].sub, "Google only")
	pressRun(m, "enter")
	require.NotNil(t, m.ov)
	typeText(m, "bing")
	press(m, "enter")
	require.NotNil(t, m.ov, "the picker stays open")
	assert.Equal(t, []string{"google", "bing"}, loadSearchConfig(m.env.DataDir).offered())
	typeText(m, "bing")
	press(m, "enter")
	assert.Equal(t, []string{"google"}, loadSearchConfig(m.env.DataDir).offered())
}

func TestCustomSearchAddressIsValidated(t *testing.T) {
	m := settingsModel(t)
	set := m.settingByID("web-search-custom").set
	engine := m.settingByID("web-search").set

	require.Error(t, engine(m, engineCustom), "custom needs an address first")
	for _, bad := range []string{"example.com/{query}", "https://example.com/", "javascript:{query}", "https://a.b/{query}{query}"} {
		assert.Error(t, set(m, bad), bad)
	}
	require.NoError(t, set(m, "https://search.example.org/?q={query}"))
	require.NoError(t, engine(m, engineCustom))
	assert.Equal(t, "https://search.example.org/?q=hi", searchURLFor(m.env.DataDir, "hi"))
	require.NoError(t, set(m, ""), "emptying the address while it is the engine falls back to Google")
	assert.Equal(t, "google", loadSearchConfig(m.env.DataDir).Engine)
}
