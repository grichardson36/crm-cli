package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
)

// Bulk verbs (patterns/bulk-actions-always). Selection is in selection.go. The verbs below act on the
// selected rows, else on the highlighted one, for every object type. A destructive verb that reaches
// more than one object shows the count and asks for it typed back (askTyped), the same way the bulk
// purge always has. One object keeps the plain y/n or its name.

// typedConfirm is a destructive bulk verb waiting for its confirmation word.
type typedConfirm struct {
	word string
	yes  func()
}

// askTyped opens the footer prompt for a destructive bulk verb. The user types word (for example
// "delete 5") and enter. Anything else, or esc, changes nothing.
func (m *Model) askTyped(what, word string, yes func()) tea.Cmd {
	m.typed = &typedConfirm{word: word, yes: yes}
	m.startInput("typed-confirm", fmt.Sprintf("type %q to %s: ", word, what), "")
	return m.input.Focus()
}

// runTypedConfirm is the enter of the typed prompt.
func (m *Model) runTypedConfirm(typed string) {
	t := m.typed
	m.typed = nil
	if t == nil {
		return
	}
	if !sameName(typed, t.word) {
		m.setStatus(fmt.Sprintf("did not match %q, nothing changed", t.word), true)
		return
	}
	t.yes()
}

// bulkWord is the word that confirms a destructive verb on n objects: "delete 5".
func bulkWord(verb string, n int) string { return fmt.Sprintf("%s %d", verb, n) }

// nouns names n objects of the given kinds: "3 people", "2 deals", "5 objects" for a mix.
func nouns(kinds []string, n int) string {
	same := len(kinds) > 0
	for _, k := range kinds {
		if k != kinds[0] {
			same = false
		}
	}
	if !same {
		return fmt.Sprintf("%d objects", n)
	}
	label := relationLabel(kinds[0])
	switch {
	case n == 1:
		return "1 " + label
	case label == "person":
		return fmt.Sprintf("%d people", n)
	}
	return fmt.Sprintf("%d %ss", n, label)
}

func refKinds(refs []repo.Ref) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Kind
	}
	return out
}

// --- delete ---

// deleteMany deletes several objects of one kind as one undo step.
func (m *Model) deleteMany(o *objDef, ids []int64) {
	m.beginGroup()
	ok, failed := 0, 0
	var first error
	for _, id := range ids {
		if err := o.archive(m, id); err != nil {
			failed++
			if first == nil {
				first = err
			}
			continue
		}
		m.deleted[o.kind] = append(m.deleted[o.kind], id)
		ok++
	}
	what := nouns(repeatKind(o.kind, ok), ok)
	m.endGroup("delete " + what)
	m.marked = map[int64]bool{}
	m.afterWrite()
	if failed > 0 {
		m.setStatus(fmt.Sprintf("%d deleted, %d failed: %v", ok, failed, first), true)
		return
	}
	m.setStatus(what+" deleted (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(what+" deleted · ctrl+z undoes", false)
}

func repeatKind(kind string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = kind
	}
	return out
}

// --- restore ---

// restoreTargets restores the selected archived objects, or the one under the cursor.
func (m *Model) restoreTargets() {
	refs := m.bulkRefs()
	if len(refs) == 0 {
		return
	}
	if len(refs) == 1 {
		ref := refs[0]
		if ref.Kind != "person" {
			if r, ok := m.selected(); ok && m.view == viewArchived && len(m.markedRefs()) == 0 {
				m.restoreArchivedObject(r)
				return
			}
		}
		m.restoreMany(refs)
		return
	}
	m.restoreMany(refs)
}

// restoreMany restores archived objects of any kind as one undo step.
func (m *Model) restoreMany(refs []repo.Ref) {
	m.beginGroup()
	ok, failed := 0, 0
	var first error
	for _, ref := range refs {
		o := objectByKind(ref.Kind)
		if o == nil {
			continue
		}
		if err := o.restore(m, ref.ID); err != nil {
			failed++
			if first == nil {
				first = err
			}
			continue
		}
		ok++
	}
	what := nouns(refKinds(refs), ok)
	m.endGroup("restore " + what)
	if len(refs) == 1 && ok == 1 {
		what = m.archivedName(refs[0])
	}
	m.marked = map[int64]bool{}
	m.afterWrite()
	switch {
	case failed > 0:
		m.setStatus(fmt.Sprintf("%d restored, %d failed: %v", ok, failed, first), true)
	case ok == 1 && len(refs) == 1:
		m.setStatus("restored "+what+" (ctrl+z undoes)", false)
	default:
		m.setStatus("restored "+what+" (ctrl+z undoes)", false)
		m.pendingCmd = m.showToast("restored "+what+" · ctrl+z undoes", false)
	}
}

// --- purge ---

func (m *Model) archivedName(ref repo.Ref) string {
	if it, ok := m.archItems[fmt.Sprintf("%s:%d", ref.Kind, ref.ID)]; ok {
		return cleanCell(it.Name)
	}
	return fmt.Sprintf("%s %d", relationLabel(ref.Kind), ref.ID)
}

// whenBulkPurgeSelected is true in the Archived view while archived rows are selected.
func whenBulkPurgeSelected(m *Model) bool {
	return m.view == viewArchived && len(m.markedRefs()) > 0
}

// whenBulkPurgeAll is true in the Archived view when the list has rows.
func whenBulkPurgeAll(m *Model) bool {
	return m.view == viewArchived && m.focus == focusList && len(m.allRefs()) > 0
}

// allRefs lists every object row of the list, in order.
func (m *Model) allRefs() []repo.Ref {
	var out []repo.Ref
	for _, r := range m.rows {
		if ref, ok := m.rowRef(r); ok {
			out = append(out, ref)
		}
	}
	return out
}

// askPurge is d in the Archived view: purge the selected objects, or the one under the cursor.
func (m *Model) askPurge() tea.Cmd {
	refs := m.bulkRefs()
	switch len(refs) {
	case 0:
		return nil
	case 1:
		name := m.archivedName(refs[0])
		return m.askTyped("delete forever", name, func() { m.purgeNow(refs) })
	}
	return m.askPurgeRefs(refs, "selected")
}

// askPurgeMany is the palette form: the selected rows, or all remaining rows of the type filter.
func (m *Model) askPurgeMany(all bool) tea.Cmd {
	refs, what := m.markedRefs(), "selected"
	if all {
		refs, what = m.allRefs(), "remaining"
	}
	if len(refs) == 0 {
		m.setStatus("nothing archived to purge in this list", true)
		return nil
	}
	return m.askPurgeRefs(refs, what)
}

func (m *Model) askPurgeRefs(refs []repo.Ref, what string) tea.Cmd {
	return m.askTyped(fmt.Sprintf("delete %s %s forever", nouns(refKinds(refs), len(refs)), what), bulkWord("purge", len(refs)), func() { m.purgeNow(refs) })
}

// purgeNow deletes the archived objects for good, one by one with the tombstones the repo writes.
func (m *Model) purgeNow(refs []repo.Ref) {
	purged, failed := 0, 0
	var first error
	var kinds []string
	single := ""
	if len(refs) == 1 {
		single = m.archivedName(refs[0])
	}
	for _, ref := range refs {
		if err := repo.PurgeObject(m.ctx, m.db, ref.Kind, ref.ID); err != nil {
			failed++
			if first == nil {
				first = err
			}
			continue
		}
		purged++
		kinds = append(kinds, ref.Kind)
	}
	m.marked = map[int64]bool{}
	m.afterWrite()
	if failed > 0 {
		m.setStatus(fmt.Sprintf("deleted %d of %d forever, %d failed: %v", purged, len(refs), failed, first), true)
		return
	}
	if single != "" {
		m.setStatus("deleted "+single+" forever", false)
		return
	}
	m.setStatus(fmt.Sprintf("deleted %s forever", nouns(kinds, purged)), false)
}

// --- mark (x) ---

// completeTasks marks the tasks done, or open again when they all are done already.
func (m *Model) completeTasks(ids []int64) {
	allDone := true
	for _, id := range ids {
		if t, err := m.tr.FindByID(m.ctx, id); err == nil && !t.Completed {
			allDone = false
		}
	}
	m.beginGroup()
	ok := 0
	var first error
	for _, id := range ids {
		var err error
		if allDone {
			err = m.hReopenTask(m.ctx, id)
		} else if t, ferr := m.tr.FindByID(m.ctx, id); ferr == nil && !t.Completed {
			_, err = m.hCompleteTask(m.ctx, id)
		} else {
			continue
		}
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		ok++
	}
	verb := "done"
	if allDone {
		verb = "open again"
	}
	m.endGroup(fmt.Sprintf("%d tasks %s", ok, verb))
	m.marked = map[int64]bool{}
	m.afterWrite()
	if first != nil {
		m.setStatus(first.Error(), true)
		return
	}
	m.setStatus(fmt.Sprintf("%d tasks %s (ctrl+z undoes)", ok, verb), false)
}
