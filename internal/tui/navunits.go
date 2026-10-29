package tui

import (
	"strings"
	"unicode"
)

// Navigation units and home destinations (patterns/navigation-units-and-homes).
//
// Every view declares, here and nowhere else, how far ctrl+up and ctrl+down jump and where
// ctrl+shift+up and ctrl+shift+down go. A new view inherits the default (a page, and the first or
// last row) and gets its own behavior by adding one entry to viewNavs.
//
//	ctrl+up, ctrl+down              jump by the view's unit: a day, a first-letter group, a page
//	ctrl+shift+up, ctrl+shift+down  the view's home destination when it declares one (today in
//	                                Birthdays), else the first row and the last row like home and end
//	home, end                       the first and last row in every view and always work
//
// Bubble Tea reports ctrl+shift+up and ctrl+shift+down only when the terminal sends the xterm
// modifier sequences (CSI 1;6A and 1;6B). Where it does not, home and end are the fallback.

// jumpUnit is the step of ctrl+up and ctrl+down.
type jumpUnit int

const (
	unitPage   jumpUnit = iota // a page, the default
	unitGroup                  // the next or previous group divider: a day in Birthdays, a confidence level in Merge
	unitLetter                 // the next or previous first-letter group (People, Organizations)
)

// viewNav is what a view declares about navigation.
type viewNav struct {
	unit jumpUnit
	// home moves the cursor to the view's home destination, such as today. nil means the view has none.
	home func(m *Model)
}

// viewNavs holds the declarations. A view that is not listed uses a page and has no home.
var viewNavs = map[viewKind]viewNav{
	viewBirthdays: {unit: unitGroup, home: func(m *Model) { m.snapBirthdays() }},
	viewMerge:     {unit: unitGroup},
	viewSettings:  {unit: unitGroup},
	viewPeople:    {unit: unitLetter},
	viewOrgs:      {unit: unitLetter},
}

func navOf(v viewKind) viewNav { return viewNavs[v] }

// jump moves by the unit the current view declares. The detail pane and the compare screen move
// by a page, because they have no groups.
func (m *Model) jump(dir int) {
	page := max(1, m.bodyHeight()-1)
	if m.cmp != nil || m.focus != focusList {
		m.step(dir * page)
		return
	}
	var starts []int
	switch navOf(m.view).unit {
	case unitGroup:
		starts = m.groupStarts()
	case unitLetter:
		if m.view == viewPeople && m.sortedRecent(viewPeople) {
			m.move(dir * page) // groups by letter mean nothing in the recently updated order
			return
		}
		starts = m.letterStarts()
	default:
		m.move(dir * page)
		return
	}
	if len(starts) == 0 {
		m.move(dir * page)
		return
	}
	target := -1
	if dir > 0 {
		for _, s := range starts {
			if s > m.cursor {
				target = s
				break
			}
		}
		if target < 0 {
			target = m.lastSelectable()
		}
	} else {
		for i := len(starts) - 1; i >= 0; i-- {
			if starts[i] < m.cursor {
				target = starts[i]
				break
			}
		}
		if target < 0 {
			target = starts[0]
		}
	}
	m.cursor = target
	m.clamp()
	m.refreshDetail(false)
}

// goHome is ctrl+shift+up and ctrl+shift+down: the view's home destination, else the first row
// (dir < 0) or the last row (dir > 0).
func (m *Model) goHome(dir int) {
	if h := navOf(m.view).home; h != nil && m.cmp == nil && m.focus == focusList {
		h(m)
		m.clamp()
		m.refreshDetail(false)
		return
	}
	m.step(dir * (1 << 20))
}

func (m *Model) lastSelectable() int {
	for i := len(m.rows) - 1; i >= 0; i-- {
		if !m.rows[i].skip {
			return i
		}
	}
	return m.cursor
}

// groupStarts lists the first selectable row under each group divider (a rule row).
func (m *Model) groupStarts() []int {
	var out []int
	for i, r := range m.rows {
		if !r.rule {
			continue
		}
		for j := i + 1; j < len(m.rows); j++ {
			if m.rows[j].rule {
				break
			}
			if !m.rows[j].skip {
				out = append(out, j)
				break
			}
		}
	}
	return out
}

// groupLetter is the first-letter group of a row title: an upper case letter, or # for anything else.
func groupLetter(title string) string {
	for _, r := range title {
		if unicode.IsLetter(r) {
			return string(unicode.ToUpper(r))
		}
		if unicode.IsDigit(r) {
			break
		}
		if !unicode.IsSpace(r) {
			break
		}
	}
	return "#"
}

// letterStarts lists the first selectable row of each first-letter group.
func (m *Model) letterStarts() []int {
	var out []int
	prev := ""
	for i, r := range m.rows {
		if r.skip {
			continue
		}
		l := groupLetter(strings.TrimSpace(r.title))
		if l != prev {
			out = append(out, i)
			prev = l
		}
	}
	return out
}
