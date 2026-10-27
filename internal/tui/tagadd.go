package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Tags are objects (owner decision 2026-10-28), so the add picker offers them with the other kinds:
// a then g. The flow is a name prompt in the pick-or-create style: the name of an existing tag picks
// it, a new name creates it. A tag exists only as a name on objects (patterns/universal-verbs), so a
// highlighted object is offered as the one to put the tag on (the grayscale "tab: link ..." hint) and
// tab accepts it. Without it the tag is created and is listed once an object carries it.

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
// object the user accepted with tab.
func (m *Model) saveNewTag(value string) {
	name := strings.TrimLeft(strings.TrimSpace(value), "#")
	if name == "" {
		m.setStatus("enter a tag name", true)
		return
	}
	p := m.addLinkPending
	m.addLinkPending = nil
	existed := false
	if tags, err := m.tagr.FindAll(m.ctx); err == nil {
		for _, t := range tags {
			if strings.EqualFold(t.Name, name) {
				existed, name = true, t.Name
			}
		}
	}
	if p != nil && p.accepted && p.tag == "" && p.ref.ID != 0 {
		if err := m.hTagApply(m.ctx, p.ref.Kind, p.ref.ID, name); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWrite()
		m.setStatus("tagged "+p.label+" #"+name+" (ctrl+z undoes)", false)
		m.pendingCmd = m.showToast("tagged "+p.label+" #"+name+" · ctrl+z undoes", false)
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

// tagSuggestion settles what the highlighted object offers to a new tag: itself, as the object to put
// the tag on, when its kind can take a tag.
func tagSuggestion(s *addSuggestion) *addSuggestion {
	cp := *s
	switch {
	case cp.reason != "":
		return &cp
	case cp.tag != "":
		return &addSuggestion{reason: "a tag cannot be put on a tag"}
	case !model.ValidTaggable(cp.ref.Kind):
		return &addSuggestion{reason: "a " + relationLabel(cp.ref.Kind) + " cannot take a tag"}
	}
	cp.generic = true
	return &cp
}
