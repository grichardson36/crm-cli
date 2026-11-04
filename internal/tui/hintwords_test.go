package tui

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-023: the hint bar uses the same word for the same action everywhere. The words are the
// constants of hintwords.go. These tests render the footer at the seam (footerSegs, hintsLine, the
// mode hint tables) and read the source for hard-coded labels.

// --- the word table ---

// wordTable reads the const block of hintwords.go: the canonical words.
func wordTable(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "hintwords.go", nil, 0)
	require.NoError(t, err)
	words := map[string]string{} // word -> constant name
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, sp := range g.Specs {
			vs := sp.(*ast.ValueSpec)
			for i, n := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				require.True(t, ok, n.Name)
				w, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				prev, dup := words[w]
				assert.False(t, dup, "%s and %s are both %q: one word, one constant", n.Name, prev, w)
				words[w] = n.Name
			}
		}
	}
	require.NotEmpty(t, words)
	return words
}

func TestHintWordTableHoldsTheCanonicalVerbs(t *testing.T) {
	words := wordTable(t)
	for _, w := range []string{"delete", "purge", "add", "link", "unlink", "restore", "favorite", "snooze", "merge", "open",
		"edit", "details", "save", "pick", "run", "apply", "select", "toggle", "cancel", "back", "close", "find", "help"} {
		assert.Contains(t, words, w)
	}
	for _, old := range []string{"archive", "dismiss", "ignore", "confirm", "ok", "close list", "add or remove", "remove"} {
		assert.NotContains(t, words, old, "%q was replaced by a canonical verb", old)
	}
	for key, labels := range hintCanon {
		for _, l := range labels {
			assert.Contains(t, words, l, "the rule for %s names a word outside the table", key)
		}
	}
}

// TestHintLabelsAreNeverLiterals: no hint label is typed into the code. fk and ak hints, the footer
// words of an action (footLabel, footDyn, footShort) and nothing else use the constants of hintwords.go,
// so one rename follows everywhere.
func TestHintLabelsAreNeverLiterals(t *testing.T) {
	fset := token.NewFileSet()
	files := parseNonTestFiles(t, fset)
	var bad []string
	isString := func(e ast.Expr) bool {
		l, ok := e.(*ast.BasicLit)
		return ok && l.Kind == token.STRING
	}
	report := func(pos token.Pos, what string) { bad = append(bad, fset.Position(pos).String()+" "+what) }
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if id, ok := x.Fun.(*ast.Ident); ok && (id.Name == "fk" || id.Name == "ak") && len(x.Args) == 2 && isString(x.Args[1]) {
					report(x.Pos(), "literal label in "+id.Name)
				}
			case *ast.KeyValueExpr:
				key, ok := x.Key.(*ast.Ident)
				if !ok {
					return true
				}
				switch key.Name {
				case "footLabel":
					if isString(x.Value) {
						report(x.Pos(), "literal footLabel")
					}
				case "footDyn":
					if fl, ok := x.Value.(*ast.FuncLit); ok {
						ast.Inspect(fl, func(n ast.Node) bool {
							if r, ok := n.(*ast.ReturnStmt); ok {
								for _, v := range r.Results {
									if isString(v) {
										report(r.Pos(), "literal word returned by footDyn")
									}
								}
							}
							return true
						})
					}
				}
			case *ast.AssignStmt:
				for i, l := range x.Lhs {
					if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == "footLabel" && i < len(x.Rhs) && isString(x.Rhs[i]) {
						report(x.Pos(), "literal footLabel")
					}
				}
			case *ast.ValueSpec:
				if len(x.Names) == 1 && x.Names[0].Name == "footShort" && len(x.Values) == 1 {
					if cl, ok := x.Values[0].(*ast.CompositeLit); ok {
						for _, e := range cl.Elts {
							if kv, ok := e.(*ast.KeyValueExpr); ok && isString(kv.Value) {
								report(kv.Pos(), "literal word in footShort")
							}
						}
					}
				}
			}
			return true
		})
	}
	assert.Empty(t, bad, "hint words come from hintwords.go")
}

func parseNonTestFiles(t *testing.T, fset *token.FileSet) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	var out []*ast.File
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err, name)
		out = append(out, f)
	}
	return out
}

// --- reading the hints a screen shows ---

type hintPair struct{ key, label string }

// parseHintLine splits a rendered hint block into key and label pairs. Hints are two spaces apart,
// the key is the first word ("any key" is the one key with a space).
func parseHintLine(line string) []hintPair {
	var out []hintPair
	for _, row := range strings.Split(ansi.Strip(line), "\n") {
		for _, h := range strings.Split(row, "  ") {
			h = strings.TrimSpace(h)
			if h == "" || h == "READ-ONLY" {
				continue
			}
			key, label, _ := strings.Cut(h, " ")
			if key == "any" {
				key, label = "any key", strings.TrimPrefix(label, "key ")
			}
			out = append(out, hintPair{key, label})
		}
	}
	return out
}

func itemPairs(m *Model, items []hintItem) []hintPair {
	var out []hintPair
	for _, h := range items {
		out = append(out, hintPair{m.keyText(h), h.label})
	}
	return out
}

func labelFor(pairs []hintPair, key string) string {
	for _, p := range pairs {
		if p.key == key {
			return p.label
		}
	}
	return "<none>"
}

func footPairs(m *Model) []hintPair {
	var out []hintPair
	for _, s := range m.footerSegs() {
		out = append(out, hintPair{s.hint, s.label})
	}
	return out
}

// --- scenario fixtures ---

func peopleWithTwoMarks(t *testing.T) *Model {
	m, _ := newTestModel(t)
	press(m, "space", "down", "space")
	require.Len(t, m.markedIDs(), 2)
	return m
}

func mergeView(t *testing.T) *Model {
	m, _ := dupeModel(t)
	m.switchView(viewMerge)
	return m
}

func syncInboxRow(t *testing.T) *Model {
	m := syncedModel(t)
	rows := inboxRows(m)
	require.NotEmpty(t, rows)
	for i, r := range m.rows {
		if r.sync != nil && r.sync.entry != nil {
			m.cursor = i
			break
		}
	}
	m.refreshDetail(true)
	return m
}

func archivedRows(t *testing.T) *Model {
	m, _ := newTestModel(t)
	require.NoError(t, repo.ArchiveObject(context.Background(), m.db, "person", 3))
	m.switchView(viewArchived)
	require.NotEmpty(t, m.rows)
	return m
}

func recordPageOnALinkedRow(t *testing.T) *Model {
	m, _ := newTestModel(t)
	org, err := m.orr.Create(m.ctx, model.CreateOrgInput{Name: "Globex Corp"})
	require.NoError(t, err)
	_, err = m.links.Link(m.ctx, repo.Ref{Kind: "person", ID: 2}, repo.Ref{Kind: "organization", ID: org.ID})
	require.NoError(t, err)
	m.afterWrite()
	press(m, "down", "tab")
	for i := 0; i < 60 && !whenLinkedRow(m); i++ {
		press(m, "down")
	}
	require.True(t, whenLinkedRow(m), "the page has a linked row")
	return m
}

// --- S1 d reads delete, purge only in Archived ---

func TestDReadsDeleteEverywhereExceptArchived(t *testing.T) {
	tests := []struct {
		name string
		m    func(t *testing.T) *Model
		want string
	}{
		{"people with marks (was archive)", peopleWithTwoMarks, "delete"},
		{"merge list (was dismiss)", mergeView, "delete"},
		{"sync inbox (was ignore)", syncInboxRow, "delete"},
		{"tasks", func(t *testing.T) *Model { return threeTasks(t) }, "delete"},
		{"organizations", func(t *testing.T) *Model { return twoOrgs(t) }, "delete"},
		{"deals", func(t *testing.T) *Model { return threeDeals(t) }, "delete"},
		{"record page", func(t *testing.T) *Model { m, _ := newTestModel(t); press(m, "down", "tab"); return m }, "delete"},
		{"compare screen (was dismiss)", func(t *testing.T) *Model { m := mergeView(t); press(m, "enter"); require.NotNil(t, m.cmp); return m }, "delete"},
		{"archived view", archivedRows, "purge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.m(t)
			assert.Equal(t, tt.want, labelFor(footPairs(m), "d"))
		})
	}
}

func TestMergeFooterNamesDeleteOnce(t *testing.T) {
	m := mergeView(t)
	n := 0
	for _, p := range footPairs(m) {
		if p.key == "d" {
			n++
		}
	}
	assert.Equal(t, 1, n, "d shows once in the Merge list")
}

func TestArchivedFooterReadsPurgeAndRestore(t *testing.T) {
	m := archivedRows(t)
	assert.Equal(t, "purge", labelFor(footPairs(m), "d"))
	assert.Equal(t, "restore", labelFor(footPairs(m), "r"))
	m, _ = newTestModel(t)
	m.switchView(viewArchived)
	assert.Equal(t, "<none>", labelFor(footPairs(m), "r"), "nothing to restore on an empty list")
	m, _ = newTestModel(t)
	assert.Equal(t, "<none>", labelFor(footPairs(m), "r"), "People has no restore hint")
}

// --- S2 enter opens, tab goes to the details ---

func TestEnterReadsOpenInEveryObjectList(t *testing.T) {
	tests := []struct {
		name string
		m    func(t *testing.T) *Model
	}{
		{"people", func(t *testing.T) *Model { m, _ := newTestModel(t); return m }},
		{"tasks (was edit)", func(t *testing.T) *Model { return threeTasks(t) }},
		{"organizations (was edit)", func(t *testing.T) *Model { return twoOrgs(t) }},
		{"deals (was edit)", func(t *testing.T) *Model { return threeDeals(t) }},
		{"tags", func(t *testing.T) *Model { m, _ := newTestModel(t); m.switchView(viewTags); return m }},
		{"home", func(t *testing.T) *Model { m, _ := newTestModel(t); m.switchView(viewHome); return m }},
		{"follow-up", func(t *testing.T) *Model { m, _ := newTestModel(t); m.switchView(viewDue); return m }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.m(t)
			assert.Equal(t, "open", labelFor(footPairs(m), "enter"))
		})
	}
}

func TestEnterKeepsItsOwnWordWhereItDoesSomethingElse(t *testing.T) {
	assert.Equal(t, "compare", labelFor(footPairs(mergeView(t)), "enter"))
	assert.Equal(t, "accept", labelFor(footPairs(syncInboxRow(t)), "enter"))
	m, _ := newTestModel(t)
	press(m, "down", "tab")
	assert.Equal(t, "edit", labelFor(footPairs(m), "enter"), "in the detail pane enter edits the field")
	m = mergeView(t)
	press(m, "enter")
	assert.Equal(t, "merge", labelFor(footPairs(m), "enter"), "the compare screen merges")
}

func TestTabOnAListReadsDetails(t *testing.T) {
	for name, m := range map[string]*Model{
		"people": func() *Model { m, _ := newTestModel(t); return m }(),
		"tasks":  threeTasks(t),
		"orgs":   twoOrgs(t),
		"deals":  threeDeals(t),
		"tags":   func() *Model { m, _ := newTestModel(t); m.switchView(viewTags); return m }(),
	} {
		assert.Equal(t, "details", labelFor(footPairs(m), "tab"), name)
	}
}

// --- S3 esc ---

func TestEscReadsCancelBackOrClose(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "down", "tab")
	assert.Equal(t, "back", labelFor(footPairs(m), "esc"), "a detail pane goes back")

	m, _ = newTestModel(t)
	m.switchView(viewTags)
	press(m, "enter")
	require.True(t, m.onTagPage())
	assert.Equal(t, "back", labelFor(footPairs(m), "esc"), "a tag page goes back")

	m, _ = newTestModel(t)
	press(m, "down", "tab", "enter")
	require.Equal(t, modeEdit, m.mode)
	assert.Equal(t, "cancel", labelFor(parseHintLine(m.hintsLine()), "esc"), "the field editor cancels")

	m, _ = newTestModel(t)
	press(m, "a", "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "cancel", labelFor(parseHintLine(m.hintsLine()), "esc"), "a prompt cancels")

	m, _ = newTestModel(t)
	press(m, "d")
	require.Equal(t, modeConfirm, m.mode)
	assert.Equal(t, "cancel", labelFor(parseHintLine(m.hintsLine()), "esc"), "a confirm cancels")

	m, _ = newTestModel(t)
	assert.Equal(t, "close", labelFor(itemPairs(m, m.helpHints()), "any key"), "help closes")
	assert.Equal(t, "back", labelFor(itemPairs(m, m.textHints()), "esc"), "a text page goes back")
}

func TestEscInPopups(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	assert.Equal(t, "cancel", labelFor(itemPairs(m, m.overlayHints()), "esc"), "the palette cancels")
	m.ov.hints = nil
	m.ov.shown = nil // nothing to pick
	assert.Equal(t, "close", labelFor(itemPairs(m, m.overlayHints()), "esc"), "a popup with nothing to pick only closes")

	m, _ = newTestModel(t)
	press(m, "x") // a step to undo
	m.openHistory(false)
	require.NotNil(t, m.ov)
	assert.Equal(t, "close", labelFor(itemPairs(m, m.overlayHints()), "esc"), "undo history closes")

	m, _ = newTestModel(t)
	press(m, "v")
	assert.Equal(t, "close", labelFor(itemPairs(m, m.overlayHints()), "esc"), "the Views popup browses and closes")

	m, _ = newTestModel(t)
	m.env.DataDir = t.TempDir()
	m.openOfferPicker(0)
	if m.ov != nil {
		assert.Equal(t, "cancel", labelFor(itemPairs(m, m.overlayHints()), "esc"), "a picker cancels")
	}

	m, _ = newTestModel(t)
	m.openListPane("deal-stages")
	assert.Equal(t, "back", labelFor(parseHintLine(m.hintsLine()), "esc"), "a Settings list pane goes back")

	m, _ = newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "task buy milk ")
	require.NotNil(t, m.ov.cmd)
	assert.Equal(t, "cancel", labelFor(itemPairs(m, m.overlayHints()), "esc"), "the command bar cancels")
}

func TestSavedViewEditorEscNeverSaves(t *testing.T) {
	m, _ := newTestModel(t)
	for _, mode := range []string{"", "text", "choice", "multi", "rel"} {
		m.sve = &svEditor{name: "x", kind: "person", labels: map[string]string{}}
		if mode != "" {
			m.sve.ed = &svCellEdit{mode: mode}
		}
		got := labelFor(itemPairs(m, m.sveHints()), "esc")
		assert.NotEqual(t, "save", got, "esc in %q mode", mode)
		want := "cancel" // the editor and its text, choice and multi cell editors cancel
		if mode == "rel" {
			want = "back" // esc keeps the picks and leaves the cell editor
		}
		assert.Equal(t, want, got, mode)
	}
	m.sve = &svEditor{name: "x", kind: "person", labels: map[string]string{}}
	assert.Equal(t, "delete", labelFor(itemPairs(m, m.sveHints()), "d"), "the condition editor deletes (was remove)")
	m.sve.ed = &svCellEdit{mode: "multi"}
	assert.Equal(t, "select", labelFor(itemPairs(m, m.sveHints()), "space"), "no add or remove")
}

// --- S4 enter in prompts and pickers ---

func TestEnterInPromptsReadsSavePickRunApply(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "a", "enter")
	require.Equal(t, modePrompt, m.mode)
	assert.Equal(t, "save", labelFor(parseHintLine(m.hintsLine()), "enter"), "an add prompt writes (was confirm)")

	m, _ = newTestModel(t)
	press(m, "t") // a task for the highlighted person
	if m.mode == modePrompt {
		assert.Equal(t, "save", labelFor(parseHintLine(m.hintsLine()), "enter"))
	}

	m, _ = newTestModel(t)
	press(m, "down", "tab", "enter")
	assert.Equal(t, "save", labelFor(parseHintLine(m.hintsLine()), "enter"))

	m, _ = newTestModel(t)
	press(m, "ctrl+k")
	assert.Equal(t, "run", labelFor(itemPairs(m, m.overlayHints()), "enter"), "the palette runs")
	typeText(m, "task buy milk ")
	assert.Equal(t, "run", labelFor(itemPairs(m, m.overlayHints()), "enter"), "the command bar runs")
	m.ov.cmd.confirming = true
	assert.Equal(t, "apply", labelFor(itemPairs(m, m.overlayHints()), "enter"), "a prepared command applies")

	m, _ = newTestModel(t)
	press(m, "l")
	require.Equal(t, modeOverlay, m.mode)
	assert.Equal(t, "pick", labelFor(itemPairs(m, m.overlayHints()), "enter"), "a picker picks")
}

func TestNoHintReadsConfirmOkOrAnOldVerb(t *testing.T) {
	old := map[string]bool{"confirm": true, "ok": true, "dismiss": true, "ignore": true, "archive": true, "close list": true, "remove": true, "add or remove": true}
	for _, sc := range hintScreens() {
		t.Run(sc.name, func(t *testing.T) {
			m := newDefaultModel(t)
			m.env.DataDir = t.TempDir()
			sc.setup(t, m)
			for _, p := range parseHintLine(m.hintsLine()) {
				assert.False(t, old[p.label], "%s %s", p.key, p.label)
			}
			if m.mode == modeOverlay && m.ov != nil {
				for _, p := range itemPairs(m, m.overlayHints()) {
					assert.False(t, old[p.label], "%s %s", p.key, p.label)
				}
			}
		})
	}
}

// --- every label is a word of the table, every verb key keeps its word ---

func TestHintLabelsStayInTheWordTable(t *testing.T) {
	words := wordTable(t)
	models := map[string]*Model{
		"people":       func() *Model { m, _ := newTestModel(t); return m }(),
		"people marks": peopleWithTwoMarks(t),
		"record page":  func() *Model { m, _ := newTestModel(t); press(m, "down", "tab"); return m }(),
		"linked row":   recordPageOnALinkedRow(t),
		"tasks":        threeTasks(t),
		"orgs":         twoOrgs(t),
		"deals":        threeDeals(t),
		"tags":         func() *Model { m, _ := newTestModel(t); m.switchView(viewTags); return m }(),
		"home":         func() *Model { m, _ := newTestModel(t); m.switchView(viewHome); return m }(),
		"merge":        mergeView(t),
		"compare":      func() *Model { m := mergeView(t); press(m, "enter"); return m }(),
		"sync":         syncInboxRow(t),
		"archived":     archivedRows(t),
		"settings":     func() *Model { m, _ := newTestModel(t); m.switchView(viewSettings); return m }(),
	}
	for name, m := range models {
		for _, w := range []int{80, 100, 160} {
			m.width = w
			for _, p := range footPairs(m) {
				assert.Contains(t, words, p.label, "%s at %d: %s %s", name, w, p.key, p.label)
				if allowed, ok := hintCanon[p.key]; ok {
					assert.Contains(t, allowed, p.label, "%s at %d: %s", name, w, p.key)
				}
			}
		}
	}
	// The mode and popup tables.
	m, _ := newTestModel(t)
	m.sve = &svEditor{name: "x", kind: "person", labels: map[string]string{}}
	tables := map[string][]hintItem{
		"edit": m.editHints(), "prompt": m.promptHints(), "names": m.namesHints(), "sve": m.sveHints(),
		"text": m.textHints(), "help": m.helpHints(),
	}
	for name, items := range tables {
		for _, p := range itemPairs(m, items) {
			assert.Contains(t, words, p.label, "%s: %s %s", name, p.key, p.label)
			if allowed, ok := hintCanon[p.key]; ok {
				assert.Contains(t, allowed, p.label, "%s: %s", name, p.key)
			}
		}
	}
}

// --- S5 contextual hints ---

func TestContextualHintsShowOnlyWhereTheVerbApplies(t *testing.T) {
	people, _ := newTestModel(t)
	assert.Equal(t, "favorite", labelFor(footPairs(people), "f"), "a person row")
	assert.Equal(t, "snooze", labelFor(footPairs(people), "s"), "a person row")

	for name, m := range map[string]*Model{
		"tasks": threeTasks(t), "orgs": twoOrgs(t), "deals": threeDeals(t), "archived": archivedRows(t), "merge": mergeView(t),
		"tags": func() *Model { m, _ := newTestModel(t); m.switchView(viewTags); return m }(),
	} {
		m.width = 200
		assert.Equal(t, "<none>", labelFor(footPairs(m), "f"), name+" has no favorite")
		assert.Equal(t, "<none>", labelFor(footPairs(m), "s"), name+" has no snooze")
	}
	empty, _ := newTestModel(t)
	empty.width = 200
	empty.rows, empty.cursor = nil, 0 // no row under the cursor
	assert.Equal(t, "<none>", labelFor(footPairs(empty), "f"), "no row, no favorite")
}

func TestUnlinkShowsOnALinkedRowOfTheDetailPane(t *testing.T) {
	m := recordPageOnALinkedRow(t)
	m.width = 200
	assert.Equal(t, "unlink", labelFor(footPairs(m), "u"))

	m, _ = newTestModel(t)
	press(m, "down", "tab") // the first row of the page is a plain field
	m.width = 200
	assert.Equal(t, "<none>", labelFor(footPairs(m), "u"), "a field row has nothing to unlink")

	m, _ = newTestModel(t)
	m.width = 200
	assert.Equal(t, "<none>", labelFor(footPairs(m), "u"), "a People list row does not show unlink")
}

func TestRestoreShowsWhereTheArchivedRowCanBeRestored(t *testing.T) {
	assert.Equal(t, "restore", labelFor(footPairs(archivedRows(t)), "r"))
}

// --- S6 footer budget ---

func TestContextualHintsAreMarkedContextual(t *testing.T) {
	m, _ := newTestModel(t)
	for _, id := range []string{"favorite", "snooze", "unlink"} {
		a := m.actionByID(id)
		require.NotNil(t, a, id)
		assert.True(t, a.footCtx, "%s drops first when the footer is short", id)
		assert.NotZero(t, a.foot, id)
	}
	assert.Less(t, m.actionByID("link").foot, m.actionByID("favorite").foot, "contextual verbs come after the existing verbs")
	assert.Less(t, m.actionByID("link").foot, m.actionByID("unlink").foot)
}

func TestContextualHintsNeverPushTheChromeOffTheFooter(t *testing.T) {
	chrome := []string{"ctrl+k", "?"}
	cases := map[string]func() *Model{
		"people":       func() *Model { m, _ := newTestModel(t); return m },
		"people marks": func() *Model { return peopleWithTwoMarks(t) },
		"home":         func() *Model { m, _ := newTestModel(t); m.switchView(viewHome); return m },
		"record page":  func() *Model { m, _ := newTestModel(t); press(m, "down", "tab"); return m },
		"linked row":   func() *Model { return recordPageOnALinkedRow(t) },
	}
	for name, mk := range cases {
		m, base := mk(), mk()
		// base is the same screen without any contextual hint.
		for i := range base.actions {
			switch base.actions[i].id {
			case "favorite", "snooze", "unlink", "unmark":
				base.actions[i].foot = 0
			}
		}
		for w := 40; w <= 130; w++ {
			m.width, base.width = w, w
			var keys, baseKeys []string
			for _, s := range m.footerSegs() {
				keys = append(keys, s.hint)
			}
			for _, s := range base.footerSegs() {
				baseKeys = append(baseKeys, s.hint)
			}
			assert.LessOrEqual(t, hintRows(m.hintsLine()), footerMaxRows, "%s at %d", name, w)
			for _, c := range append(chrome, "v") {
				if hasKey(baseKeys, c) {
					assert.Contains(t, keys, c, "%s at %d: %s dropped by a contextual hint", name, w, c)
				}
			}
			for _, b := range baseKeys {
				assert.Contains(t, keys, b, "%s at %d: %s lost to a contextual hint", name, w, b)
			}
		}
	}
}

func TestContextualHintsDropLastFirstWhenSpaceIsShort(t *testing.T) {
	m, _ := newTestModel(t)
	m.width = 200
	full := footPairs(m)
	require.Equal(t, "favorite", labelFor(full, "f"))
	require.Equal(t, "snooze", labelFor(full, "s"))
	// Find a width where only the last contextual hint (snooze) is cut: favorite stays.
	sawFavoriteWithoutSnooze := false
	for w := 200; w >= 40; w-- {
		m.width = w
		p := footPairs(m)
		if labelFor(p, "f") != "<none>" && labelFor(p, "s") == "<none>" {
			sawFavoriteWithoutSnooze = true
		}
		assert.False(t, labelFor(p, "f") == "<none>" && labelFor(p, "s") != "<none>", "snooze outlives favorite at %d", w)
	}
	assert.True(t, sawFavoriteWithoutSnooze, "snooze goes first")
}

// --- S7 the bar and the palette use the same words ---

func TestBarVerbsUseTheFooterWords(t *testing.T) {
	m, _ := newTestModel(t)
	startBar(t, m, "bob", "person:2")
	labels := barVerbLabels(m, "person:2")
	for _, want := range []string{"d delete", "l link", "u unlink", "f favorite", "s snooze", "x contacted"} {
		assert.Contains(t, labels, want)
	}
	words := wordTable(t)
	for _, l := range labels {
		if strings.HasPrefix(l, "ctrl+") {
			continue // undo, redo and the copy actions keep their titles
		}
		_, label, _ := strings.Cut(l, " ")
		assert.Contains(t, words, label, l)
	}
}

func TestPaletteHintsUseTheFooterWords(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	pairs := itemPairs(m, m.overlayHints())
	assert.Equal(t, "run", labelFor(pairs, "enter"))
	assert.Equal(t, "find", labelFor(footPairs(func() *Model { m, _ := newTestModel(t); return m }()), "ctrl+k"))
}

func hasKey(keys []string, k string) bool {
	for _, x := range keys {
		if x == k {
			return true
		}
	}
	return false
}

// While the suggestion popup of an add prompt is open, enter still saves and esc closes the popup only.
func TestPromptHintsWhileTheSuggestionPopupIsOpen(t *testing.T) {
	m := startLine(t, "deal")
	typeText(m, "Big|100|")
	require.True(t, m.pipeRefresh().ac.Open())
	pairs := parseHintLine(m.hintsLine())
	assert.Equal(t, "save", labelFor(pairs, "enter"))
	assert.Equal(t, "close", labelFor(pairs, "esc"), "esc closes the popup, not the prompt")
	assert.Equal(t, "accept", labelFor(pairs, "tab"))
}

func TestHistoryAndPanePromptsKeepTheirWords(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "x")
	m.openHistory(false)
	assert.Equal(t, "undo", labelFor(itemPairs(m, m.overlayHints()), "enter"))
	m, _ = newTestModel(t)
	press(m, "x")
	m.openHistory(true)
	if m.ov != nil {
		assert.Equal(t, "redo", labelFor(itemPairs(m, m.overlayHints()), "enter"))
	}

	m, _ = newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "task buy milk ")
	require.NotNil(t, m.ov.cmd)
	m.ov.cmd.confirming = true
	assert.Equal(t, "back", labelFor(itemPairs(m, m.overlayHints()), "esc"), "esc goes back to the command")

	m, _ = newTestModel(t)
	le := openLE(t, m, "deal-stages")
	le.phase = lpPick
	pairs := itemPairs(m, le.hints(m))
	assert.Equal(t, "pick", labelFor(pairs, "enter"), "choosing where the records go")
	assert.Equal(t, "cancel", labelFor(pairs, "esc"))
}
