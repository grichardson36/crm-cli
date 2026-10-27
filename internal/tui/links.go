package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Link and Unlink for every relation, from either end, through one pick-or-create picker
// (patterns/link-from-either-side, pick-or-create). The relations come from model.Relations.
// Tags attach to anything. Every link is one undoable step.

// tagKind is the pseudo kind of the tag relation.
const tagKind = "tag"

// relationsOf lists what an object of this kind can be linked to, tags last.
func relationsOf(kind string) []string {
	return append(model.RelatedKinds(kind), tagKind)
}

func relationLabel(kind string) string {
	if kind == tagKind {
		return tagKind
	}
	if o := objectByKind(kind); o != nil {
		return o.label
	}
	return kind
}

// cursorObject is the object Link and Unlink act on: a task row in the detail form or the Tasks
// view, an organization or deal in their views, otherwise the person under the cursor.
func (m *Model) cursorObject() (repo.Ref, bool) {
	for _, o := range objectDefs() {
		if o.kind == "person" {
			continue
		}
		if id := o.selected(m); id != 0 {
			return repo.Ref{Kind: o.kind, ID: id}, true
		}
	}
	if id := objectByKind("person").selected(m); id != 0 {
		return repo.Ref{Kind: "person", ID: id}, true
	}
	return repo.Ref{}, false
}

// refName is the name of an object for prompts, toasts and undo labels.
func (m *Model) refName(r repo.Ref) string {
	if o := objectByKind(r.Kind); o != nil {
		return truncateLabel(o.nameOf(m, r.ID))
	}
	return r.String()
}

func whenLinkable(other string) func(m *Model) bool {
	return func(m *Model) bool {
		src, ok := m.cursorObject()
		if !ok {
			return false
		}
		for _, k := range relationsOf(src.Kind) {
			if k == other {
				return true
			}
		}
		return false
	}
}

func whenAnyObject(m *Model) bool { _, ok := m.cursorObject(); return ok }

// linkActions generates Link and Unlink, and "Link <other>" and "Unlink <other>" for every kind
// an object can be linked to.
func linkActions() []action {
	out := []action{
		{id: "link", title: "Link", group: "Link", hint: "l", keys: []string{"l"},
			desc:  "link the selected person, organization, deal or task to another object (or a tag). Pick or create in one step, from either side. ctrl+z undoes",
			write: true, foot: footLink, footIn: inBoth, footLabel: "link", when: whenAnyObject,
			run: func(m *Model) tea.Cmd { return m.startLinkVerb(false) }},
		{id: "unlink", title: "Unlink", group: "Link", hint: "u", keys: []string{"u"},
			desc:  "remove a link of the selected object (or a tag). ctrl+z undoes",
			write: true, when: whenAnyObject,
			run: func(m *Model) tea.Cmd {
				if m.focus == focusDetail && whenLinkedRow(m) { // u on a linked row removes that chip
					m.askUnlinkRow(m.form[m.fcur])
					return nil
				}
				return m.startLinkVerb(true)
			}},
	}
	for _, other := range append(append([]string{}, model.ObjectKinds...), tagKind) {
		other := other
		label := relationLabel(other)
		out = append(out,
			action{id: "link-" + cliNoun(other), title: "Link " + label, group: "Link",
				desc:  "link " + article(label) + " " + label + " to the selected object: pick one or create it from the text you type (same as crm <object> link " + cliNoun(other) + ")",
				write: true, when: whenLinkable(other),
				run: func(m *Model) tea.Cmd {
					srcs, ok := m.linkSources()
					if !ok {
						return nil
					}
					return m.startLinkMany(srcs, other, false)
				}},
			action{id: "unlink-" + cliNoun(other), title: "Unlink " + label, group: "Link",
				desc:  "remove the link between the selected object and " + article(label) + " " + label + " (same as crm <object> unlink " + cliNoun(other) + ")",
				write: true, when: whenLinkable(other),
				run: func(m *Model) tea.Cmd {
					srcs, ok := m.linkSources()
					if !ok {
						return nil
					}
					return m.startLinkMany(srcs, other, true)
				}},
		)
	}
	return out
}

// startLinkVerb is the l and u key: ask which relation when there is more than one.
func (m *Model) startLinkVerb(unlink bool) tea.Cmd {
	srcs, ok := m.linkSources()
	if !ok {
		return nil
	}
	src := srcs[0]
	who := m.refName(src)
	if len(srcs) > 1 {
		who = nouns(refKinds(srcs), len(srcs))
	}
	kinds := relationsOf(src.Kind)
	title := "Link " + who + " to"
	if unlink {
		title = "Unlink from " + who
	}
	var items []overlayItem
	for i, k := range kinds {
		it := overlayItem{id: "rel-" + k, label: relationLabel(k), value: k, hint: strconv.Itoa(i + 1)}
		if unlink {
			if n := len(m.linkedUnion(srcs, k)); n == 0 {
				it.disabled = "none linked"
			} else {
				it.desc = fmt.Sprintf("%d linked", n)
			}
		}
		items = append(items, it)
	}
	m.openPicker(title, items, func(m *Model, it overlayItem) {
		if it.disabled != "" {
			m.setStatus(who+" has no "+it.label+" linked", true)
			return
		}
		m.pendingCmd = m.startLinkMany(srcs, it.value, unlink)
	})
	m.ov.digits = true
	return nil
}

// linkedItems lists what src is linked to, as picker items. Every stored link is its own item, so a
// person who was at one organization twice shows twice.
func (m *Model) linkedItems(src repo.Ref, other string) []overlayItem {
	if other == tagKind {
		tags, _ := m.tagr.GetForEntity(m.ctx, src.Kind, src.ID)
		var out []overlayItem
		for _, t := range tags {
			out = append(out, overlayItem{id: "pick", label: "#" + t.Name, value: t.Name})
		}
		return out
	}
	items, err := m.links.Linked(m.ctx, src, other)
	if err != nil {
		return nil
	}
	var out []overlayItem
	for _, li := range items {
		out = append(out, overlayItem{id: "pick", label: linkSummary(li), ref: li.ID, link: li.LinkID, value: li.Name})
	}
	return out
}

// candidateItems lists what src could be linked to: every object of the kind except src itself.
// Ones already linked stay in the list, picking one edits that link.
func (m *Model) candidateItems(src repo.Ref, other string) []overlayItem {
	if other == tagKind {
		have := map[string]bool{}
		for _, it := range m.linkedItems(src, tagKind) {
			have[it.value] = true
		}
		tags, _ := m.tagr.FindAll(m.ctx)
		var out []overlayItem
		for _, t := range tags {
			if !have[t.Name] {
				out = append(out, overlayItem{id: "pick", label: "#" + t.Name, value: t.Name})
			}
		}
		return out
	}
	o := objectByKind(other)
	if o == nil || o.items == nil {
		return nil
	}
	linked := map[int64]bool{}
	if items, err := m.links.Linked(m.ctx, src, other); err == nil {
		for _, li := range items {
			linked[li.ID] = true
		}
	}
	var out []overlayItem
	for _, it := range o.items(m) {
		if other == src.Kind && it.ref == src.ID {
			continue
		}
		if linked[it.ref] {
			it.desc = strings.TrimSpace("linked, enter edits " + it.desc)
		}
		out = append(out, it)
	}
	return out
}

// startLink opens the picker for a relation. Link lists the objects not linked yet and ends with
// "Create <typed text>". Unlink lists the linked ones and acts at once when there is only one.
func (m *Model) startLink(src repo.Ref, other string, unlink bool) tea.Cmd {
	label := relationLabel(other)
	if unlink {
		items := m.linkedItems(src, other)
		switch len(items) {
		case 0:
			m.setStatus(m.refName(src)+" has no "+label+" linked", true)
			return nil
		case 1:
			m.doUnlink(src, other, items[0])
			return m.takePending()
		}
		m.openPicker("Unlink "+label+" from "+m.refName(src), items, func(m *Model, it overlayItem) { m.doUnlink(src, other, it) })
		return nil
	}
	if m.typeaheadOK(src, other) {
		return m.beginTypeahead(src, other)
	}
	items := m.candidateItems(src, other)
	m.openPicker("Link "+label+" to "+m.refName(src), items, func(m *Model, it overlayItem) { m.doLink(src, other, it) })
	m.ov.create = func(q string) (overlayItem, bool) {
		q = strings.TrimSpace(strings.TrimPrefix(q, "#"))
		if q == "" {
			return overlayItem{}, false
		}
		return overlayItem{id: "create", label: fmt.Sprintf("Create %s %q", label, q), desc: "make it and link it", value: q}, true
	}
	m.ov.refilter()
	return nil
}

// doLink links src to the picked object. Tags apply at once. Anything else goes through the
// Link form (linkflow.go): kind, attributes, then the write.
func (m *Model) doLink(src repo.Ref, other string, it overlayItem) {
	if other != tagKind {
		m.pendingCmd = m.beginLinkDraft(src, other, it)
		return
	}
	m.beginGroup()
	defer func() { m.afterWrite() }()
	name := m.refName(src)
	tag := strings.TrimLeft(it.value, "#")
	if err := m.hTagApply(m.ctx, src.Kind, src.ID, tag); err != nil {
		m.endGroup("")
		m.setStatus(err.Error(), true)
		return
	}
	m.endGroup("tag #" + tag + " on " + name)
	m.setStatus("tagged "+name+" #"+tag+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast("tagged "+name+" #"+tag+" · ctrl+z undoes", false)
}

// doUnlink removes the picked link, or the tag.
func (m *Model) doUnlink(src repo.Ref, other string, it overlayItem) {
	defer func() { m.afterWrite() }()
	name := m.refName(src)
	var err error
	targetName := it.label
	if other == tagKind {
		err = m.hTagRemove(m.ctx, src.Kind, src.ID, it.value)
	} else {
		target := repo.Ref{Kind: other, ID: it.ref}
		targetName = m.refName(target)
		err = m.unlinkRecorded(src, target, it.link)
	}
	if err != nil {
		m.setStatus(err.Error(), true)
		m.pendingCmd = m.showToast(err.Error(), true)
		return
	}
	msg := "unlinked " + name + " from " + targetName
	m.setStatus(msg+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(msg+" · ctrl+z undoes", false)
}

// hCreateTask creates a task and records it.
func (m *Model) hCreateTask(in model.CreateTaskInput) (*model.Task, error) {
	t, err := m.tr.Create(m.ctx, in)
	if err != nil {
		return nil, err
	}
	id := t.ID
	m.record("add task "+truncateLabel(t.Title),
		func() error { return m.tr.Archive(m.ctx, id) },
		func() error { return m.tr.Restore(m.ctx, id) })
	m.afterCreate("task", id)
	return t, nil
}

// openObject shows an object in its own view with the cursor on it.
func (m *Model) openObject(kind string, id int64) {
	switch kind {
	case "person":
		m.selectPerson(id)
	case "task":
		m.selectTask(id)
	case "organization":
		m.switchView(viewOrgs)
		m.cursorOn(func(r row) bool { return r.orgID == id })
	case "deal":
		m.switchView(viewDeals)
		m.cursorOn(func(r row) bool { return r.dealID == id })
	}
}

// pageRef is the object whose record page is open in the detail pane.
func (m *Model) pageRef() repo.Ref {
	switch m.view {
	case viewOrgs:
		if r, ok := m.selected(); ok && r.orgID != 0 {
			return repo.Ref{Kind: "organization", ID: r.orgID}
		}
	case viewDeals:
		if r, ok := m.selected(); ok && r.dealID != 0 {
			return repo.Ref{Kind: "deal", ID: r.dealID}
		}
	}
	return repo.Ref{Kind: "person", ID: m.formPerson}
}

// openLinkRow is Enter on a linked row of a record page: a linked row opens that object, an empty
// row links one.
func (m *Model) openLinkRow(r formRow) tea.Cmd {
	if !r.empty {
		m.openObject(r.other, r.id)
		return nil
	}
	if m.readOnly {
		m.setStatus("read-only: link "+r.other+" is disabled", true)
		return nil
	}
	return m.startLink(m.pageRef(), r.other, false)
}

// askUnlinkRow is d on a linked row of a record page.
func (m *Model) askUnlinkRow(r formRow) {
	if r.empty || r.li == nil {
		m.setStatus("nothing to delete here", true)
		return
	}
	src := m.pageRef()
	li := *r.li
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("unlink %s %q? y/n", r.other, truncateLabel(r.value)),
		yes: func() {
			m.doUnlink(src, r.other, overlayItem{ref: li.ID, link: li.LinkID, label: li.Name})
		},
	}
}
