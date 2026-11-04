package tui

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-025: the key text of every hint comes from hintkeys.go.

func TestKeyTextTable(t *testing.T) {
	want := map[string]string{
		"up": "↑", "down": "↓", "left": "←", "right": "→", "up/down": "↑/↓", "left/right": "←/→",
		"shift+up": "shift+↑", "shift+down": "shift+↓", "ctrl+left": "ctrl+←", "ctrl+right": "ctrl+→",
		"ctrl+shift+up": "ctrl+shift+↑", "shift+up/down": "shift+↑/↓",
		"enter": "enter", "esc": "esc", "tab": "tab", "shift+tab": "shift+tab", " ": "space", "space": "space",
		"home": "home", "end": "end", "home/end": "home/end", "pgup": "pgup", "pgdown": "pgdown",
		"ctrl+k": "ctrl+k", "ctrl+z": "ctrl+z", "?": "?", "q": "q", "x": "x", "shift+r": "shift+r", "+": "+",
	}
	for in, out := range want {
		assert.Equal(t, out, showKey(in), in)
	}
	assert.Equal(t, hkUpDown, "up/down")
	assert.Equal(t, "↑/↓", showKey(hkUpDown))
	assert.Equal(t, "←/→", showKey(hkLeftRight))
	assert.Equal(t, "home/end", showKey(hkHomeEnd))
	assert.Equal(t, "shift+↑/↓", showKey(hkShiftUpDown))
	assert.Equal(t, "↑", showKey(hkUp))
	assert.Equal(t, "↓", showKey(hkDown))
	assert.Equal(t, "←", showKey(hkLeft))
	assert.Equal(t, "→", showKey(hkRight))
}

func TestKeyGlyphsAreOneCellWide(t *testing.T) {
	for _, g := range []string{"↑", "↓", "←", "→"} {
		assert.Equal(t, 1, ansi.StringWidth(g), g)
	}
	assert.Equal(t, 3, ansi.StringWidth(showKey("up/down")))
	assert.Equal(t, len("up/down")-4, ansi.StringWidth(showKey("up/down")))
}

var arrowWord = regexp.MustCompile(`^((shift|ctrl|alt)\+)*(up|down|left|right)(/(up|down|left|right))?$`)

// TestKeyTextIsNeverALiteralWord: a hint names its arrows with the hk constants, and no code shows a
// key through keys.Display, so every key a person reads goes through showKey.
func TestKeyTextIsNeverALiteralWord(t *testing.T) {
	fset := token.NewFileSet()
	files := parseNonTestFiles(t, fset)
	var bad []string
	str := func(e ast.Expr) (string, bool) {
		l, ok := e.(*ast.BasicLit)
		if !ok || l.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(l.Value)
		return s, err == nil
	}
	check := func(pos token.Pos, e ast.Expr, what string) {
		if s, ok := str(e); ok && (arrowWord.MatchString(s) || strings.ContainsAny(s, "↑↓←→")) {
			bad = append(bad, fset.Position(pos).String()+" literal key text "+strconv.Quote(s)+" in "+what)
		}
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "fk" && len(x.Args) == 2 {
					check(x.Pos(), x.Args[0], "fk")
				}
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if p, ok := sel.X.(*ast.Ident); ok && p.Name == "keys" && sel.Sel.Name == "Display" {
						bad = append(bad, fset.Position(x.Pos()).String()+" keys.Display shows a key, use showKey")
					}
				}
			case *ast.KeyValueExpr:
				if k, ok := x.Key.(*ast.Ident); ok && k.Name == "hint" {
					check(x.Pos(), x.Value, "a hint field")
				}
			case *ast.AssignStmt:
				for i, l := range x.Lhs {
					if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == "hint" && i < len(x.Rhs) {
						check(x.Pos(), x.Rhs[i], "a hint assignment")
					}
				}
			}
			return true
		})
	}
	assert.Empty(t, bad, "arrow key text comes from hintkeys.go")
}

// footerKeyWord matches a hint that still names an arrow with a word: "up/down select", "left move".
var footerKeyWord = regexp.MustCompile(`(^|\s)((shift|ctrl|alt)\+)*(up|down|left|right)(/(up|down|left|right))?\s+[a-z]`)

func TestNoFooterNamesAnArrowWithAWord(t *testing.T) {
	for _, sc := range hintScreens() {
		t.Run(sc.name, func(t *testing.T) {
			m := newDefaultModel(t)
			m.env.DataDir = t.TempDir()
			sc.setup(t, m)
			_, footer := footerOnly(m, sc.footerLines)
			for _, l := range footer {
				assert.NotRegexp(t, footerKeyWord, l)
			}
		})
	}
}

func TestMainFooterAndModesShowArrowGlyphs(t *testing.T) {
	m := newDefaultModel(t)
	assert.Contains(t, footHints(m), "↑/↓", "the main footer")
	assert.Contains(t, hintsText(m), "↑/↓ move")

	// Views popup.
	press(m, "v")
	_, f := footerOnly(m, 1)
	assert.Contains(t, f[0], "↑/↓ select")
	assert.Contains(t, f[0], "shift+↑/↓ move")
	press(m, "esc")

	// Palette: a selector row offers left and right.
	press(m, "ctrl+o")
	_, f = footerOnly(m, 1)
	assert.Contains(t, f[0], "←/→ change")
	assert.Contains(t, f[0], "↑/↓ select")
	press(m, "esc")

	// Help keys page.
	press(m, "?")
	assert.Contains(t, ansi.Strip(m.hintsLine()+m.screen()), "↑/↓ scroll")
	press(m, "esc")

	// Prompt with a suggestion popup and an input-less prompt.
	m2 := newDefaultModel(t)
	m2.status = ""
	assert.Contains(t, ansi.Strip(m2.renderHintItems(m2.sveHints())), "↑/↓ row")
	assert.Contains(t, ansi.Strip(m2.renderHintItems(m2.sveHints())), "←/→ cell")
	assert.Contains(t, ansi.Strip(m2.renderHintItems(m2.namesHints())), "↑/↓ move")
	assert.Contains(t, ansi.Strip(m2.renderHintItems(m2.textHints())), "↑/↓ scroll")
	assert.Contains(t, ansi.Strip(m2.renderHintItems(m2.textHints())), "home/end top or bottom")
}

func TestListEditorSettingsPaneAndHistoryShowArrowGlyphs(t *testing.T) {
	m := settingsModel(t)
	m.openListPane("deal-stages")
	assert.Contains(t, hintsText(m), "↑/↓")
	assert.NotRegexp(t, footerKeyWord, hintsText(m))

	m = settingsModel(t)
	cursorToSetting(t, m, "colors")
	press(m, "enter")
	require.NotNil(t, m.paneSP())
	assert.Contains(t, hintsText(m), "↑/↓ select")

	m, _ = newTestModel(t)
	press(m, "f")
	m.openHistory(false)
	require.NotNil(t, m.ov)
	assert.Contains(t, ansi.Strip(m.renderHintItems(m.ov.hints)), "↑/↓ select")
}

func TestCompareAndTreeRegistryHintsShowGlyphs(t *testing.T) {
	m, _ := newTestModel(t)
	for _, id := range []string{"cmp-left", "cmp-right", "cell-next", "cell-prev", "down", "up"} {
		a := m.actionByID(id)
		require.NotNil(t, a, id)
		assert.NotRegexp(t, arrowWord, showKey(a.hint), id)
	}
	assert.Equal(t, "←/→", showKey(m.actionByID("cmp-left").hint))
	assert.Equal(t, "→", showKey(m.actionByID("cell-next").hint))
}

func TestPaletteKeyColumnAndKeysPageShowArrowGlyphs(t *testing.T) {
	m, _ := newTestModel(t)
	help := ansi.Strip(m.helpText())
	assert.Regexp(t, `(?m)^↓\s+next row`, help, "the keys page")
	assert.Regexp(t, `(?m)^shift\+↓\s`, help)
	assert.NotRegexp(t, `(?m)^(down|up|shift\+down|shift\+up)\s`, help)

	press(m, "ctrl+k")
	typeText(m, "move down")
	var row string
	for _, l := range strings.Split(ansi.Strip(m.screen()), "\n") {
		if strings.Contains(l, "Move down") {
			row = l
		}
	}
	require.NotEmpty(t, row)
	assert.Contains(t, row, "↓ ")
	assert.NotRegexp(t, `\bdown\b`, strings.Replace(row, "Move down", "", 1))
}

func TestPickerKeyColumnShowsGlyphsAndSizesByThem(t *testing.T) {
	m, _ := newTestModel(t)
	m.openPicker("Keys", []overlayItem{{id: "a", label: "Alpha", hint: "up/down"}, {id: "b", label: "Beta", hint: "left"}}, func(*Model, overlayItem) {})
	require.NotNil(t, m.ov)
	assert.Equal(t, 3+2, m.ov.hintWidth(), "the column is as wide as the shown text")
	screen := ansi.Strip(m.screen())
	assert.Regexp(t, `↑/↓\s+Alpha`, screen)
	assert.Regexp(t, `←\s+Beta`, screen)
}

func TestKeyNamesInDescriptionsUseTheKeyText(t *testing.T) {
	m, _ := newTestModel(t)
	assert.Contains(t, m.actionByID("extend-down").desc, "shift+↑")
	assert.Contains(t, m.actionByID("extend-up").desc, "shift+↓")
	assert.Contains(t, m.actionByID("cmp-field").desc, "↑/↓")
	assert.Contains(t, m.actionByID("cmp-pick").desc, "←/→")
	assert.Contains(t, m.actionByID("focus-detail").desc, "→")
	assert.Contains(t, m.actionByID("goto-view").desc, "shift+↑ and shift+↓")
}

func TestBarVerbKeysAreShownThroughTheKeyText(t *testing.T) {
	m, _ := newTestModel(t)
	m.applyKeys(keys.Overrides{"favorite": {"alt+left"}})
	var got string
	for _, v := range m.barVerbs(barTarget{kind: "person", id: 1}) {
		if v.id == "favorite" {
			got = v.key
		}
	}
	assert.Equal(t, "alt+←", got)
}

func TestReboundKeyShowsWhatItIsReboundTo(t *testing.T) {
	assert.Equal(t, "ctrl+g", showKey("ctrl+g"))
	assert.Equal(t, "alt+←", showKey("alt+left"))
	assert.Equal(t, "ctrl+k, /", joinKeys([]string{"ctrl+k", "/"}))
	assert.Equal(t, "space, ↑", joinKeys([]string{" ", "up"}))
}

// The footer wraps and the mouse hit boxes use the width of the shown text, so a glyph counts as one cell.
func TestFooterWrapAndHitBoxesCountGlyphWidth(t *testing.T) {
	m, _ := newTestModel(t)
	for w := 40; w <= 130; w++ {
		m.width = w
		lines := strings.Split(ansi.Strip(m.hintsLine()), "\n")
		for _, s := range m.footerSegs() {
			assert.NotRegexp(t, arrowWord, s.hint, "seg text is the shown text")
			require.Less(t, s.row, len(lines), "width %d", w)
			want := s.hint + " " + s.label
			assert.Equal(t, ansi.StringWidth(want), s.x1-s.x0, "width %d: %s", w, want)
			assert.Equal(t, want, ansi.Cut(lines[s.row], s.x0, s.x1), "width %d: hit box of %q", w, want)
			assert.LessOrEqual(t, s.x1, w)
		}
		for _, l := range lines {
			assert.LessOrEqual(t, ansi.StringWidth(l), w)
		}
	}
}
