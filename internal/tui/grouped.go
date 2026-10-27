package tui

// Grouped lists with dividers (patterns/grouped-lists-with-dividers). A list view declares the field it
// groups by in viewGroupFields, and builds its rows with groupRows: one divider row appears whenever
// the group of the next item changes. The Birthdays view groups by day (under a month heading), the
// Merge view by confidence level. Any other view gets the same dividers by declaring a field and
// calling groupRows with a heads function for it.
//
// A divider is a row with skip set, so the cursor never lands on it and no verb acts on it. A divider
// with rule set is drawn as a rule: its title at the left, a line, its meta at the right edge (the same
// rendering as the Birthdays day dividers). One without rule is drawn as a plain heading. Selection and
// bulk verbs apply across groups because they only look at the selectable rows.

// viewGroupFields is the field each grouped view groups its rows by.
var viewGroupFields = map[viewKind]string{
	viewBirthdays: "day",
	viewMerge:     "confidence level",
}

// groupField is the field the view groups by, "" when the view is not grouped.
func groupField(v viewKind) string { return viewGroupFields[v] }

// groupHead is one divider above a run of rows. key says when the group changes, title and meta are
// the divider's text, rule picks the rule drawing.
type groupHead struct {
	key   string
	title string
	meta  string
	rule  bool
}

// groupedRows is the result of groupRows: the rows with dividers, and for each input item the index
// of its row and of the divider rows above it (outermost first).
type groupedRows struct {
	rows    []row
	at      []int
	headsAt [][]int
}

// groupRows builds a list: for each item heads gives its dividers from the outermost to the innermost
// group, row gives the row. A divider is added for every level whose key differs from the previous
// item, and an outer change always starts the inner levels again.
func groupRows[T any](items []T, heads func(T) []groupHead, rowOf func(T) row) groupedRows {
	var out groupedRows
	var prev []groupHead
	for _, it := range items {
		hs := heads(it)
		changed := false
		var idx []int
		for lvl, h := range hs {
			if changed || lvl >= len(prev) || prev[lvl].key != h.key {
				changed = true
				out.rows = append(out.rows, row{title: h.title, meta: h.meta, skip: true, rule: h.rule})
				idx = append(idx, len(out.rows)-1)
			} else if len(out.headsAt) > 0 {
				idx = append(idx, out.headsAt[len(out.headsAt)-1][lvl])
			}
		}
		prev = hs
		out.rows = append(out.rows, rowOf(it))
		out.at = append(out.at, len(out.rows)-1)
		out.headsAt = append(out.headsAt, idx)
	}
	return out
}
