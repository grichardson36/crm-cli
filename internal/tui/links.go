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
		{id: "link", title: "Link", group: "Link", hint: "L", keys: []string{"L"},
			desc:  "link the selected person, organization, deal or task to another object (or a tag). Pick or create in one step, from either side. ctrl+z undoes",
			write: true, foot: footLink, footIn: inBoth, footLabel: "link", when: whenAnyObject,
			run: func(m *Model) tea.Cmd { return m.startLinkVerb(false) }},
		{id: "unlink", title: "Unlink", group: "Link", hint: "U", keys: []string{"U"},
			desc:  "remove a link of the selected object (or a tag). ctrl+z undoes",
			write: true, foot: footUnlink, footIn: inBoth, footLabel: "unlink", when: whenAnyObject,
			run: func(m *Model) tea.Cmd { return m.startLinkVerb(true) }},
	}
	for _, other := range append(append([]string{}, model.ObjectKinds...), tagKind) {
		other := other
		label := relationLabel(other)
		out = append(out,
			action{id: "link-" + cliNoun(other), title: "Link " + label, group: "Link", hint: "L",
				desc:  "link " + article(label) + " " + label + " to the selected object: pick one or create it from the text you type (same as crm <object> link " + cliNoun(other) + ")",
				write: true, when: whenLinkable(other),
				run: func(m *Model) tea.Cmd {
					src, _ := m.cursorObject()
					return m.startLink(src, other, false)
				}},
			action{id: "unlink-" + cliNoun(other), title: "Unlink " + label, group: "Link", hint: "U",
				desc:  "remove the link between the selected object and " + article(label) + " " + label + " (same as crm <object> unlink " + cliNoun(other) + ")",
				write: true, when: whenLinkable(other),
				run: func(m *Model) tea.Cmd {
					src, _ := m.cursorObject()
					return m.startLink(src, other, true)
				}},
		)
	}
	return out
}

// startLinkVerb is the L and U key: ask which relation when there is more than one.
func (m *Model) startLinkVerb(unlink bool) tea.Cmd {
	src, ok := m.cursorObject()
	if !ok {
		return nil
	}
	kinds := relationsOf(src.Kind)
	title := "Link " + m.refName(src) + " to"
	if unlink {
		title = "Unlink from " + m.refName(src)
	}
	var items []overlayItem
	for i, k := range kinds {
		it := overlayItem{id: "rel-" + k, label: relationLabel(k), value: k, hint: strconv.Itoa(i + 1)}
		if unlink {
			if n := len(m.linkedItems(src, k)); n == 0 {
				it.disabled = "none linked"
			} else {
				it.desc = fmt.Sprintf("%d linked", n)
			}
		}
		items = append(items, it)
	}
	m.openPicker(title, items, func(m *Model, it overlayItem) {
		if it.disabled != "" {
			m.setStatus(m.refName(src)+" has no "+it.label+" linked", true)
			return
		}
		m.pendingCmd = m.startLink(src, it.value, unlink)
	})
	m.ov.digits = true
	return nil
}

// linkedItems lists what src is linked to, as picker items.
func (m *Model) linkedItems(src repo.Ref, other string) []overlayItem {
	if other == tagKind {
		tags, _ := m.tagr.GetForEntity(m.ctx, src.Kind, src.ID)
		var out []overlayItem
		for _, t := range tags {
			out = append(out, overlayItem{id: "pick", label: "#" + t.Name, value: t.Name})
		}
		return out
	}
	ids, err := m.links.Linked(m.ctx, src, other)
	if err != nil {
		return nil
	}
	var out []overlayItem
	for _, id := range ids {
		r := repo.Ref{Kind: other, ID: id}
		out = append(out, overlayItem{id: "pick", label: cleanCell(m.refName(r)), ref: id, value: m.refName(r)})
	}
	return out
}

// candidateItems lists what src could be linked to: everything not linked yet.
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
	if ids, err := m.links.Linked(m.ctx, src, other); err == nil {
		for _, id := range ids {
			linked[id] = true
		}
	}
	var out []overlayItem
	for _, it := range o.items(m) {
		if !linked[it.ref] {
			out = append(out, it)
		}
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

// doLink links src to the picked object, creating it first when the row was Create. The create and
// the link are one undo step.
func (m *Model) doLink(src repo.Ref, other string, it overlayItem) {
	m.beginGroup()
	defer func() { m.afterWrite() }()
	name := m.refName(src)
	if other == tagKind {
		tag := strings.TrimLeft(it.value, "#")
		if err := m.hTagApply(m.ctx, src.Kind, src.ID, tag); err != nil {
			m.endGroup("")
			m.setStatus(err.Error(), true)
			return
		}
		m.endGroup("tag #" + tag + " on " + name)
		m.setStatus("tagged "+name+" #"+tag+" (ctrl+z undoes)", false)
		m.pendingCmd = m.showToast("tagged "+name+" #"+tag+" · ctrl+z undoes", false)
		return
	}
	o := objectByKind(other)
	target := repo.Ref{Kind: other, ID: it.ref}
	if it.id == "create" {
		id, err := o.create(m, it.value)
		if err != nil {
			m.endGroup("")
			m.setStatus(err.Error(), true)
			m.pendingCmd = m.showToast(err.Error(), true)
			return
		}
		target.ID = id
	}
	res, err := m.hLink(src, target)
	targetName := m.refName(target)
	m.endGroup("link " + name + " to " + targetName)
	switch {
	case err != nil:
		m.setStatus(err.Error(), true)
		m.pendingCmd = m.showToast(err.Error(), true)
	case !res.Changed:
		m.setStatus(name+" is already linked to "+targetName, false)
		m.pendingCmd = m.showToast(name+" is already linked to "+targetName, false)
	default:
		msg := "linked " + name + " to " + targetName
		if res.Replaced != nil {
			msg += " (replaced " + m.refName(*res.Replaced) + ")"
		}
		m.setStatus(msg+" (ctrl+z undoes)", false)
		m.pendingCmd = m.showToast(msg+" · ctrl+z undoes", false)
	}
}

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
		err = m.hUnlink(src, target)
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

// holderOf says which of a and b stores the link, and which is the target.
func holderOf(a, b repo.Ref) (holder, target repo.Ref) {
	if rel, ok := model.RelationBetween(a.Kind, b.Kind); ok && rel.Holder == b.Kind {
		return b, a
	}
	return a, b
}

func (m *Model) isLinked(a, b repo.Ref) bool {
	ids, err := m.links.Linked(m.ctx, a, b.Kind)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if id == b.ID {
			return true
		}
	}
	return false
}

// hLink makes a link and records it. Undo restores the object the holder pointed at before.
func (m *Model) hLink(a, b repo.Ref) (repo.LinkResult, error) {
	res, err := m.links.Link(m.ctx, a, b)
	if err != nil || !res.Changed {
		return res, err
	}
	holder, target := holderOf(a, b)
	label := "link " + m.refName(a) + " to " + m.refName(b)
	m.record(label,
		func() error {
			if !m.isLinked(holder, target) {
				return staleError{m.refName(a) + " is no longer linked to " + m.refName(b)}
			}
			if res.Replaced != nil {
				_, err := m.links.Link(m.ctx, holder, *res.Replaced)
				return err
			}
			return m.links.Unlink(m.ctx, holder, target)
		},
		func() error {
			_, err := m.links.Link(m.ctx, a, b)
			return err
		})
	return res, nil
}

// hUnlink removes a link and records it.
func (m *Model) hUnlink(a, b repo.Ref) error {
	label := "unlink " + m.refName(a) + " from " + m.refName(b)
	if err := m.links.Unlink(m.ctx, a, b); err != nil {
		return err
	}
	m.record(label,
		func() error { _, err := m.links.Link(m.ctx, a, b); return err },
		func() error { return m.links.Unlink(m.ctx, a, b) })
	return nil
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

// openLinkRow is Enter on the organization or a deal row of a person: a linked row opens that
// object, an empty row links one.
func (m *Model) openLinkRow(r formRow) tea.Cmd {
	other := "organization"
	if r.kind == rkDeal {
		other = "deal"
	}
	if !r.empty {
		m.openObject(other, r.id)
		return nil
	}
	if m.readOnly {
		m.setStatus("read-only: link "+other+" is disabled", true)
		return nil
	}
	return m.startLink(repo.Ref{Kind: "person", ID: m.formPerson}, other, false)
}

// askUnlinkRow is d on the organization or a deal row of a person.
func (m *Model) askUnlinkRow(r formRow) {
	if r.empty {
		m.setStatus("nothing to delete here", true)
		return
	}
	other := "organization"
	if r.kind == rkDeal {
		other = "deal"
	}
	src := repo.Ref{Kind: "person", ID: m.formPerson}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("unlink %s %q? y/n", other, truncateLabel(r.value)),
		yes: func() {
			m.doUnlink(src, other, overlayItem{ref: r.id, label: r.value})
		},
	}
}
