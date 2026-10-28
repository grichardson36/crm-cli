package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/theme"
)

// Themes. All colors live in internal/theme. The Model holds the active styles in m.st and every
// view draws with them, so switching a theme is one assignment plus a redraw.

// defaultStyles are used until a theme is chosen (tests, and the instant before Run applies the
// remembered theme). It does no terminal I/O.
func defaultStyles() theme.Styles {
	t, _ := theme.Builtin("dark", true)
	return theme.NewStyles(t, colorprofile.TrueColor)
}

// setTheme makes t the active theme for this session.
func (m *Model) setTheme(t theme.Theme) {
	m.st = theme.NewStyles(t, m.profile)
	m.themeName = t.Name
}

// themeCatalog reads the built-in and custom themes from disk.
func (m *Model) themeCatalog() theme.Catalog {
	return theme.LoadCatalog(m.env.DataDir, m.darkBG)
}

// applyStartupTheme picks the remembered theme (CRM_THEME, then config.toml, then auto).
// A problem never stops the program: it falls back to auto and says why in the status line.
func (m *Model) applyStartupTheme() {
	m.darkBG = true // until the terminal answers (tea.BackgroundColorMsg)
	name, _ := theme.Chosen(m.env.DataDir, os.Getenv)
	cat := m.themeCatalog()
	t, err := cat.Find(name)
	if err != nil {
		t, _ = cat.Find(theme.AutoName)
		m.setStatus("theme: "+err.Error()+". Using auto.", true)
	}
	m.setTheme(t)
}

// themeItems lists the themes for the picker, the active one first in the cursor sense.
func (m *Model) themeItems(cat theme.Catalog) []overlayItem {
	var items []overlayItem
	for _, t := range cat.Themes {
		it := overlayItem{id: t.Name, label: t.Name, desc: t.Description}
		if !t.Builtin {
			it.desc += " (custom: " + t.Path + ")"
		}
		if t.Name == m.themeName {
			it.hint, it.desc = "*", "current. "+it.desc
		}
		items = append(items, it)
	}
	for _, p := range cat.Problems {
		items = append(items, overlayItem{id: "problem:" + p.File, label: baseName(p.File),
			desc: "cannot load", disabled: p.Err})
	}
	return items
}

func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// openThemeChooser shows the theme list. Moving the cursor previews the theme live, enter keeps
// and remembers it, esc restores the theme that was active.
func (m *Model) openThemeChooser() {
	cat := m.themeCatalog()
	original, _ := cat.Find(m.themeName)
	if original.Name == "" {
		original = m.st.Theme
	}
	items := m.themeItems(cat)
	m.openPicker("Theme", items, func(m *Model, it overlayItem) {
		if it.disabled != "" {
			m.setStatus("theme "+it.label+": "+it.disabled, true)
			m.setTheme(original)
			return
		}
		t, err := cat.Find(it.id)
		if err != nil {
			m.setStatus(err.Error(), true)
			m.setTheme(original)
			return
		}
		m.setTheme(t)
		m.rememberTheme(t.Name)
	})
	o := m.ov
	for i, idx := range o.shown {
		if o.items[idx].id == m.themeName {
			o.cursor = i
			break
		}
	}
	o.onCursor = func(m *Model, it overlayItem) {
		if it.disabled != "" {
			return
		}
		if t, err := cat.Find(it.id); err == nil {
			m.setTheme(t)
		}
	}
	o.onCancel = func(m *Model) { m.setTheme(original) }
}

// rememberTheme saves the choice for this device and reports it in the status line.
func (m *Model) rememberTheme(name string) {
	if m.env.DataDir == "" {
		m.setStatus("theme: "+name+" (not saved, the data directory is unknown)", false)
		return
	}
	if err := theme.SaveChoice(m.env.DataDir, name); err != nil {
		m.setStatus("theme "+name+" applied but not saved: "+err.Error(), true)
		return
	}
	if v := strings.TrimSpace(os.Getenv(theme.EnvName)); v != "" && !strings.EqualFold(v, name) {
		m.setStatus(fmt.Sprintf("theme: %s saved, but %s=%s overrides it at startup", name, theme.EnvName, v), false)
		return
	}
	m.setStatus("theme: "+name, false)
}

// reloadTheme re-reads the theme files and re-applies the active theme, so edits to a custom file
// show without restarting. A broken file keeps the current look and shows the error.
func (m *Model) reloadTheme() {
	cat := m.themeCatalog()
	t, err := cat.Find(m.themeName)
	if err != nil {
		m.setStatus("theme reload: "+err.Error()+" (keeping the current look)", true)
		return
	}
	m.setTheme(t)
	if n := len(cat.Problems); n > 0 {
		m.setStatus(fmt.Sprintf("theme reloaded: %s. %d theme file(s) have errors: %s", t.Name, n, cat.Problems[0].Err), true)
		return
	}
	m.setStatus("theme reloaded: "+t.Name, false)
}
