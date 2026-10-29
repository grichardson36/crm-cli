package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func words(n int) string {
	var b []string
	for i := 1; i <= n; i++ {
		b = append(b, fmt.Sprintf("word%d", i))
	}
	return strings.Join(b, " ")
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		w    int
		want []string
	}{
		{"short fits", "hello world", 20, []string{"hello world"}},
		{"wraps at spaces", "aaa bbb ccc ddd", 7, []string{"aaa bbb", "ccc ddd"}},
		{"exact width", "abcd efgh", 4, []string{"abcd", "efgh"}},
		{"newlines kept", "one\ntwo\nthree", 20, []string{"one", "two", "three"}},
		{"blank lines collapse", "one\n\n\n\ntwo", 20, []string{"one", "", "two"}},
		{"leading and trailing blanks dropped", "\n\none\n\n", 20, []string{"one"}},
		{"hard break backslash", "one\\\ntwo", 20, []string{"one", "two"}},
		{"crlf", "one\r\ntwo", 20, []string{"one", "two"}},
		{"tab becomes spaces", "a\tb", 20, []string{"a b"}},
		{"escape sequences removed", "\x1b[31mred\x1b[0m text", 20, []string{"red text"}},
		{"control characters removed", "a\x07b", 20, []string{"ab"}},
		{"long word is cut", "abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"long word after text", "ab abcdefgh", 4, []string{"ab", "abcd", "efgh"}},
		{"url longer than the column", "see https://x.io/abcdef end", 10, []string{"see", "https://x.", "io/abcdef", "end"}},
		{"bullet hangs", "- aaa bbb ccc", 9, []string{"- aaa bbb", "  ccc"}},
		{"wide characters", "日本語のテキスト", 6, []string{"日本語", "のテキ", "スト"}},
		{"wide character never splits", "ab日本", 3, []string{"ab", "日", "本"}},
		{"emoji", "😀😀😀", 4, []string{"😀😀", "😀"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.in, tt.w)
			for _, l := range got {
				assert.LessOrEqual(t, ansi.StringWidth(l), tt.w, "line %q is wider than %d", l, tt.w)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWrapTextNeverExceedsAnyWidth(t *testing.T) {
	text := "Hello 日本語のテキスト https://example.com/a/very/long/path?x=1&y=2 supercalifragilisticexpialidocious\n- item 😀😀😀 end\n\n  indented line here"
	for w := 2; w <= 60; w++ {
		for _, l := range wrapText(text, w) {
			require.LessOrEqual(t, ansi.StringWidth(l), w, "width %d line %q", w, l)
		}
	}
}

// feedFixture opens a record page of the kind with the given entries linked to it.
func feedFixture(t *testing.T, kind string, ins ...model.CreateInteractionInput) (*Model, repo.Ref) {
	t.Helper()
	m, ref := pageFixture(t, kind)
	for _, in := range ins {
		switch kind {
		case "person":
			in.PersonIDs = append(in.PersonIDs, ref.ID)
		case "organization":
			in.OrgIDs = append(in.OrgIDs, ref.ID)
		default:
			in.DealIDs = append(in.DealIDs, ref.ID)
		}
		_, err := m.ir.Create(context.Background(), in)
		require.NoError(t, err)
	}
	m.afterWrite()
	press(m, "tab")
	require.Equal(t, focusDetail, m.focus)
	return m, ref
}

func entry(typ, date, text string) model.CreateInteractionInput {
	return model.CreateInteractionInput{Type: typ, Content: sp(text), OccurredAt: sp(date + " 10:00:00")}
}

func feedRows(m *Model) []int {
	var out []int
	for i, r := range m.form {
		if isFeedRow(r) {
			out = append(out, i)
		}
	}
	return out
}

func focusEntry(t *testing.T, m *Model, n int) {
	t.Helper()
	rows := feedRows(m)
	require.Greater(t, len(rows), n)
	m.fcur, m.fcol = rows[n], 0
	m.syncDetail()
}

func plainLines(m *Model, i int) []string {
	var out []string
	for _, l := range m.renderRow(i, m.form[i], m.detail.Width()) {
		out = append(out, ansi.Strip(l))
	}
	return out
}

func TestFeedOnEveryRecordPage(t *testing.T) {
	for _, kind := range []string{"person", "organization", "deal"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := feedFixture(t, kind,
				entry("note", "2026-10-27", "newest note"),
				entry("email", "2026-08-01", "older email"))
			rows := feedRows(m)
			require.Len(t, rows, 2)
			assert.Equal(t, "Recent", m.form[rows[0]].head)
			assert.Equal(t, "newest note", m.form[rows[0]].value, "newest first")
			assert.Equal(t, "older email", m.form[rows[1]].value)
			content := ansi.Strip(m.detail.GetContent())
			assert.Contains(t, content, "newest note")
			assert.Contains(t, content, "older email")
		})
	}
}

func TestFeedLayoutDateTypeAndGroups(t *testing.T) {
	m, _ := feedFixture(t, "organization",
		entry("note", "2026-10-27", "first of the day"),
		entry("call", "2026-10-27", "second of the day"),
		entry("email", "2026-08-01", "another day"))
	rows := feedRows(m)
	require.Len(t, rows, 3)
	first, second, third := plainLines(m, rows[0])[0], plainLines(m, rows[1])[0], plainLines(m, rows[2])[0]

	assert.Contains(t, first, "2026-10-27")
	assert.Less(t, strings.Index(first, "2026-10-27"), strings.Index(first, "note"), "the type is right of the date")
	assert.Less(t, strings.Index(first, "note"), strings.Index(first, "first of the day"))
	assert.NotContains(t, second, "2026-10-27", "one date heading per day")
	assert.Contains(t, second, "call")
	assert.Equal(t, strings.Index(first, "note"), strings.Index(second, "call"), "types line up")
	assert.Contains(t, third, "2026-08-01")
	assert.Contains(t, third, "email")

	// A blank line separates the days, not the entries of one day.
	var seq []string
	for _, l := range strings.Split(ansi.Strip(m.detail.GetContent()), "\n") {
		seq = append(seq, strings.TrimSpace(l))
	}
	idx := func(s string) int {
		for i, l := range seq {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}
	assert.Equal(t, idx("first of the day")+1, idx("second of the day"))
	assert.Equal(t, idx("second of the day")+2, idx("another day"))
}

func TestFeedLinkedObjectsInTheLeftColumn(t *testing.T) {
	m, ref := pageFixture(t, "person")
	ctx := context.Background()
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme Test"})
	require.NoError(t, err)
	d, err := m.dlr.Create(ctx, model.CreateDealInput{Title: "Big Deal", Stage: "lead"})
	require.NoError(t, err)
	in := entry("note", "2026-10-27", "text of the note\nsecond line\nthird line\nfourth")
	in.PersonIDs, in.OrgIDs, in.DealIDs = []int64{ref.ID, 1}, []int64{o.ID}, []int64{d.ID}
	_, err = m.ir.Create(ctx, in)
	require.NoError(t, err)
	m.afterWrite()
	press(m, "tab")

	rows := feedRows(m)
	require.Len(t, rows, 1)
	lines := plainLines(m, rows[0])
	require.GreaterOrEqual(t, len(lines), 4)
	body := strings.Join(lines[1:], "\n")
	for _, want := range []string{"Zed Adams", "Acme Test", "Big Deal"} {
		assert.Contains(t, body, want, "in the left column")
	}
	for _, l := range lines {
		assert.NotContains(t, l, "Bob Baker", "the page's own object is left out")
	}
	assert.Contains(t, lines[1], "second line", "the text continues on the lines beside the objects")
	assert.NotContains(t, strings.Join(lines, "\n"), "o:", "a colored theme has no prefix")

	// Without color the prefix names the kind, the same prefixes as the related segment.
	m.st.Kind = map[string]lipgloss.Style{}
	m.syncDetail()
	text := ansi.Strip(m.detail.GetContent())
	assert.Contains(t, text, "o:Acme Test")
	assert.Contains(t, text, "p:Zed Adams")
	assert.Contains(t, text, "d:Big Deal")
}

func TestFeedLinkedObjectsAreColoredByKind(t *testing.T) {
	m, _ := pageFixture(t, "organization")
	r, _ := m.selected()
	in := entry("note", "2026-10-27", "hi")
	in.OrgIDs, in.PersonIDs = []int64{r.orgID}, []int64{1}
	_, err := m.ir.Create(context.Background(), in)
	require.NoError(t, err)
	m.afterWrite()
	press(m, "tab")
	rows := feedRows(m)
	require.Len(t, rows, 1)
	lines := m.renderRow(rows[0], m.form[rows[0]], m.detail.Width())
	require.NotEqual(t, "Zed Adams", m.st.KindStyle("person").Render("Zed Adams"), "the test theme colors the person kind")
	assert.Contains(t, lines[1], m.st.KindStyle("person").Render("Zed Adams"))
}

func TestFeedTruncatesToFiveLinesWithMarker(t *testing.T) {
	tests := []struct {
		name       string
		lines      int
		wantLines  int
		wantMarker bool
	}{
		{"one line", 1, 1, false},
		{"four lines", 4, 4, false},
		{"exactly five", 5, 5, false},
		{"six lines", 6, 5, true},
		{"twenty lines", 20, 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var text []string
			for i := 1; i <= tt.lines; i++ {
				text = append(text, fmt.Sprintf("line %d", i))
			}
			m, _ := feedFixture(t, "person", entry("note", "2026-10-27", strings.Join(text, "\n")))
			lines := plainLines(m, feedRows(m)[0])
			assert.Len(t, lines, tt.wantLines)
			joined := strings.Join(lines, "\n")
			assert.Equal(t, tt.wantMarker, strings.Contains(joined, "..."), joined)
			if tt.wantMarker {
				assert.True(t, strings.HasSuffix(strings.TrimRight(lines[len(lines)-1], " "), "..."), "the marker ends the last line")
				assert.NotContains(t, joined, fmt.Sprintf("line %d", tt.lines), "hidden text is not shown")
			}
		})
	}
}

func TestFeedMarkerIsDim(t *testing.T) {
	m, _ := feedFixture(t, "person", entry("note", "2026-10-27", words(80)))
	rows := feedRows(m)
	m.fcur = 0 // not selected
	lines := m.renderRow(rows[0], m.form[rows[0]], m.detail.Width())
	assert.Contains(t, lines[len(lines)-1], m.st.Dim.Render("..."))
}

func TestFeedWrapsLongTextToThePane(t *testing.T) {
	for _, w := range []int{70, 90, 100, 120, 160} {
		t.Run(fmt.Sprint(w), func(t *testing.T) {
			m, _ := feedFixture(t, "person", entry("email", "2026-10-27", words(60)+" https://example.com/"+strings.Repeat("x", 120)+" 日本語のテキスト"))
			m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
			rows := feedRows(m)
			for _, l := range m.renderRow(rows[0], m.form[rows[0]], m.detail.Width()) {
				assert.LessOrEqual(t, ansi.StringWidth(l), m.detail.Width(), "%q", ansi.Strip(l))
			}
			for _, l := range strings.Split(ansi.Strip(m.detail.GetContent()), "\n") {
				assert.LessOrEqual(t, ansi.StringWidth(l), m.detail.Width())
			}
		})
	}
}

func TestFeedResizeRewrapsAndStacksWhenNarrow(t *testing.T) {
	m, _ := feedFixture(t, "person", entry("email", "2026-10-27", words(30)))
	rows := feedRows(m)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	wide := plainLines(m, rows[0])
	assert.False(t, m.feedLayoutFor(m.detail.Width()).stacked)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	narrow := plainLines(m, rows[0])
	for _, l := range m.renderRow(rows[0], m.form[rows[0]], m.detail.Width()) {
		assert.LessOrEqual(t, ansi.StringWidth(l), m.detail.Width())
	}
	assert.NotEqual(t, wide, narrow)
	assert.True(t, m.feedLayoutFor(m.detail.Width()).stacked, "a narrow pane stacks the text under the heading, width %d", m.detail.Width())
	assert.Contains(t, narrow[0], "2026-10-27")
	assert.Contains(t, narrow[0], "email")
	assert.NotContains(t, narrow[0], "word1")
}

func TestFeedEnterExpandsAndCollapsesInPlace(t *testing.T) {
	m, _ := feedFixture(t, "person", entry("note", "2026-10-27", words(80)), entry("note", "2026-10-20", "older"))
	rows := feedRows(m)
	focusEntry(t, m, 0)
	collapsed := len(plainLines(m, rows[0]))
	assert.Equal(t, feedMaxLines, collapsed)

	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode, "enter expands, it does not edit")
	expanded := plainLines(m, rows[0])
	assert.Greater(t, len(expanded), collapsed)
	assert.NotContains(t, strings.Join(expanded, "\n"), "...", "an expanded entry shows everything")
	assert.Contains(t, strings.Join(expanded, "\n"), "word80")
	assert.Equal(t, rows[0], m.fcur, "the cursor stays on the entry")

	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode)
	assert.Len(t, plainLines(m, rows[0]), collapsed, "enter again collapses")
	assert.Contains(t, strings.Join(plainLines(m, rows[0]), "\n"), "...")

	press(m, "e")
	assert.Equal(t, modeNormal, m.mode, "e is the same action as enter")
	assert.Greater(t, len(plainLines(m, rows[0])), collapsed)
}

func TestFeedEnterEditsAnEntryThatShowsEverything(t *testing.T) {
	m, _ := feedFixture(t, "person", entry("note", "2026-10-27", "short"))
	focusEntry(t, m, 0)
	press(m, "enter")
	assert.Equal(t, modeEdit, m.mode)
}

func TestFeedLongEntryEditsFromThePalette(t *testing.T) {
	m, _ := feedFixture(t, "person", entry("note", "2026-10-27", words(80)))
	focusEntry(t, m, 0)
	runPalette(m, "edit note")
	assert.Equal(t, modeEdit, m.mode, "Edit note opens the editor of a long entry")
}

func TestFeedEditsTextInPlaceWithAltEnterNewLine(t *testing.T) {
	m, _ := feedFixture(t, "organization", entry("note", "2026-10-27", "draft"))
	focusEntry(t, m, 0)
	id := m.form[m.fcur].id
	press(m, "enter")
	require.Equal(t, modeEdit, m.mode)
	replaceText(m, "line one")
	press(m, "alt+enter")
	typeText(m, "line two")
	assert.Contains(t, ansi.Strip(m.screen()), "alt+enter new line")
	press(m, "enter")
	assert.Equal(t, "note saved", m.status)
	assert.Equal(t, "line one\nline two", str(noteContent(t, m, id).Content))
	assert.Equal(t, id, m.form[m.fcur].id)
	lines := plainLines(m, m.fcur)
	assert.Contains(t, lines[0], "line one")
	assert.Contains(t, lines[1], "line two")
}

func TestFeedEscCancelsEdit(t *testing.T) {
	m, _ := feedFixture(t, "deal", entry("note", "2026-10-27", "keep"))
	focusEntry(t, m, 0)
	id := m.form[m.fcur].id
	press(m, "enter")
	replaceText(m, "lost")
	press(m, "esc")
	assert.Equal(t, "keep", str(noteContent(t, m, id).Content))
}

func TestFeedDeleteAndRestore(t *testing.T) {
	for _, kind := range []string{"person", "organization", "deal"} {
		t.Run(kind, func(t *testing.T) {
			m, ref := feedFixture(t, kind, entry("note", "2026-10-27", "one"), entry("email", "2026-10-26", "two"))
			focusEntry(t, m, 0)
			press(m, "d")
			require.Equal(t, modeConfirm, m.mode)
			press(m, "y")
			assert.Len(t, notesAbout(t, m, ref), 1)
			assert.Len(t, feedRows(m), 1)
			runPalette(m, "restore note")
			assert.Len(t, notesAbout(t, m, ref), 2)
			assert.Len(t, feedRows(m), 2)
		})
	}
}

func TestFeedSpaceSelectsAndBulkDeletes(t *testing.T) {
	m, ref := feedFixture(t, "person",
		entry("note", "2026-10-27", "one"), entry("email", "2026-10-26", "two"), entry("call", "2026-10-25", "three"))
	rows := feedRows(m)
	focusEntry(t, m, 0)
	press(m, " ")
	assert.Equal(t, rows[1], m.fcur, "space selects and moves down")
	press(m, " ")
	assert.Len(t, m.feed.marks, 2)
	m.fcur = 0
	assert.Contains(t, ansi.Strip(strings.Join(m.renderRow(rows[0], m.form[rows[0]], m.detail.Width()), "\n")), "*")

	focusEntry(t, m, 0)
	press(m, "d")
	require.NotEqual(t, modeNormal, m.mode, "a bulk delete asks first")
	require.NotEqual(t, modeConfirm, m.mode, "a bulk delete types the count back")
	typeText(m, "delete 2")
	press(m, "enter")
	left := notesAbout(t, m, ref)
	require.Len(t, left, 1)
	assert.Equal(t, "three", str(left[0].Content))
	assert.Empty(t, m.feed.marks)
}

func TestFeedLinkVerbActsOnTheEntry(t *testing.T) {
	m, _ := feedFixture(t, "person", entry("note", "2026-10-27", "one"))
	focusEntry(t, m, 0)
	src, ok := m.cursorObject()
	require.True(t, ok)
	assert.Equal(t, model.NoteKind, src.Kind)
}

func TestFeedTypeCellStillEdits(t *testing.T) {
	m, _ := feedFixture(t, "person", entry("note", "2026-10-27", "one"))
	focusEntry(t, m, 0)
	id := m.form[m.fcur].id
	setCell(t, m, 1, "meeting")
	assert.Equal(t, "meeting", noteContent(t, m, id).Type)
	assert.Contains(t, plainLines(m, m.fcur)[0], "meet")
}

func TestFeedSubjectShowsFirstInBold(t *testing.T) {
	in := entry("email", "2026-10-27", "body text")
	in.Subject = sp("Re: pricing")
	m, _ := feedFixture(t, "person", in)
	rows := feedRows(m)
	m.fcur = 0
	lines := plainLines(m, rows[0])
	assert.Contains(t, lines[0], "Re: pricing")
	assert.Contains(t, lines[1], "body text")
	raw := m.renderRow(rows[0], m.form[rows[0]], m.detail.Width())
	assert.Contains(t, raw[0], m.st.Head.Render("Re: pricing"))
}

func TestFeedScrollsAsOneDocumentAndTheCursorMovesByEntry(t *testing.T) {
	var ins []model.CreateInteractionInput
	for i := 0; i < 12; i++ {
		ins = append(ins, entry("note", fmt.Sprintf("2026-09-%02d", 20-i), fmt.Sprintf("entry%d\nbody\nbody\nbody", i)))
	}
	m, _ := feedFixture(t, "person", ins...)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	rows := feedRows(m)
	require.Len(t, rows, 12)
	focusEntry(t, m, 0)
	inView := func() {
		assert.GreaterOrEqual(t, m.rowStart[m.fcur], m.detail.YOffset(), "the viewport follows the cursor")
		assert.LessOrEqual(t, m.rowEnd[m.fcur], m.detail.YOffset()+m.detail.Height()-1)
	}
	for i := 1; i < 12; i++ {
		press(m, "down")
		assert.Equal(t, rows[i], m.fcur, "down moves entry by entry")
		inView()
	}
	for i := 10; i >= 0; i-- {
		press(m, "up")
		assert.Equal(t, rows[i], m.fcur)
		inView()
	}
	assert.Contains(t, ansi.Strip(m.screen()), "entry0")
}

func TestFeedCtrlUpAndDownJumpByEntry(t *testing.T) {
	m, _ := feedFixture(t, "organization",
		entry("note", "2026-10-27", "one"), entry("email", "2026-10-26", "two"), entry("call", "2026-10-25", "three"))
	rows := feedRows(m)
	focusEntry(t, m, 0)
	press(m, "ctrl+down")
	assert.Equal(t, rows[1], m.fcur)
	press(m, "ctrl+down")
	assert.Equal(t, rows[2], m.fcur)
	press(m, "ctrl+up")
	assert.Equal(t, rows[1], m.fcur)
}

func TestFeedExpandedEntryTallerThanThePaneScrollsInside(t *testing.T) {
	var text []string
	for i := 1; i <= 120; i++ {
		text = append(text, fmt.Sprintf("row %03d", i))
	}
	m, _ := feedFixture(t, "person", entry("note", "2026-10-27", strings.Join(text, "\n")), entry("note", "2026-10-20", "after"))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	rows := feedRows(m)
	focusEntry(t, m, 0)
	press(m, "enter")
	require.True(t, m.feed.open[m.form[m.fcur].id])
	top, bottom := m.rowStart[m.fcur], m.rowEnd[m.fcur]
	h := m.detail.Height()
	require.Greater(t, bottom-top+1, h)
	assert.Equal(t, top, m.detail.YOffset(), "the viewport starts at the top of the entry")

	press(m, "down")
	assert.Equal(t, rows[0], m.fcur, "down scrolls inside the entry first")
	assert.Equal(t, top+1, m.detail.YOffset())
	press(m, "pgdown")
	assert.Equal(t, rows[0], m.fcur)
	assert.Greater(t, m.detail.YOffset(), top+1)
	for i := 0; i < 200 && m.fcur == rows[0]; i++ {
		press(m, "pgdown")
	}
	assert.Equal(t, rows[1], m.fcur, "at the end of the entry the cursor moves on")
	assert.Contains(t, ansi.Strip(m.screen()), "after")

	press(m, "up")
	assert.Equal(t, rows[0], m.fcur)
	assert.Equal(t, bottom-h+1, m.detail.YOffset(), "moving up shows the end of the tall entry")
	press(m, "up")
	assert.Equal(t, bottom-h, m.detail.YOffset())
	assert.Contains(t, ansi.Strip(m.screen()), "row 119")
}

func TestFeedOlderEntriesNoteWhenMoreThanTheLimit(t *testing.T) {
	var ins []model.CreateInteractionInput
	for i := 0; i < feedLimit+3; i++ {
		ins = append(ins, entry("note", "2026-01-01", fmt.Sprintf("n%d", i)))
	}
	m, _ := feedFixture(t, "person", ins...)
	assert.Len(t, feedRows(m), feedLimit)
	assert.Contains(t, ansi.Strip(m.detail.GetContent()), "older entries are not shown")
}

func TestFeedExpandedStateIsDroppedWhenTheEntryIsDeleted(t *testing.T) {
	m, _ := feedFixture(t, "person", entry("note", "2026-10-27", words(80)))
	focusEntry(t, m, 0)
	id := m.form[m.fcur].id
	press(m, "enter")
	require.True(t, m.feed.open[id])
	press(m, "d", "y")
	assert.False(t, m.feed.open[id])
}
