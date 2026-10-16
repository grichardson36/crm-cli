package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Settings in the bar. Typing a setting name finds it with its current value, and enter opens the
// same action the palette has for it. The `=` prefix lists every setting.

type setting struct {
	id     string
	name   string
	words  string // extra words that find it
	desc   string
	value  func(m *Model) string
	action string // the registry action enter runs
	run    func(m *Model) tea.Cmd
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func (m *Model) settingList() []setting {
	return []setting{
		{id: "theme", name: "Theme", words: "colors colours dark light appearance look", desc: "choose a color theme, previewed live",
			value: func(m *Model) string { return m.themeName }, action: "theme-choose"},
		{id: "theme-reload", name: "Theme files", words: "theme reload refresh themes", desc: "re-read ~/.crm/themes and apply the theme again",
			action: "theme-reload"},
		{id: "keys", name: "Keys", words: "keybindings shortcuts rebind customize", desc: "rebind an action: pick it, press the new key",
			value: func(m *Model) string {
				if n := len(m.keyOverrides); n > 0 {
					return fmt.Sprintf("%d custom", n)
				}
				return "defaults"
			}, action: "keys-customize"},
		{id: "keys-reset", name: "Keys: reset", words: "keybindings shortcuts defaults", desc: "put one or all custom keys back to the defaults",
			action: "keys-reset"},
		{id: "syntax", name: "Short syntax", words: "inline tokens hash at toggle", desc: "#tag @Name !high ~30m by fri every 3m in the bar and prompts. Enter toggles",
			value: func(m *Model) string { return onOff(!m.noSyntax) }, action: "syntax-toggle"},
		{id: "read-only", name: "Read-only", words: "readonly lock safe writes", desc: "set with crm tui --read-only when the TUI starts",
			value: func(m *Model) string { return onOff(m.readOnly) },
			run: func(m *Model) tea.Cmd {
				return m.showToast("read-only is "+onOff(m.readOnly)+". Start crm tui --read-only to turn it on", false)
			}},
		{id: "database", name: "Database", words: "db where path file domain schema size info", desc: "the open database: path, schema version, size",
			value: func(m *Model) string {
				if m.env.DBPath == "" {
					return "unknown"
				}
				return domainOf(m.env.DBPath)
			}, action: "where"},
		{id: "domains", name: "Domains", words: "databases data directory files list", desc: "the databases in the data directory",
			action: "domains"},
		{id: "doctor", name: "Doctor", words: "health check install problems fix", desc: "check the installation and database, with a fix for each problem",
			action: "doctor"},
		{id: "sync", name: "Sync sources", words: "adapters carddav inbox", desc: "open the Sync view: sources, last runs and the inbox",
			value: func(m *Model) string { return fmt.Sprintf("%d", len(m.registry.Names())) },
			run:   func(m *Model) tea.Cmd { m.switchView(viewSync); return nil }},
	}
}

func hasWordPrefix(words []string, prefix string) bool {
	for _, h := range words {
		if strings.HasPrefix(h, prefix) {
			return true
		}
	}
	return false
}

// settingItems lists the settings that match the words of q (all of them when q is empty; words match from their start, two letters or more), each
// with its current value in the label.
func (m *Model) settingItems(q string) []overlayItem {
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(q, ":", " ")))
	var out []overlayItem
	for _, s := range m.settingList() {
		hay := strings.Fields(strings.ToLower(strings.ReplaceAll(s.name, ":", "") + " " + s.words + " settings setting"))
		ok := true
		for _, w := range words {
			if len(w) < 2 || !hasWordPrefix(hay, w) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		label := s.name
		if s.value != nil {
			if v := s.value(m); v != "" {
				label += ": " + v
			}
		}
		out = append(out, overlayItem{id: "setting:" + s.id, kind: kindSetting, label: label, desc: s.desc, value: s.action})
	}
	return out
}

// runSetting opens the action behind a setting, or does what the setting itself does.
func (m *Model) runSetting(it overlayItem) tea.Cmd {
	if m.cmp != nil {
		m.setStatus("settings are not available in the compare screen", true)
		return nil
	}
	id := strings.TrimPrefix(it.id, "setting:")
	for _, s := range m.settingList() {
		if s.id != id {
			continue
		}
		if s.run != nil {
			return s.run(m)
		}
		if a := m.actionByID(s.action); a != nil {
			return m.runAction(a)
		}
	}
	return nil
}
