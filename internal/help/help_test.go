package help_test

import (
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/help"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var requiredTopics = []string{
	"overview", "people", "contact-methods", "keep-in-touch", "birthdays", "tasks", "notes", "tags",
	"duplicates", "archive", "domains", "import", "sync", "dav", "output", "agents", "tui", "theme", "install",
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
		if name == "tui" {
			continue // interactive: its examples cannot run in a test
		}
		assert.Positive(t, topics[name], "topic %s has no runnable example", name)
	}
}
