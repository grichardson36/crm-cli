package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/keys"
	"github.com/stretchr/testify/require"
)

// auditFamily groups actions that are the same verb on different objects, so a key that means
// "delete" on a person, an organization and a task is one meaning and not three.
func auditFamily(id, key string) string {
	switch id {
	case "archive", "delete", "org-delete", "deal-delete", "task-delete", "bar-tag-delete":
		return "delete"
	case "person-edit", "edit", "org-edit", "deal-edit", "task-edit":
		return "edit/open"
	case "new", "org-add", "deal-add", "add":
		return "add"
	case "task":
		if key == "t" {
			return "add task"
		}
		return "add"
	case "note", "note-object":
		return "note"
	case "tag", "task-tag":
		return "tag"
	case "restore", "org-restore", "deal-restore", "task-restore":
		return "restore"
	case "mark", "org-mark":
		return "mark"
	case "link", "bar-tag-link":
		return "link"
	case "unlink", "bar-tag-unlink":
		return "unlink"
	case "down", "up", "top", "bottom", "half-down", "half-up":
		return "move"
	case "focus-detail", "back":
		return "pane focus"
	}
	if strings.HasPrefix(id, "view-") {
		return "go to view"
	}
	return id
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

	var b strings.Builder
	b.WriteString("| Key | What it runs, by action (contexts) |\n|---|---|\n")
	var conflicts [][]string
	for _, k := range ks {
		byAction := map[string][]string{}
		var order []string
		families := map[string]bool{}
		shadow := false
		for _, c := range ctxs {
			ids := c.answers[k]
			if len(ids) == 0 {
				continue
			}
			if len(ids) > 1 {
				shadow = true
			}
			for _, id := range ids {
				if _, ok := byAction[id]; !ok {
					order = append(order, id)
				}
				byAction[id] = append(byAction[id], c.name)
				families[auditFamily(id, k)] = true
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
		if len(families) > 1 || shadow {
			var fam []string
			for f := range families {
				fam = append(fam, f)
			}
			sort.Strings(fam)
			note := strings.Join(fam, " / ")
			if shadow {
				note += " (two actions answer in one context)"
			}
			conflicts = append(conflicts, []string{keys.Display(k), note})
		}
	}
	b.WriteString("\n| Key | Different verbs on the same key |\n|---|---|\n")
	for _, c := range conflicts {
		b.WriteString(fmt.Sprintf("| `%s` | %s |\n", c[0], c[1]))
	}
	if out := os.Getenv("KEYS_AUDIT_OUT"); out != "" {
		require.NoError(t, os.WriteFile(out, []byte(b.String()), 0o644))
	}
	require.NotEmpty(t, ks)

	// A new key with two verbs fails here. Decide it with the owner (patterns/audit-keybindings-2026-10)
	// before adding it to this list, never silently.
	known := map[string]bool{"c": true, "e": true, "l": true, "m": true, "o": true, "r": true, "s": true, "u": true, "T": true, "enter": true}
	for _, c := range conflicts {
		require.True(t, known[c[0]], "new key conflict %s: %s", c[0], c[1])
	}
}
