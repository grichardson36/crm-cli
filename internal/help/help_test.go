package help_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/help"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var requiredTopics = []string{
	"overview", "people", "orgs", "links", "relations", "deal-roles", "deal-stages", "convert", "org-merge", "merge-log", "modules", "contact-methods", "keep-in-touch", "birthdays", "tasks", "notes", "note-types", "short-syntax", "tags",
	"duplicates", "archive", "purge", "domains", "import", "sync", "sync-inbox", "dav", "dav-server", "output", "errors", "validation", "agents",
	"tui", "tui-keys", "tui-selection", "tui-prompts", "tui-key-changes", "keys", "theme", "install", "backup",
}

func TestTopicsExistInOrder(t *testing.T) {
	assert.Equal(t, requiredTopics, help.Names())
}

func TestEveryTopicHasTitleSummaryBody(t *testing.T) {
	for _, topic := range help.List() {
		t.Run(topic.Name, func(t *testing.T) {
			assert.NotEmpty(t, topic.Title)
			assert.NotEqual(t, topic.Name, topic.Title, "title comes from the # heading")
			assert.NotEmpty(t, topic.Summary)
			assert.NotContains(t, topic.Summary, "\n")
			assert.Greater(t, len(topic.Body), 200)
			assert.False(t, strings.Contains(topic.Body, "\r"), "body is normalized to LF")
		})
	}
}

// Minimal-documentation rule (FEATURES K07): a topic fits one screen, answers the basic question
// first and ends by naming the next topics to read. Overflow belongs in a deeper topic.
const maxTopicLines = 39

var parenthetical = regexp.MustCompile(`\([^)]*\)`)

func TestTopicsAreShortAndEndWithSeeAlso(t *testing.T) {
	names := map[string]bool{}
	for _, n := range help.Names() {
		names[n] = true
	}
	for _, topic := range help.List() {
		t.Run(topic.Name, func(t *testing.T) {
			lines := strings.Split(strings.TrimSpace(topic.Body), "\n")
			assert.LessOrEqual(t, len(lines), maxTopicLines, "move overflow into a deeper topic")
			last := lines[len(lines)-1]
			require.True(t, strings.HasPrefix(last, "See also: "), "last line must start with 'See also: ', got %q", last)
			assert.Equal(t, 1, strings.Count(topic.Body, "See also:"), "one See also line")
			// Every name in the See also line that looks like a topic must exist.
			rest := parenthetical.ReplaceAllString(strings.TrimPrefix(last, "See also: "), "")
			for _, item := range strings.Split(rest, ",") {
				word := strings.TrimSuffix(strings.Fields(strings.TrimSpace(item))[0], ".")
				assert.True(t, names[word], "See also names unknown topic %q", word)
				assert.NotEqual(t, topic.Name, word, "See also names itself")
			}
		})
	}
}

func TestGet(t *testing.T) {
	topic, ok := help.Get("Duplicates")
	require.True(t, ok, "lookup ignores case")
	assert.Equal(t, "duplicates", topic.Name)
	_, ok = help.Get("nope")
	assert.False(t, ok)
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	lower := help.Search("merge")
	upper := help.Search("MERGE")
	require.NotEmpty(t, lower)
	assert.Equal(t, lower, upper)
	topics := map[string]bool{}
	for _, m := range lower {
		topics[m.Topic] = true
		assert.Contains(t, strings.ToLower(m.Text), "merge")
		assert.Positive(t, m.Line)
	}
	assert.True(t, topics["duplicates"])
	assert.Empty(t, help.Search("zzzz-not-in-any-topic"))
	assert.Empty(t, help.Search("  "))
}

func TestExamplesAreCrmCommands(t *testing.T) {
	examples := help.Examples()
	require.NotEmpty(t, examples)
	topics := map[string]int{}
	for _, e := range examples {
		assert.True(t, strings.HasPrefix(e.Command, "crm "), "%s: %q", e.Topic, e.Command)
		if !e.Skip {
			topics[e.Topic]++
		}
	}
	for _, name := range requiredTopics {
		switch name {
		case "tui", "tui-keys", "tui-selection", "tui-prompts", "tui-key-changes", "dav-server":
			continue // interactive or long-running: their examples cannot run in a test
		}
		assert.Positive(t, topics[name], "topic %s has no runnable example", name)
	}
}
