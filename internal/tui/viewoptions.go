package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/dedupe"
)

// The view options overlay (ctrl+o). Sort, the Archived type filter and the Merge minimum level are
// one overlay shared by every list view: the same keys and the same drawing, with the options each
// view declares in viewOptionsFor (patterns/one-abstraction-many-contexts). A view that declares no
// options still opens it and says so. up and down pick an option, left, right, space and enter change
// its value, esc closes.

// optChoice is one value of an option.
type optChoice struct{ value, label string }

// viewOption is one setting of a view: its choices, how to read the current one and how to apply one.
type viewOption struct {
	id, label string
	choices   []optChoice
	get       func(m *Model) string
	set       func(m *Model, value string)
}

// viewOptionsFor lists the options of a view, in the order the overlay shows them.
func viewOptionsFor(v viewKind) []viewOption {
	switch v {
	case viewPeople, viewOrgs, viewDeals, viewTasks:
		return []viewOption{sortOption(v), orderOption(v)}
	case viewTags:
		return tagViewOptions()
	case viewMerge:
		return []viewOption{{
			id: "level", label: "Minimum level",
			choices: []optChoice{{"fuzzy", "fuzzy (everything)"}, {"low", "low and above"}, {"medium", "medium and above"}, {"high", "high only"}},
			get:     func(m *Model) string { return strings.ToLower(m.minLevel.String()) },
			set: func(m *Model, value string) {
				if l, err := dedupe.ParseLevel(value); err == nil {
					m.minLevel = l
				}
			},
		}}
	case viewDates:
		return []viewOption{{
			id: "show", label: "Show",
			choices: []optChoice{{"all", "birthdays and anniversaries"}, {"birthdays", "birthdays only"}, {"anniversaries", "anniversaries only"}},
			get: func(m *Model) string {
				if m.datesShow == "" {
					return "all"
				}
				return m.datesShow
			},
			set: func(m *Model, value string) { m.datesShow = value },
		}}
	case viewArchived:
		return []viewOption{{
			id: "type", label: "Type",
			choices: []optChoice{{"", "all types"}, {"person", "people"}, {"organization", "organizations"}, {"deal", "deals"}, {"task", "tasks"}, {"interaction", "notes"}},
			get:     func(m *Model) string { return m.archKind },
			set:     func(m *Model, value string) { m.archKind = value },
		}}
	}
	return nil
}

// sortChoices are the sorts of a list, its default first: name, plus when it changed last (People,
// Organizations), when it was created (Deals) or when it is due (Tasks).
func sortChoices(v viewKind) []optChoice {
	name := optChoice{"name", "name"}
	var other optChoice
	switch v {
	case viewDeals:
		other = optChoice{"recent", "recently created"}
	case viewTasks:
		other = optChoice{"due", "due date"}
	default:
		other = optChoice{"recent", "recently updated"}
	}
	if defaultSort(v) == "name" {
		return []optChoice{name, other}
	}
	return []optChoice{other, name}
}

// sortOption is the sort of a list: by name, or by a date that depends on the view (sortChoices).
func sortOption(v viewKind) viewOption {
	choices := sortChoices(v)
	return viewOption{
		id: "sort", label: "Sort", choices: choices,
		get: func(m *Model) string { return m.sortOf(v) },
		set: func(m *Model, value string) {
			if m.sortChoice == nil {
				m.sortChoice = map[viewKind]string{}
			}
			m.sortChoice[v] = value
		},
	}
}

// orderOption is the direction of the sort of a view: ascending or descending.
func orderOption(v viewKind) viewOption {
	return viewOption{
		id: "order", label: "Order",
		choices: []optChoice{{"asc", "ascending"}, {"desc", "descending"}},
		get:     func(m *Model) string { return m.orderOf(v) },
		set: func(m *Model, value string) {
			if m.orderChoice == nil {
				m.orderChoice = map[viewKind]string{}
			}
			m.orderChoice[v] = value
		},
	}
}

// orderOf is the direction of a view: "asc" or "desc". Name ascending is A to Z. Recently updated
// ascending is oldest first and number of objects ascending is fewest first. Until the user sets the
// order of a view it is the natural one of the current sort: ascending for name, descending for
// recent and count (newest first, most first, which is how those lists were always ordered) and
// ascending for due (soonest first).
func (m *Model) orderOf(v viewKind) string {
	if o, ok := m.orderChoice[v]; ok {
		return o
	}
	return naturalOrder(m.sortOf(v))
}

// naturalOrder is the direction a sort has until the user changes it.
func naturalOrder(sort string) string {
	if sort == "name" || sort == "due" {
		return "asc"
	}
	return "desc"
}

// flipped says whether the user's order runs against the natural order of the sort, so a list built
// in the natural order (A to Z, newest first, most first) must be turned around.
func (m *Model) flipped(v viewKind) bool {
	return m.orderOf(v) != naturalOrder(m.sortOf(v))
}

// defaultSort is the order a list has until the view options change it. People are listed by name,
// organizations by when they changed last.
func defaultSort(v viewKind) string {
	switch v {
	case viewOrgs, viewDeals:
		return "recent"
	case viewTasks:
		return "due"
	}
	return "name"
}

// sortOf is the sort of a view: "name" or "recent".
func (m *Model) sortOf(v viewKind) string {
	if s, ok := m.sortChoice[v]; ok {
		return s
	}
	return defaultSort(v)
}

// sortedRecent says whether the view sorts by last update instead of by name.
func (m *Model) sortedRecent(v viewKind) bool { return m.sortOf(v) == "recent" }

func (o viewOption) choiceLabel(value string) string {
	for _, c := range o.choices {
		if c.value == value {
			return c.label
		}
	}
	return value
}

// next is the choice after the current one (or before it with back).
func (o viewOption) next(cur string, back bool) string {
	if len(o.choices) == 0 {
		return cur
	}
	idx := 0
	for i, c := range o.choices {
		if c.value == cur {
			idx = i
		}
	}
	if back {
		idx += len(o.choices) - 1
	} else {
		idx++
	}
	return o.choices[idx%len(o.choices)].value
}

func (m *Model) viewOptionItems(opts []viewOption) []overlayItem {
	if len(opts) == 0 {
		return []overlayItem{{id: "none", label: "No options for the " + viewNames[m.view] + " view", disabled: "nothing to set"}}
	}
	items := make([]overlayItem, 0, len(opts))
	for _, o := range opts {
		var names []string
		for _, c := range o.choices {
			names = append(names, c.label)
		}
		// desc lists the choices for the filter (searchable, not drawn): the row shows the current value.
		items = append(items, overlayItem{id: "opt:" + o.id, label: o.label, current: o.current(m), desc: strings.Join(names, " | "), selector: true, value: o.id})
	}
	return items
}

func (o viewOption) current(m *Model) string { return o.choiceLabel(o.get(m)) }

// openViewOptions shows the overlay for the current view.
func (m *Model) openViewOptions() tea.Cmd {
	opts := m.optionsForView()
	view := m.view
	ov := m.newOverlay("View options, "+viewNames[view], m.viewOptionItems(opts), false)
	ov.noFilter = true
	ov.onKey = func(m *Model, key string) bool {
		var back bool
		switch key {
		case "left":
			back = true
		case "right", " ", "enter":
		default:
			return false
		}
		if ov.cursor < 0 || ov.cursor >= len(ov.shown) || len(opts) == 0 {
			return true
		}
		id := ov.items[ov.shown[ov.cursor]].value
		for _, o := range opts {
			if o.id != id {
				continue
			}
			o.set(m, o.next(o.get(m), back))
			m.cursor, m.offset = 0, 0
			m.report(m.rebuild(false))
			ov.items = m.viewOptionItems(opts)
			ov.refilter()
			for i, idx := range ov.shown {
				if ov.items[idx].value == id {
					ov.cursor = i
				}
			}
			m.setStatus(strings.ToLower(o.label)+": "+o.current(m), false)
		}
		return true
	}
	return m.showOverlay(ov)
}

// viewOptionsAction is the one ctrl+o action. It is a framework action: every list view has it.
func viewOptionsAction() action {
	return action{id: "options", title: "View options", group: "Views", hint: "ctrl+o", keys: []string{"ctrl+o"},
		desc: "sort, filter and group settings of this view: Sort in People and Organizations, Minimum level in Merge, Type in Archived, Show in Dates. One overlay for every list view, left and right change a value",
		foot: 50, footIn: inList, footLabel: hwOptions, footIf: func(m *Model) bool { return len(m.optionsForView()) > 0 }, when: whenList,
		run: func(m *Model) tea.Cmd { return m.openViewOptions() }}
}
