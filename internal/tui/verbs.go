package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// The universal verbs that need one action for the whole framework (patterns/universal-verbs).
//
// The per-object verbs (Delete organization, Restore deal, Link, Unlink, ...) are generated from the
// object registry in objects.go and already share one key per verb. The verbs below differ by
// context, so one action with one key finds the right handler for the highlighted thing:
//
//	activate (enter, e)  open the highlighted thing further, or edit the field under the cursor
//	mark-state (x)       one meaning per type: a task is done (or open again), a person is contacted
//	merge (m)            merge the space-selected rows, or the highlighted duplicate group
//
// The handlers are the same actions the palette lists (Edit organization, Mark contacted, Complete
// task, Merge marked, ...). They keep their ids and have no key of their own, so [keys] overrides of
// those ids still work.

// activateTargets are the handlers of activate, tried in order. The first one that applies in the
// current context runs.
var activateTargets = []string{"edit", "setting-open", "home-open", "compare", "org-edit", "deal-edit", "task-edit", "person-edit", "inbox-accept", "tag-open", "tag-rename"}

// mergeTargets are the handlers of merge, tried in order.
var mergeTargets = []string{"compare", "org-merge", "merge-marked", "tag-merge-marked"}

// applies says whether the action would run for a key in the current context.
func (m *Model) applies(a *action) bool {
	return a != nil && a.enabled(m) && (a.keyIf == nil || a.keyIf(m)) && (a.keyFor == nil || a.keyFor(m, "enter"))
}

// firstApplying returns the first of the named actions that applies here.
func (m *Model) firstApplying(ids []string) *action {
	for _, id := range ids {
		if a := m.actionByID(id); m.applies(a) {
			return a
		}
	}
	return nil
}

func (m *Model) whenActivate() bool { return m.firstApplying(activateTargets) != nil }

func (m *Model) runActivate() tea.Cmd {
	if a := m.firstApplying(activateTargets); a != nil {
		return m.runAction(a)
	}
	return nil
}

// whenMarkState is true on a task or a person.
func whenMarkState(m *Model) bool { return m.taskTarget() != 0 || whenPerson(m) }

func (m *Model) runMarkState() tea.Cmd {
	if ids := m.bulkOfKind("task"); len(ids) > 1 && m.view == viewTasks {
		m.completeTasks(ids)
		return nil
	}
	if m.taskTarget() != 0 {
		m.completeTask()
		return nil
	}
	if whenPerson(m) {
		m.markContacted()
	}
	return nil
}

func (m *Model) whenMerge() bool {
	return m.barTag == "" && m.firstApplying(mergeTargets) != nil
}

func (m *Model) runMerge() tea.Cmd {
	if a := m.firstApplying(mergeTargets); a != nil {
		if a.id == "merge-marked" && len(m.markedIDs()) < 2 {
			m.setStatus("mark two or more people with space, then m", true)
			return nil
		}
		return m.runAction(a)
	}
	return nil
}

// verbActions are the framework verbs defined in this file plus the text-entry key.
func verbActions() []action {
	return []action{
		{id: "activate", title: "Activate", group: "Verbs", hint: "enter", keys: []string{"enter", "e"}, verbAll: true,
			desc:  "open the highlighted thing further, or edit the field under the cursor (rename is just this). enter and e are the same action",
			write: false, foot: 10, footIn: inBoth, footLabel: "open",
			writeFor: func(m *Model) bool {
				a := m.firstApplying(activateTargets)
				return a != nil && a.write
			},
			footDyn: func(m *Model) string {
				switch {
				case m.barTag != "":
					return "rename"
				case m.focus == focusDetail:
					return "edit"
				case m.view == viewMerge:
					return "compare"
				case m.view == viewTasks, m.view == viewOrgs, m.view == viewDeals:
					return "edit"
				case m.view == viewSync:
					return "accept"
				}
				return "open"
			},
			when: func(m *Model) bool { return m.whenActivate() },
			run:  func(m *Model) tea.Cmd { return m.runActivate() }},
		{id: "mark-state", title: "Mark", group: "Verbs", hint: "x", keys: []string{"x"}, verbAll: true,
			desc:  "mark the highlighted thing with the one meaning its type has: a task is done (again to reopen it), a person is contacted now. Marked rows (space) all get it",
			write: true, foot: 30, footIn: inBoth, footLabel: "mark",
			footDyn: func(m *Model) string {
				if m.taskTarget() != 0 {
					return "done"
				}
				return "contacted"
			},
			when: whenMarkState, run: func(m *Model) tea.Cmd { return m.runMarkState() }},
		{id: "merge", title: "Merge", group: "Verbs", hint: "m", keys: []string{"m"},
			desc:  "merge the space-selected rows (people, organizations) or the highlighted duplicate group (opens the compare). m on a tag row moves it into another tag",
			write: true, foot: 34, footIn: inList, footLabel: "merge",
			footIf: func(m *Model) bool { return len(m.markedIDs()) >= 2 || len(m.markedOrgIDs()) >= 2 || whenMergeRows(m) || whenTagList(m) },
			when:   func(m *Model) bool { return m.whenMerge() },
			run:    func(m *Model) tea.Cmd { return m.runMerge() }},
		{id: "prompt-tab", title: "Prompt: tab", group: "Verbs", hint: "tab", keys: []string{"tab"}, mode: keyText,
			desc: "in a footer prompt: accept the highlighted suggestion of the popup, else change the note type, else complete a role",
			when: func(m *Model) bool { return m.mode == modePrompt },
			run:  func(m *Model) tea.Cmd { return m.promptTab(true) }},
		{id: "prompt-tab-back", title: "Prompt: shift+tab", group: "Verbs", hint: "shift+tab", keys: []string{"shift+tab"}, mode: keyText,
			desc: "in a footer prompt: change the note type back",
			when: func(m *Model) bool { return m.mode == modePrompt },
			run:  func(m *Model) tea.Cmd { return m.promptTab(false) }},
	}
}

// promptTab handles tab and shift+tab in a footer prompt. It returns nil when the key does nothing,
// and the text input then gets it.
func (m *Model) promptTab(forward bool) tea.Cmd {
	switch {
	case m.promptKind == "note" || m.promptKind == "note-object" || m.promptKind == "note-free":
		m.cycleNoteType(forward)
	case m.promptKind == "link-attrs" && forward:
		m.completeRole()
	}
	return nil
}

// hReopenTask opens a done task again and records the step for undo.
func (m *Model) hReopenTask(ctx context.Context, id int64) error {
	title := fmt.Sprintf("task %d", id)
	if t, err := m.tr.FindByID(ctx, id); err == nil {
		title = "task " + truncateLabel(t.Title)
	}
	if err := m.tr.Reopen(ctx, id); err != nil {
		return err
	}
	m.record("reopen "+title,
		func() error { _, err := m.tr.Complete(m.ctx, id); return err },
		func() error { return m.tr.Reopen(m.ctx, id) })
	return nil
}
