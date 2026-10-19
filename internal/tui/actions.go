package tui

import (
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
)

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// focusMask selects the panes in which an action shows in the footer.
type focusMask int

const (
	inList focusMask = 1 << iota
	inDetail
	inBoth = inList | inDetail
)

// action is the single source of truth for a TUI command. Key handling, the footer, the help
// screen and the command palette are all generated from the registry built by buildActions.
type action struct {
	id    string
	title string
	group string
	hint  string   // key label shown to the user
	keys  []string // key strings that trigger it (tea.KeyMsg.String)
	desc  string
	write bool // changes data (or needs the local machine): blocked by --read-only

	// inCompare lets the action run while the merge compare screen is open. All other actions
	// are disabled there so a stray key cannot change the people behind the screen.
	inCompare bool

	// foot orders the action in the footer (0 = not in the footer). footIn limits the panes.
	foot   int
	footIn focusMask
	footIf func(m *Model) bool

	// when says if the action applies in the current context. nil means always.
	when func(m *Model) bool
	// keyIf limits the keys (not the palette entry) to a context, for an action that works from any view.
	keyIf func(m *Model) bool
	// module is the module a view action belongs to (modules.go). An off module hides the action
	// from the palette. isView marks the action that opens the view and takes the module's number key.
	module string
	isView bool
	// keyFor limits one key at a time, for a verb whose keys mean different things in different views.
	keyFor func(m *Model, key string) bool
	// footLabel is the word in the footer. It falls back to footShort[id].
	footLabel string
	run   func(m *Model) tea.Cmd
}

func (a *action) hasKey(k string) bool {
	for _, x := range a.keys {
		if x == k {
			return true
		}
	}
	return false
}

func (a *action) enabled(m *Model) bool {
	if m.cmp != nil && !a.inCompare {
		return false
	}
	return a.when == nil || a.when(m)
}

func (m *Model) focusBit() focusMask {
	if m.focus == focusDetail {
		return inDetail
	}
	return inList
}

// Context predicates.
func whenList(m *Model) bool   { return m.focus == focusList }
func whenDetail(m *Model) bool { return m.focus == focusDetail && len(m.form) > 0 }
func whenPerson(m *Model) bool { return m.curPerson() != 0 && m.view != viewArchived }
func whenMarks(m *Model) bool  { return m.focus == focusList && len(m.markedIDs()) > 0 }
func whenAdd(m *Model) bool    { return whenDetail(m) && addable(m.form[m.fcur].kind) }
func whenObjectPage(m *Model) bool {
	return m.onObjectPage() && (m.selOrg() != 0 || m.selDeal() != 0)
}
func whenLinkedRow(m *Model) bool {
	return whenDetail(m) && m.form[m.fcur].kind == rkLinked && !m.form[m.fcur].empty
}

func whenMarkable(m *Model) bool {
	r, ok := m.selected()
	return m.focus == focusList && ok && r.personID != 0
}

// buildActions returns the full action registry in key-resolution order.
func buildActions() []action {
	viewAction := func(v viewKind) action {
		id := viewActionID(v)
		desc := "show " + strings.ToLower(viewNames[v])
		if v == viewDue {
			desc += "; people due to reach out"
		}
		mod, _ := moduleOf(v)
		a := action{
			id: id, title: viewNames[v] + " view", group: "Views", module: mod, isView: true, desc: desc,
			run: func(m *Model) tea.Cmd { m.switchView(v); return nil },
		}
		if pos := modules.Default().Position(mod); pos >= 1 && pos <= 9 { // the number key of the default arrangement
			a.hint, a.keys = strconv.Itoa(pos), []string{strconv.Itoa(pos)}
		}
		return a
	}
	addKind := func(id, title, what string, k rowKind) action {
		return action{
			id: id, title: title, group: "Edit pane", desc: "add " + what + " to the selected person",
			write: true, when: whenPerson,
			run: func(m *Model) tea.Cmd { return m.addRow(k) },
		}
	}

	acts := []action{
		// Merge compare screen. These come first so their keys win while the screen is open.
		{id: "cmp-left", title: "Use candidate 1", group: "Compare", hint: "←/→", keys: []string{"left", "h", "1"}, desc: "compare: the field takes the left person's value (a list item goes in). Also h and 1",
			foot: 6, footIn: inBoth, inCompare: true, when: whenCmpEdit, run: func(m *Model) tea.Cmd { m.cmpTake(0); return nil }},
		{id: "cmp-right", title: "Use candidate 2", group: "Compare", hint: "→", keys: []string{"right", "l", "2"}, desc: "compare: the field takes the right person's value (a list item goes in). Also l and 2",
			inCompare: true, when: whenCmpEdit, run: func(m *Model) tea.Cmd { m.cmpTake(1); return nil }},
		{id: "cmp-pick", title: "Toggle value or item", group: "Compare", hint: "space", keys: []string{"tab", " "}, desc: "compare: switch a field to the other person's value, or put a list item in or out of the result (tab or space)",
			foot: 7, footIn: inBoth, inCompare: true, when: whenCmpEdit, run: func(m *Model) tea.Cmd { m.cmp.toggle(); m.cmpPreview(); return nil }},
		{id: "cmp-pick-back", title: "Toggle value or item (back)", group: "Compare", hint: "shift+tab", keys: []string{"shift+tab"}, desc: "compare: same as space",
			inCompare: true, when: whenCmpEdit, run: func(m *Model) tea.Cmd { m.cmp.toggle(); m.cmpPreview(); return nil }},
		{id: "cmp-survivor", title: "Change survivor", group: "Compare", hint: "s", keys: []string{"s"}, desc: "compare: keep the other person's record and id as the survivor",
			foot: 8, footIn: inBoth, inCompare: true, when: whenCmpEdit, run: func(m *Model) tea.Cmd { m.cmp.nextSurvivor(); m.cmpPreview(); return nil }},
		{id: "cmp-pair", title: "Change pair", group: "Compare", hint: "p", keys: []string{"p"}, desc: "compare: groups of three or more merge two people at a time, p moves to the next pair",
			foot: 9, footIn: inBoth, footIf: whenCmpGroup, inCompare: true, when: whenCmpEdit, run: func(m *Model) tea.Cmd { m.cmp.nextPair(); m.cmpPreview(); return nil }},
		{id: "cmp-view", title: "Switch pane", group: "Compare", hint: "r", keys: []string{"r"}, desc: "compare: narrow terminals show fewer panes, r switches between candidates and the result",
			foot: 5, footIn: inBoth, footIf: whenCmpNarrow, inCompare: true, when: whenCmpEdit, run: func(m *Model) tea.Cmd { m.cmp.nextPage(); return nil }},
		{id: "cmp-confirm", title: "Merge (with summary)", group: "Compare", hint: "enter", keys: []string{"enter", "y"}, desc: "compare: enter shows what will be archived and moved, enter again merges",
			write: true, foot: 11, footIn: inBoth, inCompare: true, when: whenCompare, run: func(m *Model) tea.Cmd {
				if m.cmp.confirm {
					m.applyMerge()
					return nil
				}
				m.cmpPreview()
				m.cmp.confirm = true
				return nil
			}},
		{id: "cmp-apply", title: "Apply merge", group: "Compare", hint: "m", keys: []string{"m"}, desc: "compare: merge now without the summary, the dropped person is archived and can be restored",
			write: true, inCompare: true, when: whenCompare, run: func(m *Model) tea.Cmd { m.applyMerge(); return nil }},
		{id: "cmp-back", title: "Close compare", group: "Compare", hint: "esc", keys: []string{"esc", "q"}, desc: "compare: leave without merging (also q), from the summary go back to the fields",
			foot: 12, footIn: inBoth, inCompare: true, when: whenCompare, run: func(m *Model) tea.Cmd {
				if m.cmp.confirm {
					m.cmp.confirm = false
					return nil
				}
				m.cmp = nil
				return nil
			}},

		// Contact actions. The verbs of every object (Add, Edit, Delete, Restore, Link, Unlink) are generated
		// from the registry in objects.go and inserted after them.
		{id: "contacted", title: "Mark contacted", group: "Contact", hint: "c", keys: []string{"c", "e"}, desc: "mark contacted now (resets the keep-in-touch clock)",
			write: true, foot: 30, footIn: inBoth, when: whenPerson, run: func(m *Model) tea.Cmd { m.markContacted(); return nil }},
		{id: "note", title: "Add note", group: "Contact", hint: "n", keys: []string{"n"}, desc: "add a note of the last used type (tab in the prompt changes the type)",
			write: true, foot: 40, footIn: inBoth, when: whenPerson, run: func(m *Model) tea.Cmd { return m.startNotePrompt() }},
		{id: "note-object", title: "Add note to organization or deal", group: "Contact", hint: "n", keys: []string{"n"}, desc: "add a note attached to the organization or deal on screen (same as crm log add --org / --deal)",
			write: true, foot: 40, footIn: inBoth, when: whenObjectPage, run: func(m *Model) tea.Cmd { m.startObjectNote(); return m.takePending() }},
		{id: "link-edit", title: "Edit link", group: "Link", hint: "E", keys: []string{"E"}, desc: "edit the role, dates, primary flag, kind or notes of the linked row under the cursor (detail pane). ctrl+z undoes",
			write: true, foot: 13, footIn: inDetail, footLabel: "edit link", footIf: whenLinkedRow, when: whenLinkedRow,
			run: func(m *Model) tea.Cmd { r, _ := m.rowFocused(); return m.editLinkRow(r) }},
		{id: "note-pick", title: "Add note (pick type)", group: "Contact", hint: "N", keys: []string{"N"}, desc: "pick a note type from a list, then add the note",
			write: true, when: whenPerson, run: func(m *Model) tea.Cmd { m.pickNoteType(); return nil }},
		{id: "note-types", title: "Manage note types", group: "Contact", desc: "add or archive note types (call, email, meeting, ...)",
			write: true, run: func(m *Model) tea.Cmd { return m.openNoteTypes() }},
		{id: "note-retype", title: "Change note type", group: "Edit pane", hint: "t", keys: []string{"t"}, desc: "pick a new type for the note under the cursor (detail pane)",
			write: true, foot: 16, footIn: inDetail, footIf: whenNoteRow, when: whenNoteRow, run: func(m *Model) tea.Cmd { m.retypeNote(); return nil }},
		{id: "note-restore", title: "Restore last deleted note", group: "Edit pane", desc: "bring back the note deleted last in this session",
			write: true, when: whenDeletedNote, run: func(m *Model) tea.Cmd { m.restoreLastNote(); return nil }},
		{id: "snooze", title: "Snooze keep-in-touch", group: "Contact", hint: "s", keys: []string{"s"}, desc: "snooze keep-in-touch for N days",
			write: true, when: whenPerson, run: func(m *Model) tea.Cmd {
				m.startPrompt("snooze", "snooze days: ", "7")
				return m.input.Focus()
			}},
		{id: "favorite", title: "Toggle favorite", group: "Contact", hint: "f", keys: []string{"f"}, desc: "toggle favorite (all marked people if any)",
			write: true, when: whenPerson, run: func(m *Model) tea.Cmd { m.toggleFavorite(); return nil }},
		{id: "task-tag", title: "Tag task", group: "Contact", hint: "T", keys: []string{"T"}, desc: "add a tag to the selected task (prefix with - to remove). In the Tasks view, or on a task row in the detail pane",
			write: true, when: whenTaskTarget, run: func(m *Model) tea.Cmd {
				m.startInput("tasktag", "tag task (-name removes): ", "")
				return m.input.Focus()
			}},
		{id: "tag", title: "Tag", group: "Contact", hint: "T", keys: []string{"T"}, desc: "add a tag (prefix with - to remove) on the person or all marked",
			write: true, foot: 31, footIn: inList, footIf: whenMarks, when: whenPerson, run: func(m *Model) tea.Cmd {
				m.startPrompt("tag", "tag (-name removes): ", "")
				return m.input.Focus()
			}},
		{id: "tag-delete", title: "Delete tag", group: "Contact", desc: "delete a tag from every record that has it (asks y/n, ctrl+z undoes; same as crm tag delete)",
			write: true, run: func(m *Model) tea.Cmd { m.openTagDelete(); return nil }},
		{id: "cadence", title: "Set keep-in-touch", group: "Contact", hint: "F", keys: []string{"F"}, desc: "pick a keep-in-touch cadence for the person or all marked",
			write: true, foot: 32, footIn: inList, footIf: whenMarks, when: whenPerson, run: func(m *Model) tea.Cmd { m.pickCadence(m.targets()); return nil }},
		{id: "tier", title: "Set priority tier", group: "Contact", hint: "p", keys: []string{"p"}, desc: "pick a priority tier for the person or all marked",
			write: true, foot: 33, footIn: inList, footIf: whenMarks, when: whenPerson, run: func(m *Model) tea.Cmd { m.pickTier(m.targets()); return nil }},
		{id: "done", title: "Complete task", group: "Contact", hint: "x", keys: []string{"x"}, desc: "complete the selected task (Tasks view, or a task row)",
			write: true, run: func(m *Model) tea.Cmd { m.completeTask(); return nil }},

		// Home.
		{id: "home-open", title: "Open", group: "Home", hint: "enter", keys: []string{"enter"}, desc: "open the person (People view, detail pane) or the view the row stands for",
			foot: 20, footIn: inList, when: whenHomeRow, run: func(m *Model) tea.Cmd { return m.openHomeRow() }},

		// Sync view: sources and inbox.
		{id: "sync-now", title: "Sync now", group: "Sync", hint: "s", keys: []string{"s"}, desc: "run the selected source in the background (also: Sync now: <source> in the palette)",
			write: true, foot: 20, footIn: inList, when: whenSource, run: func(m *Model) tea.Cmd { return m.startSync(m.selectedSource(), false) }},
		{id: "sync-dry", title: "Dry-run sync", group: "Sync", hint: "d", keys: []string{"d"}, desc: "show what the selected source would change, writes nothing (also: Dry-run sync: <source>)",
			foot: 21, footIn: inList, when: whenSource, run: func(m *Model) tea.Cmd { return m.startSync(m.selectedSource(), true) }},
		{id: "inbox-accept", title: "Accept inbox record", group: "Sync", hint: "a", keys: []string{"a"}, desc: "create a person from the selected inbox record",
			write: true, foot: 22, footIn: inList, when: whenInboxRow, run: func(m *Model) tea.Cmd { m.acceptInbox(); return nil }},
		{id: "inbox-attach", title: "Attach inbox record", group: "Sync", hint: "p", keys: []string{"p"}, desc: "attach the selected inbox record to a person you search for",
			write: true, foot: 23, footIn: inList, when: whenInboxRow, run: func(m *Model) tea.Cmd { m.attachInbox(); return nil }},
		{id: "inbox-ignore", title: "Ignore inbox record", group: "Sync", hint: "i", keys: []string{"i"}, desc: "dismiss the selected inbox record",
			write: true, foot: 24, footIn: inList, when: whenInboxRow, run: func(m *Model) tea.Cmd { m.ignoreInbox(); return nil }},
		{id: "open-inbox", title: "Open inbox", group: "Sync", desc: "show the Sync view with the cursor on the first pending inbox record",
			run: func(m *Model) tea.Cmd { m.openInbox(); return nil }},

		// Merge view and manual merge.
		{id: "find-dupes", title: "Find duplicates", group: "Merge", desc: "show groups of probable duplicate people (Merge view)", module: "merge",
			run: func(m *Model) tea.Cmd { m.switchView(viewMerge); return nil }},
		{id: "compare", title: "Compare group", group: "Merge", hint: "enter", keys: []string{"enter"}, desc: "open the side by side compare for the selected group",
			foot: 20, footIn: inList, when: whenMergeRows, run: func(m *Model) tea.Cmd { m.openGroupCompare(); return nil }},
		{id: "dupe-level", title: "Duplicate level", group: "Merge", hint: "m", keys: []string{"m"}, desc: "cycle the lowest confidence shown: high, medium, low, fuzzy",
			foot: 21, footIn: inList, when: whenMergeList, run: func(m *Model) tea.Cmd { m.cycleLevel(); return nil }},
		{id: "dismiss", title: "Dismiss group", group: "Merge", hint: "D", keys: []string{"D"}, desc: "mark the group as not duplicates (u undoes)",
			write: true, foot: 22, footIn: inBoth, inCompare: true, when: whenDismiss, run: func(m *Model) tea.Cmd { m.dismissGroup(); return nil }},
		{id: "undo-dismiss", title: "Undo dismiss", group: "Merge", hint: "u", keys: []string{"u"}, desc: "bring back the group dismissed last in this session",
			write: true, foot: 23, footIn: inList, footIf: func(m *Model) bool { return len(m.dismissed) > 0 }, when: whenMergeList, run: func(m *Model) tea.Cmd { m.undoDismiss(); return nil }},
		{id: "merge-marked", title: "Merge marked", group: "Merge", hint: "M", keys: []string{"M"}, desc: "compare and merge the marked people (mark with space)",
			write: true, foot: 34, footIn: inList, footIf: func(m *Model) bool { return len(m.markedIDs()) >= 2 }, when: whenMarks, run: func(m *Model) tea.Cmd { m.openMarkedCompare(); return nil }},

		// Archive.
		{id: "purge", title: "Purge forever", group: "Archive", hint: "X", keys: []string{"X"}, desc: "permanently delete the archived person after typing the name",
			write: true, foot: 21, footIn: inList, when: whenArchivedPerson, run: func(m *Model) tea.Cmd { return m.askPurge() }},
		{id: "archived-type", title: "Filter archived by type", group: "Archive", hint: "T", keys: []string{"T"}, desc: "show only archived people, organizations, deals or tasks (Archived view). u restores the one under the cursor",
			foot: 22, footIn: inList, footLabel: "type", when: func(m *Model) bool { return m.view == viewArchived && m.focus == focusList },
			run: func(m *Model) tea.Cmd { m.pickArchivedType(); return nil }},

		// Detail pane (form).
		{id: "edit", title: "Edit row", group: "Edit pane", hint: "enter", keys: []string{"enter"}, desc: "edit the selected row inline (enter saves, esc cancels)",
			write: true, foot: 10, footIn: inDetail, when: whenDetail, run: func(m *Model) tea.Cmd { return m.startEdit() }},
		{id: "add", title: "Add row", group: "Edit pane", hint: "a", keys: []string{"a"}, desc: "add a row of the kind under the cursor (email, phone, link, tag, task, note)",
			write: true, foot: 11, footIn: inDetail, when: whenAdd, run: func(m *Model) tea.Cmd { return m.addRow(m.form[m.fcur].kind) }},
		{id: "delete", title: "Delete row", group: "Edit pane", hint: "d", keys: []string{"d"}, desc: "delete the selected row (asks y/n). Fields are cleared",
			write: true, foot: 12, footIn: inDetail, when: whenDetail, run: func(m *Model) tea.Cmd { m.askDelete(); return nil }},
		{id: "copy", title: "Copy value", group: "Edit pane", hint: "y", keys: []string{"y"}, desc: "copy the row value to the clipboard (OSC52, works over SSH)",
			foot: 13, footIn: inDetail, when: whenDetail, run: func(m *Model) tea.Cmd { m.copyRow(); return nil }},
		{id: "open", title: "Open link", group: "Edit pane", hint: "o", keys: []string{"o"}, desc: "open the row as a URL, mailto: or tel: on this machine",
			write: true, foot: 14, footIn: inDetail, when: whenDetail, run: func(m *Model) tea.Cmd { m.openRow(); return nil }},
		addKind("add-email", "Add email", "an email", rkEmail),
		addKind("add-phone", "Add phone", "a phone number", rkPhone),
		addKind("add-handle", "Add link", "a social or website link", rkHandle),
		{id: "focus-detail", title: "Focus detail pane", group: "Edit pane", hint: "tab", keys: []string{"tab", "shift+tab", "right", "l"}, desc: "move into the detail form (also right, l)",
			foot: 20, footIn: inList, when: func(m *Model) bool { return whenList(m) && len(m.rows) > 0 && len(m.form) > 0 },
			run: func(m *Model) tea.Cmd { m.focus = focusDetail; m.syncDetail(); return nil }},
		{id: "back", title: "Back to list", group: "Edit pane", hint: "esc", keys: []string{"esc", "tab", "shift+tab", "left", "h"}, desc: "return to the list (also left, h, tab)",
			foot: 15, footIn: inDetail, when: func(m *Model) bool { return m.focus == focusDetail },
			run: func(m *Model) tea.Cmd { m.focus = focusList; m.syncDetail(); return nil }},

		// Bulk.
		{id: "mark", title: "Mark row", group: "Bulk", hint: "space", keys: []string{" "}, desc: "mark or unmark the row, then T / F / p / f / c apply to all marked",
			foot: 25, footIn: inList, when: whenMarkable, run: func(m *Model) tea.Cmd { m.toggleMark(); return nil }},
		{id: "mark-all", title: "Mark all visible", group: "Bulk", hint: "*", keys: []string{"*"}, desc: "mark every visible person (again to clear)",
			when: whenList, run: func(m *Model) tea.Cmd { m.markAll(); return nil }},

		// Movement.
		{id: "down", inCompare: true, title: "Move down", group: "Move", hint: "j", keys: []string{"j", "down", "ctrl+n"}, desc: "next row (also down arrow)",
			foot: 5, footIn: inBoth, run: func(m *Model) tea.Cmd { m.step(1); return nil }},
		{id: "up", inCompare: true, title: "Move up", group: "Move", hint: "k", keys: []string{"k", "up", "ctrl+p"}, desc: "previous row (also up arrow)",
			run: func(m *Model) tea.Cmd { m.step(-1); return nil }},
		{id: "half-down", inCompare: true, title: "Half page down", group: "Move", hint: "ctrl+d", keys: []string{"ctrl+d", "pgdown"}, desc: "half page down",
			run: func(m *Model) tea.Cmd { m.step(m.bodyHeight() / 2); return nil }},
		{id: "half-up", inCompare: true, title: "Half page up", group: "Move", hint: "ctrl+u", keys: []string{"ctrl+u", "pgup"}, desc: "half page up",
			run: func(m *Model) tea.Cmd { m.step(-m.bodyHeight() / 2); return nil }},
		{id: "top", inCompare: true, title: "Go to top", group: "Move", hint: "g", keys: []string{"g", "home"}, desc: "first row",
			run: func(m *Model) tea.Cmd { m.step(-1 << 20); return nil }},
		{id: "bottom", inCompare: true, title: "Go to bottom", group: "Move", hint: "G", keys: []string{"G", "end"}, desc: "last row",
			run: func(m *Model) tea.Cmd { m.step(1 << 20); return nil }},
		{id: "scroll-down", title: "Scroll detail down", group: "Move", hint: "J", keys: []string{"J"}, desc: "scroll the detail pane down",
			run: func(m *Model) tea.Cmd { m.detail.LineDown(3); return nil }},
		{id: "scroll-up", title: "Scroll detail up", group: "Move", hint: "K", keys: []string{"K"}, desc: "scroll the detail pane up",
			run: func(m *Model) tea.Cmd { m.detail.LineUp(3); return nil }},

		// Views and search.
		viewAction(viewHome), viewAction(viewPeople), viewAction(viewDue), viewAction(viewTasks), viewAction(viewOrgs), viewAction(viewBirthdays), viewAction(viewMerge), viewAction(viewArchived), viewAction(viewSync), viewAction(viewDeals),
		{id: "goto-view", title: "Go to view", group: "Views", hint: "v", keys: []string{"v"}, desc: "list the views that are on with their number key (1-9) and jump to one. Turn views on or off with Modules",
			foot: 85, footIn: inList, run: func(m *Model) tea.Cmd { return m.openViewSwitcher() }},
		{id: "next-view", title: "Next view", group: "Views", hint: "]", keys: []string{"]"}, desc: "next view",
			run: func(m *Model) tea.Cmd { m.switchView(m.nextView(m.view, 1)); return nil }},
		{id: "prev-view", title: "Previous view", group: "Views", hint: "[", keys: []string{"["}, desc: "previous view",
			run: func(m *Model) tea.Cmd { m.switchView(m.nextView(m.view, -1)); return nil }},
		{id: "clear-search", title: "Clear search", group: "Views", hint: "esc", keys: []string{"esc"}, desc: "clear the People filter set from the bar (a tag, an organization or Show all)",
			when: func(m *Model) bool { return m.focus == focusList && m.query != "" },
			run: func(m *Model) tea.Cmd {
				m.query = ""
				m.cursor, m.offset = 0, 0
				m.report(m.rebuild(false))
				return nil
			}},
		{id: "sort", title: "Toggle sort", group: "Views", hint: "o", keys: []string{"o"}, desc: "toggle sort: name / recently updated",
			when: func(m *Model) bool { return m.focus == focusList && m.view == viewPeople },
			run: func(m *Model) tea.Cmd {
				m.sortRecent = !m.sortRecent
				m.cursor, m.offset = 0, 0
				m.report(m.rebuild(false))
				if m.sortRecent {
					m.setStatus("sorted by recently updated", false)
				} else {
					m.setStatus("sorted by name", false)
				}
				return nil
			}},
		{id: "reload", title: "Reload", group: "Views", hint: "r", keys: []string{"r"}, desc: "reload from the database",
			run: func(m *Model) tea.Cmd {
				m.report(m.reload())
				m.setStatus("reloaded", false)
				return nil
			}},

		// App.
		{id: "palette", inCompare: true, title: "Command palette", group: "App", hint: "ctrl+k", keys: []string{"ctrl+k", ":", "/"}, desc: "the one bar: type to find a person, org, deal, task, note, action or setting (also : and /). > actions, @ people, # tags, = settings, ? help. A command line works too: n Sam called #deal remind fri",
			foot: 1, footIn: inBoth, run: func(m *Model) tea.Cmd { return m.openPalette() }},
		{id: "undo", title: "Undo", group: "App", hint: "ctrl+z", keys: []string{"ctrl+z"}, desc: "undo the last data change of this session. A toast says what was undone",
			write: true, run: func(m *Model) tea.Cmd { return m.undoLast() }},
		{id: "redo", title: "Redo", group: "App", hint: "ctrl+y", keys: []string{"ctrl+y"}, desc: "redo the change that was just undone",
			write: true, run: func(m *Model) tea.Cmd { return m.redoLast() }},
		{id: "keys-customize", title: "Keys: customize", group: "App", desc: "rebind an action: pick it, press the new key, confirm. Saved in config.toml [keys]; same as crm keys set",
			write: true, run: func(m *Model) tea.Cmd { m.openKeysCustomize(); return nil }},
		{id: "keys-reset", title: "Keys: reset", group: "App", desc: "put one or all custom keys back to the defaults; same as crm keys reset",
			write: true, run: func(m *Model) tea.Cmd { m.openKeysReset(); return m.takePending() }},
		{id: "syntax-toggle", title: "Toggle short syntax", group: "App", desc: "turn #tag @Name !high ~30m by fri every 3m on or off in the command bar and the note and task prompts",
			run: func(m *Model) tea.Cmd { m.toggleSyntax(); return nil }},
		{id: "help", inCompare: true, title: "Help", group: "App", hint: "?", keys: []string{"?"}, desc: "show every key",
			foot: 80, footIn: inBoth, run: func(m *Model) tea.Cmd { m.openHelp(); return nil }},
		{id: "help-topics", title: "Help: topics", group: "App", desc: "list the built-in help topics, filter and read one (same as crm help)",
			run: func(m *Model) tea.Cmd { m.openHelpTopics(""); return nil }},
		{id: "help-search", title: "Help: search", group: "App", desc: "find a word across all help topics (same as crm help --search)",
			run: func(m *Model) tea.Cmd {
				m.startInput("help-search", "search help: ", "")
				return m.input.Focus()
			}},
		{id: "doctor", title: "Doctor", group: "App", desc: "check the installation and database, with a fix for each problem (same as crm doctor)",
			run: func(m *Model) tea.Cmd { m.showDoctor(); return nil }},
		{id: "where", title: "Where", group: "App", desc: "show the active database: path, domain, schema version, device id, size (same as crm where)",
			run: func(m *Model) tea.Cmd { m.showWhere(); return nil }},
		{id: "domains", title: "Domains", group: "App", desc: "list the databases in the data directory (same as crm domains)",
			run: func(m *Model) tea.Cmd { m.showDomains(); return nil }},
		{id: "dav-info", title: "DAV: info", group: "App", desc: "phone sync connection details and setup steps, read-only (same as crm dav info)",
			run: func(m *Model) tea.Cmd { m.showDavInfo(); return nil }},
		{id: "dav-conflicts", title: "DAV: conflicts", group: "App", desc: "fields where the phone and the CRM both changed, read-only (same as crm dav conflicts)",
			run: func(m *Model) tea.Cmd { m.showDavConflicts(); return nil }},
		{id: "dav-status", title: "DAV: status", group: "App", desc: "contacts exposed, last device activity and conflict counts (same as crm dav status)",
			run: func(m *Model) tea.Cmd { m.showDavStatus(); return nil }},
		{id: "theme-choose", title: "Theme: choose", group: "App", desc: "pick a color theme, previewed live as you move, remembered on this device (same as crm theme set)",
			run: func(m *Model) tea.Cmd { m.openThemeChooser(); return nil }},
		{id: "theme-reload", title: "Theme: reload", group: "App", desc: "re-read ~/.crm/themes/*.toml and apply the current theme again",
			run: func(m *Model) tea.Cmd { m.reloadTheme(); return nil }},
		{id: "quit", inCompare: true, title: "Quit", group: "App", hint: "q", keys: []string{"q", "ctrl+c"}, desc: "quit",
			run: func(m *Model) tea.Cmd { return tea.Quit }},
	}
	// The generated verbs come after the contact actions. Each key is limited to the views where it
	// means that object (keyFor), so the order does not decide what a key does.
	at := len(acts)
	for i := range acts {
		if acts[i].id == "home-open" {
			at = i
			break
		}
	}
	out := append(convertActions(), acts[:at]...)
	out = append(out, objectActions()...)
	out = append(out, acts[at:]...)
	out = append(out, orgMergeActions()...)
	return append(out, moduleActions()...)
}

// step moves the cursor in the focused pane, or the row cursor of the compare screen.
func (m *Model) step(delta int) {
	if m.cmp != nil {
		m.cmp.move(delta)
		return
	}
	if m.focus == focusDetail {
		m.fcur = clampInt(m.fcur+delta, 0, len(m.form)-1)
		m.syncDetail()
		return
	}
	m.move(delta)
}

func clampInt(v, lo, hi int) int {
	if v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v
}

func parseDays(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("not a number of days")
	}
	return n, nil
}

// --- person actions ---

func (m *Model) markContacted() {
	ids := m.targets()
	when := m.now().UTC().Format("2006-01-02 15:04:05")
	empty := ""
	m.bulk(ids, "marked as contacted", "contacted", func(id int64) error {
		_, err := m.hUpdatePerson(m.ctx, id, model.UpdatePersonInput{LastContactedAt: &when, SnoozedUntil: &empty})
		return err
	})
}

func (m *Model) isFavorite(id int64) bool {
	for _, p := range m.people {
		if p.ID == id {
			return p.IsFavorite
		}
	}
	return false
}

func (m *Model) toggleFavorite() {
	ids := m.targets()
	// Favorite everyone unless they all already are.
	all := true
	for _, id := range ids {
		if !m.isFavorite(id) {
			all = false
		}
	}
	fav := !all
	msg := "favorited"
	if !fav {
		msg = "unfavorited"
	}
	m.bulk(ids, msg, msg, func(id int64) error {
		_, err := m.hUpdatePerson(m.ctx, id, model.UpdatePersonInput{IsFavorite: &fav})
		return err
	})
}

func (m *Model) completeTask() {
	var id int64
	if m.focus == focusDetail && m.fcur < len(m.form) && m.form[m.fcur].kind == rkTask && !m.form[m.fcur].empty {
		id = m.form[m.fcur].id
	} else if r, ok := m.selected(); ok {
		id = r.taskID
	}
	if id == 0 {
		m.setStatus("select a task in the Tasks view (4) or a task row", true)
		return
	}
	task, err := m.hCompleteTask(m.ctx, id)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	msg := "task done"
	if task.NextTaskID != nil {
		msg += ", next occurrence created"
	}
	m.setStatus(msg, false)
	m.afterWrite()
}

func (m *Model) toggleMark() {
	r, ok := m.selected()
	if !ok || r.personID == 0 {
		return
	}
	if m.marked[r.personID] {
		delete(m.marked, r.personID)
	} else {
		m.marked[r.personID] = true
	}
	m.move(1)
}

func (m *Model) markAll() {
	ids := map[int64]bool{}
	for _, r := range m.rows {
		if r.personID != 0 {
			ids[r.personID] = true
		}
	}
	if len(ids) > 0 && len(m.markedIDs()) == len(ids) {
		m.marked = map[int64]bool{}
		return
	}
	m.marked = ids
}

func (m *Model) pickCadence(ids []int64) {
	items := []overlayItem{{label: "none", desc: "clear the cadence", value: ""}}
	for _, c := range model.KeepInTouchCadences {
		items = append(items, overlayItem{label: c, value: c})
	}
	m.openPicker("Keep in touch", items, func(m *Model, it overlayItem) {
		v := it.value
		m.bulk(ids, "keep in touch set", "keep in touch set", func(id int64) error {
			_, err := m.hUpdatePerson(m.ctx, id, model.UpdatePersonInput{KeepInTouch: &v})
			return err
		})
	})
}

func (m *Model) pickTier(ids []int64) {
	items := make([]overlayItem, len(tierNames))
	for i, n := range tierNames {
		items[i] = overlayItem{label: n, value: strconv.Itoa(i)}
		if i == 0 {
			items[i].desc = "clear the tier"
		}
	}
	m.openPicker("Priority tier", items, func(m *Model, it overlayItem) {
		n, _ := strconv.Atoi(it.value)
		m.bulk(ids, "priority set", "priority set", func(id int64) error {
			_, err := m.hUpdatePerson(m.ctx, id, model.UpdatePersonInput{PriorityTier: &n})
			return err
		})
	})
}

// --- clipboard and links ---

// osc52Copy asks the terminal to put s on the clipboard. It works through SSH and the web terminal.
func osc52Copy(w io.Writer, s string) {
	if len(s) > 100000 {
		return
	}
	_, _ = fmt.Fprintf(w, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(s)))
}

func openURL(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// newInput is a helper for overlays.
func newInput(prompt string) textinput.Model {
	in := textinput.New()
	in.Prompt = prompt
	in.CharLimit = 200
	return in
}
