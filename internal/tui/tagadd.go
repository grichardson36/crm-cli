package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Tags are objects (owner decision 2026-10-28), so the add picker offers them with the other kinds:
// a then g. The flow is a name prompt in the pick-or-create style: the name of an existing tag picks
// it, a new name creates it. A tag exists only as a name on objects (patterns/universal-verbs), so the
// prompt is a pipe line, name | on: the "on" segment names the objects to put the tag on, and a
// highlighted object is its first suggestion (tab accepts it). Without one the tag is created and is
// listed once an object carries it.

func tagDef() *objDef {
	return &objDef{
		kind: tagKind, label: "tag", short: "tag", group: "Tags", view: viewPeople,
		shortcut: "g", // t is the task, so the tag takes the g of tag
		can:      map[verb]bool{vAdd: true},
		selected: func(m *Model) int64 { return 0 },
		nameOf:   func(m *Model, id int64) string { return "tag" },
		add:      func(m *Model) tea.Cmd { return m.startTagAdd() },
	}
}

// startTagAdd opens the name prompt of a new tag.
func (m *Model) startTagAdd() tea.Cmd {
	m.startInput("tag-new", "tag: ", "")
	return m.input.Focus()
}

// saveNewTag is the enter of the tag name prompt: find the tag by name or create it, and put it on the
// objects named in the "on" segment (pipelink.go resolved them already).
func (m *Model) saveNewTag(value string) {
	name := strings.TrimLeft(strings.TrimSpace(value), "#")
	if name == "" {
		m.setStatus("enter a tag name", true)
		return
	}
	links := m.pipeLinks
	m.pipeLinks, m.pipeObj = nil, ""
	m.pipeRes = map[string]pendingLink{}
	existed := false
	if tags, err := m.tagr.FindAll(m.ctx); err == nil {
		for _, t := range tags {
			if strings.EqualFold(t.Name, name) {
				existed, name = true, t.Name
			}
		}
	}
	if len(links) > 0 {
		var labels []string
		m.beginGroup()
		for _, l := range links {
			if l.ref.ID == 0 {
				continue
			}
			if err := m.hTagApply(m.ctx, l.ref.Kind, l.ref.ID, name); err != nil {
				m.endGroup("")
				m.setStatus(err.Error(), true)
				return
			}
			labels = append(labels, l.label)
		}
		m.endGroup("tag #" + name + " on " + strings.Join(labels, ", "))
		m.afterWrite()
		msg := "tagged " + strings.Join(labels, ", ") + " #" + name + " (ctrl+z undoes)"
		m.setStatus(msg, false)
		m.pendingCmd = m.showToast("tagged "+strings.Join(labels, ", ")+" #"+name+" · ctrl+z undoes", false)
		return
	}
	if _, err := m.tagr.FindOrCreate(m.ctx, name); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	msg := "tag #" + name + " created, it is listed once an object carries it (l tags the highlighted one)"
	if existed {
		msg = "tag #" + name + " exists already"
	}
	m.setStatus(msg, false)
	m.pendingCmd = m.showToast(msg, false)
}
