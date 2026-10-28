package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relFixture is a small web of links for the related segment:
//
//	deal "Big deal" -- person Zed Adams (1), organization Acme, tag vip
//	person Bob Baker (2) -- organization Globex
type relFixture struct {
	m                  *Model
	deal, acme, globex repo.Ref
	zed, bob           repo.Ref
}

func newRelFixture(t *testing.T) *relFixture {
	t.Helper()
	m, _ := newTestModel(t)
	ctx := context.Background()
	f := &relFixture{m: m, zed: repo.Ref{Kind: "person", ID: 1}, bob: repo.Ref{Kind: "person", ID: 2}}
	acme, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	globex, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Globex Corp"})
	require.NoError(t, err)
	deal, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Big deal", Stage: "lead"})
	require.NoError(t, err)
	f.acme, f.globex, f.deal = repo.Ref{Kind: "organization", ID: acme.ID}, repo.Ref{Kind: "organization", ID: globex.ID}, repo.Ref{Kind: "deal", ID: deal.ID}
	for _, l := range [][2]repo.Ref{{f.deal, f.zed}, {f.deal, f.acme}, {f.bob, f.globex}} {
		_, err := m.links.Link(ctx, l[0], l[1], model.LinkAttrs{})
		require.NoError(t, err)
	}
	require.NoError(t, m.tagr.Apply(ctx, "deal", deal.ID, "vip"))
	m.qtags = nil
	m.afterWrite()
	return f
}

// highlightDeal puts the cursor on the deal in the Deals view.
func (f *relFixture) highlightDeal() {
	goDeals(f.m)
	f.m.cursorOn(func(r row) bool { return r.dealID == f.deal.ID })
}

// segText sets the text of the open pipe line and returns the popup rows. The cursor is at the end.
func (f *relFixture) rows(line string) []Suggestion {
	in, _ := f.m.pipeInput()
	in.SetValue(line)
	in.CursorEnd()
	s := f.m.pipeRefresh()
	s.ac.sig = ""
	return f.m.pipeRefresh().ac.Rows()
}

func kindsOf2(rows []Suggestion) map[string]bool {
	out := map[string]bool{}
	for _, r := range rows {
		out[r.Kind] = true
	}
	return out
}

func hasLabel(rows []Suggestion, label string) bool {
	for _, r := range rows {
		if r.Label == label {
			return true
		}
	}
	return false
}

// Reproduction of the report: a deal is highlighted, a then n (and a t). Before the related segment the
// popup of the people and organizations segments was empty, because only the highlighted object itself was
// offered, and only in the segment that took its kind (the deal). Now the deal's people, organizations
// and tags are suggested next to it.
func TestHighlightedDealSuggestsItsPeopleOrganizationsAndTags(t *testing.T) {
	cases := []struct {
		name     string
		letter   string
		sep      string // pipes that reach the related segment
		want     []string
		wantNone []string
	}{
		{"note", "n", "|", []string{"Big deal", "Zed Adams", "Acme", "vip"}, nil},
		{"task", "t", "||", []string{"Big deal", "Zed Adams", "Acme", "vip"}, nil},
		{"organization", "o", strings.Repeat("|", 9), []string{"Big deal", "Zed Adams", "Acme", "vip"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRelFixture(t)
			f.highlightDeal()
			press(f.m, "a", tc.letter)
			require.Equal(t, modePrompt, f.m.mode)
			rows := f.rows("X" + tc.sep)
			for _, w := range tc.want {
				assert.True(t, hasLabel(rows, w), "%s is suggested: %v", w, rowLabels(rows))
			}
			assert.Equal(t, "Big deal", rows[0].Label, "the highlighted object leads")
		})
	}
}

// The order of the context: the object, its links, links of what is chosen (one hop), recents.
func TestRelatedContextOrderAndOneHop(t *testing.T) {
	f := newRelFixture(t)
	f.highlightDeal()
	press(f.m, "a", "n")
	rows := f.rows("X|")
	require.NotEmpty(t, rows)
	assert.Equal(t, "Big deal", rows[0].Label)
	assert.False(t, hasLabel(rows, "Globex Corp"), "Bob's organization is two hops away until Bob is chosen")

	// Choosing Bob brings in the objects linked to Bob.
	rows = f.rows("X|Bob Baker, ")
	assert.True(t, hasLabel(rows, "Globex Corp"), "one hop from the chosen person: %v", rowLabels(rows))
	assert.False(t, hasLabel(rows, "Bob Baker"), "a chosen object is never suggested again")
	assert.True(t, hasLabel(rows, "Zed Adams"), "the context stays")

	// Ranking: the anchor's links come before the hop.
	var order []string
	for _, r := range rows {
		order = append(order, r.Label)
	}
	assert.Less(t, indexOf(order, "Big deal"), indexOf(order, "Globex Corp"))
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return len(list)
}

// Typing narrows by name with the shared ranker, so a nickname finds the full name, and a chosen object
// is not offered again.
func TestRelatedTypingNarrowsByNameWithAliases(t *testing.T) {
	f := newRelFixture(t)
	ctx := context.Background()
	p, err := f.m.pr.Create(ctx, model.CreatePersonInput{FirstName: "Zachary", LastName: sp("Hill")})
	require.NoError(t, err)
	f.m.afterWrite()
	f.m.pendingCmd = objectByKind("task").add(f.m)
	rows := f.rows("Call||Zach")
	require.NotEmpty(t, rows)
	assert.Equal(t, "Zachary Hill", rows[0].Label)
	assert.Equal(t, repo.Ref{Kind: "person", ID: p.ID}, rows[0].Ref)
	rows = f.rows("Call||Zachary Hill, Zach")
	assert.False(t, hasLabel(rows, "Zachary Hill"), "never suggest what is chosen")
}

// Only kinds the Relations registry joins to the new object are suggested, for every kind of new object.
func TestRelatedSuggestsOnlyKindsTheRegistryAllows(t *testing.T) {
	cases := []struct {
		kind   string
		letter string
		line   func(pl *pipeLine) string
		allow  []string
	}{
		{"task", "t", nil, []string{"person", "organization", "deal", tagKind}},
		{model.NoteKind, "n", nil, []string{"person", "organization", "deal", tagKind}},
		{"organization", "o", nil, []string{"person", "organization", "deal", "task", model.NoteKind, tagKind}},
		{"deal", "d", nil, []string{"person", "organization", "deal", "task", model.NoteKind, tagKind}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			f := newRelFixture(t)
			// the highlighted object is a task so that tasks and notes could be offered
			tk := newObjectOf(t, f.m, "task", new(int))
			_, err := f.m.links.Link(context.Background(), tk, f.zed, model.LinkAttrs{})
			require.NoError(t, err)
			f.m.addSugg = &addSuggestion{ref: f.zed, label: "Zed Adams"}
			pl := startAddOf(t, f.m, tc.kind)
			idx := segFor(pl, "person")
			require.True(t, pl.segs[len(pl.segs)-1].Related)
			assert.Equal(t, relatedKindsOf(tc.kind), pl.segs[len(pl.segs)-1].Kinds)
			rows := f.rows("X" + strings.Repeat("|", len(pl.segs)-1))
			_ = idx
			allowed := map[string]bool{}
			for _, k := range tc.allow {
				allowed[k] = true
			}
			require.NotEmpty(t, rows)
			for _, r := range rows {
				k := r.Kind
				assert.True(t, allowed[k], "%s may not be related to a new %s (%q)", k, tc.kind, r.Label)
				if r.Ref.ID != 0 {
					_, ok := model.RelationBetween(tc.kind, r.Ref.Kind)
					assert.True(t, ok, "registry joins %s and %s", tc.kind, r.Ref.Kind)
				}
			}
		})
	}
}

// A prompt opened on a record page offers the page's links, not the page itself (the new object is
// already linked to it).
func TestRelatedOnARecordPageOffersThePagesLinks(t *testing.T) {
	f := newRelFixture(t)
	f.highlightDeal()
	f.m.focus = focusList
	f.m.pendingCmd = f.m.startTaskAdd() // a task for the deal whose page is open
	require.Equal(t, modePrompt, f.m.mode)
	require.Equal(t, "task-page", f.m.promptKind)
	rows := f.rows("Call||")
	assert.True(t, hasLabel(rows, "Zed Adams"), "the page's people: %v", rowLabels(rows))
	assert.True(t, hasLabel(rows, "Acme"), "the page's organizations")
	assert.False(t, hasLabel(rows, "Big deal"), "the page itself is already linked")
}

// Rows are grouped by kind under headings. The group of the best row comes first and the best row leads.
func TestRelatedGroupsRowsByKindWithHeadings(t *testing.T) {
	rows := []Suggestion{
		{Label: "Acme", Kind: "organization"}, {Label: "Zed", Kind: "person"},
		{Label: "Big deal", Kind: "deal"}, {Label: "Pia", Kind: "person"}, {Label: "vip", Tag: "vip", Kind: tagKind},
	}
	got := relatedGroup(rows)
	assert.Equal(t, []string{"Acme", "Zed", "Pia", "Big deal", "vip"}, rowLabels(got))
	var heads []string
	for _, r := range got {
		if r.Group != "" {
			heads = append(heads, r.Group)
		}
	}
	assert.Equal(t, []string{"Organizations", "People", "Deals", "Tags"}, heads)

	st := theme.NewStyles(mustTheme(t, "dark"), colorprofile.TrueColor)
	ac := &Autocomplete{rows: got}
	lines := ac.Lines(30, st)
	require.Len(t, lines, 9, "five rows and four headings")
	assert.Contains(t, plain(lines[0]), "Organizations", "the first line is a heading")
	assert.Contains(t, plain(lines[1]), "o:Acme")
}

func mustTheme(t *testing.T, name string) theme.Theme {
	t.Helper()
	th, ok := theme.Builtin(name, true)
	require.True(t, ok)
	return th
}

// A long popup scrolls inside acMaxLines and keeps the highlighted row in view.
func TestRelatedPopupScrollsInsideItsLines(t *testing.T) {
	var rows []Suggestion
	for i, k := range []string{"person", "person", "person", "person", "organization", "organization", "organization", "deal", "deal", "deal", "task", "task"} {
		rows = append(rows, Suggestion{Label: string(rune('a' + i)), Kind: k})
	}
	rows = relatedGroup(rows)
	ac := &Autocomplete{rows: rows, cur: len(rows) - 1}
	st := theme.NewStyles(mustTheme(t, "dark"), colorprofile.TrueColor)
	lines := ac.Lines(30, st)
	assert.Len(t, lines, acMaxLines)
	assert.Contains(t, plain(lines[len(lines)-1]), "t:l", "the last row is in view")
}

// Kinds are colored from theme tokens and read without color through the short prefix.
func TestKindColorsAndPrefixes(t *testing.T) {
	kinds := []string{"person", "organization", "deal", "task", model.NoteKind, tagKind}
	prefixes := map[string]string{"person": "p:", "organization": "o:", "deal": "d:", "task": "t:", model.NoteKind: "n:", tagKind: "#"}
	for _, name := range theme.BuiltinNames() {
		th, _ := theme.Builtin(name, true)
		seen := map[string]string{}
		for _, k := range kinds {
			c := th.KindColor(k)
			assert.NotEmpty(t, c, "%s has a color for %s", name, k)
			if prev, dup := seen[c]; dup {
				t.Errorf("%s: %s and %s share %s", name, prev, k, c)
			}
			seen[c] = k
		}
	}
	for _, k := range kinds {
		assert.Equal(t, prefixes[k], kindPrefix(k))
	}

	// Truecolor: a chip carries the color of its token. No color: only the prefix tells the kind.
	th := mustTheme(t, "nord")
	tc := colorprofile.TrueColor
	groups := []chipGroup{{kind: "person", names: []string{"Zed Adams"}}, {kind: "organization", names: []string{"Acme"}}}
	out := strings.Join(renderChips(groups, 80, theme.NewStyles(th, tc)), "\n")
	assert.Contains(t, out, "38;2;129;161;193", "people use kind_person (#81A1C1)")
	assert.Contains(t, out, "38;2;208;135;112", "organizations use kind_org (#D08770)")

	mono := colorprofile.Ascii
	out = strings.Join(renderChips(groups, 80, theme.NewStyles(th, mono)), "\n")
	assert.NotContains(t, out, "38;")
	assert.Equal(t, "[p:Zed Adams]  [o:Acme]", out)
}

// Chips are grouped by kind in a stable order whatever the order typed, and wrap onto more lines.
func TestChipsAreGroupedInStableOrderAndWrap(t *testing.T) {
	chosen := []chosenLink{
		{tag: "vip", label: "vip"},
		{ref: repo.Ref{Kind: "organization", ID: 1}, label: "Acme"},
		{ref: repo.Ref{Kind: "person", ID: 1}, label: "Zed"},
		{ref: repo.Ref{Kind: "deal", ID: 1}, label: "Big deal"},
		{ref: repo.Ref{Kind: "person", ID: 2}, label: "Bob"},
		{ref: repo.Ref{Kind: "task", ID: 1}, label: "Call"},
		{ref: repo.Ref{Kind: model.NoteKind, ID: 1}, label: "note"},
	}
	groups := chipGroups(chosen)
	var kinds []string
	for _, g := range groups {
		kinds = append(kinds, g.kind)
	}
	assert.Equal(t, []string{"person", "organization", "deal", "task", model.NoteKind, tagKind}, kinds)
	assert.Equal(t, []string{"Zed", "Bob"}, groups[0].names, "the typed order holds inside a group")

	mono := colorprofile.Ascii
	st := theme.NewStyles(mustTheme(t, "dark"), mono)
	one := renderChips(groups, 200, st)
	require.Len(t, one, 1)
	assert.Equal(t, "[p:Zed] [p:Bob]  [o:Acme]  [d:Big deal]  [t:Call]  [n:note]  [#vip]", one[0])
	wrapped := renderChips(groups, 24, st)
	assert.Greater(t, len(wrapped), 1, "chips wrap onto extra lines")
	for _, l := range wrapped {
		assert.LessOrEqual(t, lipgloss.Width(l), 24)
	}
	assert.Equal(t, strings.Join(strings.Fields(one[0]), " "), strings.Join(strings.Fields(strings.Join(wrapped, " ")), " "), "nothing is lost when wrapping")
}

// The chips show on the screen above the prompt once an object is chosen, and the prompt stays one line.
func TestChosenObjectsShowAsChipsAndThePromptStaysOneLine(t *testing.T) {
	f := newRelFixture(t)
	f.m.pendingCmd = objectByKind("task").add(f.m)
	typeText(f.m, "Call||Zed")
	press(f.m, "tab")
	in, _ := f.m.pipeInput()
	assert.Equal(t, "Call|| Zed Adams, ", in.Value())
	screen := plainView(f.m)
	assert.Contains(t, screen, "[p:Zed Adams]")
	assert.NotContains(t, in.Value(), "\n")
}

// Tab accepts and adds a comma, a pipe takes the comma back, and backspacing over a token unlinks it.
func TestRelatedKeepsTabCommaAndBackspaceBehavior(t *testing.T) {
	f := newRelFixture(t)
	ctx := context.Background()
	f.m.pendingCmd = objectByKind("task").add(f.m)
	typeText(f.m, "Call||Zed")
	press(f.m, "tab")
	in, _ := f.m.pipeInput()
	assert.Equal(t, "Call|| Zed Adams, ", in.Value(), "tab accepts and adds a comma and a space")
	typeText(f.m, "Bob Baker")
	press(f.m, "tab")
	assert.Equal(t, "Call|| Zed Adams, Bob Baker, ", in.Value())
	// Backspacing over the last token removes the link before saving.
	for i := 0; i < len([]rune("Bob Baker, ")); i++ {
		press(f.m, "backspace")
	}
	assert.Equal(t, "Call|| Zed Adams, ", in.Value())
	press(f.m, "enter")
	tasks, err := f.m.tr.FindAll(ctx, model.TaskFilters{})
	require.NoError(t, err)
	var made *model.Task
	for _, tk := range tasks {
		if tk.Title == "Call" {
			made = tk
		}
	}
	require.NotNil(t, made)
	assert.Equal(t, []int64{1}, made.PersonIDs, "only the person that is still in the line is linked")

	// Typing | right after an accepted name takes the comma back.
	f.m.pendingCmd = objectByKind("task").add(f.m)
	typeText(f.m, "Two|")
	typeText(f.m, "|Bob")
	press(f.m, "tab")
	in, _ = f.m.pipeInput()
	typeText(f.m, "|")
	assert.Equal(t, "Two|| Bob Baker |", in.Value())
}

// Unresolved text offers pick-or-create, with a create choice for each kind the segment takes.
func TestRelatedUnresolvedTextOffersPickOrCreate(t *testing.T) {
	f := newRelFixture(t)
	f.m.pendingCmd = objectByKind("task").add(f.m)
	typeText(f.m, "Call||Initrode")
	press(f.m, "enter")
	require.Equal(t, modeOverlay, f.m.mode)
	var labels []string
	for _, it := range f.m.ov.items {
		labels = append(labels, it.label)
	}
	assert.Contains(t, labels, `Create person "Initrode"`)
	assert.Contains(t, labels, `Create organization "Initrode"`)
	assert.Contains(t, labels, `Create deal "Initrode"`)
	assert.Contains(t, labels, "Tag #Initrode")
	for i, it := range f.m.ov.items {
		if it.label == `Create organization "Initrode"` {
			f.m.ov.cursor = i
			f.m.ov.shown = []int{i}
			f.m.ov.cursor = 0
		}
	}
	press(f.m, "enter")
	orgs, err := f.m.orr.FindAll(context.Background(), 0)
	require.NoError(t, err)
	found := false
	for _, o := range orgs {
		found = found || o.Name == "Initrode"
	}
	assert.True(t, found, "the organization was created")
	tasks, _ := f.m.tr.FindAll(context.Background(), model.TaskFilters{})
	require.NotEmpty(t, tasks)
	linked, err := f.m.links.Linked(context.Background(), repo.Ref{Kind: "task", ID: tasks[len(tasks)-1].ID}, "organization")
	require.NoError(t, err)
	require.Len(t, linked, 1)
	assert.Equal(t, "Initrode", linked[0].Name)
}

// A deal line keeps its links: the people and organization chosen in related are linked through the
// registry, the first organization as primary.
func TestDealLineLinksRelatedPeopleAndOrganizations(t *testing.T) {
	f := newRelFixture(t)
	f.m.pendingCmd = objectByKind("deal").add(f.m)
	require.Equal(t, modePrompt, f.m.mode)
	pl := f.m.pipeFor(f.m.promptKind)
	typeText(f.m, "New deal||||||"+"Zed Adams, Acme, #vip")
	require.True(t, pl.segs[len(pl.segs)-1].Related)
	press(f.m, "enter")
	created := findNamed(t, f.m, "New deal")
	people, err := f.m.links.Linked(context.Background(), created, "person")
	require.NoError(t, err)
	require.Len(t, people, 1)
	orgs, err := f.m.links.Linked(context.Background(), created, "organization")
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	assert.True(t, orgs[0].Primary, "the first organization is the primary one")
	tags, _ := f.m.tagr.GetForEntity(context.Background(), "deal", created.ID)
	require.Len(t, tags, 1)
	assert.Equal(t, "vip", tags[0].Name)
}

func findNamed(t *testing.T, m *Model, title string) repo.Ref {
	t.Helper()
	list, err := m.dlr.FindAll(context.Background(), model.DealFilters{})
	require.NoError(t, err)
	for _, d := range list {
		if d.Title == title {
			return repo.Ref{Kind: "deal", ID: d.ID}
		}
	}
	t.Fatalf("no deal %q", title)
	return repo.Ref{}
}

// Every pipe line has exactly one related segment, last, and the hint names it.
func TestEveryPipeLineHasOneRelatedSegment(t *testing.T) {
	m, _ := newTestModel(t)
	kinds := []string{"org-add", "deal-add", "task-new", "task", "task-page", "note", "note-object", "note-free", personAddKind, "tag-new"}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			pl := m.pipeFor(kind)
			require.NotNil(t, pl)
			related := 0
			for i, s := range pl.segs {
				if s.Related {
					related++
					assert.Equal(t, len(pl.segs)-1, i, "the related segment is last")
					assert.Equal(t, "related", s.Label)
					assert.True(t, s.Link)
				}
				if s.Link && !s.Related {
					assert.Equal(t, 1, len(s.Kinds), "other link segments are core fields of one kind (a deal's person)")
				}
			}
			assert.Equal(t, 1, related)
			labels := pl.labels()
			assert.Equal(t, "related", labels[len(labels)-1])
			for _, l := range labels {
				assert.NotContains(t, []string{"people", "organizations", "tags", "deals", "tasks", "related people"}, l, "no per-kind link segment")
			}
			assert.Contains(t, pipeHint(labels, ""), "related")
		})
	}
	// Edit lines have no related segment.
	for _, kind := range []string{"org-edit", "deal-edit"} {
		if pl := m.pipeFor(kind); pl != nil {
			for _, s := range pl.segs {
				assert.False(t, s.Related, kind)
			}
		}
	}
}
