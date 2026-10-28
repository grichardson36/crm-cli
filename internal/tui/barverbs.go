package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/keys"
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
	"pgup": true, "pgdown": true, "ctrl+c": true, "ctrl+k": true,
	"/": true, "q": true, "?": true, "a": true,
}

// barVerbGroups are the registry groups whose actions act on a selected object.
var barVerbGroups = map[string]bool{"Contact": true, "Link": true, "Organizations": true, "Deals": true, "Tasks": true, "Tags": true}

// barNoVerb are actions in those groups that do not act on one selected object.
var barNoVerb = map[string]bool{"org-mark": true, "org-merge": true, "merge-marked": true, "merge": true, "new": true, "note-pick": true, "note-types": true, "deal-roles": true}

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
	kind           string
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
		status: m.status, statusIsErr: m.statusIsErr, kind: m.barKind}
	m.barKind = t.kind
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
		// The tag is shown as the Tags view with one row, so the bar runs the very same tag actions
		// (tagview.go). The id is a marker: tag verbs find the tag by m.barTag.
		m.view, m.rows = viewTags, []row{{tagID: -1, tag: t.name, title: t.name}}
		m.barTag = t.name
	case "blank-person":
		m.view, m.rows = viewPeople, nil
	case "blank-organization":
		m.view, m.rows = viewOrgs, nil
	case "blank-deal":
		m.view, m.rows = viewDeals, nil
	case "blank-task":
		m.view, m.rows = viewTasks, nil
	default:
		m.view, m.rows = viewHome, nil
	}
	return s
}

// leaveBarContext puts the screen back and refreshes it, because the verb may have changed data.
func (m *Model) leaveBarContext(s barSaved) {
	m.view, m.rows, m.cursor, m.offset, m.focus, m.query, m.marked, m.barTag, m.barKind = s.view, s.rows, s.cursor, s.offset, s.focus, s.query, s.marked, s.tag, s.kind
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
	if a.mode != keyNormal {
		return false
	}
	if a.verbAll {
		// activate and mark work on the object of any kind, but a person already opens with enter in the bar.
		return !(a.id == "activate" && m.barKind == "person")
	}
	if tag {
		return a.barOnly || a.tagVerb
	}
	return barVerbGroups[a.group] && !barNoVerb[a.id] && !a.barOnly && !a.tagVerb
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
		m.view, m.rows, m.cursor, m.offset, m.focus, m.query, m.marked, m.barTag, m.barKind = saved.view, saved.rows, saved.cursor, saved.offset, saved.focus, saved.query, saved.marked, saved.tag, saved.kind
	}()
	var out []barVerb
	seen := map[string]bool{}
	for i := range m.actions {
		a := &m.actions[i]
		if !inScopes(a, m.scopeChain()) {
			continue
		}
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
			if a.footDyn != nil {
				label = a.footDyn(m)
			}
			if label == "" {
				label = strings.ToLower(a.title)
			}
			out = append(out, barVerb{id: a.id, key: keys.Display(k), label: label, write: a.writes(m)})
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
	return ansi.Truncate(line, max(10, m.width-24), "…") + "   tab or esc: input"
}

// highlighted returns the row under the bar cursor.
func (o *overlay) highlighted() (overlayItem, bool) {
	if o.cursor < 0 || o.cursor >= len(o.shown) {
		return overlayItem{}, false
	}
	return o.items[o.shown[o.cursor]], true
}

// barFocusKey handles Tab and the results focus. handled is false to let the bar treat the key as usual.
func (m *Model) barFocusKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	o := m.ov
	key := keyName(msg)
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
	case "enter", "up", "down", "pgup", "pgdown", "ctrl+c":
		return m, nil, false
	}
	return m, m.runBarVerb(msg), true
}

// runBarVerb runs the key as the highlighted object's verb. The screen is borrowed for the call: it is
// shown as the object's view with the object selected, handleKey runs, and the screen is put back. A
// verb that opens a prompt, a confirm or a picker keeps the borrowed screen until the model is back
// in normal mode, then the bar returns (see resumeBar).
func (m *Model) runBarVerb(msg tea.KeyPressMsg) tea.Cmd {
	o := m.ov
	it, ok := o.highlighted()
	if !ok {
		return nil
	}
	t, has := barTargetOf(it)
	if !has {
		t = barTarget{kind: "none"} // only undo and redo run on a row with no verbs
	}
	key := keyName(msg)
	saved := m.enterBarContext(t)
	act := m.lookupKeyWith(key, keyNormal, func(a *action, key string) bool { return m.barVerbKeyOK(a, key, t.kind == "tag") })
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
	if m.mode != modeNormal || m.ov != nil {
		return nil
	}
	if m.addResume != nil {
		return m.finishGlobalAdd()
	}
	if r := m.barResume; r != nil {
		return m.finishBarVerb(r)
	}
	return nil
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
