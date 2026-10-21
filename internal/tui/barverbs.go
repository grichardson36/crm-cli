package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/mattn/go-runewidth"
)

// Results act like views (pattern bar-results-act-like-views). In the bar, Tab moves focus from the
// input to the results. With the focus on the results a plain registry hotkey works on the
// highlighted object exactly as it does in that object's own view: c marks a person contacted, s
// snoozes, f favorites, n adds a note, t a task, L links, d deletes, x completes a task, e edits an
// organization. The registry decides which verbs apply to which kind, because the key runs in a
// context that looks like the object's view with that object selected. Tab or Esc returns to the
// input. A verb that needs a prompt, a confirm or a picker takes the screen and the bar comes back
// when it is done. Nothing here has its own key table, so [keys] rebinding applies.

// barReserved are keys the bar keeps for itself in the results.
var barReserved = map[string]bool{
	"enter": true, "esc": true, "tab": true, "shift+tab": true, "up": true, "down": true, "left": true, "right": true,
	"pgup": true, "pgdown": true, "ctrl+n": true, "ctrl+p": true, "ctrl+c": true, "ctrl+k": true, "j": true, "k": true,
	":": true, "/": true, "q": true, "?": true, "a": true, "+": true,
}

// barVerbGroups are the registry groups whose actions act on a selected object.
var barVerbGroups = map[string]bool{"Contact": true, "Link": true, "Organizations": true, "Deals": true, "Tasks": true, "Tags": true}

// barNoVerb are actions in those groups that do not act on one selected object.
var barNoVerb = map[string]bool{"org-mark": true, "org-merge": true, "convert-open": true, "new": true, "note-pick": true, "note-types": true}

// barTarget is what a bar row stands for: the kind of object and its id (or tag name).
type barTarget struct {
	kind string // person, org, deal, task, tag
	id   int64
	name string // tag name
}

// barTargetOf reads the object a row stands for. ok is false for rows with no verbs (actions,
// settings, help, notes, create).
func barTargetOf(it overlayItem) (t barTarget, ok bool) {
	kind := it.kind
	if kind == kindRecent {
		kind = it.origin
	}
	prefix, rest, found := strings.Cut(it.id, ":")
	switch kind {
	case kindPerson:
		if found && prefix == "person" {
			id, err := strconv.ParseInt(rest, 10, 64)
			return barTarget{kind: "person", id: id}, err == nil
		}
	case kindOrg:
		if found && prefix == "org" {
			id, err := strconv.ParseInt(rest, 10, 64)
			return barTarget{kind: "org", id: id}, err == nil
		}
	case kindDeal:
		if found && prefix == "deal" {
			id, err := strconv.ParseInt(rest, 10, 64)
			return barTarget{kind: "deal", id: id}, err == nil
		}
	case kindTask:
		if found && prefix == "task" {
			id, err := strconv.ParseInt(rest, 10, 64)
			return barTarget{kind: "task", id: id}, err == nil
		}
	case kindTag:
		if found && prefix == "tag" {
			return barTarget{kind: "tag", name: rest}, true
		}
	}
	return barTarget{}, false
}

// barSaved is the screen state the bar borrows to run a verb.
type barSaved struct {
	view           viewKind
	rows           []row
	cursor, offset int
	focus          focus
	query          string
	marked         map[int64]bool
	tag            string
	status         string
	statusIsErr    bool
}

// barResume remembers a bar that is waiting for a verb to finish (a prompt, a confirm, a picker).
type barResume struct {
	ov      *overlay
	saved   barSaved
	itemID  string
	label   string
	started bool
}

// enterBarContext makes the model look like the object's own view with that object selected and
// returns what restoreBarContext needs. Nothing is read from the database here.
func (m *Model) enterBarContext(t barTarget) barSaved {
	s := barSaved{view: m.view, rows: m.rows, cursor: m.cursor, offset: m.offset, focus: m.focus, query: m.query, marked: m.marked, tag: m.barTag,
		status: m.status, statusIsErr: m.statusIsErr}
	m.query = ""
	m.marked = map[int64]bool{}
	m.focus = focusList
	m.cursor, m.offset = 0, 0
	switch t.kind {
	case "person":
		m.view, m.rows = viewPeople, []row{{personID: t.id}}
	case "org":
		m.view, m.rows = viewOrgs, []row{{orgID: t.id}}
	case "deal":
		m.view, m.rows = viewDeals, []row{{dealID: t.id}}
	case "task":
		m.view, m.rows = viewTasks, []row{{taskID: t.id}}
	case "tag":
		m.view, m.rows = viewHome, nil
		m.barTag = t.name
	default:
		m.view, m.rows = viewHome, nil
	}
	return s
}

// leaveBarContext puts the screen back and refreshes it, because the verb may have changed data.
func (m *Model) leaveBarContext(s barSaved) {
	m.view, m.rows, m.cursor, m.offset, m.focus, m.query, m.marked, m.barTag = s.view, s.rows, s.cursor, s.offset, s.focus, s.query, s.marked, s.tag
	m.report(m.rebuild(true))
}

// barVerbKeyOK reports whether the action runs for the key in the current (borrowed) context.
func (m *Model) barVerbKeyOK(a *action, key string, tag bool) bool {
	if barReserved[key] || !a.hasKey(key) || !a.enabled(m) {
		return false
	}
	if a.keyIf != nil && !a.keyIf(m) {
		return false
	}
	if a.keyFor != nil && !a.keyFor(m, key) {
		return false
	}
	if a.id == "undo" || a.id == "redo" {
		return true // ctrl+z and ctrl+y undo and redo the verb just run
	}
	if tag {
		return a.barOnly
	}
	return barVerbGroups[a.group] && !barNoVerb[a.id] && !a.barOnly
}

// barVerb is one verb of the highlighted object with its key.
type barVerb struct {
	id, key, label string
	write          bool
}

// barVerbs lists the verbs of the object in the borrowed context, in registry order, one per action.
func (m *Model) barVerbs(t barTarget) []barVerb {
	saved := m.enterBarContext(t)
	defer func() {
		m.view, m.rows, m.cursor, m.offset, m.focus, m.query, m.marked, m.barTag = saved.view, saved.rows, saved.cursor, saved.offset, saved.focus, saved.query, saved.marked, saved.tag
	}()
	var out []barVerb
	seen := map[string]bool{}
	for i := range m.actions {
		a := &m.actions[i]
		for _, k := range a.keys {
			if !m.barVerbKeyOK(a, k, t.kind == "tag") {
				continue
			}
			if seen[a.id] {
				break
			}
			seen[a.id] = true
			label := footShort[a.id]
			if a.footLabel != "" {
				label = a.footLabel
			}
			if label == "" {
				label = strings.ToLower(a.title)
			}
			out = append(out, barVerb{id: a.id, key: keys.Display(k), label: label, write: a.write})
			break
		}
	}
	return out
}

// barVerbLine is the footer of the bar while the results have the focus: the verbs of the
// highlighted object with their keys, cut to the width.
func (m *Model) barVerbLine(it overlayItem) string {
	t, ok := barTargetOf(it)
	if !ok {
		return "no verbs for this row (enter opens it)   tab or esc: back to the input"
	}
	var parts []string
	for _, v := range m.barVerbs(t) {
		if v.write && m.readOnly {
			continue
		}
		parts = append(parts, v.key+" "+v.label)
	}
	line := strings.Join(parts, "  ")
	if line == "" {
		line = "no verbs for this " + t.kind
	}
	return runewidth.Truncate(line, max(10, m.width-24), "…") + "   tab or esc: input"
}

// highlighted returns the row under the bar cursor.
func (o *overlay) highlighted() (overlayItem, bool) {
	if o.cursor < 0 || o.cursor >= len(o.shown) {
		return overlayItem{}, false
	}
	return o.items[o.shown[o.cursor]], true
}

// barFocusKey handles Tab and the results focus. handled is false to let the bar treat the key as usual.
func (m *Model) barFocusKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	o := m.ov
	key := msg.String()
	if !o.focusResults {
		if key != "tab" {
			return m, nil, false
		}
		if _, completes := verbCompletion(o.filter.Value()); completes {
			return m, nil, false // Tab completes a typed verb (n, t, set, tag, snooze)
		}
		it, ok := o.highlighted()
		if !ok {
			return m, nil, false
		}
		if _, has := barTargetOf(it); !has {
			return m, nil, false // an action, setting or help row: Tab keeps moving down
		}
		o.focusResults = true
		return m, nil, true
	}
	switch key {
	case "tab", "esc", "shift+tab":
		o.focusResults = false
		return m, nil, true
	case "enter", "up", "down", "pgup", "pgdown", "ctrl+n", "ctrl+p", "ctrl+c":
		return m, nil, false
	case "j":
		o.move(1, m.overlayPage())
		return m, nil, true
	case "k":
		o.move(-1, m.overlayPage())
		return m, nil, true
	}
	return m, m.runBarVerb(msg), true
}

// runBarVerb runs the key as the highlighted object's verb. The screen is borrowed for the call: it is
// shown as the object's view with the object selected, handleKey runs, and the screen is put back. A
// verb that opens a prompt, a confirm or a picker keeps the borrowed screen until the model is back
// in normal mode, then the bar returns (see resumeBar).
func (m *Model) runBarVerb(msg tea.KeyMsg) tea.Cmd {
	o := m.ov
	it, ok := o.highlighted()
	if !ok {
		return nil
	}
	t, has := barTargetOf(it)
	if !has {
		t = barTarget{kind: "none"} // only undo and redo run on a row with no verbs
	}
	key := msg.String()
	var act *action
	saved := m.enterBarContext(t)
	for i := range m.actions {
		a := &m.actions[i]
		if m.barVerbKeyOK(a, key, t.kind == "tag") {
			act = a
			break
		}
	}
	if act == nil {
		m.leaveBarContext(saved)
		o.ensureCursor(it.id)
		return m.showToast(fmt.Sprintf("%s has no verb on %s", barTargetName(t, it), keys.Display(key)), true)
	}
	target := barTargetName(t, it)
	title := act.title
	// Run it as normal mode.
	m.mode, m.ov = modeNormal, nil
	m.status, m.statusIsErr = "", false
	cmd := m.runAction(act)
	if m.pendingCmd != nil {
		cmd = tea.Batch(cmd, m.pendingCmd)
		m.pendingCmd = nil
	}
	res := &barResume{ov: o, saved: saved, itemID: it.id, label: title + ": " + target}
	m.pushRecent(it)
	if m.mode == modeNormal && m.ov == nil {
		// Done at once: bring the bar back and say what happened.
		return tea.Batch(cmd, m.finishBarVerb(res))
	}
	m.barResume = res
	return cmd
}

// finishBarVerb restores the screen and the bar, refreshes the results and toasts the outcome.
func (m *Model) finishBarVerb(r *barResume) tea.Cmd {
	failed, text := m.statusIsErr, m.status
	m.barResume = nil
	m.leaveBarContext(r.saved)
	m.ov, m.mode = r.ov, modeOverlay
	r.ov.refilterBar()
	r.ov.ensureCursor(r.itemID)
	r.ov.focusResults = true
	if failed && text != "" {
		return m.showToast(text, true)
	}
	return m.showToast(r.label, false)
}

// resumeBar runs after every message. When a verb that took the screen is finished the bar returns.
func (m *Model) resumeBar() tea.Cmd {
	r := m.barResume
	if r == nil || m.mode != modeNormal || m.ov != nil {
		return nil
	}
	return m.finishBarVerb(r)
}

// ensureCursor puts the cursor on the row with the id when it is still listed.
func (o *overlay) ensureCursor(id string) {
	for pos, idx := range o.shown {
		if o.items[idx].id == id {
			o.cursor = pos
			if o.cursor < o.offset {
				o.offset = o.cursor
			}
			return
		}
	}
	if o.cursor >= len(o.shown) {
		o.cursor = max(0, len(o.shown)-1)
	}
}

func barTargetName(t barTarget, it overlayItem) string {
	if t.kind == "tag" {
		return "#" + t.name
	}
	return it.label
}

// --- tag rows: the verbs of a tag, registry actions that only apply in the bar ---

func barTagActions() []action {
	return []action{
		{id: "bar-tag-delete", title: "Delete tag", group: "Tags", hint: "d", keys: []string{"d"}, barOnly: true, write: true,
			desc: "delete the tag from every record (y/n). ctrl+z puts it back. Same as crm tag delete",
			when: func(m *Model) bool { return m.barTag != "" },
			run:  func(m *Model) tea.Cmd { m.askDeleteTag(m.barTag); return nil }},
		{id: "bar-tag-link", title: "Tag a person", group: "Tags", hint: "L", keys: []string{"L"}, barOnly: true, write: true,
			desc: "pick a person to give this tag",
			when: func(m *Model) bool { return m.barTag != "" },
			run:  func(m *Model) tea.Cmd { m.pickPersonForTag(m.barTag, true); return nil }},
		{id: "bar-tag-unlink", title: "Untag a person", group: "Tags", hint: "U", keys: []string{"U"}, barOnly: true, write: true,
			desc: "pick a person who has this tag to take it off",
			when: func(m *Model) bool { return m.barTag != "" },
			run:  func(m *Model) tea.Cmd { m.pickPersonForTag(m.barTag, false); return nil }},
	}
}

func (m *Model) askDeleteTag(name string) {
	n := 0
	for _, e := range taggableEntities {
		ids, _ := m.tagr.GetEntities(m.ctx, name, e)
		n += len(ids)
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("delete tag #%s from %d records? y/n", name, n),
		yes:    func() { m.deleteTag(name) },
	}
}

// pickPersonForTag lists the people who can take the tag (add) or who have it (remove) and applies the pick.
func (m *Model) pickPersonForTag(tag string, add bool) {
	have := map[int64]bool{}
	ids, _ := m.tagr.GetEntities(m.ctx, tag, "person")
	for _, id := range ids {
		have[id] = true
	}
	var items []overlayItem
	for _, p := range m.people {
		if have[p.ID] == add {
			continue
		}
		items = append(items, overlayItem{id: fmt.Sprintf("person:%d", p.ID), label: p.Name, desc: p.Company, ref: p.ID, value: p.Name})
	}
	title := "Give #" + tag + " to"
	if !add {
		title = "Take #" + tag + " off"
	}
	if len(items) == 0 {
		m.setStatus("nobody to pick for #"+tag, true)
		return
	}
	m.openPicker(title, items, func(m *Model, it overlayItem) {
		id := it.ref
		if add {
			m.report(m.hTagApply(m.ctx, "person", id, tag))
			m.setStatus("#"+tag+" added to "+it.value+" (ctrl+z undoes)", false)
		} else {
			m.report(m.hTagRemove(m.ctx, "person", id, tag))
			m.setStatus("#"+tag+" removed from "+it.value+" (ctrl+z undoes)", false)
		}
		m.afterWrite()
	})
}
