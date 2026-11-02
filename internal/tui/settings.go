package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
	"github.com/jdanielnd/crm-cli/internal/theme"
)

// The settings registry (patterns/settings-view). Every setting is declared once, here: its id, the
// section it sits in, its label, a description, the words that find it, the scope (where it is
// stored) and how it is read and written. The Settings module (a grouped list with one divider per
// section) and the `=` settings of the command bar both read settingList, so there is no second list.
//
// A setting is one of three kinds:
//
//	field    has a field spec (the field-type vocabulary) and a set function. enter or e edits it in
//	         place with the typed editor of its type.
//	action   opens an existing screen or runs an existing registry action (theme choose, deal stages).
//	run      does something small itself (opens a view, shows a message).

// settingScope says where a setting is stored.
type settingScope string

const (
	// scopeDevice settings live in config.toml on this computer. Other devices keep their own.
	scopeDevice settingScope = "device"
	// scopeSynced settings live in the database and travel with it to other devices.
	scopeSynced settingScope = "synced"
	// scopeSession settings are not stored: they last until the program closes.
	scopeSession settingScope = "session"
)

// scopeNote is the sentence the detail pane shows for a scope.
func (s settingScope) note() string {
	switch s {
	case scopeDevice:
		return "stored in config.toml on this device. Other devices keep their own"
	case scopeSynced:
		return "stored in the database, so it syncs with the database to your other devices"
	case scopeSession:
		return "not stored. It lasts until you close the program"
	}
	return ""
}

// Section names, in the order the Settings view shows them.
const (
	secAppearance = "Appearance"
	secColors     = "Colors"
	secKeys       = "Keys"
	secViews      = "Views"
	secDeals      = "Deals"
	secTypes      = "Types and vocabularies"
	secNames      = "Names"
	secBehavior   = "Behavior"
	secData       = "Data"
	secSync       = "Sync"
	secArchive    = "Archive"
)

var settingSections = []string{secAppearance, secColors, secViews, secDeals, secTypes, secNames, secBehavior, secData, secSync, secArchive, secKeys}

type setting struct {
	id      string
	section string
	name    string
	words   string // extra words that find it
	desc    string
	scope   settingScope
	value   func(m *Model) string
	// field makes the setting editable in place, set stores the new value.
	field *model.FieldSpec
	set   func(m *Model, v string) error
	// action is the registry action enter runs, run does it directly. The bar prefers them to the
	// field editor when both exist.
	action string
	run    func(m *Model) tea.Cmd
	// colorKind is set on the rows of the Colors section: the detail pane shows a live sample in it.
	colorKind string
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func countText(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %ss", n, one)
}

var webSearchField = engineField()

var syntaxField = model.FieldSpec{Key: "syntax", Label: "Short syntax", Type: model.FieldChoice,
	Choices: []model.FieldChoiceOption{{Value: "on", Label: "on"}, {Value: "off", Label: "off"}}}

// engineField is the default web search engine, a choice over the catalog.
func engineField() model.FieldSpec {
	f := model.FieldSpec{Key: "web-search", Label: "Web search", Type: model.FieldChoice}
	for _, e := range searchEngines {
		f.Choices = append(f.Choices, model.FieldChoiceOption{Value: e.id, Label: e.name})
	}
	return f
}

var customSearchField = model.FieldSpec{Key: "web-search-custom", Label: "Custom address", Type: model.FieldText, Max: validate.MaxSearchTemplate,
	Hint: "https://example.com/search?q={query}"}

var fillField = model.FieldSpec{Key: "fill-background", Label: "Fill the whole screen with the theme background", Type: model.FieldChoice,
	Choices: []model.FieldChoiceOption{{Value: "auto", Label: "auto"}, {Value: "on", Label: "on"}, {Value: "off", Label: "off"}}}


// settingList is the registry. The bar and the Settings view both read it.
func (m *Model) settingList() []setting {
	vocabCount := func(kind, noun string) func(m *Model) string {
		return func(m *Model) string {
			list, err := m.vr.List(m.ctx, kind, false)
			if err != nil {
				return ""
			}
			return countText(len(list), noun)
		}
	}
	return append(m.colorSettings(), []setting{
		// Appearance
		{id: "theme", section: secAppearance, name: "Theme", words: "colors colours dark light appearance look", desc: "choose a color theme, previewed live", scope: scopeDevice,
			value: func(m *Model) string { return m.themeName }, action: "theme-choose"},
		{id: "fill-background", section: secAppearance, name: "Fill the whole screen with the theme background", words: "background paint light dark terminal force colors", scope: scopeDevice,
			desc: "paint every cell with the theme background so a light theme stays light on a dark terminal. auto: on for light themes, for dark themes only on a light terminal. Needs a theme with a bg color and a color terminal",
			value: func(m *Model) string { return string(m.fillMode) }, field: &fillField,
			set: func(m *Model, v string) error {
				f, err := theme.ParseFill(v)
				if err != nil {
					return fmt.Errorf("%v: %w", err, model.ErrValidation)
				}
				if m.env.DataDir == "" {
					return fmt.Errorf("no data directory in this session, so the setting cannot be saved: %w", model.ErrValidation)
				}
				if err := theme.SaveFill(m.env.DataDir, f); err != nil {
					return err
				}
				m.fillMode = f
				return nil
			}},
		{id: "theme-reload", section: secAppearance, name: "Theme files", words: "theme reload refresh themes", desc: "re-read ~/.crm/themes and apply the theme again", scope: scopeDevice,
			action: "theme-reload"},

		// Keys
		{id: "keys", section: secKeys, name: "Keys", words: "keybindings shortcuts rebind customize", desc: "rebind an action: pick it, press the new key", scope: scopeDevice,
			value: func(m *Model) string {
				if n := len(m.keyOverrides); n > 0 {
					return fmt.Sprintf("%d custom", n)
				}
				return "defaults"
			}, action: "keys-customize"},
		{id: "keys-reset", section: secKeys, name: "Keys: reset", words: "keybindings shortcuts defaults", desc: "put one or all custom keys back to the defaults", scope: scopeDevice,
			action: "keys-reset"},

		// Views
		{id: "views", section: secViews, name: "Views", words: "modules tabs order hide show arrange", desc: "show, hide and reorder the views (the v key)", scope: scopeDevice,
			value: func(m *Model) string {
				n := m.mods.Normalize()
				return fmt.Sprintf("%d shown, %d hidden", len(n.Order)-len(n.Hidden), len(n.Hidden))
			},
			run: func(m *Model) tea.Cmd { return m.openViews() }},
		{id: "views-reset", section: secViews, name: "Views: reset", words: "modules default arrangement", desc: "show the default views in the default order", scope: scopeDevice,
			action: "modules-reset"},

		// Deals
		{id: "deal-stages", section: secDeals, name: "Deal stages", words: "pipeline stage won lost outcome vocabulary", desc: "add, rename, order and remove the stages of a deal", scope: scopeSynced,
			value: vocabCount(model.DealStageKind, "stage"), action: "deal-stages"},
		{id: "deal-roles", section: secDeals, name: "Deal roles", words: "role champion buyer vocabulary", desc: "add, rename, order and remove the roles a person has on a deal", scope: scopeSynced,
			value: vocabCount(model.DealRoleKind, "role"), action: "deal-roles"},

		// Types and vocabularies
		{id: "note-types", section: secTypes, name: "Note types", words: "call email meeting kinds interaction vocabulary", desc: "add or archive note types (call, email, meeting, ...)", scope: scopeSynced,
			value: func(m *Model) string {
				list, err := m.ntr.List(m.ctx, false)
				if err != nil {
					return ""
				}
				return countText(len(list), "type")
			}, action: "note-types"},
		{id: "relation-kinds", section: secTypes, name: "Relation kinds", words: "friend parent manager inverse mutual link kinds vocabulary", desc: "add, rename, order and remove the kinds of person and organization links (friend, parent, manager, ...)", scope: scopeSynced,
			value: func(m *Model) string {
				kr := repo.NewKindRepo(m.db)
				n := 0
				for _, t := range []string{"person", "organization"} {
					if list, err := kr.List(m.ctx, t); err == nil {
						n += len(list)
					}
				}
				return countText(n, "kind")
			},
			run: func(m *Model) tea.Cmd { return m.openListEditor("relation-kinds") }},

		// Names
		{id: "aliases", section: secNames, name: "Name aliases", words: "nicknames zach zachary alias table", desc: "the table that lets Zach find Zachary. Manage it with: crm alias list, add, remove", scope: scopeSynced,
			run: func(m *Model) tea.Cmd {
				return m.showToast("name aliases have no screen yet. Use crm alias list, crm alias add and crm alias remove", false)
			}},
		{id: "company-ignore", section: secNames, name: "Ignored company texts", words: "company ignore organization backfill never create", desc: "texts that never create an organization", scope: scopeSynced,
			action: "org-ignore"},

		// Behavior
		{id: "syntax", section: secBehavior, name: "Short syntax", words: "inline tokens hash at toggle", desc: "#tag @Name !high ~30m by fri every 3m in the bar and prompts. Enter toggles", scope: scopeSession,
			value: func(m *Model) string { return onOff(!m.noSyntax) }, field: &syntaxField,
			set: func(m *Model, v string) error { m.noSyntax = v == "off"; return nil }, action: "syntax-toggle"},
		{id: "read-only", section: secBehavior, name: "Read-only", words: "readonly lock safe writes", desc: "set with crm tui --read-only when the TUI starts", scope: scopeSession,
			value: func(m *Model) string { return onOff(m.readOnly) },
			run: func(m *Model) tea.Cmd {
				return m.showToast("read-only is "+onOff(m.readOnly)+". Start crm tui --read-only to turn it on", false)
			}},
		{id: "web-search", section: secBehavior, name: "Web search", words: "open o engine google bing duckduckgo brave ecosia linkedin youtube maps browser default", desc: "the search engine o uses for the focused text. Custom takes your own address", scope: scopeDevice,
			value: func(m *Model) string {
				c := loadSearchConfig(m.env.DataDir)
				return c.nameOf(c.Engine)
			}, field: &webSearchField,
			set: func(m *Model, v string) error {
				c := loadSearchConfig(m.env.DataDir)
				if v == engineCustom && c.Custom == "" {
					return fmt.Errorf("type the custom address first (Web search: custom address): %w", model.ErrValidation)
				}
				c.Engine = v
				return c.save(m.env.DataDir)
			}},
		{id: "web-search-custom", section: secBehavior, name: "Web search: custom address", words: "open o url template query custom engine", desc: "your own search address, with {query} where the text goes. Used when Web search is Custom", scope: scopeDevice,
			value: func(m *Model) string { return loadSearchConfig(m.env.DataDir).Custom }, field: &customSearchField,
			set: func(m *Model, v string) error {
				res := validate.SearchTemplate(v)
				if err := res.Err(); err != nil {
					return err
				}
				c := loadSearchConfig(m.env.DataDir)
				c.Custom = res.Value
				if c.Custom == "" && c.Engine == engineCustom {
					c.Engine = defaultEngine
				}
				return c.save(m.env.DataDir)
			}},
		{id: "web-search-offer", section: secBehavior, name: "Web search: offer on o", words: "open o engines picker several choose mark", desc: "mark engines to choose from when you press o. With more than one marked, o opens a small picker with the default first", scope: scopeDevice,
			value: func(m *Model) string { return loadSearchConfig(m.env.DataDir).summary() },
			run:   func(m *Model) tea.Cmd { m.openOfferPicker(0); return nil }},

		// Data
		{id: "database", section: secData, name: "Database", words: "db where path file domain schema size info", desc: "the open database: path, schema version, size", scope: scopeDevice,
			value: func(m *Model) string {
				if m.env.DBPath == "" {
					return "unknown"
				}
				return domainOf(m.env.DBPath)
			}, action: "where"},
		{id: "domains", section: secData, name: "Domains", words: "databases data directory files list", desc: "the databases in the data directory", scope: scopeDevice,
			action: "domains"},
		{id: "doctor", section: secData, name: "Doctor", words: "health check install problems fix", desc: "check the installation and database, with a fix for each problem", scope: scopeDevice,
			action: "doctor"},
		{id: "backups", section: secData, name: "Backups", words: "backup migration copy restore automatic", desc: "the automatic copies taken before a database upgrade", scope: scopeDevice,
			value: func(m *Model) string {
				if m.env.DBPath == "" {
					return ""
				}
				return countText(len(db.ListBackups(m.env.DBPath)), "copy")
			},
			run: func(m *Model) tea.Cmd { m.showBackups(); return nil }},

		// Sync
		{id: "sync", section: secSync, name: "Open Sync", words: "sources adapters carddav inbox sync", desc: "open the Sync view: sources, last runs and the inbox", scope: scopeDevice,
			value: func(m *Model) string { return countText(len(m.registry.Names()), "source") },
			run:   func(m *Model) tea.Cmd { m.switchView(viewSync); return nil }},
		{id: "inbox", section: secSync, name: "Inbox", words: "sync pending records accept attach ignore", desc: "the records that wait for you to accept, attach or ignore", scope: scopeSynced,
			action: "open-inbox"},

		// Archive
		{id: "archived", section: secArchive, name: "Open Archived", words: "archive restore purge deleted trash", desc: "open the Archived view: restore or purge archived records", scope: scopeSynced,
			run: func(m *Model) tea.Cmd { m.switchView(viewArchived); return nil }},
	}...)
}

// settingByID finds a setting of the registry.
func (m *Model) settingByID(id string) *setting {
	for _, s := range m.settingList() {
		if s.id == id {
			s := s
			return &s
		}
	}
	return nil
}

// valueOf is the current value shown for a setting.
func (s *setting) valueOf(m *Model) string {
	if s.value == nil {
		return ""
	}
	return s.value(m)
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
// with its current value in the label and its scope as the hint.
func (m *Model) settingItems(q string) []overlayItem {
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(q, ":", " ")))
	var out []overlayItem
	for _, s := range m.settingList() {
		hay := strings.Fields(strings.ToLower(strings.ReplaceAll(s.name, ":", "") + " " + s.words + " " + s.section + " settings setting " + string(s.scope)))
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
		if v := s.valueOf(m); v != "" {
			label += ": " + v
		}
		out = append(out, overlayItem{id: "setting:" + s.id, kind: kindSetting, label: label, desc: s.desc, hint: string(s.scope), value: s.action})
	}
	return out
}

// runSetting is enter on a setting in the bar. It opens the action behind the setting, does what the
// setting itself does, or for a setting that is only edited in place goes to its row in the Settings view.
func (m *Model) runSetting(it overlayItem) tea.Cmd {
	if m.cmp != nil {
		m.setStatus("settings are not available in the compare screen", true)
		return nil
	}
	id := strings.TrimPrefix(it.id, "setting:")
	s := m.settingByID(id)
	if s == nil {
		return nil
	}
	if s.run != nil {
		return s.run(m)
	}
	if a := m.actionByID(s.action); a != nil {
		return m.runAction(a)
	}
	if s.field != nil {
		m.gotoSetting(id)
	}
	return nil
}

// --- the Settings module ---

// settingRows builds the rows of the Settings view: one divider per section (the grouped list
// component), then the settings of the section with the value and the scope tag.
func (m *Model) settingRows() []row {
	list := m.settingList()
	// An item is a setting or, in the Keys section, an action with its key.
	type item struct {
		s   *setting
		act *action
	}
	var ordered []item
	for _, sec := range settingSections {
		for i := range list {
			if list[i].section == sec {
				ordered = append(ordered, item{s: &list[i]})
			}
		}
		if sec == secKeys {
			for _, a := range m.keyActions() {
				ordered = append(ordered, item{act: a})
			}
		}
	}
	counts := map[string]int{}
	var keyActs []*action
	for _, it := range ordered {
		switch {
		case it.act != nil:
			counts[secKeys]++
			keyActs = append(keyActs, it.act)
		default:
			counts[it.s.section]++
		}
	}
	layerN, areaN := keyCounts(keyActs)
	defaults := defaultKeysByID()
	g := groupRows(ordered,
		func(it item) []groupHead {
			sec := secKeys
			if it.s != nil {
				sec = it.s.section
			}
			heads := []groupHead{{key: sec, title: sec, meta: countText(counts[sec], "setting"), rule: true}}
			if it.act != nil {
				heads = append(heads, keyHeads(it.act, layerN, areaN)...)
			}
			return heads
		},
		func(it item) row {
			if it.act != nil {
				return keyRowFor(it.act, defaults)
			}
			s := it.s
			return row{setting: s.id, title: s.name, sub: cleanCell(s.valueOf(m)), meta: string(s.scope)}
		})
	return g.rows
}

// gotoSetting switches to the Settings view with the cursor on a setting.
func (m *Model) gotoSetting(id string) {
	m.switchView(viewSettings)
	for i, r := range m.rows {
		if r.setting == id {
			m.cursor = i
			m.clamp()
			m.refreshDetail(true)
			return
		}
	}
}

// curSetting is the setting under the cursor of the Settings view.
func (m *Model) curSetting() *setting {
	if m.view != viewSettings {
		return nil
	}
	r, ok := m.selected()
	if !ok || r.setting == "" {
		return nil
	}
	return m.settingByID(r.setting)
}

// buildSettingForm is the detail pane of a setting: what it is, where it is stored and, for a field
// setting, the value as a typed row that enter or e edits in place.
func (m *Model) buildSettingForm(id string) []formRow {
	if strings.HasPrefix(id, "key:") {
		return m.keyDetail(strings.TrimPrefix(id, "key:"))
	}
	m.formTitle = ""
	s := m.settingByID(id)
	if s == nil {
		return nil
	}
	m.formTitle = s.name
	rows := []formRow{infoRow("section", s.section)}
	rows[0].head = "Setting"
	rows = append(rows, infoRow("about", s.desc), infoRow("stored", string(s.scope)+": "+s.scope.note()))
	switch {
	case s.field != nil && s.set != nil:
		r := specRow(repo.Ref{Kind: "setting"}, *s.field, s.valueOf(m))
		r.head = "Value"
		rows = append(rows, r)
	default:
		v := s.valueOf(m)
		if s.colorKind != "" {
			sr := infoRow("preview", m.colorSample(s.colorKind, s.name))
			sr.head = "Preview"
			rows = append(rows, sr)
		}
		if v != "" {
			vr := infoRow("value", v)
			vr.head = "Value"
			rows = append(rows, vr)
		}
		if s.run == nil && s.action == "" {
			rows = append(rows, infoRow("", "nothing to open"))
		}
	}
	return rows
}

// openSetting is enter on a row of the Settings view: a field setting starts its in-place editor,
// any other setting opens what it is for.
func (m *Model) openSetting() tea.Cmd {
	if a := m.curKeyAction(); a != nil {
		if m.readOnly {
			m.setStatus("read-only: keys cannot be changed", true)
			return nil
		}
		m.startCapture(a.id)
		return nil
	}
	s := m.curSetting()
	if s == nil {
		return nil
	}
	if s.field != nil && s.set != nil {
		for i, r := range m.form {
			if r.kind == rkTyped {
				m.fcur, m.fcol = i, 0
				return m.beginTyped()
			}
		}
		return nil
	}
	if s.run != nil {
		return s.run(m)
	}
	if a := m.actionByID(s.action); a != nil {
		return m.runAction(a)
	}
	return nil
}

// saveSetting writes a field setting from the typed editor.
func (m *Model) saveSetting(id, v string) error {
	s := m.settingByID(id)
	if s == nil || s.set == nil {
		return fmt.Errorf("%q is not an editable setting: %w", id, model.ErrValidation)
	}
	return s.set(m, v)
}

func settingActions() []action {
	return []action{
		{id: "setting-open", title: "Open setting", group: "Verbs", desc: "in Settings: edit the setting under the cursor in place, or open the screen it belongs to ",
			when: func(m *Model) bool { return m.view == viewSettings && m.focus == focusList && (m.curSetting() != nil || m.curKeyAction() != nil) },
			run:  func(m *Model) tea.Cmd { return m.openSetting() }},
	}
}

// showBackups lists the automatic copies taken before a database upgrade.
func (m *Model) showBackups() {
	if m.env.DBPath == "" {
		m.setStatus("backups: the database path is not known in this session", true)
		return
	}
	list := db.ListBackups(m.env.DBPath)
	var b strings.Builder
	fmt.Fprintf(&b, "The program copies the database before it upgrades the schema and keeps the newest %d.\n", db.KeepPreMigrationBackups)
	fmt.Fprintf(&b, "Folder: %s\n\n", filepath.Join(filepath.Dir(m.env.DBPath), "backups"))
	if len(list) == 0 {
		b.WriteString("No backups yet. One is made the first time an upgrade changes this database.\n")
	}
	for _, bk := range list {
		stamp := bk.Stamp
		if len(stamp) == 15 {
			stamp = stamp[:4] + "-" + stamp[4:6] + "-" + stamp[6:8] + " " + stamp[9:11] + ":" + stamp[11:13] + ":" + stamp[13:]
		}
		fmt.Fprintf(&b, "%s  schema v%d  %s  %s\n", stamp, bk.Version, humanSize(bk.Size), bk.Name)
	}
	b.WriteString("\nTo go back, close the program and copy a backup over the database file.\n")
	m.openText("Backups", b.String(), nil)
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
