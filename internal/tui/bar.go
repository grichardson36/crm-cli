package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/help"
	"github.com/jdanielnd/crm-cli/internal/model"
	sg "github.com/jdanielnd/crm-cli/internal/suggest"
)

// The unified bar (ctrl+k) is one box for three jobs: run an action, find a record, and type a
// command. Without a prefix it lists matching actions, settings and records in groups. A prefix
// narrows it: > actions, @ people, # tags, = settings, ? help. A verb and a space (n, t, log, set,
// tag, snooze) switches to the command line. An empty box lists the things used recently. The
// keys ctrl+k, : and / all open it.

const (
	kindAction  = ""
	kindRecent  = "recent"
	kindPerson  = "person"
	kindOrg     = "org"
	kindDeal    = "deal"
	kindTask    = "task"
	kindNote    = "note"
	kindTag     = "tag"
	kindHelp    = "help"
	kindSetting = "setting"
	kindAll     = "all" // "Show all N people matching": filters the People list
	kindVerb    = "verb"
	kindCreate  = "create"
)

// barRank orders the groups in the bar (owner decision 2026-10-16): Recent when the box is empty, then
// objects (people, organizations, deals, tasks, notes, tags), then actions, then settings. A typed verb
// (n, t, set, tag, snooze) is the exception: its action line comes first, see verbRank.
var barRank = map[string]int{
	kindRecent: 0, kindPerson: 1, kindAll: 1, kindOrg: 2, kindDeal: 3, kindTask: 4, kindNote: 5, kindTag: 6,
	kindAction: 7, kindVerb: 7, kindSetting: 8, kindHelp: 9, kindCreate: 10,
}

// verbRank is the rank of a verb line (Add note (n), Snooze (snooze)). While the first word being
// typed is the start of the verb it leads the list, ahead of the objects, so typing n or snooze still
// shows the action first. Otherwise it ranks with the actions.
func verbRank(it overlayItem, q string) int {
	first := strings.ToLower(strings.Fields(q + " ")[0])
	word := strings.ToLower(strings.TrimSpace(it.value))
	if first != "" && strings.HasPrefix(word, first) {
		return 0
	}
	return barRank[kindVerb]
}

// barGroup is the label shown at the start of each group.
var barGroup = map[string]string{
	kindRecent: "Recent", kindAction: "Actions", kindVerb: "Actions", kindSetting: "Settings", kindPerson: "People", kindAll: "People", kindOrg: "Orgs",
	kindDeal: "Deals", kindTask: "Tasks", kindNote: "Notes", kindTag: "Tags", kindHelp: "Help", kindCreate: "Create",
}

const (
	barActionCap = 12 // actions shown while searching for something
	barHitCap    = 4  // records shown per group
	barRecentCap = 5
)

// verbAliases are the typed verbs of the actions that have one, shown as "Add note (n)".
var verbAliases = map[string]string{"note": "n", "task": "t"}

// barScope splits the box text into a scope prefix and the query.
func barScope(raw string) (scope byte, q string) {
	if raw != "" && strings.ContainsRune(">@#?=", rune(raw[0])) {
		return raw[0], strings.TrimSpace(raw[1:])
	}
	return 0, strings.TrimSpace(raw)
}

// refilterBar rebuilds the bar list for the text in the box.
func (o *overlay) refilterBar() {
	scope, q := barScope(o.filter.Value())
	var items []overlayItem
	if scope == 0 && q == "" && o.recent != nil {
		items = append(items, o.recent()...)
	}
	if scope == 0 || scope == '>' {
		items = append(items, o.base...)
	}
	if o.search != nil && scope != '>' && (q != "" || scope == '#' || scope == '?' || scope == '=') {
		items = append(items, o.search(scope, q)...)
	}
	if scope == 0 && q != "" {
		items = append(items, verbItems()...)
	}

	type scored struct{ idx, rank, score, length int }
	weak := map[int]bool{} // actions that only match as scattered letters
	var list []scored
	for i, it := range items {
		score := 0
		switch it.kind {
		case kindAction, kindVerb:
			if q != "" {
				s, ok := sg.Fuzzy(q, it.label+" "+it.hint+" "+it.id+" "+it.desc)
				if !ok {
					continue
				}
				score = s
				weak[i] = s >= sg.FuzzyWeak
			}
			if it.disabled != "" {
				score += 1000 // after every enabled action
				weak[i] = true
			}
		case kindRecent:
			score = i
		default:
			score = i // the database already ranked these
		}
		rank := barRank[it.kind]
		if it.kind == kindVerb {
			rank = verbRank(it, q)
		}
		if it.kind == kindTag && it.exact {
			rank, score = barRank[kindPerson], -1 // a tag named exactly like the query leads the objects
		}
		list = append(list, scored{i, rank, score, len(it.label)})
	}
	sort.SliceStable(list, func(a, b int) bool {
		x, y := list[a], list[b]
		if x.rank != y.rank {
			return x.rank < y.rank
		}
		if x.score != y.score {
			return x.score < y.score
		}
		return q != "" && x.length < y.length // a shorter label wins ties
	})
	o.items = items
	o.shown = o.shown[:0]
	actions, hits := 0, map[string]int{}
	for _, s := range list {
		it := items[s.idx]
		if q != "" && scope == 0 {
			switch it.kind {
			case kindAction, kindVerb:
				if it.disabled != "" {
					// A greyed-out action is listed only when the text really appears in it.
					if s.score >= 1030 {
						continue
					}
					break
				}
				if actions++; actions > barActionCap {
					continue
				}
			case kindPerson, kindOrg, kindDeal, kindTask, kindNote, kindSetting, kindTag:
				if hits[it.kind]++; hits[it.kind] > barHitCap {
					continue
				}
			}
		}
		o.shown = append(o.shown, s.idx)
	}
	if len(o.shown) == 0 && q != "" && scope == 0 {
		o.items = append(o.items,
			overlayItem{id: "create-person", kind: kindCreate, label: fmt.Sprintf("Create person %q", q), desc: "add a contact with this name", value: q},
			overlayItem{id: "create-note", kind: kindCreate, label: fmt.Sprintf("Add note %q", q), desc: "write a note with this text", value: q},
		)
		o.shown = append(o.shown, len(o.items)-2, len(o.items)-1)
	}
	// The cursor starts on the first row that is a real match. An action that only matches as
	// scattered letters does not take enter away from a person typed by name.
	o.cursor, o.offset = 0, 0
	if q != "" && scope == 0 {
		for pos, idx := range o.shown {
			if !weak[idx] {
				o.cursor = pos
				break
			}
		}
	}
}

func verbItems() []overlayItem {
	return []overlayItem{
		{id: "verb:n", kind: kindVerb, label: "Add note (n)", desc: "n <name> text #tag remind fri", value: "n "},
		{id: "verb:t", kind: kindVerb, label: "Add task (t)", desc: "t <name> text fri 3pm !high", value: "t "},
		{id: "verb:set", kind: kindVerb, label: "Set fields (set)", desc: "set <name>: company=Acme title=CTO", value: "set "},
		{id: "verb:tag", kind: kindVerb, label: "Tag people (tag)", desc: "tag <name> #friend", value: "tag "},
		{id: "verb:snooze", kind: kindVerb, label: "Snooze (snooze)", desc: "snooze <name> 2w", value: "snooze "},
	}
}

// barSearch finds records and tags for the box text. Errors give no hits.
func (m *Model) barSearch(scope byte, q string) []overlayItem {
	var out []overlayItem
	switch scope {
	case '#':
		low := strings.ToLower(strings.TrimPrefix(q, "#"))
		for _, t := range m.quickEnv().Tags {
			if low == "" || strings.Contains(strings.ToLower(t), low) {
				out = append(out, overlayItem{id: "tag:" + t, kind: kindTag, label: "#" + t, desc: "people with this tag", value: t})
			}
		}
		return out
	case '=':
		return m.settingItems(q)
	case '?':
		type hit struct {
			it    overlayItem
			score int
		}
		var hits []hit
		for _, t := range help.List() {
			if s, ok := sg.Fuzzy(q, t.Name+" "+t.Summary); ok {
				hits = append(hits, hit{overlayItem{id: "help:" + t.Name, kind: kindHelp, label: t.Name, desc: t.Summary, value: t.Name}, s})
			}
		}
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].score < hits[b].score })
		for _, h := range hits {
			out = append(out, h.it)
		}
		return out
	}
	// A tag is an object: its name in the query (any case, whole or part) lists it without the # prefix.
	if low := strings.ToLower(strings.TrimSpace(q)); low != "" {
		for _, t := range m.quickEnv().Tags {
			lt := strings.ToLower(t)
			if lt == low || strings.Contains(lt, low) {
				out = append(out, overlayItem{id: "tag:" + t, kind: kindTag, label: "#" + t, desc: "tag: people with this tag", value: t, exact: lt == low})
			}
		}
	}
	terms := strings.Fields(strings.ToLower(q))
	people := 0
	var tagged []overlayItem // people who match only through a tag come after the ones whose own text matches
	for _, p := range m.people {
		if !matchesAlias(p, terms, m.aliasTable()) {
			continue
		}
		desc := p.Company
		if desc == "" {
			desc = p.Title
		}
		it := overlayItem{id: fmt.Sprintf("person:%d", p.ID), kind: kindPerson, label: p.Name, desc: desc, ref: p.ID}
		bare := p
		bare.Tags = ""
		if !matchesAlias(bare, terms, m.aliasTable()) {
			// The text is not in the person, only in a tag: say which one, so the match is not a mystery.
			it.desc = "tagged " + taggedBy(p.Tags, terms)
			tagged = append(tagged, it)
		} else {
			out = append(out, it)
		}
		people++
	}
	out = append(out, tagged...)
	if scope == '@' {
		return out
	}
	if people > barHitCap {
		out = append(out, overlayItem{id: "people-all", kind: kindAll, label: fmt.Sprintf("Show all %d people matching %q", people, q), desc: "filter the People list (esc clears)", value: q})
	}
	out = append(out, m.settingItems(q)...)
	if orgs, err := repo.NewOrgRepo(m.db).Search(m.ctx, q, barHitCap); err == nil {
		for _, o := range orgs {
			out = append(out, overlayItem{id: fmt.Sprintf("org:%d", o.ID), kind: kindOrg, label: o.Name, desc: "people at this organization", ref: o.ID, value: o.Name})
		}
	}
	if deals, err := repo.NewDealRepo(m.db).Search(m.ctx, q, barHitCap); err == nil {
		for _, d := range deals {
			var pid int64
			if len(d.PersonIDs) > 0 {
				pid = d.PersonIDs[0]
			}
			out = append(out, overlayItem{id: fmt.Sprintf("deal:%d", d.ID), kind: kindDeal, label: d.Title, desc: d.Stage, ref: pid})
		}
	}
	if tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{}); err == nil {
		low := strings.ToLower(q)
		for _, t := range tasks {
			if !t.Completed && strings.Contains(strings.ToLower(t.Title), low) {
				out = append(out, overlayItem{id: fmt.Sprintf("task:%d", t.ID), kind: kindTask, label: cleanCell(t.Title), desc: "open task", ref: t.ID})
			}
		}
	}
	if notes, err := m.ir.Search(m.ctx, q, barHitCap); err == nil {
		for _, n := range notes {
			text := ""
			switch {
			case n.Subject != nil && *n.Subject != "":
				text = *n.Subject
			case n.Content != nil:
				text = *n.Content
			}
			var pid int64
			if len(n.PersonIDs) > 0 {
				pid = n.PersonIDs[0]
			}
			out = append(out, overlayItem{id: fmt.Sprintf("note:%d", n.ID), kind: kindNote, label: cleanCell(text), desc: n.Type, ref: pid})
		}
	}
	return out
}

// recentItems are what the bar lists first when the box is empty.
func (m *Model) recentItems() []overlayItem {
	out := make([]overlayItem, 0, len(m.recent))
	for _, it := range m.recent {
		it.origin, it.kind = it.kind, kindRecent
		out = append(out, it)
	}
	return out
}

// pushRecent remembers a picked item, newest first, for this session.
func (m *Model) pushRecent(it overlayItem) {
	if it.kind == kindRecent {
		it.kind = it.origin
	}
	if it.kind == kindCreate || it.kind == kindVerb || it.kind == kindSetting || it.kind == kindAll || it.disabled != "" {
		return
	}
	list := []overlayItem{it}
	for _, r := range m.recent {
		if r.id != it.id {
			list = append(list, r)
		}
	}
	if len(list) > barRecentCap {
		list = list[:barRecentCap]
	}
	m.recent = list
}

// pickBar runs a record, tag, setting, help, verb or create item from the bar. handled is false for
// actions, which the palette runs itself.
func (m *Model) pickBar(it overlayItem) (handled bool, cmd tea.Cmd) {
	kind := it.kind
	if kind == kindRecent {
		kind = it.origin
		it.kind = kind
	}
	switch kind {
	case kindPerson:
		m.selectPerson(it.ref)
	case kindOrg:
		m.showPeopleQuery(it.value)
	case kindTag:
		m.showPeopleQuery("#" + it.value)
	case kindAll:
		m.showPeopleQuery(it.value)
	case kindSetting:
		return true, m.runSetting(it)
	case kindDeal, kindNote:
		if it.ref == 0 {
			m.setStatus("no person is linked to this "+kind, true)
		} else {
			m.selectPerson(it.ref)
		}
	case kindTask:
		m.selectTask(it.ref)
	case kindHelp:
		m.showTopic(it.value, nil)
	case kindVerb:
		cmd = m.openPalette()
		m.ov.setLine(it.value, len(it.value))
		m.syncCmd()
	case kindCreate:
		if it.id == "create-note" {
			line := "n " + it.value
			cmd = m.openPalette()
			m.ov.setLine(line, len(line))
			m.syncCmd()
		} else {
			cmd = m.createPersonFromBar(it.value)
		}
	default:
		return false, nil
	}
	m.pushRecent(it)
	return true, cmd
}

func (m *Model) createPersonFromBar(name string) tea.Cmd {
	if m.readOnly {
		return m.showToast("read-only: create person is disabled", true)
	}
	m.beginGroup()
	p, err := m.createNamed(name)
	m.endGroup("create contact " + name)
	if err != nil {
		return m.showToast(err.Error(), true)
	}
	m.qix = nil
	m.afterWrite()
	m.selectPerson(p.ID)
	return m.showToast("Created "+fullName(p), false)
}

// selectPerson shows the People view with the person under the cursor.
func (m *Model) selectPerson(id int64) {
	m.query = ""
	if m.view != viewPeople {
		m.switchView(viewPeople)
	} else {
		m.report(m.rebuild(false))
	}
	m.focus = focusList
	m.cursorOn(func(r row) bool { return r.personID == id })
}

// selectTask shows the Tasks view with the task under the cursor.
func (m *Model) selectTask(id int64) {
	if m.view != viewTasks {
		m.switchView(viewTasks)
	}
	m.cursorOn(func(r row) bool { return r.taskID == id })
}

// showPeopleQuery shows the People view filtered by text.
func (m *Model) showPeopleQuery(q string) {
	if m.view != viewPeople {
		m.switchView(viewPeople)
	}
	m.query = q
	m.cursor, m.offset = 0, 0
	m.report(m.rebuild(false))
}

func (m *Model) cursorOn(match func(row) bool) {
	for i, r := range m.rows {
		if !r.skip && match(r) {
			m.cursor = i
			m.clamp()
			m.refreshDetail(true)
			return
		}
	}
	m.setStatus("not in this list", true)
}

// taggedBy names the tags of a person that carry the search terms, "#friend #the-well".
func taggedBy(tags string, terms []string) string {
	var hit []string
	for _, t := range strings.Split(tags, ",") {
		t = strings.TrimSpace(t)
		low := strings.ToLower(t)
		for _, term := range terms {
			if strings.Contains(low, strings.TrimPrefix(term, "#")) {
				hit = append(hit, "#"+t)
				break
			}
		}
	}
	if len(hit) == 0 {
		return "#" + strings.TrimSpace(tags)
	}
	return strings.Join(hit, " ")
}
