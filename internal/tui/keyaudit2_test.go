package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditFamily groups actions that are the same verb on different objects, so a key that means
// "delete" on a person, an organization and a task is one meaning and not three.
func auditFamily(id string) string {
	switch id {
	case "archive", "delete", "org-delete", "deal-delete", "task-delete", "bar-tag-delete", "dismiss", "cmp-dismiss", "purge", "inbox-ignore":
		return "delete"
	case "activate", "edit", "home-open", "compare", "person-edit", "org-edit", "deal-edit", "task-edit", "inbox-accept", "tag-rename", "cmp-confirm":
		return "activate"
	case "add-any", "new", "org-add", "deal-add", "task", "note", "note-object", "add":
		return "add"
	case "restore", "org-restore", "deal-restore", "task-restore":
		return "restore"
	case "mark", "select-all", "extend-down", "extend-up":
		return "select"
	case "mark-state", "contacted", "done":
		return "mark"
	case "merge", "merge-marked", "org-merge", "tag-merge", "cmp-apply":
		return "merge"
	case "link", "bar-tag-link":
		return "link"
	case "unlink", "bar-tag-unlink":
		return "unlink"
	case "down", "up", "top", "bottom", "half-down", "half-up", "page-down", "page-up", "jump-up", "jump-down", "home-top", "home-bottom":
		return "move"
	case "focus-detail", "back", "cell-next", "cell-prev", "cmp-field":
		return "pane_focus"
	case "cmp-left", "cmp-right", "cmp-pick":
		return "pick"
	}
	if strings.HasPrefix(id, "view-") {
		return "go_to_view"
	}
	return id
}

type auditFinding struct{ kind, text string }

func (f auditFinding) line() string { return f.kind + " " + f.text }

// auditFindings runs the checks over every context and returns what it found.
func auditFindings(ctxs []auditCtx) []auditFinding {
	var out []auditFinding
	seen := map[string]bool{}
	add := func(kind, text string) {
		f := auditFinding{kind, text}
		if !seen[f.line()] {
			seen[f.line()] = true
			out = append(out, f)
		}
	}
	families := map[string]map[string]bool{}
	for _, c := range ctxs {
		for k, ids := range c.answers {
			for i := 0; i < len(ids); i++ {
				for j := i + 1; j < len(ids); j++ {
					a, b := ids[i], ids[j]
					if c.scopes[a] == c.scopes[b] {
						pair := []string{a, b}
						sort.Strings(pair)
						add("same-scope", fmt.Sprintf("%s %s %s %s", k, c.scopes[a], pair[0], pair[1]))
					} else {
						add("shadow", fmt.Sprintf("%s %s %s", k, a, b))
					}
				}
			}
			if c.modal {
				continue
			}
			if families[k] == nil {
				families[k] = map[string]bool{}
			}
			for _, id := range ids {
				families[k][auditFamily(id)] = true
			}
		}
	}
	for k, fam := range families {
		if len(fam) > 1 {
			var names []string
			for f := range fam {
				names = append(names, f)
			}
			sort.Strings(names)
			add("verbs", keys.Display(k)+" "+strings.Join(names, "_/_"))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].line() < out[j].line() })
	return out
}

func TestKeyAudit(t *testing.T) {
	ctxs := auditContexts(t)
	all := map[string]bool{}
	for _, c := range ctxs {
		for k := range c.answers {
			all[k] = true
		}
	}
	var ks []string
	for k := range all {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return keyOrder(ks[i]) < keyOrder(ks[j]) })
	require.NotEmpty(t, ks)

	if out := os.Getenv("KEYS_AUDIT_OUT"); out != "" {
		var b strings.Builder
		b.WriteString("| Key | What it runs, by action (contexts) |\n|---|---|\n")
		for _, k := range ks {
			byAction := map[string][]string{}
			var order []string
			for _, c := range ctxs {
				for _, id := range c.answers[k] {
					if _, ok := byAction[id]; !ok {
						order = append(order, id)
					}
					byAction[id] = append(byAction[id], c.name)
				}
			}
			var parts []string
			for _, id := range order {
				cs := byAction[id]
				where := strings.Join(cs, ", ")
				if len(cs) >= 6 {
					where = fmt.Sprintf("%d of %d contexts", len(cs), len(ctxs))
				}
				parts = append(parts, fmt.Sprintf("%s (%s)", id, where))
			}
			b.WriteString(fmt.Sprintf("| `%s` | %s |\n", keys.Display(k), strings.Join(parts, "; ")))
		}
		require.NoError(t, os.WriteFile(out, []byte(b.String()), 0o644))
	}

	// D9: any conflict not in the allow-list fails, and so does an allow-list line nothing needs.
	allow := readAllowList(t)
	found := map[string]bool{}
	for _, f := range auditFindings(ctxs) {
		found[f.line()] = true
		assert.True(t, allow[f.line()], "new key conflict, decide it with the owner before allowing it in testdata/keyaudit_allow.txt: %s", f.line())
	}
	for _, line := range sortedKeys(allow) {
		if strings.HasPrefix(line, "static ") || strings.HasPrefix(line, "layer ") {
			continue // checked by their own tests
		}
		assert.True(t, found[line], "stale allow-list line, the audit no longer finds it: %s", line)
	}
}

// The static check: no two actions share a key in the same scope and mode, with no exception.
func TestNoTwoActionsShareAKeyInOneScope(t *testing.T) {
	allow := readAllowList(t)
	for _, c := range staticConflicts(buildActions()) {
		line := fmt.Sprintf("static %s %s %s %s", c.key, c.scope, c.a, c.b)
		assert.True(t, allow[line], "two actions share a key in one scope: %s", c)
	}
}

// The app layer may add actions but may not use a key the framework layer owns.
func TestAppLayerKeepsOffFrameworkKeys(t *testing.T) {
	acts := buildActions()
	owned := frameworkKeys(acts)
	allow := readAllowList(t)
	for i := range acts {
		a := &acts[i]
		if a.layerOf() != layerApp {
			continue
		}
		for _, k := range a.keys {
			if owner, ok := owned[a.mode][k]; ok {
				assert.True(t, allow["layer "+k+" "+a.id], "app action %s uses %s, which the framework action %s owns", a.id, keys.Display(k), owner)
			}
		}
	}
}

// Every action is in a layer and every scope an action names exists.
func TestEveryActionHasALayerAndAKnownScope(t *testing.T) {
	ids := map[string]bool{}
	for _, a := range buildActions() {
		ids[a.id] = true
		assert.Contains(t, []string{layerFramework, layerApp}, a.layerOf(), a.id)
		if !strings.HasPrefix(a.scopeOf(), "view:") {
			_, ok := scopeTable[a.scopeOf()]
			assert.True(t, ok, "action %s names the unknown scope %q", a.id, a.scopeOf())
		}
	}
	for id := range frameworkIDs {
		assert.True(t, ids[id], "frameworkIDs names %s, which is not an action", id)
	}
	for id := range scopeByID {
		assert.True(t, ids[id], "scopeByID names %s, which is not an action", id)
	}
}

// Scope lookup: the most specific scope answers first, an erase stops the search, and text-entry mode
// never meets a bare key.
func TestScopeLookupInheritAndErase(t *testing.T) {
	scopeTable["view:test"] = scopeDef{parent: "type:person", erase: []string{"zz"}}
	defer delete(scopeTable, "view:test")
	mk := func(id, scope, key string, mode keyMode) action {
		return action{id: id, scope: scope, keys: []string{key}, mode: mode, run: func(m *Model) tea.Cmd { return nil }}
	}
	m, _ := newTestModel(t)
	m.actions = []action{
		mk("app-x", "app", "x", keyNormal),
		mk("person-x", "type:person", "x", keyNormal),
		mk("app-y", "app", "y", keyNormal),
		mk("app-zz", "app", "zz", keyNormal),
		mk("text-x", "app", "x", keyText),
	}
	annotateActions(m.actions)
	m.view = viewPeople
	require.Equal(t, "person-x", m.lookupKey("x", keyNormal).id, "the narrower scope answers first")
	require.Equal(t, "app-y", m.lookupKey("y", keyNormal).id, "a key only the app scope has is inherited")
	require.Equal(t, "text-x", m.lookupKey("x", keyText).id, "text mode is looked up on its own")
	m.view = viewOrgs
	require.Equal(t, "app-x", m.lookupKey("x", keyNormal).id, "the person scope is not live in Organizations")
	require.Equal(t, []string{"view:organizations", "type:organization", "app"}, m.scopeChain())

	// An erase in a view scope stops the inheritance of that key.
	scopeTable["view:people"] = scopeDef{parent: "type:person", erase: []string{"zz"}}
	defer delete(scopeTable, "view:people")
	m.view = viewPeople
	require.Nil(t, m.lookupKey("zz", keyNormal), "view:people erases zz")
	require.NotNil(t, m.lookupKey("y", keyNormal))
}

// Conflicts count only in one effective scope and mode.
func TestConflictCountsOnlyInTheSameScopeAndMode(t *testing.T) {
	acts := []action{
		{id: "a", keys: []string{"q"}},                                      // app, normal
		{id: "b", keys: []string{"q"}, scope: "type:task"},                  // inherits from app: overlaps a
		{id: "c", keys: []string{"q"}, scope: "type:person"},                // sibling of type:task
		{id: "d", keys: []string{"q"}, mode: keyText},                       // text mode: never meets the bare key
		{id: "e", keys: []string{"q"}, scope: "type:person", mode: keyText}, // text mode in a narrower scope
	}
	require.Empty(t, staticConflicts(acts), "no two share a scope and mode")
	assert.True(t, sameContext(&acts[0], &acts[1]), "a view type inherits the app scope, so the keys meet")
	assert.False(t, sameContext(&acts[1], &acts[2]), "type:task and type:person never show together")
	assert.False(t, sameContext(&acts[0], &acts[3]), "a text-entry key does not conflict with a bare key")
	assert.False(t, sameContext(&acts[2], &acts[4]), "same scope, different mode")
	acts = append(acts, action{id: "f", keys: []string{"q"}, scope: "type:person"})
	require.Len(t, staticConflicts(acts), 1, "c and f share a key in one scope and mode")
	assert.Contains(t, staticConflicts(acts)[0].String(), "type:person")
}

// The real registry: the text-entry tab is not a conflict with any bare key.
func TestTextEntryKeysAreSeparateFromBareKeys(t *testing.T) {
	var tab *action
	acts := buildActions()
	for i := range acts {
		if acts[i].id == "prompt-tab" {
			tab = &acts[i]
		}
	}
	require.NotNil(t, tab)
	assert.Equal(t, keyText, tab.mode)
	assert.NotEmpty(t, ConflictFor(nil, "snooze", []string{"tab"}), "tab is the focus key in lists, a bare key")
	for i := range acts {
		if acts[i].id != "prompt-tab" && acts[i].hasKey("tab") {
			assert.False(t, sameContext(tab, &acts[i]), "the prompt's tab never meets %s", acts[i].id)
		}
	}
}

func TestConflictForIsScopedAndNamesTheOtherAction(t *testing.T) {
	// snooze is a person action and sync-now an inbox action. They never show together.
	assert.Equal(t, "", ConflictFor(nil, "sync-now", []string{"s"}), "s is free on the Sync screen")
	assert.Contains(t, ConflictFor(nil, "sync-now", []string{"x"}), "Mark", "x is a framework verb live in every view")
	assert.NotEmpty(t, ConflictFor(nil, "snooze", []string{"d"}), "d is the delete verb, live in every view")
	assert.Contains(t, ConflictFor(nil, "options", []string{"ctrl+d"}), "Half page down")
	assert.Equal(t, "", ConflictFor(nil, "options", []string{"ctrl+g"}))
}
