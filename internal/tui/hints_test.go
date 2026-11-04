package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A key hint is a key name followed by what it does ("enter save", "esc cancel", "ctrl+z undoes").
// patterns/key-hints-in-one-place: such text may only be on the footer of a screen.
var keyHintText = regexp.MustCompile(`(^|[^A-Za-z+/])(enter|esc|tab|shift\+tab|ctrl\+[a-z]|alt\+enter|up/down|left/right|home/end|y/n|any key)\s+[a-z]{3,}`)

// footerOnly renders the screen and returns the lines above the footer, and the footer.
func footerOnly(m *Model, footerLines int) (body []string, footer []string) {
	lines := strings.Split(ansi.Strip(m.screen()), "\n")
	if m.mode == modeOverlay && m.ov != nil && m.ov.draw == nil {
		// A popup keeps its hints in the last row of its box. The screen behind it is not its own.
		top, bottom := -1, -1
		for i, l := range lines {
			if top < 0 && strings.Contains(l, "┌") {
				top = i
			}
			if strings.Contains(l, "└") {
				bottom = i
			}
		}
		if top >= 0 && bottom > top+2 {
			cut := func(l string) string {
				l = strings.TrimSpace(l)
				return strings.TrimSpace(strings.Trim(l, "│"))
			}
			for _, l := range lines[top+1 : bottom-2] {
				body = append(body, cut(l))
			}
			return body, []string{cut(lines[bottom-1])}
		}
	}
	n := len(lines) - footerLines
	if n < 0 {
		n = 0
	}
	return lines[:n], lines[n:]
}

type hintScreen struct {
	name string
	// footerLines is how many last lines are the footer: 2 on the main screen and its modes (status
	// line and key hints), 1 on a screen that takes over everything.
	footerLines int
	setup       func(t *testing.T, m *Model)
}

func hintScreens() []hintScreen {
	view := func(v viewKind) func(t *testing.T, m *Model) {
		return func(t *testing.T, m *Model) { m.switchView(v) }
	}
	return []hintScreen{
		{"home", 2, view(viewHome)},
		{"people list", 2, view(viewPeople)},
		{"follow-up", 2, view(viewDue)},
		{"tasks", 2, view(viewTasks)},
		{"dates", 2, view(viewDates)},
		{"merge", 2, view(viewMerge)},
		{"archived", 2, view(viewArchived)},
		{"sync", 2, view(viewSync)},
		{"organizations", 2, view(viewOrgs)},
		{"deals", 2, view(viewDeals)},
		{"tags", 2, view(viewTags)},
		{"settings", 2, view(viewSettings)},
		{"settings keys row", 2, func(t *testing.T, m *Model) {
			m.switchView(viewSettings)
			for i, r := range m.rows {
				if r.setting == "key:add-any" {
					m.cursor = i
				}
			}
			m.refreshDetail(true)
		}},
		{"record page", 2, func(t *testing.T, m *Model) { press(m, "tab") }},
		{"field edit", 2, func(t *testing.T, m *Model) { press(m, "tab", "enter") }},
		{"prompt", 2, func(t *testing.T, m *Model) { press(m, "n") }},
		{"add prompt", 2, func(t *testing.T, m *Model) { press(m, "a", "enter") }},
		{"confirm", 2, func(t *testing.T, m *Model) { press(m, "d") }},
		{"capture", 2, func(t *testing.T, m *Model) { m.startCapture("add-any") }},
		{"offer picker", 1, func(t *testing.T, m *Model) { m.env.DataDir = t.TempDir(); m.openOfferPicker(0) }},
		{"list pane", 2, func(t *testing.T, m *Model) { m.openListPane("deal-stages") }},
		{"list pane add", 2, func(t *testing.T, m *Model) { m.openListPane("deal-stages"); press(m, "a") }},
		{"list pane reset", 2, func(t *testing.T, m *Model) { m.openListPane("deal-stages"); press(m, "tab", "R") }},
		{"views screen", 1, func(t *testing.T, m *Model) { press(m, "v") }},
		{"palette", 1, func(t *testing.T, m *Model) { press(m, "ctrl+k") }},
		{"command bar", 1, func(t *testing.T, m *Model) { press(m, "ctrl+k"); typeText(m, "task buy milk ") }},
		{"no matches", 1, func(t *testing.T, m *Model) { press(m, "ctrl+k"); typeText(m, "zzzzqqqq") }},
		{"help topics", 1, func(t *testing.T, m *Model) { m.openHelpTopics("") }},
		{"help topic text", 1, func(t *testing.T, m *Model) { m.showTopic("keys", nil) }},
		{"help keys page", 1, func(t *testing.T, m *Model) { press(m, "?") }},
		{"names panel", 2, func(t *testing.T, m *Model) {
			m.names = &namesPanel{refs: nil}
			m.mode = modeNames
		}},
		{"saved view editor", 1, func(t *testing.T, m *Model) {
			m.sve = &svEditor{name: "x", kind: "person", labels: map[string]string{}}
		}},
	}
}

func TestKeyHintsAreOnlyInTheFooter(t *testing.T) {
	for _, sc := range hintScreens() {
		t.Run(sc.name, func(t *testing.T) {
			m := newDefaultModel(t)
			m.env.DataDir = t.TempDir()
			sc.setup(t, m)
			body, footer := footerOnly(m, sc.footerLines)
			if sc.name == "help keys page" {
				body = nil // the keys page lists keys by design, only its footer is checked
			}
			for _, l := range body {
				if keyHintText.MatchString(l) {
					t.Errorf("key hint outside the footer: %q", strings.TrimSpace(l))
				}
			}
			last := strings.TrimSpace(footer[len(footer)-1])
			assert.NotEmpty(t, last, "every screen ends with its key hints")
		})
	}
}

func TestEveryScreenFooterNamesAKey(t *testing.T) {
	for _, sc := range hintScreens() {
		t.Run(sc.name, func(t *testing.T) {
			m := newDefaultModel(t)
			m.env.DataDir = t.TempDir()
			sc.setup(t, m)
			_, footer := footerOnly(m, sc.footerLines)
			last := footer[len(footer)-1]
			assert.Regexp(t, `\S+ [a-z]`, last, "the last line is hints: a key and its label")
		})
	}
}

// TestFooterHintsFollowTheKeymap: a hint that names a registry action shows the key bound today.
func TestFooterHintsFollowTheKeymap(t *testing.T) {
	m := newDefaultModel(t)
	m.runAction(m.actionByID("note"))
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, ansi.Strip(m.hintsLine()), "tab type", "the prompt hints read tab from the registry")
	press(m, "esc")

	m.applyKeys(keys.Overrides{"prompt-tab": {"ctrl+t"}})
	m.runAction(m.actionByID("note"))
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, ansi.Strip(m.hintsLine()), "ctrl+t type")
	assert.NotContains(t, ansi.Strip(m.hintsLine()), "tab type")
}

func TestFooterOrderIsVerbsThenNavigationThenSpecifics(t *testing.T) {
	m := newDefaultModel(t)
	m.switchView(viewPeople)
	segs := m.footerSegs()
	require.NotEmpty(t, segs)
	last := -1
	for _, s := range segs {
		a := m.actionByID(s.id)
		require.NotNil(t, a, s.id)
		c := hintClassOf(a)
		assert.GreaterOrEqual(t, c, last, "%s is out of order", s.id)
		last = c
	}
}

func TestHintClassOfActions(t *testing.T) {
	tests := []struct {
		id    string
		class int
	}{
		{"add-any", hintVerb}, {"activate", hintVerb}, {"mark", hintVerb}, {"down", hintNav},
		{"focus-detail", hintNav}, {"open", hintSpecific}, {"snooze", hintSpecific}, {"help", hintChrome},
		{"palette", hintChrome}, {"goto-view", hintChrome},
	}
	m := newDefaultModel(t)
	for _, tt := range tests {
		a := m.actionByID(tt.id)
		require.NotNil(t, a, tt.id)
		assert.Equal(t, tt.class, hintClassOf(a), tt.id)
	}
}

// CRM-015: the footer shows ctrl+o in every list view that has view options.
func TestFooterShowsViewOptionsWhereTheViewHasThem(t *testing.T) {
	m := newDefaultModel(t)
	m.width = 160
	for _, v := range []viewKind{viewPeople, viewOrgs, viewTags, viewDeals, viewTasks, viewMerge, viewArchived, viewDates} {
		m.switchView(v)
		assert.Contains(t, footHints(m), "ctrl+o", "%s has view options", viewNames[v])
	}
	for _, v := range []viewKind{viewHome, viewDue, viewSync} {
		m.switchView(v)
		assert.NotContains(t, footHints(m), "ctrl+o", "%s has no view options", viewNames[v])
	}
}

func TestFooterViewOptionsHintFollowsTheKeymap(t *testing.T) {
	m := newDefaultModel(t)
	m.width = 160
	m.switchView(viewPeople)
	m.applyKeys(keys.Overrides{"options": {"ctrl+g"}})
	h := footHints(m)
	assert.Contains(t, h, "ctrl+g")
	assert.NotContains(t, h, "ctrl+o")
}
