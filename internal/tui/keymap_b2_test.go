package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/db/repo"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/modules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Keymap part B2 (owner decisions 2026-10-28): cleanup of the keys, the grouped Merge list, bulk
// actions, the generic purge, copy and open, tags in the add picker.

// keyCmd presses one key and returns the command the model answered with.
func keyCmd(m *Model, k string) tea.Cmd {
	_, cmd := m.Update(keyPress(k))
	return cmd
}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// --- 1. the removed, moved and kept keys ---

func TestRemovedKeysDoNothing(t *testing.T) {
	m, _ := newTestModel(t)
	// ctrl+n, ctrl+p and ":" are gone.
	press(m, "ctrl+n")
	assert.Equal(t, 0, m.cursor)
	press(m, "down", "ctrl+p")
	assert.Equal(t, 1, m.cursor, "ctrl+p no longer moves up")
	press(m, ":")
	assert.Equal(t, modeNormal, m.mode, ": no longer opens the bar")
	press(m, "/")
	assert.Equal(t, modeOverlay, m.mode, "/ still does")
	press(m, "ctrl+n") // inside the bar it is not a down key either
	press(m, "esc")
	press(m, "ctrl+k")
	assert.Equal(t, modeOverlay, m.mode, "ctrl+k still opens the bar")
	press(m, "esc")

	for _, id := range []string{"down", "up"} {
		assert.Equal(t, []string{id}, m.actionByID(id).keys)
	}
	assert.Equal(t, []string{"ctrl+k", "/"}, m.actionByID("palette").keys)
}

func TestKeysMovedToThePaletteHaveNoKeyAndStayInThePalette(t *testing.T) {
	m, _ := newTestModel(t)
	for _, id := range []string{"cadence", "cmp-pair", "cmp-view", "modules-reset"} {
		a := m.actionByID(id)
		require.NotNil(t, a, id)
		assert.Empty(t, a.keys, id)
		assert.NotEmpty(t, a.title, id)
	}
	// F no longer opens the cadence picker, the palette entry does.
	press(m, "F")
	assert.Equal(t, modeNormal, m.mode)
	runID(m, "cadence")
	assert.Equal(t, modeOverlay, m.mode)
	press(m, "esc")
	// modules reset is a palette action that puts the default arrangement back.
	m.env.DataDir = t.TempDir()
	require.NoError(t, m.setModules(modules.Arrangement{Order: []string{"people", "home", "follow-up", "tasks", "birthdays", "merge", "archived", "sync", "organizations", "deals"}}))
	require.False(t, m.mods.IsDefault())
	runID(m, "modules-reset")
	assert.True(t, m.mods.IsDefault())
}

func TestKeptKeysStillWork(t *testing.T) {
	m, _ := dupeModel(t)
	// Digits, [ ], v and the confirm answers stay.
	press(m, "2")
	assert.Equal(t, viewPeople, m.view)
	press(m, "]")
	assert.Equal(t, viewDue, m.view)
	press(m, "[")
	assert.Equal(t, viewPeople, m.view)
	press(m, "v")
	require.NotNil(t, m.ov)
	press(m, "esc")
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	press(m, "n")
	assert.Equal(t, modeNormal, m.mode)
	// Half pages keep ctrl+d and ctrl+u.
	assert.Equal(t, []string{"ctrl+d"}, m.actionByID("half-down").keys)
	assert.Equal(t, []string{"ctrl+u"}, m.actionByID("half-up").keys)
	// ctrl+e changes the survivor, m merges, d dismisses in the compare screen.
	press(m, "8", "enter")
	require.NotNil(t, m.cmp)
	press(m, "ctrl+e")
	assert.Equal(t, 1, m.cmp.s)
	assert.Equal(t, []string{"ctrl+e"}, m.actionByID("cmp-survivor").keys)
	assert.Equal(t, []string{"m"}, m.actionByID("cmp-apply").keys)
	assert.Equal(t, []string{"d"}, m.actionByID("cmp-dismiss").keys)
}

func TestCtrlArrowsAreAliasesOfTheViewSwitchKeys(t *testing.T) {
	m, _ := newTestModel(t)
	assert.Equal(t, []string{"]", "ctrl+right"}, m.actionByID("next-view").keys)
	assert.Equal(t, []string{"[", "ctrl+left"}, m.actionByID("prev-view").keys)
	assert.Equal(t, "ctrl+right", keyName(keyPress("ctrl+right")))
	start := m.view
	press(m, "ctrl+right")
	assert.NotEqual(t, start, m.view)
	next := m.view
	press(m, "ctrl+left")
	assert.Equal(t, start, m.view)
	press(m, "]")
	assert.Equal(t, next, m.view, "same as ]")
	// No view uses ctrl+left or ctrl+right for a jump unit: jumps stay on ctrl+up and ctrl+down.
	for _, a := range m.actions {
		if a.id == "next-view" || a.id == "prev-view" {
			continue
		}
		assert.False(t, a.hasKey("ctrl+left") || a.hasKey("ctrl+right"), "%s", a.id)
	}
	assert.Equal(t, "ctrl+up", m.actionByID("jump-up").keys[0])
}

func TestTextViewerAndModulesScreenLostTheirExtraKeys(t *testing.T) {
	m, _ := newTestModel(t)
	m.env.DBPath = "test.db"
	runID(m, "where")
	require.NotNil(t, m.tv)
	press(m, "q")
	assert.NotNil(t, m.tv, "q no longer closes the text viewer")
	press(m, "esc")
	assert.Nil(t, m.tv)

	m.env.DataDir = t.TempDir()
	press(m, "v")
	require.NotNil(t, m.ov)
	press(m, "down", "K")
	assert.Equal(t, "home", m.mods.Normalize().Order[0], "K no longer moves a module")
	press(m, "J")
	assert.Equal(t, "home", m.mods.Normalize().Order[0])
	press(m, "shift+down") // shift+up and shift+down still reorder inside the Modules screen
	assert.Equal(t, "follow-up", m.mods.Normalize().Order[1], "the row under the cursor (People) moved down")
	press(m, "esc")
}

// --- 2. grouped lists and the view options overlay ---

func TestGroupedListHelperInsertsADividerWhenTheGroupChanges(t *testing.T) {
	type item struct{ g, name string }
	items := []item{{"a", "1"}, {"a", "2"}, {"b", "3"}, {"c", "4"}, {"c", "5"}}
	g := groupRows(items, func(i item) []groupHead {
		return []groupHead{{key: i.g, title: strings.ToUpper(i.g), meta: "group " + i.g, rule: true}}
	}, func(i item) row { return row{title: i.name} })
	var got []string
	for _, r := range g.rows {
		if r.skip {
			assert.True(t, r.rule)
			got = append(got, "["+r.title+"|"+r.meta+"]")
		} else {
			got = append(got, r.title)
		}
	}
	assert.Equal(t, []string{"[A|group a]", "1", "2", "[B|group b]", "3", "[C|group c]", "4", "5"}, got)
	assert.Equal(t, []int{1, 2, 4, 6, 7}, g.at, "the row index of every item")

	// Two levels: an outer change restarts the inner heading.
	g = groupRows(items, func(i item) []groupHead {
		return []groupHead{{key: "all", title: "ALL"}, {key: i.g, title: i.g, rule: true}}
	}, func(i item) row { return row{title: i.name} })
	heads := 0
	for _, r := range g.rows {
		if r.skip && !r.rule {
			heads++
		}
	}
	assert.Equal(t, 1, heads, "the outer heading is drawn once")

	assert.Equal(t, "day", groupField(viewBirthdays))
	assert.Equal(t, "confidence level", groupField(viewMerge))
	assert.Equal(t, "", groupField(viewPeople))
}

func TestOneViewOptionsOverlayServesEveryListView(t *testing.T) {
	m, _ := dupeModel(t)
	assert.Equal(t, []string{"ctrl+o"}, m.actionByID("options").keys)
	for _, id := range []string{"sort", "dupe-level", "archived-type"} {
		assert.Nil(t, m.actionByID(id), "%s is replaced by the one overlay", id)
	}
	for _, k := range []string{"ctrl+l", "ctrl+f"} {
		for _, a := range m.actions {
			assert.False(t, a.hasKey(k), "%s is not a key any more (%s)", k, a.id)
		}
	}
	for _, v := range []viewKind{viewHome, viewPeople, viewDue, viewTasks, viewBirthdays, viewMerge, viewArchived, viewSync, viewOrgs, viewDeals} {
		m.switchView(v)
		press(m, "ctrl+o")
		require.Equal(t, modeOverlay, m.mode, viewNames[v])
		assert.Contains(t, m.ov.title, "View options, "+viewNames[v])
		opts := viewOptionsFor(v)
		if len(opts) == 0 {
			assert.Contains(t, m.ov.items[0].label, "No options", viewNames[v])
		} else {
			assert.Len(t, m.ov.items, len(opts), viewNames[v])
		}
		press(m, "esc")
		assert.Equal(t, modeNormal, m.mode)
	}
	// The options each view declares.
	ids := func(v viewKind) []string {
		var out []string
		for _, o := range viewOptionsFor(v) {
			out = append(out, o.id)
		}
		return out
	}
	assert.Equal(t, []string{"sort"}, ids(viewPeople))
	assert.Equal(t, []string{"sort"}, ids(viewOrgs))
	assert.Equal(t, []string{"level"}, ids(viewMerge))
	assert.Equal(t, []string{"type"}, ids(viewArchived))
}

func TestViewOptionsSortOrganizationsAndFilterArchivedByType(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	a, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	b, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Bolt"})
	_, err := m.db.Exec(`UPDATE organizations SET updated_at = '2099-01-01 00:00:00' WHERE id = ?`, b.ID)
	require.NoError(t, err)
	_ = a
	m.switchView(viewOrgs)
	assert.Equal(t, "Bolt", m.rows[0].title, "organizations start recently updated first")
	press(m, "ctrl+o", "right", "esc")
	assert.Equal(t, "Acme", m.rows[0].title, "the other choice sorts by name")
	assert.False(t, m.sortedRecent(viewPeople) && !m.sortedRecent(viewOrgs), "the People sort is a separate setting")
	assert.Equal(t, "name", m.sortOf(viewPeople))

	// Archived: the type option.
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "organization", a.ID))
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "person", 3))
	m.switchView(viewArchived)
	assert.Len(t, m.rows, 2)
	press(m, "ctrl+o", "right", "esc") // people
	assert.Equal(t, "person", m.archKind)
	require.Len(t, m.rows, 1)
	assert.Equal(t, "person", m.rows[0].akind)
}

// --- 3. selection, cells, compare ---

func TestCtrlASelectsAllAndShiftArrowsExtendTheSelection(t *testing.T) {
	m, _ := newTestModel(t)
	assert.Equal(t, []string{"ctrl+a"}, m.actionByID("select-all").keys)
	assert.Nil(t, m.actionByID("mark-all"), "there is no mark all visible action")
	press(m, "*")
	assert.Empty(t, m.marked, "* is gone")
	press(m, "ctrl+a")
	assert.Len(t, m.markedIDs(), 3)
	press(m, "ctrl+a")
	assert.Empty(t, m.markedIDs())

	// shift+down selects the row and the next, again extends by one more row.
	press(m, "shift+down")
	assert.Equal(t, 1, m.cursor)
	assert.Len(t, m.markedIDs(), 2)
	press(m, "shift+down")
	assert.Equal(t, 2, m.cursor)
	assert.Len(t, m.markedIDs(), 3)
	// Back over a row takes it out again.
	press(m, "shift+up")
	assert.Equal(t, 1, m.cursor)
	assert.Len(t, m.markedIDs(), 2)
	// Any other key ends the extension; a new one starts from the cursor and keeps what was selected.
	press(m, "down")
	press(m, "shift+down")
	assert.Equal(t, 2, m.cursor)
	assert.Len(t, m.markedIDs(), 3, "the first extension stays, the new one adds the row")
	press(m, "ctrl+a") // everything is selected, so this clears
	assert.Empty(t, m.markedIDs())

	// The View shows the check marks.
	press(m, "shift+down")
	assert.Contains(t, m.screen(), "✓")
}

func TestShiftArrowsStillReorderInsideTheModulesScreen(t *testing.T) {
	m, _ := newTestModel(t)
	m.env.DataDir = t.TempDir()
	press(m, "v")
	require.NotNil(t, m.ov)
	press(m, "down", "shift+up")
	assert.Equal(t, "people", m.mods.Normalize().Order[0], "shift+up moved People above Home")
	assert.Empty(t, m.marked, "the screen's own scope took the key, nothing was selected")
	press(m, "esc")
}

func TestLinkCellsAreFieldsEditedWithEnterAndMovedAcrossWithArrows(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	acme, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	_, err = m.links.Link(ctx, personRef(1), repo.Ref{Kind: "organization", ID: acme.ID}, model.LinkAttrs{Role: sp("CTO")})
	require.NoError(t, err)
	m.afterWrite()
	press(m, "tab")
	goTo(t, m, "organization")
	row := m.form[m.fcur]
	require.Len(t, row.cells, 5)
	assert.Equal(t, 0, m.fcol)

	// right then tab walk the cells, tab from the last one goes back to the list.
	for i := 1; i <= 5; i++ {
		press(m, "right")
		assert.Equal(t, i, m.fcol)
	}
	press(m, "right")
	assert.Equal(t, 5, m.fcol, "right stops at the last cell")
	press(m, "tab")
	assert.Equal(t, focusList, m.focus, "tab from the last cell returns to the list like before")
	press(m, "tab")
	goTo(t, m, "organization")
	assert.Equal(t, 0, m.fcol, "the cursor starts on the row")
	assert.Equal(t, focusDetail, m.focus)

	// A wrong date keeps the editor open with a message, esc cancels, the link is unchanged.
	press(m, "right", "right")
	press(m, "enter")
	require.Equal(t, modeEdit, m.mode)
	replaceText(m, "not a date")
	press(m, "enter")
	assert.Equal(t, modeEdit, m.mode, "a bad value keeps the editor open")
	assert.True(t, m.statusIsErr)
	press(m, "esc")
	assert.Nil(t, linkedOrgs(t, m, 1)[0].StartedOn)

	// enter on the name cell still opens the linked object inside the app.
	press(m, "left", "left")
	press(m, "enter")
	assert.Equal(t, viewOrgs, m.view)
	assert.Equal(t, acme.ID, m.selOrg())
}

func TestDealAndKindCellsEditTheDealAndTheRelationKind(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	deal, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Launch", Stage: "lead", PersonID: ptr64(1)})
	require.NoError(t, err)
	m.afterWrite()
	press(m, "tab")
	goTo(t, m, "deal")
	var keysSeen []string
	for _, c := range m.form[m.fcur].cells {
		keysSeen = append(keysSeen, c.key)
	}
	assert.Equal(t, []string{"role", "stage", "value"}, keysSeen)
	setCell(t, m, 2, "proposal")
	d, err := m.dlr.FindByID(ctx, deal.ID)
	require.NoError(t, err)
	assert.Equal(t, "proposal", d.Stage, "the stage cell edits the deal")
	setCell(t, m, 3, "$12,500")
	d, _ = m.dlr.FindByID(ctx, deal.ID)
	require.NotNil(t, d.Value)
	assert.Equal(t, 12500.0, *d.Value)

	// A person link has its kind as a cell.
	other, err := m.pr.Create(ctx, model.CreatePersonInput{FirstName: "Kim"})
	require.NoError(t, err)
	kind := "friend"
	_, err = m.links.Link(ctx, personRef(1), personRef(other.ID), model.LinkAttrs{Kind: &kind})
	require.NoError(t, err)
	m.afterWrite()
	goTo(t, m, "friend")
	require.Equal(t, "kind", m.form[m.fcur].cells[0].key)
	setCell(t, m, 1, "colleague")
	items, err := m.links.Linked(ctx, personRef(1), "person")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "colleague", items[0].Label)
}

func TestCompareTabCyclesFieldsAndLeftRightChooseTheSide(t *testing.T) {
	m, c := openBobs(t)
	assert.Equal(t, []string{"tab"}, m.actionByID("cmp-field").keys)
	assert.Nil(t, m.actionByID("cmp-pick-back"), "shift+tab is gone from the compare")
	assert.Equal(t, []string{" "}, m.actionByID("cmp-pick").keys, "tab no longer toggles")
	first := c.cur
	press(m, "tab")
	assert.Greater(t, c.cur, first, "tab moves to the next field")
	// From the last field tab goes back to the first.
	for i := 0; i < len(c.rows)+1; i++ {
		press(m, "tab")
	}
	assert.GreaterOrEqual(t, c.cur, 0)
	last := c.cur
	for c.cur >= last {
		prev := c.cur
		press(m, "tab")
		if c.cur <= prev {
			break
		}
		last = c.cur
	}
	assert.Less(t, c.cur, last, "after the last field comes the first")
	// left and right choose the candidate whose value is kept.
	company := rowIndex(c, "company")
	c.cur = company
	press(m, "right")
	assert.Equal(t, 1, c.resolved(company))
	press(m, "left")
	assert.Equal(t, 0, c.resolved(company))
	// shift+tab does nothing in the compare.
	c.cur = company
	press(m, "shift+tab")
	assert.Equal(t, company, c.cur)
	assert.Equal(t, 0, c.resolved(company))
}

// --- 4. copy ---

func TestCtrlCCopiesAFocusedFieldAndOtherwiseQuits(t *testing.T) {
	m, _ := newTestModel(t)
	var copied string
	m.clip = func(s string) { copied = s }
	// In a list ctrl+c still quits.
	assert.True(t, quits(keyCmd(m, "ctrl+c")), "ctrl+c quits from a list")
	// With a field focused it copies and does not quit.
	press(m, "down", "tab")
	goTo(t, m, "email")
	cmd := keyCmd(m, "ctrl+c")
	assert.False(t, quits(cmd), "ctrl+c does not quit while a field is focused")
	assert.Equal(t, "bob@globex.com", copied)
	assert.Contains(t, m.status, "copied")
	// In a prompt it cancels.
	press(m, "esc")
	press(m, "a", "t")
	require.Equal(t, modePrompt, m.mode)
	press(m, "esc")
	// y is not a key any more.
	press(m, "tab")
	goTo(t, m, "email")
	copied = ""
	press(m, "y")
	assert.Empty(t, copied)
	// Copy value and Copy contact as vCard are palette entries.
	for _, id := range []string{"copy", "copy-vcard"} {
		a := m.actionByID(id)
		require.NotNil(t, a, id)
		assert.NotEmpty(t, a.title)
	}
	assert.Equal(t, "Copy value", m.actionByID("copy").title)
	assert.Equal(t, "Copy contact as vCard", m.actionByID("copy-vcard").title)
}

func TestCopyContactAsVCard(t *testing.T) {
	m, _ := newTestModel(t)
	var copied string
	m.clip = func(s string) { copied = s }
	press(m, "down") // Bob Baker
	runID(m, "copy-vcard")
	assert.Contains(t, copied, "BEGIN:VCARD")
	assert.Contains(t, copied, "Bob")
	assert.Contains(t, copied, "bob@globex.com")
	assert.Contains(t, copied, "END:VCARD")
	assert.Contains(t, m.status, "Bob Baker as a vCard")
	// The chord is bound but terminals swallow it, and the warning list says so.
	assert.Equal(t, []string{"ctrl+shift+c"}, m.actionByID("copy-vcard").keys)
	assert.Contains(t, SwallowedChordWarning("ctrl+shift+c"), "Windows Terminal")
	assert.Contains(t, SwallowedChordWarning("ctrl+shift+c"), "palette")
	// In an organization view the entry is not offered.
	m.switchView(viewOrgs)
	assert.False(t, m.actionByID("copy-vcard").enabled(m) && m.vcardPerson() != 0)
}

// --- 5. open ---

func TestOOpensOutsideForAnyObject(t *testing.T) {
	m, _ := newTestModel(t)
	var opened []string
	m.open = func(u string) error { opened = append(opened, u); return nil }
	last := func() string { return opened[len(opened)-1] }

	// A person in the list with no link: a web search for the name.
	press(m, "down")
	press(m, "o")
	assert.Equal(t, "https://www.google.com/search?q=Bob+Baker", last())
	assert.Contains(t, m.status, "web search for Bob Baker")

	// URL, mail and phone fields open the browser, mail app and dialer.
	press(m, "tab")
	goTo(t, m, "email")
	press(m, "o")
	assert.Equal(t, "mailto:bob@globex.com", last())
	// A plain text field searches for its text.
	press(m, "home")
	goTo(t, m, "company")
	press(m, "o")
	assert.Equal(t, "https://www.google.com/search?q=Globex", last())
	press(m, "esc")

	// A task and a deal search for their titles, an organization opens its website.
	ctx := context.Background()
	org, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme", Domain: sp("acme.example")})
	require.NoError(t, err)
	_, err = m.orr.Create(ctx, model.CreateOrgInput{Name: "Plain Org"})
	require.NoError(t, err)
	m.switchView(viewOrgs)
	m.cursorOn(func(r row) bool { return r.orgID == org.ID })
	press(m, "o")
	assert.Equal(t, "https://acme.example", last())
	m.cursorOn(func(r row) bool { return r.title == "Plain Org" })
	press(m, "o")
	assert.Equal(t, "https://www.google.com/search?q=Plain+Org", last(), "an object with no link searches for its name")
	_, err = m.dlr.Create(ctx, model.CreateDealInput{Title: "Big Deal", Stage: "lead"})
	require.NoError(t, err)
	m.switchView(viewDeals)
	press(m, "o")
	assert.Equal(t, "https://www.google.com/search?q=Big+Deal", last())
	_, err = m.tr.Create(ctx, model.CreateTaskInput{Title: "Send deck", Priority: "medium"})
	require.NoError(t, err)
	m.switchView(viewTasks)
	press(m, "o")
	assert.Equal(t, "https://www.google.com/search?q=Send+deck", last())

	// enter stays "open further inside the app": it never opens the browser.
	before := len(opened)
	m.switchView(viewPeople)
	press(m, "enter")
	assert.Len(t, opened, before)
	assert.Equal(t, "o", m.actionByID("open").hint)
	assert.NotContains(t, m.actionByID("activate").desc, "browser")
}

func TestUrlForValue(t *testing.T) {
	cases := map[string]string{
		"https://x.org/a":    "https://x.org/a",
		"bob@globex.com":     "mailto:bob@globex.com",
		"+1 (555) 010-2000":  "tel:+1 (555) 010-2000",
		"www.acme.example":   "https://www.acme.example",
		"acme.example/about": "https://acme.example/about",
		"CTO":                "",
		"Acme Inc":           "",
		"12":                 "",
		"line one\nline two": "",
	}
	for in, want := range cases {
		assert.Equal(t, want, urlForValue(in), in)
	}
}

func TestTheSearchAddressIsConfigurableWithADefault(t *testing.T) {
	assert.Equal(t, "https://www.google.com/search?q=Ann+Lee", searchURLFor("", "Ann Lee"))
	dir := t.TempDir()
	assert.Equal(t, "https://www.google.com/search?q=a%26b", searchURLFor(dir, "a&b"), "no config file means the default")
	writeConfig := func(body string) {
		require.NoError(t, os.WriteFile(modules.ConfigFile(dir), []byte(body), 0o644))
	}
	writeConfig("[open]\nsearch_url = \"https://duckduckgo.com/?q={query}&ia=web\"\n")
	assert.Equal(t, "https://duckduckgo.com/?q=Ann+Lee&ia=web", searchURLFor(dir, "Ann Lee"))
	writeConfig("[open]\nsearch_url = \"https://example.org/find/\"\n")
	assert.Equal(t, "https://example.org/find/Ann+Lee", searchURLFor(dir, "Ann Lee"), "a template without {query} is a prefix")
	writeConfig("[modules]\norder = [\"people\"]\n")
	assert.Equal(t, "https://www.google.com/search?q=Ann", searchURLFor(dir, "Ann"), "other tables leave the default")
	writeConfig("not toml [[[")
	assert.Equal(t, "https://www.google.com/search?q=Ann", searchURLFor(dir, "Ann"), "a broken file falls back to the default")

	m, _ := newTestModel(t)
	m.env.DataDir = dir
	writeConfig("[open]\nsearch_url = \"https://duckduckgo.com/?q={query}\"\n")
	var opened string
	m.open = func(u string) error { opened = u; return nil }
	press(m, "o")
	assert.Equal(t, "https://duckduckgo.com/?q=Zed+Adams", opened)
}

// --- 6. bulk actions ---

func taskIDs(t *testing.T, m *Model, titles ...string) []int64 {
	t.Helper()
	var ids []int64
	for _, title := range titles {
		task, err := m.tr.Create(context.Background(), model.CreateTaskInput{Title: title, Priority: "medium"})
		require.NoError(t, err)
		ids = append(ids, task.ID)
	}
	m.afterWrite()
	return ids
}

func TestBulkDeleteOfTasksShowsTheCountAndNeedsItTyped(t *testing.T) {
	m, _ := newTestModel(t)
	ids := taskIDs(t, m, "one", "two", "three")
	m.switchView(viewTasks)
	press(m, "ctrl+a", "d")
	require.Equal(t, modePrompt, m.mode, "several objects ask for the count typed back")
	assert.Contains(t, m.screen(), `type "delete 3" to delete 3 tasks`)
	typeText(m, "delete 2")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	open, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Len(t, open, 3, "a wrong word deletes nothing")

	press(m, "d") // the selection is still there after a wrong word
	typeText(m, "delete 3")
	press(m, "enter")
	assert.False(t, m.statusIsErr, m.status)
	open, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Empty(t, open)
	assert.Empty(t, m.marked)

	// One undo step brings all three back.
	press(m, "ctrl+z")
	open, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Len(t, open, 3)

	// With nothing selected d acts on the highlighted row, with the plain y/n.
	m.marked = map[int64]bool{}
	press(m, "d")
	assert.Equal(t, modeConfirm, m.mode)
	press(m, "y")
	open, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	assert.Len(t, open, 2)
	_ = ids
}

func TestBulkVerbsWorkForOrganizationsAndDeals(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	for _, n := range []string{"Acme", "Bolt", "Cyan"} {
		_, err := m.orr.Create(ctx, model.CreateOrgInput{Name: n})
		require.NoError(t, err)
	}
	for _, n := range []string{"D1", "D2"} {
		_, err := m.dlr.Create(ctx, model.CreateDealInput{Title: n, Stage: "lead"})
		require.NoError(t, err)
	}
	m.switchView(viewOrgs)
	press(m, " ", " ", " ")
	press(m, "d")
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "delete 3")
	press(m, "enter")
	list, _ := m.orr.FindAll(ctx, 0)
	assert.Empty(t, list)

	m.switchView(viewDeals)
	press(m, "ctrl+a")
	runID(m, "set-field")
	require.NotNil(t, m.ov, "set field on selected opens a picker of fields")
	assert.Contains(t, m.ov.title, "2 deals")
	typeText(m, "stage")
	press(m, "enter")
	require.NotNil(t, m.ov)
	typeText(m, "won")
	press(m, "enter")
	deals, _ := m.dlr.FindAll(ctx, model.DealFilters{})
	require.Len(t, deals, 2)
	for _, d := range deals {
		assert.Equal(t, "won", d.Stage)
	}
	assert.Empty(t, m.marked)
	press(m, "ctrl+z")
	deals, _ = m.dlr.FindAll(ctx, model.DealFilters{})
	for _, d := range deals {
		assert.Equal(t, "lead", d.Stage, "one undo step for the whole set")
	}
}

func TestSetFieldOnSelectedBringsBackBulkTierAndTaskFields(t *testing.T) {
	m, pr := newTestModel(t)
	press(m, "ctrl+a")
	require.NotNil(t, m.actionByID("set-field"))
	assert.Empty(t, m.actionByID("set-field").keys, "palette only")
	runID(m, "set-field")
	require.NotNil(t, m.ov)
	var labels []string
	for _, it := range m.ov.items {
		labels = append(labels, it.label)
	}
	assert.Contains(t, labels, "priority")
	assert.Contains(t, labels, "birthday")
	typeText(m, "priority")
	press(m, "enter")
	typeText(m, "High")
	press(m, "enter")
	for id := int64(1); id <= 3; id++ {
		p := personByID(t, pr, id)
		require.NotNil(t, p.PriorityTier, id)
		assert.Equal(t, 2, *p.PriorityTier, id)
	}

	// A date field takes a phrase and is checked by its type.
	press(m, "ctrl+a")
	runID(m, "set-field")
	typeText(m, "birthday")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "no date")
	press(m, "enter")
	assert.True(t, m.statusIsErr, "a bad date is refused with the field's message")
	require.Equal(t, modePrompt, m.mode, "and asked again")
	m.input.SetValue("1990-05-17")
	press(m, "enter")
	assert.False(t, m.statusIsErr, m.status)
	assert.Equal(t, "1990-05-17", formatBirthdayParts(personByID(t, pr, 3)))

	// Tasks: due date and priority.
	taskIDs(t, m, "a", "b")
	m.switchView(viewTasks)
	press(m, "ctrl+a")
	runID(m, "set-field")
	typeText(m, "due")
	press(m, "enter")
	typeText(m, "2027-01-15")
	press(m, "enter")
	tasks, _ := m.tr.FindAll(context.Background(), model.TaskFilters{})
	for _, tk := range tasks {
		require.NotNil(t, tk.DueAt)
		assert.True(t, strings.HasPrefix(*tk.DueAt, "2027-01-15"))
	}
	// It acts on the highlighted row when nothing is selected.
	m.marked = map[int64]bool{}
	runID(m, "set-field")
	typeText(m, "priority")
	press(m, "enter")
	typeText(m, "high")
	press(m, "enter")
	tasks, _ = m.tr.FindAll(context.Background(), model.TaskFilters{})
	high := 0
	for _, tk := range tasks {
		if tk.Priority == "high" {
			high++
		}
	}
	assert.Equal(t, 1, high)

	// An object kind with no such fields says so.
	m.switchView(viewOrgs)
	assert.False(t, m.actionByID("set-field").enabled(m) && whenSetField(m))
}

func TestBulkMarkLinkAndUnlinkActOnTheSelection(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	// x completes every selected task.
	taskIDs(t, m, "a", "b", "c")
	m.switchView(viewTasks)
	press(m, "ctrl+a", "x")
	open, _ := m.tr.FindAll(ctx, model.TaskFilters{})
	assert.Empty(t, open, "every selected task is done")
	press(m, "ctrl+z")
	open, _ = m.tr.FindAll(ctx, model.TaskFilters{})
	assert.Len(t, open, 3)

	// l with a tag links every selected person; u removes it again (typed count for several).
	m.switchView(viewPeople)
	press(m, "ctrl+a", "l")
	require.NotNil(t, m.ov)
	assert.Contains(t, m.ov.title, "3 people")
	typeText(m, "tag")
	press(m, "enter")
	require.NotNil(t, m.ov)
	typeText(m, "vip")
	press(m, "enter")
	for id := int64(1); id <= 3; id++ {
		tags, _ := m.tagr.GetForEntity(ctx, "person", id)
		var names []string
		for _, tg := range tags {
			names = append(names, tg.Name)
		}
		assert.Contains(t, names, "vip", id)
	}
	press(m, "ctrl+a", "u")
	require.NotNil(t, m.ov)
	typeText(m, "tag")
	press(m, "enter")
	require.NotNil(t, m.ov)
	typeText(m, "vip")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode, "unlinking several asks for the count typed")
	typeText(m, "unlink 3")
	press(m, "enter")
	for id := int64(1); id <= 3; id++ {
		tags, _ := m.tagr.GetForEntity(ctx, "person", id)
		for _, tg := range tags {
			assert.NotEqual(t, "vip", tg.Name, id)
		}
	}

	// Link people to an organization in one step, with one role for all.
	acme, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	_ = acme
	m.marked = map[int64]bool{}
	press(m, "ctrl+a", "l")
	typeText(m, "organization")
	press(m, "enter")
	require.NotNil(t, m.ov)
	typeText(m, "acme")
	press(m, "enter")
	require.Equal(t, modePrompt, m.mode, "the attributes are asked once")
	submit(m, "Advisor")
	advisors := func() int {
		n := 0
		for id := int64(1); id <= 3; id++ {
			for _, it := range linkedOrgs(t, m, id) {
				if it.Name == "Acme" && it.Role != nil && *it.Role == "Advisor" {
					n++
				}
			}
		}
		return n
	}
	assert.Equal(t, 3, advisors())
	press(m, "ctrl+z")
	assert.Equal(t, 0, advisors(), "one undo step")
}

func TestBulkRestoreInTheArchivedViewTakesEveryKind(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	org, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	deal, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "organization", org.ID))
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "deal", deal.ID))
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "person", 3))
	m.switchView(viewArchived)
	require.Len(t, m.rows, 3)
	// Ids of different kinds can be equal, the selection keeps them apart.
	press(m, "ctrl+a")
	assert.Len(t, m.markedRefs(), 3)
	press(m, "r")
	assert.Empty(t, m.rows)
	_, err := m.orr.FindByID(ctx, org.ID)
	assert.NoError(t, err)
	_, err = m.dlr.FindByID(ctx, deal.ID)
	assert.NoError(t, err)
	_, err = m.pr.FindByID(ctx, 3)
	assert.NoError(t, err)
	press(m, "ctrl+z")
	assert.Len(t, m.rows, 3, "one undo step")
}

// --- 7. generic purge ---

func TestBulkPurgeInTheArchivedViewTakesEveryType(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	org, _ := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	deal, _ := m.dlr.Create(ctx, model.CreateDealInput{Title: "Site", Stage: "lead"})
	task, _ := m.tr.Create(ctx, model.CreateTaskInput{Title: "Send deck", Priority: "medium"})
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "organization", org.ID))
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "deal", deal.ID))
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "task", task.ID))
	require.NoError(t, repo.ArchiveObject(ctx, m.db, "person", 3))
	m.switchView(viewArchived)
	require.Len(t, m.rows, 4)

	runID(m, "purge-all")
	require.Equal(t, modePrompt, m.mode)
	assert.Contains(t, m.screen(), `type "purge 4" to delete 4 objects remaining forever`)
	typeText(m, "purge 3")
	press(m, "enter")
	assert.True(t, m.statusIsErr)
	assert.Len(t, m.rows, 4, "a wrong count deletes nothing")

	press(m, "ctrl+a", "d") // d on the selection asks for the count too
	require.Equal(t, modePrompt, m.mode)
	typeText(m, "purge 4")
	press(m, "enter")
	assert.False(t, m.statusIsErr, m.status)
	assert.Contains(t, m.status, "deleted 4 objects forever")
	assert.Empty(t, m.rows)
	for table, want := range map[string]int{"organizations": 1, "deals": 1, "tasks": 1, "people": 1} {
		var n int
		require.NoError(t, m.db.QueryRow(`SELECT COUNT(*) FROM tombstones WHERE table_name = ?`, table).Scan(&n))
		assert.Equal(t, want, n, "tombstone for "+table)
	}
	_, err := m.dlr.FindByID(ctx, deal.ID)
	assert.Error(t, err)
}

func TestTheSelectionViewsLeaveTheDefaultsAlone(t *testing.T) {
	// Merge groups can be selected and dismissed together.
	m, _ := dupeModel(t)
	m.switchView(viewMerge)
	press(m, "ctrl+a")
	assert.Len(t, m.markedRefs(), 2)
	press(m, "d")
	assert.Empty(t, groupRowsOf(m))
	assert.Empty(t, m.marked)

}
