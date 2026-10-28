package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startLine opens the add prompt of a kind and returns the model.
func startLine(t *testing.T, kind string) *Model {
	t.Helper()
	m, _ := newTestModel(t)
	if kind == "task" {
		m.switchView(viewTasks) // a task added on the People view is for the person on screen
		m.settle()
	}
	m.pendingCmd = objectByKind(kind).add(m)
	require.Equal(t, modePrompt, m.mode, kind)
	return m
}

func rowLabels(rows []Suggestion) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Label)
	}
	return out
}

// plainView is the screen without color codes.
func plainView(m *Model) string { return plain(m.View()) }

// Live validation (owner decision 2026-10-29): typing a pipe closes a segment and the segment is checked
// through internal/validate at once. The error shows inline with its code, what was expected and the fix.
func TestClosingASegmentValidatesItAtOnce(t *testing.T) {
	cases := []struct {
		name  string
		kind  string
		typed string
		want  string // text the screen must show; empty means no error line
	}{
		{"organization domain is not a domain", "organization", "Acme|acme|", "E_DOMAIN_INVALID"},
		{"organization domain is fine", "organization", "Acme|acme.com|", ""},
		{"organization website", "organization", "Acme||||||not a url|", "E_URL_INVALID"},
		{"task due is not a date", "task", "Call Ann|someday soon|", "E_DATE_INVALID"},
		{"task due is a date", "task", "Call Ann|next friday|", ""},
		{"task title is required", "task", "|fri", "E_FIELD_REQUIRED"},
		{"deal value is not a number", "deal", "Big|abc|", "E_NUMBER_INVALID"},
		{"deal value is a number", "deal", "Big|$12,500|", ""},
		{"deal stage is not a stage", "deal", "Big|100|nope|", "E_CHOICE_INVALID"},
		{"deal stage is a stage", "deal", "Big|100|lead|", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := startLine(t, tc.kind)
			typeText(m, tc.typed)
			s := m.pipeRefresh()
			rep := m.pipeReportFor(s, m.input.Value(), m.input.Position())
			screen := plainView(m)
			if tc.want == "" {
				assert.False(t, rep.hasErr, "no error: %s", issueText(rep.errIssue))
				assert.NotContains(t, screen, "! E_")
				return
			}
			require.True(t, rep.hasErr)
			assert.Equal(t, tc.want, rep.errIssue.Code)
			assert.Contains(t, screen, "! "+tc.want, "the error is drawn inline")
			assert.NotEmpty(t, rep.errIssue.Fix, "with its fix")
		})
	}
}

// While a segment is still being typed nothing is an error: only a soft indicator shows.
func TestTheSegmentBeingTypedShowsASoftIndicatorNeverAnError(t *testing.T) {
	cases := []struct {
		name  string
		kind  string
		typed string
		soft  string
	}{
		{"a date phrase shows the day it means", "task", "Call Ann|next friday", "next friday -> Fri Oct 23"},
		{"a date that is not one yet", "task", "Call Ann|someday", "a date"},
		{"a stage shows what matches", "deal", "Big|100|pro", "matches prospect, proposal"},
		{"no stage matches", "deal", "Big|100|zzz", "none of"},
		{"a value shows the number", "deal", "Big|$12,500", "-> 12500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := startLine(t, tc.kind)
			typeText(m, tc.typed)
			s := m.pipeRefresh()
			rep := m.pipeReportFor(s, m.input.Value(), m.input.Position())
			assert.False(t, rep.hasErr, "an open segment is never an error")
			assert.Contains(t, rep.soft, tc.soft)
			assert.Contains(t, plainView(m), tc.soft)
			assert.NotContains(t, plainView(m), "! E_")
		})
	}
}

// A smart correction of the validation framework shows as a hint, and is never applied.
func TestSmartCorrectionShowsAsAHint(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "a", "p")
	require.True(t, m.creating)
	typeText(m, "ann@example.com")
	s := m.pipeRefresh()
	rep := m.pipeReportFor(s, m.rowInput.Value(), m.rowInput.Position())
	assert.Contains(t, rep.suggest, "S_EMAIL_IN_TEXT_FIELD")
	assert.Contains(t, plainView(m), "looks like an email address")
	assert.Equal(t, "ann@example.com", m.rowInput.Value(), "the text is not changed")
}

// Enter keeps the final full validation: a closed segment with an error stops the save and keeps the line.
func TestEnterStillValidatesTheWholeLine(t *testing.T) {
	m := startLine(t, "organization")
	typeText(m, "Acme|acme")
	press(m, "enter")
	assert.Equal(t, modePrompt, m.mode, "the prompt stays open")
	assert.True(t, m.statusIsErr)
	assert.Contains(t, m.status, "E_DOMAIN_INVALID")
	assert.Equal(t, "Acme|acme", m.input.Value())
	list, _ := m.orr.FindAll(m.ctx, 0)
	assert.Empty(t, list)
}

// The popup rows come from the field type of the segment under the cursor.
func TestPopupRowsComeFromTheFieldType(t *testing.T) {
	cases := []struct {
		name  string
		kind  string
		typed string
		want  []string // the rows, in order; nil means the popup is closed
		first string   // or: only the first row
	}{
		{"relation: people by name", "task", "Call||Ba", []string{"Bob Baker"}, ""},
		{"relation: a typo is forgiven", "task", "Call||Bobb", []string{"Bob Baker"}, ""},
		{"relation: nothing typed, nothing offered", "task", "Call||", nil, ""},
		{"relation: no match", "task", "Call||Qqq", nil, ""},
		{"choice: every stage when empty", "deal", "Big|100|", []string{"lead", "prospect", "proposal", "negotiation", "won", "lost"}, ""},
		{"choice: filtered by what is typed", "deal", "Big|100|pro", []string{"prospect", "proposal"}, ""},
		{"date: phrases when empty", "task", "Call|", nil, "today"},
		{"date: phrases that fit", "task", "Call|fr", []string{"fri"}, ""},
		{"date: next phrases", "task", "Call|next", []string{"next week", "next month"}, ""},
		{"text: no popup", "task", "Cal", nil, ""},
		{"text: no popup in a note field", "organization", "Acme|ac", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := startLine(t, tc.kind)
			typeText(m, tc.typed)
			rows := rowLabels(m.pipeRefresh().ac.Rows())
			switch {
			case tc.first != "":
				require.NotEmpty(t, rows)
				assert.Equal(t, tc.first, rows[0])
			case tc.want == nil:
				assert.Empty(t, rows)
			default:
				assert.Equal(t, tc.want, rows)
			}
		})
	}
}

func TestHashNarrowsTheRelatedPopupToExistingTags(t *testing.T) {
	m, _ := newTestModel(t)
	for _, n := range []string{"alpha", "alpine", "alto", "also", "alms", "alum", "alder", "algae"} {
		require.NoError(t, m.tagr.Apply(m.ctx, "person", 1, n))
	}
	m.qtags = nil // read the tags again
	m.pendingCmd = objectByKind("task").add(m)
	typeText(m, "Call||#al")
	s := m.pipeRefresh()
	rows := s.ac.Rows()
	assert.Len(t, rows, 8, "all eight tags fit the related popup")
	assert.LessOrEqual(t, len(s.ac.Lines(40, m.st)), acMaxLines, "the popup scrolls inside its lines")
	for _, r := range rows {
		assert.NotEmpty(t, r.Tag, "a # looks among the existing tags only")
	}
	typeText(m, "g")
	assert.Equal(t, []string{"algae"}, rowLabels(s.ac.Rows()))
}

// tab accepts the highlighted row, up and down move, esc closes the popup, enter submits.
func TestPopupKeys(t *testing.T) {
	m := startLine(t, "deal")
	typeText(m, "Big|100|")
	s := m.pipeRefresh()
	require.True(t, s.ac.Open())
	assert.Contains(t, plainView(m), "tab accept", "the footer says what the keys do while the popup is open")

	press(m, "down", "down")
	hl, _ := s.ac.Highlighted()
	assert.Equal(t, "proposal", hl.Label)
	press(m, "up")
	hl, _ = s.ac.Highlighted()
	assert.Equal(t, "prospect", hl.Label)

	press(m, "esc")
	assert.Equal(t, modePrompt, m.mode, "esc closes the popup first")
	assert.False(t, s.ac.Open())
	typeText(m, "w")
	assert.True(t, s.ac.Open(), "typing opens it again")
	assert.Equal(t, []string{"won"}, rowLabels(s.ac.Rows()))

	press(m, "tab")
	assert.Equal(t, "Big|100| won", m.input.Value(), "tab puts the row into the segment")
	press(m, "esc")
	assert.Equal(t, modeNormal, m.mode, "esc with no popup cancels the prompt as before")
}

func TestEnterSubmitsEvenWhileThePopupIsOpen(t *testing.T) {
	m := startLine(t, "deal")
	typeText(m, "Big|100|pro")
	require.True(t, m.pipeRefresh().ac.Open())
	press(m, "enter")
	assert.True(t, m.statusIsErr, "pro is no stage: the line is validated as typed")
	assert.Equal(t, modePrompt, m.mode)

	m = startLine(t, "task")
	typeText(m, "Call||Bob Baker")
	require.True(t, m.pipeRefresh().ac.Open())
	press(m, "enter")
	assert.Equal(t, modeNormal, m.mode, "enter submitted although the popup was open")
}

// Tab with no popup keeps its old job.
func TestTabWithoutAPopupStillCyclesTheNoteType(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "a", "n")
	require.Equal(t, "note: ", m.input.Prompt)
	assert.False(t, m.pipeRefresh().ac.Open())
	press(m, "tab")
	assert.NotEqual(t, "note: ", m.input.Prompt, "the type changed")
}

// After a name is accepted a comma and a space follow so another can be chosen, unless a pipe is typed.
func TestCommaFollowsAnAcceptedNameUnlessAPipeIsTyped(t *testing.T) {
	m := startLine(t, "task")
	typeText(m, "Call||Bo")
	press(m, "tab")
	assert.Equal(t, "Call|| Bob Baker, ", m.input.Value())

	typeText(m, "Cara")
	assert.Equal(t, []string{"Cara Cole"}, rowLabels(m.pipeRefresh().ac.Rows()), "Bob is already in the segment and is not offered again")
	press(m, "tab")
	assert.Equal(t, "Call|| Bob Baker, Cara Cole, ", m.input.Value())

	typeText(m, "|")
	assert.Equal(t, "Call|| Bob Baker, Cara Cole |", m.input.Value(), "typing a pipe takes the comma back and starts the next segment")
	press(m, "enter")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.ElementsMatch(t, []int64{2, 3}, tasks[0].PersonIDs, "several people in one segment")
}

func TestSeveralNamesTypedByHandAreAllLinked(t *testing.T) {
	m := startLine(t, "task")
	typeText(m, "Call||Zed Adams, bob baker ,Cara")
	press(m, "enter")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.ElementsMatch(t, []int64{1, 2, 3}, tasks[0].PersonIDs)
	assert.Equal(t, 1, len(m.hist), "one undo step")
	press(m, "ctrl+z")
	left, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	assert.Empty(t, left)
}

// Backspacing over a name takes that link out before it is saved.
func TestBackspacingOverANameUnlinksItBeforeSaving(t *testing.T) {
	m := startLine(t, "task")
	typeText(m, "Call||Bo")
	press(m, "tab")
	typeText(m, "Ca")
	press(m, "tab")
	require.Equal(t, "Call|| Bob Baker, Cara Cole, ", m.input.Value())
	// Delete ", " and the name "Cara Cole".
	for i := 0; i < len(", Cara Cole, "); i++ {
		press(m, "backspace")
	}
	assert.Equal(t, "Call|| Bob Baker", m.input.Value())
	press(m, "enter")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, []int64{2}, tasks[0].PersonIDs, "Cara was taken out of the line, so she was never linked")
}

// Edit: a name taken out of a deal's people segment is unlinked when the line is saved.
func TestEditingADealUnlinksTheNamesTakenOut(t *testing.T) {
	m, _ := newTestModel(t)
	ctx := context.Background()
	d, err := m.hCreateDeal(model.CreateDealInput{Title: "Rollout", Stage: "lead"})
	require.NoError(t, err)
	deal := repo.Ref{Kind: "deal", ID: d.ID}
	for _, id := range []int64{1, 2} {
		_, err := m.links.Link(ctx, deal, repo.Ref{Kind: "person", ID: id})
		require.NoError(t, err)
	}
	m.pendingCmd = m.startDealEdit(d.ID)
	assert.Equal(t, "Rollout |  | lead |  | Zed Adams, Bob Baker", m.input.Value(), "every linked person is in the line")
	m.input.SetValue("Rollout |  | lead |  | Bob Baker")
	press(m, "enter")
	linked, err := m.links.Linked(ctx, deal, "person")
	require.NoError(t, err)
	require.Len(t, linked, 1)
	assert.Equal(t, int64(2), linked[0].ID)
}

// A deal's organization segment takes several organizations, the first is the primary one.
func TestDealOrganizationSegmentTakesSeveralAndTheFirstIsPrimary(t *testing.T) {
	m := startLine(t, "deal")
	a, err := m.hCreateOrg(model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	b, err := m.hCreateOrg(model.CreateOrgInput{Name: "Globex Corp"})
	require.NoError(t, err)
	typeText(m, "Big||| Globex Corp, Acme")
	press(m, "enter")
	deals, _ := m.dlr.FindAll(m.ctx, model.DealFilters{})
	require.Len(t, deals, 1)
	assert.ElementsMatch(t, []int64{a.ID, b.ID}, deals[0].OrgIDs)
	require.NotNil(t, deals[0].OrgID)
	assert.Equal(t, b.ID, *deals[0].OrgID, "the first organization typed is the primary one")
}

// A name that is not an object yet offers pick-or-create; a name two objects fit asks which.
func TestUnresolvedNamesOfferPickOrCreate(t *testing.T) {
	m := startLine(t, "task")
	typeText(m, "Call||Nina Park")
	assert.Contains(t, m.pipeReportFor(m.pipeRefresh(), m.input.Value(), m.input.Position()).soft, "enter offers to create")
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode, "pick-or-create")
	assert.Contains(t, m.ov.title, "Nina Park")
	var labels []string
	for _, i := range m.ov.shown {
		labels = append(labels, m.ov.items[i].label)
	}
	assert.Contains(t, labels, "Create person \"Nina Park\"")
	press(m, "enter")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.Len(t, tasks, 1)
	require.Len(t, tasks[0].PersonIDs, 1)
	nina, err := m.pr.FindByID(m.ctx, tasks[0].PersonIDs[0])
	require.NoError(t, err)
	assert.Equal(t, "Nina", nina.FirstName)
	press(m, "ctrl+z")
	left, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	assert.Empty(t, left, "the task and the link undo together")
}

func TestAmbiguousNameAsksWhichOne(t *testing.T) {
	m := startLine(t, "task")
	_, err := m.hCreatePerson(m.ctx, model.CreatePersonInput{FirstName: "Bob", LastName: sp("Brown")})
	require.NoError(t, err)
	m.afterWrite()
	m.pendingCmd = objectByKind("task").add(m)
	typeText(m, "Call||Bob")
	press(m, "enter")
	require.Equal(t, modeOverlay, m.mode)
	var labels []string
	for _, i := range m.ov.shown {
		labels = append(labels, m.ov.items[i].label)
	}
	assert.Contains(t, labels, "Bob Baker")
	assert.Contains(t, labels, "Bob Brown")
	press(m, "esc")
	assert.Equal(t, modePrompt, m.mode, "esc comes back to the line")
	assert.Equal(t, "Call||Bob", m.input.Value())
}

// A nickname finds the full name in the popup and when the line is entered.
func TestNicknamesMatchInThePopupAndOnEnter(t *testing.T) {
	m, _ := newTestModel(t)
	z, err := m.hCreatePerson(m.ctx, model.CreatePersonInput{FirstName: "Zachary", LastName: sp("Hill")})
	require.NoError(t, err)
	m.afterWrite()
	m.switchView(viewTasks)
	m.settle()
	m.pendingCmd = objectByKind("task").add(m)
	typeText(m, "Call||Zach")
	assert.Equal(t, []string{"Zachary Hill"}, rowLabels(m.pipeRefresh().ac.Rows()))
	press(m, "enter")
	tasks, _ := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.Len(t, tasks, 1)
	assert.Equal(t, []int64{z.ID}, tasks[0].PersonIDs)
}

// A note line: text, then the links. The first person is the note's subject so short syntax still reads.
func TestNoteLineLinksPeopleAndOrganizations(t *testing.T) {
	m, _ := newTestModel(t)
	o, err := m.hCreateOrg(model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	m.afterWrite()
	m.startInput("note-free", "note: ", "")
	_ = m.input.Focus()
	typeText(m, "met the team #vip|Bob Baker, Acme")
	press(m, "enter")
	bob := int64(2)
	notes, err := m.ir.FindAll(m.ctx, model.InteractionFilters{PersonID: &bob, Limit: 10})
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, "met the team", *notes[0].Content)
	byOrg, err := m.ir.FindAll(m.ctx, model.InteractionFilters{OrgID: &o.ID, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, byOrg, 1)
	tags, _ := m.tagr.GetForEntity(m.ctx, "interaction", notes[0].ID)
	require.Len(t, tags, 1, "#vip still reads")
}

// The grayscale syntax hint keeps working alongside the popup.
func TestSyntaxHintStillShowsAlongsideThePopup(t *testing.T) {
	m := startLine(t, "task")
	m.width = 200
	typeText(m, "Call|fr")
	require.True(t, m.pipeRefresh().ac.Open())
	screen := plainView(m)
	assert.Contains(t, screen, "| related", "the hint of the remaining segments")
	assert.Contains(t, screen, "fri", "and the popup")
	assert.Equal(t, "Call|fr", m.input.Value(), "neither is part of the text")
}

// The command bar is untouched: its results list is its popup and no pipe session starts.
func TestTheBarIsUnaffected(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "bob|")
	assert.Nil(t, m.pipe, "no pipe line in the bar")
	assert.Equal(t, "bob|", m.ov.filter.Value())
	assert.Nil(t, m.pipeStack())
	press(m, "tab")
	assert.NotNil(t, m.ov, "tab in the bar behaves as before")
}

// The component works for any small input behind the two-method interface.
type fakeCompleter struct{ words []string }

func (f fakeCompleter) Suggest(text string, cursor int) []Suggestion {
	var out []Suggestion
	for _, w := range f.words {
		if strings.HasPrefix(w, text) && w != text {
			out = append(out, Suggestion{Label: w})
		}
	}
	return out
}

func (f fakeCompleter) Accept(text string, cursor int, s Suggestion) (string, int) {
	return s.Label, len([]rune(s.Label))
}

func TestAutocompleteComponentWorksForAnyInput(t *testing.T) {
	in := textinput.New()
	in.Focus()
	ac := NewAutocomplete(fakeCompleter{words: []string{"alpha", "alps", "beta", "al1", "al2", "al3", "al4", "al5"}})

	in.SetValue("al")
	ac.Refresh(in.Value(), in.Position())
	assert.Len(t, ac.Rows(), acMaxRows)
	consumed, acc := ac.Key("down")
	assert.True(t, consumed)
	assert.Nil(t, acc)
	h, _ := ac.Highlighted()
	assert.Equal(t, "alps", h.Label)

	consumed, acc = ac.Key("tab")
	require.True(t, consumed)
	require.NotNil(t, acc)
	ac.Accept(&in, *acc)
	assert.Equal(t, "alps", in.Value())

	in.SetValue("be")
	ac.Refresh(in.Value(), in.Position())
	consumed, _ = ac.Key("esc")
	assert.True(t, consumed)
	assert.False(t, ac.Open())
	consumed, _ = ac.Key("tab")
	assert.False(t, consumed, "a closed popup takes no keys, tab keeps its job")
	consumed, _ = ac.Key("enter")
	assert.False(t, consumed, "enter is never taken")

	in.SetValue("zzz")
	ac.Refresh(in.Value(), in.Position())
	assert.False(t, ac.Open())
	assert.Empty(t, ac.Lines(30, defaultStyles()))
}

func TestPopupLinesAreExactlyAsWideAsAsked(t *testing.T) {
	ac := NewAutocomplete(fakeCompleter{words: []string{"alpha", "alps"}})
	ac.Refresh("al", 2)
	for _, w := range []int{12, 30} {
		for _, ln := range ac.Lines(w, defaultStyles()) {
			assert.Equal(t, w, len([]rune(plain(ln))))
		}
	}
}
