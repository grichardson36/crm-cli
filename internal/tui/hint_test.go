package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hint text is a pure function of the field list and the text typed, so it renders the same
// every time.
func TestPipeHintShowsTheRemainingFields(t *testing.T) {
	fields := []string{"title", "value", "stage"}
	tests := []struct {
		name  string
		typed string
		want  string
	}{
		{"empty shows the whole shape", "", "title | value | stage"},
		{"typing the first field shows the rest", "Web", " | value | stage"},
		{"a trailing space joins cleanly", "Website ", "| value | stage"},
		{"a pipe moves to the next field by name", "Website |", " value | stage"},
		{"pipe and space", "Website | ", "value | stage"},
		{"second field filled", "Website | 500", " | stage"},
		{"last field named", "Website | 500 |", " stage"},
		{"every field filled", "Website | 500 | won", ""},
		{"more pipes than fields", "a | b | c | d", ""},
		{"the field you empty comes back", "Website | | won", ""},
		{"an emptied middle field is named again", "Website | ", "value | stage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pipeHint(fields, tt.typed))
		})
	}
	assert.Equal(t, "", pipeHint(nil, "x"), "no fields, no hint")
}

func TestBarHintDropsFieldsAsTheyAreFilled(t *testing.T) {
	task := objectByKind("task").barFields
	note := objectByKind(model.NoteKind).barFields
	tests := []struct {
		name   string
		fields []hintField
		line   string
		rest   string
		want   string
	}{
		{"verb alone", task, "task", "", " title by date @person #tag !priority every 3m"},
		{"verb and space", task, "task ", "", "title by date @person #tag !priority every 3m"},
		{"title typed", task, "task Call mom", "Call mom", " by date @person #tag !priority every 3m"},
		{"deadline typed", task, "task Call mom by fri", "Call mom by fri", " @person #tag !priority every 3m"},
		{"tokens typed", task, "task Call !high #family ", "Call !high #family ", "by date @person every 3m"},
		{"a note", note, "note ", "", "text @person #tag remind fri"},
		{"a note with a person", note, "note @Sam met", "@Sam met", " #tag remind fri"},
		{"everything filled", note, "note met @Sam #x remind fri", "met @Sam #x remind fri", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, barHint(tt.fields, tt.line, tt.rest))
		})
	}
}

// Every object that registers pipe prompts and fields gets the hint from the one component, and the
// fields shown are the registry's own list.
func TestEveryRegisteredPromptHasItsFieldsFromTheRegistry(t *testing.T) {
	m, _ := newTestModel(t)
	for _, o := range objectDefs() {
		for _, kind := range o.prompts {
			t.Run(kind, func(t *testing.T) {
				require.NotEmpty(t, o.fields, "%s registers a prompt and needs fields", o.kind)
				assert.Equal(t, o.fields, m.promptFieldsFor(kind))
			})
		}
	}
	for _, kind := range []string{"org-add", "org-edit", "deal-add", "deal-edit", "task-new", "task-edit", "task", "task-page", "note", "note-object"} {
		assert.NotEmpty(t, m.promptFieldsFor(kind), kind)
	}
	assert.Empty(t, m.promptFieldsFor("snooze"), "a plain prompt has no shape")
}

// footerText is the prompt line as plain text with runs of spaces collapsed (the cursor takes a cell).
func footerText(m *Model) string {
	m.width = 120
	first := strings.SplitN(plain(m.footerView()), string(rune(10)), 2)[0]
	return strings.Join(strings.Fields(first), " ")
}

func TestPromptShowsTheShapeWhileTyping(t *testing.T) {
	tests := []struct {
		name   string
		start  func(m *Model)
		typed  string
		wantIn string
		not    string
	}{
		{"org empty", func(m *Model) { m.startInput("org-add", orgPrompt(), "") }, "",
			"organization: name | domain | industry | size | location | website | linkedin | description | notes", ""},
		{"org name typed", func(m *Model) { m.startInput("org-add", orgPrompt(), "") }, "Acme",
			"Acme | domain | industry | size", "name"},
		{"deal second field", func(m *Model) { m.startInput("deal-add", dealPrompt(), "") }, "Site | 500 |",
			"Site | 500 | stage | organization | person | notes", "value"},
		{"task title", func(m *Model) { m.startInput("task-new", taskNewPrompt, "") }, "Call back",
			"task: Call back | due", ""},
		{"task empty", func(m *Model) { m.startInput("task-new", taskNewPrompt, "") }, "",
			"task: title | due", ""},
		{"task due filled", func(m *Model) { m.startInput("task-new", taskNewPrompt, "") }, "Call back | fri", "task: Call back | fri", "due"},
		{"note empty", func(m *Model) { m.startNotePromptForTest() }, "", "text", ""},
		{"note typed", func(m *Model) { m.startNotePromptForTest() }, "met Sam", "met Sam", "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			tt.start(m)
			m.input.SetValue(tt.typed)
			m.input.CursorEnd()
			got := footerText(m)
			assert.Contains(t, got, tt.wantIn)
			if tt.not != "" {
				assert.NotContains(t, strings.TrimPrefix(got, m.input.Prompt), tt.not)
			}
		})
	}
}

func (m *Model) startNotePromptForTest() {
	press(m, "n")
}

func TestHintIsNeverSubmittedAndHiddenWhenTheCursorIsNotAtTheEnd(t *testing.T) {
	m, _ := newTestModel(t)
	m.startInput("org-add", orgPrompt(), "")
	m.input.SetValue("Acme")
	m.input.CursorEnd()
	assert.Contains(t, footerText(m), "| domain | industry")
	assert.Equal(t, "Acme", m.input.Value(), "the hint is drawn after the input, not in it")

	m.input.SetCursor(2)
	assert.NotContains(t, footerText(m), "domain", "no hint while editing in the middle")

	m.input.CursorEnd()
	press(m, "enter")
	list := openOrgs(t, m)
	require.Len(t, list, 1)
	assert.Equal(t, "Acme", list[0].Name)
	assert.Nil(t, list[0].Domain, "hint text never reaches a field")
	assert.Nil(t, list[0].Industry)
}

func TestHintIsDimAndFitsTheWidth(t *testing.T) {
	m, _ := newTestModel(t)
	m.startInput("org-add", orgPrompt(), "")
	m.width = 40
	line := strings.SplitN(plain(m.footerView()), string(rune(10)), 2)[0]
	assert.LessOrEqual(t, len([]rune(line)), 41, "the hint is cut to the line")
	assert.Contains(t, line, "…")
	assert.Equal(t, "", m.promptHint(m.width), "no room, no hint")
}

func TestLinkAttributesShowAsAHintFromTheRelationRegistry(t *testing.T) {
	m, _ := newTestModel(t)
	rel, ok := model.RelationBetween("person", "organization")
	require.True(t, ok)
	m.draft = &linkDraft{rel: rel}
	m.startInput("link-attrs", linkAttrsPrompt(rel), "")
	assert.Contains(t, footerText(m), "Link: role | from | to | primary | notes")
}

func TestCommandBarShowsTheVerbShape(t *testing.T) {
	tests := []struct {
		name  string
		typed string
		want  string
		not   string
	}{
		{"task verb", "task ", "title by date @person #tag !priority every 3m", ""},
		{"task alias", "t Call", "by date @person", "title"},
		{"note verb", "note ", "text @person #tag remind fri", ""},
		{"note with a tag", "n met #x", "remind fri", "#tag"},
		{"set verb", "set ", "[name:] key=value every 3m !priority #tag", ""},
		{"snooze", "snooze ", "[name] days", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m.width = 120
			press(m, "ctrl+k")
			typeText(m, tt.typed)
			require.Equal(t, tt.typed, m.ov.filter.Value(), "the hint is not part of the line")
			line := plain(m.cmdInputLine(m.ov))
			if tt.want != "" {
				assert.Contains(t, line, tt.want)
			}
			if tt.not != "" {
				assert.NotContains(t, strings.TrimPrefix(line, "> "+tt.typed), tt.not)
			}
		})
	}
}

func TestPlainSearchTextShowsNoBarHint(t *testing.T) {
	m, _ := newTestModel(t)
	assert.Equal(t, "", m.barSyntaxHint("acme", 80))
	assert.Equal(t, "", m.barSyntaxHint("", 80))
}

func TestCommandBarRunsTheTypedLineNotTheHint(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "ctrl+k")
	typeText(m, "task Call mom")
	assert.Contains(t, plain(m.cmdInputLine(m.ov)), "by date")
	press(m, "enter")
	list, err := m.tr.FindAll(m.ctx, model.TaskFilters{})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Call mom", list[0].Title)
}

// The hint is drawn with the Dim style, and the typed text is not.
func TestHintIsRenderedWithTheDimStyle(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m, _ := newTestModel(t)
	m.width = 120
	m.startInput("org-add", orgPrompt(), "")
	m.input.SetValue("Acme")
	m.input.CursorEnd()
	hint := pipeHint(m.promptFieldsFor("org-add"), "Acme")
	require.NotEmpty(t, hint)
	raw := m.footerView()
	assert.Contains(t, raw, m.st.Dim.Render(hint), "the hint carries the dim style")
	assert.NotEqual(t, hint, m.st.Dim.Render(hint), "the dim style adds color codes")
	assert.NotContains(t, raw, m.st.Dim.Render("Acme"), "the typed text is not dim")
}
