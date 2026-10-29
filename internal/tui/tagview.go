package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/tree"
)

// The Tags module (patterns/features-tags-module). Tags are a view of their own: a list with the name,
// the number of objects that carry it and a dim breakdown by kind. The verbs are the universal ones
// (patterns/universal-verbs) and every one works on the selected tags (patterns/bulk-actions-always):
//
//	enter     open the tag page: the objects that carry the tag, grouped by kind under dim headings in
//	          the kind colors. enter on an object opens it, esc goes back to the list
//	tab       move into the detail pane, where enter or e on the name edits it in place. A name another
//	          tag has asks once to merge into it
//	d         delete the tag (untag everything, or remove an unused name), one undo step
//	m         merge the space-selected tags into the highlighted one
//	l, u      give the tag to an object / take it off (on a tag page: of the highlighted objects)
//	a g       add a tag (the add picker)
//
// The rows of a tag in the command bar are this view with one row: enterBarContext (barverbs.go) shows the
// tag as a Tags view so the bar runs the very same actions.

// onTagPage is true while the list pane shows the objects of one tag.
func (m *Model) onTagPage() bool { return m.view == viewTags && m.tagPage != "" && m.barKind == "" }

// whenTagList is true on the list of tags with a tag highlighted (not in the bar, not on a page).
func whenTagList(m *Model) bool {
	return m.view == viewTags && m.focus == focusList && !m.onTagPage() && m.barKind == "" && m.cursorTagName() != ""
}

// whenTagVerb is true wherever a tag verb applies: the list, a tag page and a tag row of the bar.
func whenTagVerb(m *Model) bool {
	if m.barTag != "" {
		return true
	}
	if m.view != viewTags || m.focus != focusList {
		return false
	}
	if m.onTagPage() {
		return true
	}
	return m.cursorTagName() != ""
}

// cursorTagName is the tag under the cursor of the list, or the tag of the bar row.
func (m *Model) cursorTagName() string {
	if m.barTag != "" {
		return m.barTag
	}
	if m.view != viewTags || m.onTagPage() {
		return ""
	}
	if r, ok := m.selected(); ok && !r.skip {
		return r.tag
	}
	return ""
}

// tagRowRef is the object a row of the Tags view stands for: a tag in the list, an object on a tag page.
func (m *Model) tagRowRef(r row) (repo.Ref, bool) {
	if m.onTagPage() {
		if r.objID != 0 && r.akind != "" {
			return repo.Ref{Kind: r.akind, ID: r.objID}, true
		}
		return repo.Ref{}, false
	}
	if r.tagID != 0 {
		return repo.Ref{Kind: tagKind, ID: r.tagID}, true
	}
	return repo.Ref{}, false
}

// bulkTagNames are the tags a verb acts on: the selected ones, else the highlighted one.
func (m *Model) bulkTagNames() []string {
	if m.barTag != "" {
		return []string{m.barTag}
	}
	if m.view != viewTags || m.onTagPage() {
		return nil
	}
	var names []string
	for _, ref := range m.bulkRefs() {
		for _, r := range m.rows {
			if !r.skip && r.tagID == ref.ID && ref.Kind == tagKind {
				names = append(names, r.tag)
				break
			}
		}
	}
	return names
}

// --- words ---

var tagKindWords = map[string][2]string{
	"person": {"person", "people"}, "organization": {"org", "orgs"}, "deal": {"deal", "deals"},
	"task": {"task", "tasks"}, model.NoteKind: {"note", "notes"},
}

func kindCount(kind string, n int) string {
	w := tagKindWords[kind]
	if n == 1 {
		return "1 " + w[0]
	}
	return fmt.Sprintf("%d %s", n, w[1])
}

// tagBreakdown is the dim part of a row: "2 people, 1 org".
func tagBreakdown(s repo.TagStat) string {
	var parts []string
	for _, k := range model.TaggableTypes {
		if n := s.ByKind[k]; n > 0 {
			parts = append(parts, kindCount(k, n))
		}
	}
	if s.Archived > 0 {
		parts = append(parts, fmt.Sprintf("%d archived", s.Archived))
	}
	if len(parts) == 0 {
		return "unused"
	}
	return strings.Join(parts, ", ")
}

// --- view options (ctrl+o) ---

func tagViewOptions() []viewOption {
	return []viewOption{
		{
			id: "sort", label: "Sort",
			choices: []optChoice{{"name", "name"}, {"count", "number of objects"}},
			get:     func(m *Model) string { return m.sortOf(viewTags) },
			set: func(m *Model, value string) {
				if m.sortChoice == nil {
					m.sortChoice = map[viewKind]string{}
				}
				m.sortChoice[viewTags] = value
			},
		},
		{
			id: "filter", label: "Filter",
			choices: []optChoice{{"", "tags in use"}, {"unused", "unused tags"}, {"duplicates", "possible case duplicates"}, {"all", "all tags"}},
			get:     func(m *Model) string { return m.tagFilter },
			set:     func(m *Model, value string) { m.tagFilter = value },
		},
		{
			id: "layout", label: "Layout",
			choices: []optChoice{{layoutFlat, "flat list"}, {layoutTree, "tree (work/clients under work)"}},
			get: func(m *Model) string {
				if m.tagLayout == layoutTree {
					return layoutTree
				}
				return layoutFlat
			},
			set: func(m *Model, value string) { m.setTagLayout(value) },
		},
		{
			id: "page", label: "Tag page",
			choices: []optChoice{{"own", "own objects"}, {"children", "include children"}},
			get: func(m *Model) string {
				if m.tagChildren {
					return "children"
				}
				return "own"
			},
			set: func(m *Model, value string) { m.setTagChildren(value == "children") },
		},
	}
}

// --- rows ---

// tagRows builds the rows of the Tags view: the list of tags, or the objects of the open tag page.
func (m *Model) tagRows() ([]row, error) {
	if m.tagPage != "" {
		rows, err := m.tagPageRows()
		if err == nil {
			return rows, nil
		}
		m.tagPage = "" // the tag is gone (merged, renamed elsewhere): back to the list
	}
	stats, err := m.tagr.Stats(m.ctx)
	if err != nil {
		return nil, err
	}
	groups, err := m.tagr.Duplicates(m.ctx)
	if err != nil {
		return nil, err
	}
	m.tagStats = map[string]repo.TagStat{}
	m.tagDups = map[string]repo.TagDupGroup{}
	for _, s := range stats {
		m.tagStats[s.Name] = s
	}
	for _, g := range groups {
		for _, t := range g.Tags {
			m.tagDups[t.Name] = g
		}
	}
	terms := strings.Fields(strings.ToLower(m.tagQuery))
	var list []repo.TagStat
	for _, s := range stats {
		_, dup := m.tagDups[s.Name]
		switch m.tagFilter {
		case "":
			if s.Unused() {
				continue
			}
		case "unused":
			if !s.Unused() {
				continue
			}
		case "duplicates":
			if !dup {
				continue
			}
		}
		low := strings.ToLower(s.Name)
		match := true
		for _, t := range terms {
			match = match && strings.Contains(low, strings.TrimPrefix(t, "#"))
		}
		if match {
			list = append(list, s)
		}
	}
	if m.tagTreeOn() {
		return m.tagTreeRows(list)
	}
	rowOf := func(s repo.TagStat) row {
		return row{tagID: s.ID, tag: s.Name, title: cleanCell(s.Name), sub: tagBreakdown(s), meta: strconv.Itoa(s.Count)}
	}
	if m.tagFilter == "duplicates" {
		// Spellings of one tag sit together under a divider that names the spelling they merge into.
		sort.SliceStable(list, func(i, j int) bool {
			a, b := m.tagDups[list[i].Name], m.tagDups[list[j].Name]
			if a.Key != b.Key {
				return a.Key < b.Key
			}
			return dupPos(a, list[i].Name) < dupPos(a, list[j].Name) // the spelling they merge into first
		})
		g := groupRows(list, func(s repo.TagStat) []groupHead {
			d := m.tagDups[s.Name]
			return []groupHead{{key: d.Key, title: "merge into " + d.Keep, meta: fmt.Sprintf("%d spellings", len(d.Tags)), rule: true}}
		}, rowOf)
		return g.rows, nil
	}
	if m.sortOf(viewTags) == "count" {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Count > list[j].Count })
	}
	rows := make([]row, 0, len(list))
	for _, s := range list {
		rows = append(rows, rowOf(s))
	}
	return rows, nil
}

// tagPageRows lists the objects that carry the open tag, grouped by kind under headings in the kind color.
func (m *Model) tagPageRows() ([]row, error) {
	var objs []repo.TaggedObject
	var err error
	if m.tagChildren {
		objs, err = m.tagr.TaggedObjectsWithChildren(m.ctx, m.tagPage)
	} else {
		objs, err = m.tagr.TaggedObjects(m.ctx, m.tagPage)
	}
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, o := range objs {
		counts[o.Kind]++
	}
	g := groupRows(objs, func(o repo.TaggedObject) []groupHead {
		return []groupHead{{key: o.Kind, title: kindHeading[o.Kind], meta: strconv.Itoa(counts[o.Kind]), rule: true}}
	}, func(o repo.TaggedObject) row {
		sub := cleanCell(o.Detail)
		if o.Tag != "" && o.Tag != m.tagPage {
			sub = strings.TrimSpace(sub + " #" + o.Tag) // came in through a child tag
		}
		return row{tag: m.tagPage, akind: o.Kind, objID: o.ID, title: cleanCell(o.Name), sub: sub}
	})
	// A divider takes the kind of the rows under it, so it is drawn in that kind's color.
	for i := range g.rows {
		if g.rows[i].skip && i+1 < len(g.rows) {
			g.rows[i].akind = g.rows[i+1].akind
		}
	}
	return g.rows, nil
}

func (m *Model) tagCountText() string {
	if m.onTagPage() {
		n := 0
		for _, r := range m.rows {
			if !r.skip {
				n++
			}
		}
		if m.tagChildren {
			return fmt.Sprintf("#%s, %d objects with children", m.tagPage, n)
		}
		return fmt.Sprintf("#%s, %d objects", m.tagPage, n)
	}
	names := map[string]string{"": "in use", "unused": "unused", "duplicates": "possible duplicates", "all": "all"}
	return fmt.Sprintf("%d tags, %s", len(m.rows), names[m.tagFilter])
}

func (m *Model) tagEmptyText() string {
	switch {
	case m.onTagPage():
		return "No object carries #" + m.tagPage + ". l gives it to one, esc goes back."
	case m.tagQuery != "":
		return "No tags match. esc clears the search."
	case m.tagFilter == "unused":
		return "No unused tags. ctrl+o changes the filter."
	case m.tagFilter == "duplicates":
		return "No tags differ only by case or separators. ctrl+o changes the filter."
	}
	return "No tags in use. a then g adds one, ctrl+o shows unused tags."
}

// --- the detail pane ---

// buildTagForm is the detail pane of the Tags view: the name field of the highlighted tag (edited in
// place) and its counts, or on a tag page a preview of the highlighted object.
func (m *Model) buildTagForm(r row) []formRow {
	m.formTitle = ""
	if m.onTagPage() {
		if r.objID == 0 {
			return nil
		}
		m.formTitle = "#" + m.tagPage
		rows := []formRow{infoRow(tagKindWords[r.akind][0], r.title)}
		rows[0].head = "On this tag"
		if r.sub != "" {
			rows = append(rows, infoRow("detail", r.sub))
		}
		rows = append(rows, infoRow("open", "enter opens it, u takes the tag off"))
		return rows
	}
	if r.tagID == 0 {
		return nil
	}
	s, ok := m.tagStats[r.tag]
	if !ok {
		if r.virtual {
			return m.virtualTagForm(r.tag)
		}
		return []formRow{infoRow("tag", r.tag)}
	}
	m.formTitle = "#" + s.Name
	name := specRow(repo.Ref{Kind: tagKind, ID: s.ID}, tagNameSpec(), s.Name)
	name.head = "Tag"
	rows := []formRow{name, infoRow("objects", strconv.Itoa(s.Count))}
	if n, ok := m.tagNodeOf(s.Name); ok && n.Children > 0 {
		rows = append(rows, infoRow("with children", strconv.Itoa(n.Total)), infoRow("children", strconv.Itoa(n.Children)))
	}
	for _, k := range model.TaggableTypes {
		if n := s.ByKind[k]; n > 0 {
			rows = append(rows, infoRow(tagKindWords[k][1], strconv.Itoa(n)))
		}
	}
	if s.Archived > 0 {
		rows = append(rows, infoRow("archived", strconv.Itoa(s.Archived)))
	}
	if g, dup := m.tagDups[s.Name]; dup {
		var others []string
		for _, t := range g.Tags {
			if t.Name != s.Name {
				others = append(others, "#"+t.Name)
			}
		}
		rows = append(rows, infoRow("similar", strings.Join(others, " ")+"  (m merges them)"))
		rows[len(rows)-1].head = "Now"
	}
	return rows
}

// tagNameSpec is the name field of a tag: the hierarchy field the schema declares (model.Fields["tag"]),
// so the editor and the validator treat the name as a path.
func tagNameSpec() model.FieldSpec {
	f, _ := model.HierarchyOf("tag")
	f.Hint = "a new path renames the tag and the tags below it. A name another tag has offers to merge into it"
	return f
}

// commitTagName is the end of the in-place edit of a tag name: rename, or when the name is another tag's
// ask once to merge into it. One undo step either way (tagops.go).
func (m *Model) commitTagName(ref repo.Ref, from, to string) {
	m.cancelEdit()
	to = repo.NormalizeTagName(to)
	if to == "" || to == from {
		return
	}
	m.tagRenaming = from
	m.runTagRename(to)
}

// openNote shows a note by the first person, organization or deal it is attached to.
func (m *Model) openNote(id int64) {
	n, err := m.ir.FindByID(m.ctx, id)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	switch {
	case len(n.PersonIDs) > 0:
		m.selectPerson(n.PersonIDs[0])
	case len(n.OrgIDs) > 0:
		m.openObject("organization", n.OrgIDs[0])
	case len(n.DealIDs) > 0:
		m.openObject("deal", n.DealIDs[0])
	default:
		m.setStatus("this note is not attached to a person, organization or deal", true)
	}
}

// --- search ---

// showTagQuery narrows the Tags list by name (the bar's "Show all tags matching").
func (m *Model) showTagQuery(q string) {
	m.tagQuery = q
	m.tagPage = ""
	m.cursor, m.offset = 0, 0
	m.report(m.rebuild(false))
}

// --- verbs ---

func tagViewActions() []action {
	inRow := func(m *Model) bool { return m.barTag != "" }
	return []action{
		{id: "tag-open", title: "Open tag page", group: "Tags", desc: "open the tag page: the objects that carry the tag, grouped by kind. enter on an object opens it. This is enter (activate) in the Tags view",
			when: func(m *Model) bool {
				return m.view == viewTags && m.focus == focusList && m.barKind == "" && len(m.rows) > 0
			},
			run: func(m *Model) tea.Cmd { return m.openTagPage() }},
		{id: "tag-back", title: "Back to the tag list", group: "Tags", hint: "esc", keys: []string{"esc"},
			desc: "close the tag page, or clear the tag search",
			foot: 15, footIn: inList, footLabel: "back", footIf: func(m *Model) bool { return m.onTagPage() },
			when: func(m *Model) bool {
				return m.view == viewTags && m.focus == focusList && m.barKind == "" && (m.tagPage != "" || m.tagQuery != "")
			},
			run: func(m *Model) tea.Cmd {
				if m.tagPage != "" {
					name := m.tagPage
					m.tagPage = ""
					m.marked = map[int64]bool{}
					m.cursor, m.offset = 0, 0
					m.report(m.rebuild(false))
					m.cursorOn(func(r row) bool { return r.tag == name })
					return nil
				}
				m.tagQuery = ""
				m.cursor, m.offset = 0, 0
				m.report(m.rebuild(false))
				return nil
			}},
		{id: "tag-rename", title: "Rename tag", group: "Tags", keyIf: inRow, tagVerb: true, write: true, footLabel: "rename",
			desc: "give a tag a new name. A name that exists offers a merge instead. In the Tags view tab moves into the detail pane and enter or e edits the name in place, on a tag row of the bar e does this. Same as crm tag rename",
			run:  func(m *Model) tea.Cmd { m.askTagRename(m.cursorTagName()); return nil }},
		{id: "tag-merge", title: "Merge tag into another", group: "Tags", keys: []string{"m"}, keyIf: inRow, tagVerb: true, write: true, footLabel: "merge",
			desc: "move every record of this tag to another tag and remove this one (y/n). In the Tags view m merges the selected tags into the highlighted one. Same as crm tag merge",
			run:  func(m *Model) tea.Cmd { return m.mergeTagVerb() }},
		{id: "tag-merge-marked", title: "Merge selected tags into the highlighted one", group: "Tags", write: true,
			desc: "merge the space-selected tags into the highlighted tag, with nothing selected pick the tag to merge into (m in the Tags view). The count is typed back for several tags. One undo step",
			when: whenTagList, run: func(m *Model) tea.Cmd { return m.mergeTagVerb() }},
		{id: "tags-delete", title: "Delete tag", group: "Tags", hint: "d", keys: []string{"d"}, tagVerb: true, write: true,
			desc: "delete the tag from every record (y/n), or the selected tags (the count typed back). An unused tag name is removed. ctrl+z puts it back. Same as crm tag delete",
			foot: 46, footIn: inList, footLabel: "delete", footIf: func(m *Model) bool { return m.view == viewTags && !m.onTagPage() },
			when: func(m *Model) bool { return m.barTag != "" || whenTagList(m) },
			run:  func(m *Model) tea.Cmd { return m.deleteTagsVerb() }},
		{id: "tag-link", title: "Give tag to an object", group: "Tags", hint: "l", keys: []string{"l"}, tagVerb: true, write: true,
			desc: "pick an object of any kind to give the tag (or every selected tag). On a tag page this links another object to the page's tag. ctrl+z undoes it",
			foot: 60, footIn: inList, footLabel: "link", footIf: func(m *Model) bool { return m.view == viewTags },
			when: whenTagVerb, run: func(m *Model) tea.Cmd { return m.tagLinkVerb() }},
		{id: "tag-unlink", title: "Take tag off an object", group: "Tags", hint: "u", keys: []string{"u"}, tagVerb: true, write: true,
			desc: "on a tag page take the tag off the highlighted or selected objects, elsewhere pick an object that has it. ctrl+z undoes it",
			foot: 61, footIn: inList, footLabel: "unlink", footIf: func(m *Model) bool { return m.view == viewTags },
			when: whenTagVerb, run: func(m *Model) tea.Cmd { return m.tagUnlinkVerb() }},
		{id: "tag-merge-duplicates", title: "Tags: merge case duplicates", group: "Tags", write: true,
			desc: "merge every group of tags that differ only by case or separators into its most used spelling, for the groups in the list (ctrl+o filter: possible case duplicates). The count is typed back. One undo step. Same as crm tag duplicates --apply",
			when: func(m *Model) bool { return m.view == viewTags && !m.onTagPage() && m.barKind == "" },
			run:  func(m *Model) tea.Cmd { return m.mergeDuplicatesVerb() }},
		{id: "tag-delete-unused", title: "Tags: delete unused tags", group: "Tags", write: true,
			desc: "remove every tag name no object carries. The count is typed back. One undo step. Same as crm tag unused --apply",
			when: func(m *Model) bool { return m.view == viewTags && !m.onTagPage() && m.barKind == "" },
			run:  func(m *Model) tea.Cmd { return m.deleteUnusedVerb() }},
	}
}

// openTagPage is enter on a tag: show its page. On a page it opens the highlighted object.
func (m *Model) openTagPage() tea.Cmd {
	if m.onTagPage() {
		if ref, ok := m.cursorRef(); ok {
			m.openObject(ref.Kind, ref.ID)
		}
		return nil
	}
	name := m.cursorTagName()
	if name == "" {
		return nil
	}
	m.tagPage = name
	m.marked = map[int64]bool{}
	m.cursor, m.offset = 0, 0
	m.report(m.rebuild(false))
	return nil
}

// mergeTagVerb is m: the selected tags merge into the highlighted one. With nothing else selected it
// asks which tag to merge the highlighted one into.
func (m *Model) mergeTagVerb() tea.Cmd {
	cur := m.cursorTagName()
	if cur == "" {
		return nil
	}
	var sources []string
	names := m.bulkTagNames()
	for _, n := range names {
		under := false
		for _, o := range names {
			under = under || tree.IsUnder(n, o) // a tag below another selected tag goes with it
		}
		if n != cur && !under {
			sources = append(sources, n)
		}
	}
	switch len(sources) {
	case 0:
		m.askTagMerge(cur)
	case 1:
		m.confirmTagMerge(sources[0], cur)
	default:
		return m.askTyped(fmt.Sprintf("merge %d tags into #%s", len(sources), cur), bulkWord("merge", len(sources)), func() { m.runTagMerges(sources, cur) })
	}
	return nil
}

// runTagMerges merges several tags into one as a single undo step.
func (m *Model) runTagMerges(sources []string, target string) {
	m.beginGroup()
	for _, s := range sources {
		res, err := m.tagr.MergeTree(m.ctx, s, target)
		if err != nil {
			m.endGroup(fmt.Sprintf("merge tags into #%s", target))
			m.afterWrite()
			m.setStatus(err.Error(), true)
			return
		}
		m.record(res.Label(),
			func() error { return m.tagr.UndoTree(m.ctx, res) },
			func() error { return m.tagr.RedoTree(m.ctx, res) })
	}
	m.endGroup(fmt.Sprintf("merge %d tags into #%s", len(sources), target))
	m.marked = map[int64]bool{}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("%d tags merged into #%s (ctrl+z undoes)", len(sources), target), false)
	m.pendingCmd = m.showToast(fmt.Sprintf("%d tags merged into #%s · ctrl+z undoes", len(sources), target), false)
}

// deleteTagsVerb is d: delete the highlighted or selected tags with the tags below them. One plain tag
// asks y/n. A parent, a virtual parent or several tags show the count of taggings and ask for it typed
// back (patterns/delete-asks-yes-no, bulk-actions-always).
func (m *Model) deleteTagsVerb() tea.Cmd {
	roots := m.tagRoots()
	if len(roots) == 0 {
		return nil
	}
	var tags, taggings int
	for _, r := range roots {
		st, err := m.tagr.SubtreeStat(m.ctx, r)
		if err != nil {
			m.setStatus(err.Error(), true)
			return nil
		}
		tags += st.Tags
		taggings += st.Taggings
	}
	if len(roots) == 1 && tags == 1 {
		if virtual, _ := m.tagr.IsVirtual(m.ctx, roots[0]); !virtual {
			m.askDeleteTag(roots[0])
			return nil
		}
	}
	what := fmt.Sprintf("delete %d tags (%d taggings)", tags, taggings)
	if len(roots) == 1 {
		what = fmt.Sprintf("delete #%s and %d tags below it (%d taggings)", roots[0], tags-1, taggings)
		if v, _ := m.tagr.IsVirtual(m.ctx, roots[0]); v {
			what = fmt.Sprintf("delete the %d tags below #%s (%d taggings)", tags, roots[0], taggings)
		}
	}
	return m.askTyped(what, bulkWord("delete", tags), func() { m.deleteTags(roots) })
}

// askDeleteTag asks y/n before deleting one tag.
func (m *Model) askDeleteTag(name string) {
	s, err := m.tagStat(name)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	prompt := fmt.Sprintf("delete tag #%s from %d records? y/n", name, s.Count+s.Archived)
	if s.Unused() {
		prompt = fmt.Sprintf("remove the unused tag #%s? y/n", name)
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{prompt: prompt, yes: func() { m.deleteTags([]string{name}) }}
}

func (m *Model) tagStat(name string) (repo.TagStat, error) {
	all, err := m.tagr.Stats(m.ctx)
	if err != nil {
		return repo.TagStat{}, err
	}
	for _, s := range all {
		if s.Name == name {
			return s, nil
		}
	}
	return repo.TagStat{}, fmt.Errorf("tag %q: %w", name, model.ErrNotFound)
}

// deleteTags deletes tags and everything below them as one undo step: a used subtree is untagged
// everywhere, a subtree no object carries is removed.
func (m *Model) deleteTags(names []string) {
	stats, err := m.tagr.Stats(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.beginGroup()
	for _, n := range names {
		var members []string
		used := false
		for _, s := range stats {
			if tree.InSubtree(s.Name, n) {
				members = append(members, s.Name)
				used = used || s.Count+s.Archived > 0
			}
		}
		if !used && len(members) > 0 {
			res, err := m.tagr.DeleteUnused(m.ctx, false, members...)
			if err != nil {
				m.endGroup("")
				m.afterWrite()
				m.setStatus(err.Error(), true)
				return
			}
			m.record(res.Label(),
				func() error { return m.tagr.UndoDeleteUnused(m.ctx, res) },
				func() error { return m.tagr.RedoDeleteUnused(m.ctx, res) })
			continue
		}
		res, err := m.tagr.DeleteTagTree(m.ctx, n)
		if err != nil {
			m.endGroup("")
			m.afterWrite()
			m.setStatus(err.Error(), true)
			return
		}
		m.record(res.Label(),
			func() error { return m.tagr.UndoDelete(m.ctx, res) },
			func() error { return m.tagr.RedoDelete(m.ctx, res) })
	}
	label := "delete tag #" + names[0]
	if len(names) > 1 {
		label = fmt.Sprintf("delete %d tags", len(names))
	}
	m.endGroup(label)
	m.marked = map[int64]bool{}
	m.afterWrite()
	msg := label[len("delete "):]
	m.setStatus("deleted "+msg+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast("deleted "+msg+" · ctrl+z undoes", false)
}

// objectItems lists the objects of every taggable kind for a picker, each with its kind in front.
func (m *Model) objectItems() []overlayItem {
	var out []overlayItem
	for _, kind := range model.TaggableTypes {
		o := objectByKind(kind)
		if o == nil || o.items == nil {
			continue
		}
		for _, it := range o.items(m) {
			out = append(out, overlayItem{id: fmt.Sprintf("obj:%s:%d", kind, it.ref), label: tagKindWords[kind][0] + ": " + it.label,
				desc: it.desc, value: kind, ref: it.ref, origin: it.label})
		}
	}
	return out
}

// tagLinkVerb is l: pick an object to give the tag (the page's tag, or every selected tag) to.
func (m *Model) tagLinkVerb() tea.Cmd {
	tags := m.bulkTagNames()
	if m.onTagPage() {
		tags = []string{m.tagPage}
	}
	if len(tags) == 0 {
		return nil
	}
	title := "Give #" + tags[0] + " to"
	if len(tags) > 1 {
		title = fmt.Sprintf("Give %d tags to", len(tags))
	}
	items := m.objectItems()
	if len(items) == 0 {
		m.setStatus("there is no object to tag", true)
		return nil
	}
	m.openPicker(title, items, func(m *Model, it overlayItem) {
		m.beginGroup()
		for _, tag := range tags {
			if err := m.hTagApply(m.ctx, it.value, it.ref, tag); err != nil {
				m.endGroup("")
				m.setStatus(err.Error(), true)
				return
			}
		}
		label := "tag " + strings.Join(prefixAll("#", tags), " ") + " on " + it.origin
		m.endGroup(label)
		m.afterWrite()
		m.setStatus("tagged "+it.origin+" "+strings.Join(prefixAll("#", tags), " ")+" (ctrl+z undoes)", false)
		m.pendingCmd = m.showToast("tagged "+it.origin+" · ctrl+z undoes", false)
	})
	return nil
}

func prefixAll(p string, in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = p + s
	}
	return out
}

// tagUnlinkVerb is u. On a tag page it takes the page's tag off the highlighted or selected objects. On a
// list or a bar row it lists the objects that carry the tag(s) and takes the tags off the one picked.
func (m *Model) tagUnlinkVerb() tea.Cmd {
	if m.onTagPage() {
		refs := m.bulkRefs()
		switch len(refs) {
		case 0:
			return nil
		case 1:
			m.unlinkFromPage(refs)
		default:
			return m.askTyped(fmt.Sprintf("take #%s off %d objects", m.tagPage, len(refs)), bulkWord("unlink", len(refs)), func() { m.unlinkFromPage(refs) })
		}
		return nil
	}
	tags := m.bulkTagNames()
	if len(tags) == 0 {
		return nil
	}
	var items []overlayItem
	seen := map[string]bool{}
	for _, tag := range tags {
		objs, err := m.tagr.TaggedObjects(m.ctx, tag)
		if err != nil {
			continue
		}
		for _, o := range objs {
			id := fmt.Sprintf("obj:%s:%d", o.Kind, o.ID)
			if seen[id] {
				continue
			}
			seen[id] = true
			items = append(items, overlayItem{id: id, label: tagKindWords[o.Kind][0] + ": " + o.Name, desc: o.Detail, value: o.Kind, ref: o.ID, origin: o.Name})
		}
	}
	if len(items) == 0 {
		m.setStatus("no object carries "+strings.Join(prefixAll("#", tags), " "), true)
		return nil
	}
	m.openPicker("Take "+strings.Join(prefixAll("#", tags), " ")+" off", items, func(m *Model, it overlayItem) {
		m.beginGroup()
		for _, tag := range tags {
			if m.hasTag(m.ctx, it.value, it.ref, tag) {
				if err := m.hTagRemove(m.ctx, it.value, it.ref, tag); err != nil {
					m.endGroup("")
					m.setStatus(err.Error(), true)
					return
				}
			}
		}
		m.endGroup("untag " + it.origin)
		m.afterWrite()
		m.setStatus("took "+strings.Join(prefixAll("#", tags), " ")+" off "+it.origin+" (ctrl+z undoes)", false)
		m.pendingCmd = m.showToast("untagged "+it.origin+" · ctrl+z undoes", false)
	})
	return nil
}

// unlinkFromPage takes the open page's tag off objects as one undo step.
func (m *Model) unlinkFromPage(refs []repo.Ref) {
	tag := m.tagPage
	m.beginGroup()
	for _, r := range refs {
		if err := m.hTagRemove(m.ctx, r.Kind, r.ID, tag); err != nil {
			m.endGroup("")
			m.afterWrite()
			m.setStatus(err.Error(), true)
			return
		}
	}
	label := fmt.Sprintf("take #%s off %d objects", tag, len(refs))
	if len(refs) == 1 {
		label = "take #" + tag + " off " + m.refName(refs[0])
	}
	m.endGroup(label)
	m.marked = map[int64]bool{}
	m.afterWrite()
	m.setStatus(label+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(label+" · ctrl+z undoes", false)
}

// mergeDuplicatesVerb is the palette action Tags: merge case duplicates. It acts on the groups that have a
// tag in the list, shows the count and asks for it typed back.
func (m *Model) mergeDuplicatesVerb() tea.Cmd {
	var names []string
	for _, r := range m.rows {
		if !r.skip && r.tag != "" {
			names = append(names, r.tag)
		}
	}
	if len(names) == 0 {
		m.setStatus("no tags in the list", true)
		return nil
	}
	dry, err := m.tagr.MergeDuplicates(m.ctx, true, names...)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	if dry.Merged == 0 {
		m.setStatus("no tags differ only by case or separators", false)
		return nil
	}
	return m.askTyped(fmt.Sprintf("merge %d tags in %d groups into their most used spelling", dry.Merged, dry.Groups), bulkWord("merge", dry.Merged), func() {
		res, err := m.tagr.MergeDuplicates(m.ctx, false, names...)
		if err != nil && res == nil {
			m.setStatus(err.Error(), true)
			return
		}
		if res != nil && res.Merged > 0 {
			m.record(res.Label(),
				func() error { return m.tagr.UndoDuplicates(m.ctx, res) },
				func() error { return m.tagr.RedoDuplicates(m.ctx, res) })
		}
		m.marked = map[int64]bool{}
		m.afterWrite()
		if err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.setStatus(fmt.Sprintf("%d duplicate tags merged (ctrl+z undoes)", res.Merged), false)
		m.pendingCmd = m.showToast(fmt.Sprintf("%d duplicate tags merged · ctrl+z undoes", res.Merged), false)
	})
}

// deleteUnusedVerb is the palette action Tags: delete unused tags.
func (m *Model) deleteUnusedVerb() tea.Cmd {
	dry, err := m.tagr.DeleteUnused(m.ctx, true)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	if dry.Count == 0 {
		m.setStatus("there are no unused tags", false)
		return nil
	}
	return m.askTyped(fmt.Sprintf("delete %d unused tags", dry.Count), bulkWord("delete", dry.Count), func() {
		res, err := m.tagr.DeleteUnused(m.ctx, false)
		if err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.record(res.Label(),
			func() error { return m.tagr.UndoDeleteUnused(m.ctx, res) },
			func() error { return m.tagr.RedoDeleteUnused(m.ctx, res) })
		m.marked = map[int64]bool{}
		m.afterWrite()
		m.setStatus(fmt.Sprintf("%d unused tags deleted (ctrl+z undoes)", res.Count), false)
		m.pendingCmd = m.showToast(fmt.Sprintf("%d unused tags deleted · ctrl+z undoes", res.Count), false)
	})
}

// dupPos is the place of a spelling in its group: 0 for the most used one.
func dupPos(g repo.TagDupGroup, name string) int {
	for i, t := range g.Tags {
		if t.Name == name {
			return i
		}
	}
	return len(g.Tags)
}
