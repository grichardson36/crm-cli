package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/doctor"
	"github.com/jdanielnd/crm-cli/internal/help"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
)

// Env describes where the open database lives. The CLI fills it in. Fields may be empty
// (for example in tests), the tools then say what is missing.
type Env struct {
	DBPath   string // absolute path of the open database
	Source   string // how it was chosen: --db, CRM_DB or default
	DataDir  string // directory scanned by Domains
	Version  string // binary version shown by Doctor
	MCPTools int    // number of MCP tools, shown by Doctor
}

// textView is a read-only scrollable page: help topics, doctor, where, domains.
type textView struct {
	title string
	vp    viewport.Model
	back  func(m *Model) // runs after esc, nil just closes
}

// openText shows body in a scrollable page.
func (m *Model) openText(title, body string, back func(m *Model)) {
	vp := viewport.New(m.width, max(1, m.height-2))
	if m.width > 0 {
		body = lipgloss.NewStyle().Width(m.width).Render(body)
	}
	vp.SetContent(body)
	m.tv = &textView{title: title, vp: vp, back: back}
}

func (m *Model) handleTextKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tv := m.tv
	switch msg.String() {
	case "down", "ctrl+n":
		tv.vp.LineDown(1)
	case "up", "ctrl+p":
		tv.vp.LineUp(1)
	case "pgdown":
		tv.vp.ViewDown()
	case "ctrl+d", " ":
		tv.vp.HalfViewDown()
	case "pgup":
		tv.vp.ViewUp()
	case "ctrl+u":
		tv.vp.HalfViewUp()
	case "home", "ctrl+shift+up":
		tv.vp.GotoTop()
	case "end", "ctrl+shift+down":
		tv.vp.GotoBottom()
	case "esc", "q", "ctrl+c":
		m.tv = nil
		if tv.back != nil {
			tv.back(m)
		}
	}
	return m, nil
}

func (m *Model) textViewRender() string {
	cut := lipgloss.NewStyle().MaxWidth(m.width)
	title := m.st.Head.Render(m.tv.title)
	foot := m.st.Dim.Render("up/down scroll  home/end top/bottom  esc back  ctrl+k for every action")
	return cut.Render(title) + "\n" + m.tv.vp.View() + "\n" + cut.Render(foot)
}

// --- help topics ---

// openHelpTopics lists every help topic with a fuzzy filter. Enter opens the topic.
func (m *Model) openHelpTopics(filter string) {
	topics := help.List()
	items := make([]overlayItem, len(topics))
	for i, t := range topics {
		items[i] = overlayItem{id: t.Name, label: t.Name, desc: t.Summary, value: t.Name}
	}
	m.openPicker("Help topics", items, func(m *Model, it overlayItem) {
		f := m.lastFilter
		m.showTopic(it.value, func(m *Model) { m.openHelpTopics(f) })
	})
	if filter != "" {
		m.ov.filter.SetValue(filter)
		m.ov.refilter()
	}
}

func (m *Model) showTopic(name string, back func(m *Model)) {
	t, ok := help.Get(name)
	if !ok {
		m.setStatus("no help topic "+name, true)
		return
	}
	m.openText("Help: "+t.Name+" - "+t.Title, t.Body, back)
}

// openHelpSearch lists the topic lines that contain text. Enter opens that topic.
func (m *Model) openHelpSearch(text string) {
	matches := help.Search(text)
	if len(matches) == 0 {
		m.setStatus(fmt.Sprintf("no help topic mentions %q", text), true)
		return
	}
	items := make([]overlayItem, len(matches))
	for i, mt := range matches {
		id := fmt.Sprintf("%s:%d", mt.Topic, mt.Line)
		items[i] = overlayItem{id: id, label: id, desc: strings.TrimSpace(mt.Text), value: mt.Topic}
	}
	m.openPicker(fmt.Sprintf("Help search: %s (%d)", text, len(matches)), items, func(m *Model, it overlayItem) {
		m.showTopic(it.value, func(m *Model) { m.openHelpSearch(text) })
	})
}

// --- doctor ---

func (m *Model) showDoctor() {
	if m.env.DBPath == "" {
		m.setStatus("doctor: the database path is not known in this session", true)
		return
	}
	rep := doctor.Run(m.ctx, doctor.Options{
		DBPath:      m.env.DBPath,
		Version:     m.env.Version,
		MCPTools:    m.env.MCPTools,
		SyncSources: crmsync.DefaultRegistry(os.Getenv).Names(),
	})
	var b strings.Builder
	for _, c := range rep.Checks {
		fmt.Fprintf(&b, "%-4s  %-13s %s\n", c.Status, c.Name, c.Message)
		if c.Fix != "" {
			fmt.Fprintf(&b, "      fix: %s\n", c.Fix)
		}
	}
	fmt.Fprintf(&b, "\n%d ok, %d warn, %d fail\n", rep.Summary[doctor.OK], rep.Summary[doctor.Warn], rep.Summary[doctor.Fail])
	m.openText("Doctor", b.String(), nil)
}

// --- where and domains ---

func domainOf(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func (m *Model) showWhere() {
	path := m.env.DBPath
	if path == "" {
		m.setStatus("where: the database path is not known in this session", true)
		return
	}
	var b strings.Builder
	line := func(k, v string) { fmt.Fprintf(&b, "%-16s %s\n", k+":", v) }
	line("path", path)
	if m.env.Source != "" {
		line("chosen by", m.env.Source)
	}
	line("domain", domainOf(path))
	st, err := os.Stat(path)
	if err != nil {
		line("exists", "no")
		m.openText("Where", b.String(), nil)
		return
	}
	info, err := db.Inspect(path)
	if err != nil {
		line("note", "cannot open the database read-only: "+err.Error())
		m.openText("Where", b.String(), nil)
		return
	}
	line("schema version", fmt.Sprintf("%d (this build expects %d)", info.SchemaVersion, db.LatestVersion()))
	if info.DeviceID != "" {
		line("device id", info.DeviceID)
	}
	line("size", fmt.Sprintf("%d bytes", st.Size()))
	keys := make([]string, 0, len(info.Counts))
	for k := range info.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		line(k, fmt.Sprintf("%d", info.Counts[k]))
	}
	m.openText("Where", b.String(), nil)
}

func (m *Model) showDomains() {
	dir := m.env.DataDir
	if dir == "" && m.env.DBPath != "" {
		dir = filepath.Dir(m.env.DBPath)
	}
	if dir == "" {
		m.setStatus("domains: the data directory is not known in this session", true)
		return
	}
	var active os.FileInfo
	if m.env.DBPath != "" {
		active, _ = os.Stat(m.env.DBPath)
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		m.setStatus("domains: "+err.Error(), true)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "data directory: %s\n\n", dir)
	fmt.Fprintf(&b, "%-2s %-16s %-7s %s\n", "", "domain", "schema", "people")
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".db") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := db.Inspect(path)
		if err != nil || !info.IsCRM {
			continue
		}
		mark := ""
		if st, err := os.Stat(path); err == nil && active != nil && os.SameFile(active, st) {
			mark = "*"
		}
		fmt.Fprintf(&b, "%-2s %-16s %-7d %d\n   %s\n", mark, domainOf(path), info.SchemaVersion, info.Counts["people"], path)
		n++
	}
	if n == 0 {
		b.WriteString("no CRM databases found\n")
	} else {
		b.WriteString("\n* active database. Select a domain with --db PATH or CRM_DB.\n")
	}
	m.openText("Domains", b.String(), nil)
}
