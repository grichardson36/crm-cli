package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
)

// Link and Unlink on the selection (patterns/bulk-actions-always). With rows selected, l links every
// selected object to the one target you pick or create, and u unlinks them from it. The relation
// attributes (role, dates, kind) are asked once and apply to every link. The whole step is one undo.

// linkSources are the objects Link and Unlink act on: the selected rows when several are selected,
// else the single object of cursorObject.
func (m *Model) linkSources() ([]repo.Ref, bool) {
	if m.focus == focusList {
		if refs := m.markedRefs(); len(refs) > 1 {
			kind := refs[0].Kind
			for _, r := range refs {
				if r.Kind != kind {
					m.setStatus("select objects of one type to link them together", true)
					return nil, false
				}
			}
			if kind == groupKind {
				return nil, false
			}
			return refs, true
		}
		if refs := m.markedRefs(); len(refs) == 1 && refs[0].Kind != groupKind {
			return refs, true
		}
	}
	src, ok := m.cursorObject()
	if !ok {
		return nil, false
	}
	return []repo.Ref{src}, true
}

// startLinkMany is the picker of a bulk link or unlink.
func (m *Model) startLinkMany(srcs []repo.Ref, other string, unlink bool) tea.Cmd {
	if len(srcs) == 1 {
		return m.startLink(srcs[0], other, unlink)
	}
	label := relationLabel(other)
	who := nouns(refKinds(srcs), len(srcs))
	if unlink {
		items := m.linkedUnion(srcs, other)
		if len(items) == 0 {
			m.setStatus("none of the "+who+" has a "+label+" linked", true)
			return nil
		}
		m.openPicker("Unlink "+label+" from "+who, items, func(m *Model, it overlayItem) {
			m.pendingCmd = m.askTyped("unlink "+who+" from "+it.label, bulkWord("unlink", len(srcs)), func() { m.unlinkMany(srcs, other, it) })
		})
		return nil
	}
	items := m.candidateItems(srcs[0], other)
	m.openPicker("Link "+label+" to "+who, items, func(m *Model, it overlayItem) { m.doLinkMany(srcs, other, it) })
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

// linkedUnion lists what any of the objects is linked to, once each.
func (m *Model) linkedUnion(srcs []repo.Ref, other string) []overlayItem {
	var out []overlayItem
	seen := map[string]bool{}
	for _, src := range srcs {
		for _, it := range m.linkedItems(src, other) {
			key := fmt.Sprintf("%s/%d", it.value, it.ref)
			if seen[key] {
				continue
			}
			seen[key] = true
			if other != tagKind {
				it.label = cleanCell(it.value)
			}
			it.link = 0
			out = append(out, it)
		}
	}
	return out
}

// doLinkMany links the objects to the picked or created target. Tags apply at once, any other link
// goes through the link form once for all.
func (m *Model) doLinkMany(srcs []repo.Ref, other string, it overlayItem) {
	if other != tagKind {
		m.linkAlso = srcs[1:]
		m.pendingCmd = m.beginLinkDraft(srcs[0], other, it)
		m.linkAlso = nil
		return
	}
	tag := strings.TrimLeft(it.value, "#")
	m.beginGroup()
	defer m.afterWrite()
	for _, src := range srcs {
		if err := m.hTagApply(m.ctx, src.Kind, src.ID, tag); err != nil {
			m.endGroup("")
			m.setStatus(err.Error(), true)
			return
		}
	}
	who := nouns(refKinds(srcs), len(srcs))
	m.endGroup("tag #" + tag + " on " + who)
	m.marked = map[int64]bool{}
	m.setStatus("tagged "+who+" #"+tag+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast("tagged "+who+" #"+tag+" · ctrl+z undoes", false)
}

// finishLinkMany writes the link of every selected object to the target. finishLink began the group.
func (m *Model) finishLinkMany(d *linkDraft, targetName string) tea.Cmd {
	srcs := append([]repo.Ref{d.src}, d.also...)
	linked, skipped := 0, 0
	var first error
	for _, src := range srcs {
		if src.Kind == d.target.Kind && src.ID == d.target.ID {
			skipped++
			continue
		}
		src := src
		label := "link " + m.refName(src) + " to " + targetName
		var res repo.LinkResult
		err := m.recordLinkChange(label, src, d.target, func() error {
			var e error
			if d.addNew {
				res, e = m.links.LinkNew(m.ctx, src, d.target, d.attrs)
			} else {
				res, e = m.links.Link(m.ctx, src, d.target, d.attrs)
			}
			return e
		})
		if err != nil {
			if first == nil {
				first = err
			}
			skipped++
			continue
		}
		if res.Changed {
			linked++
		}
	}
	who := nouns(refKinds(srcs), len(srcs))
	m.endGroup("link " + who + " to " + targetName)
	m.marked = map[int64]bool{}
	msg := fmt.Sprintf("linked %d of %s to %s", linked, who, targetName)
	if first != nil {
		msg += fmt.Sprintf(", %d skipped: %v", skipped, first)
		m.setStatus(msg, true)
		return m.showToast(msg, true)
	}
	m.setStatus(msg+" (ctrl+z undoes)", false)
	return m.showToast(msg+" · ctrl+z undoes", false)
}

// unlinkMany removes the links between each object and the target, as one undo step. An object that
// is not linked to it is skipped.
func (m *Model) unlinkMany(srcs []repo.Ref, other string, it overlayItem) {
	defer m.afterWrite()
	m.beginGroup()
	done := 0
	for _, src := range srcs {
		var err error
		if other == tagKind {
			err = m.hTagRemove(m.ctx, src.Kind, src.ID, it.value)
		} else {
			err = m.unlinkRecorded(src, repo.Ref{Kind: other, ID: it.ref}, 0)
		}
		if err == nil {
			done++
		}
	}
	who := nouns(refKinds(srcs), len(srcs))
	m.endGroup("unlink " + who + " from " + it.label)
	m.marked = map[int64]bool{}
	msg := fmt.Sprintf("unlinked %d of %s from %s", done, who, it.label)
	m.setStatus(msg+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(msg+" · ctrl+z undoes", false)
}
