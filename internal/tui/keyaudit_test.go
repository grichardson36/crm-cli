package tui

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"

	"github.com/stretchr/testify/require"
)

// The key audit. For every context (a view with the list focused, the person detail form, the bar
// results on each kind of row) it asks the registry which action each key would run, exactly as
// handleKey does (most specific scope first, see keymap.go), and lists every action that answers to
// a key, so a key with two answers shows up. Run it to regenerate the tables in
// patterns/audit-keybindings-2026-10.md:
//
//	KEYS_AUDIT_OUT=audit.md go test ./internal/tui -run TestKeyAudit
//
// The test fails on a new conflict. A deliberate overlap goes in testdata/keyaudit_allow.txt, one
// line each, with the reason after a #. See readAllowList for the kinds of line.

type auditCtx struct {
	name string
	// modal marks the compare screen, which takes over the whole keyboard: the same key may mean a
	// different verb there, so the one-verb-per-key check skips it.
	modal bool
	// answers maps a key to every action that would run for it in this context, in lookup order:
	// the most specific scope first, then registry order.
	answers map[string][]string
	// scopes maps an action id to its scope in this context.
	scopes map[string]string
}

func auditAnswers(m *Model, bar bool, tag bool) (map[string][]string, map[string]string) {
	out := map[string][]string{}
	scopes := map[string]string{}
	chain := m.scopeChain()
	for _, sc := range chain {
		for i := range m.actions {
			a := &m.actions[i]
			if a.scopeOf() != sc || a.mode != keyNormal {
				continue
			}
			if a.barOnly && !bar {
				continue
			}
			for _, k := range a.keys {
				if bar {
					if !m.barVerbKeyOK(a, k, tag) {
						continue
					}
				} else {
					if !a.enabled(m) || (a.keyIf != nil && !a.keyIf(m)) || (a.keyFor != nil && !a.keyFor(m, k)) {
						continue
					}
				}
				out[k] = append(out[k], a.id)
				scopes[a.id] = sc
			}
		}
	}
	return out, scopes
}

func auditContexts(t *testing.T) []auditCtx {
	var ctxs []auditCtx
	add := func(name string, m *Model, bar, tag bool) {
		a, s := auditAnswers(m, bar, tag)
		ctxs = append(ctxs, auditCtx{name: name, answers: a, scopes: s})
	}
	m, _ := newTestModel(t)
	ctx := m.ctx
	acme, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	_, err = m.links.Link(ctx, repo.Ref{Kind: "person", ID: 1}, repo.Ref{Kind: "organization", ID: acme.ID}, model.LinkAttrs{Role: sp("CTO")})
	require.NoError(t, err)
	_, err = m.dlr.Create(ctx, model.CreateDealInput{Title: "Big deal", Stage: "lead"})
	require.NoError(t, err)
	_, err = m.tr.Create(ctx, model.CreateTaskInput{Title: "Call", Priority: "medium"})
	require.NoError(t, err)
	arch, err := m.pr.Create(ctx, model.CreatePersonInput{FirstName: "Old"})
	require.NoError(t, err)
	require.NoError(t, m.pr.Archive(ctx, arch.ID))
	m.afterWrite()

	for _, v := range []viewKind{viewHome, viewPeople, viewDue, viewTasks, viewOrgs, viewDeals, viewBirthdays, viewArchived, viewSync} {
		m.switchView(v)
		m.focus = focusList
		m.settle()
		add(viewNames[v]+" (list)", m, false, false)
	}
	// The detail form of a person, of an organization and of a deal.
	for _, v := range []viewKind{viewPeople, viewOrgs, viewDeals} {
		m.switchView(v)
		m.settle()
		m.focus = focusDetail
		add(viewNames[v]+" (detail form)", m, false, false)
		m.focus = focusList
	}

	// A person's detail form with the cursor on a linked row, which has cells: left, right and tab
	// move across them.
	m.switchView(viewPeople)
	m.settle()
	m.focus = focusDetail
	for i, r := range m.form {
		if r.kind == rkLinked && len(r.cells) > 0 {
			m.fcur = i
		}
	}
	add("Person (detail form, link cells)", m, false, false)
	m.focus = focusList

	mm, _ := dupeModel(t)
	mm.switchView(viewMerge)
	mm.settle()
	add("Merge (list)", mm, false, false)
	press(mm, "enter")
	if mm.cmp != nil {
		add("Merge compare screen", mm, false, false)
		ctxs[len(ctxs)-1].modal = true
		press(mm, "esc")
	}

	// Bar results: the verbs of each kind of row.
	m.switchView(viewPeople)
	m.focus = focusList
	for _, tc := range []struct{ name, kind string }{{"Bar: person", "person"}, {"Bar: organization", "org"}, {"Bar: deal", "deal"}, {"Bar: task", "task"}, {"Bar: tag", "tag"}} {
		tg := barTarget{kind: tc.kind, id: 1, name: "friend"}
		saved := m.enterBarContext(tg)
		add(tc.name, m, true, tc.kind == "tag")
		m.leaveBarContext(saved)
	}
	return ctxs
}

// keyOrder sorts keys: bare letters and digits, symbols, then ctrl and named keys.
func keyOrder(k string) string {
	switch {
	case len(k) == 1 && k[0] >= '0' && k[0] <= '9':
		return "1" + k
	case len(k) == 1 && strings.ContainsRune("abcdefghijklmnopqrstuvwxyz", rune(k[0])):
		return "2" + k
	case len(k) == 1 && strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZ", rune(k[0])):
		return "3" + strings.ToLower(k)
	case len(k) == 1:
		return "4" + k
	case strings.HasPrefix(k, "ctrl+"):
		return "6" + k
	}
	return "5" + k
}

// readAllowList reads testdata/keyaudit_allow.txt, the deliberate overlaps. Each line is
//
//	same-scope <key> <scope> <action> <action>   two actions share a key in one scope and context
//	shadow <key> <winner> <loser>                a narrower scope answers a key a wider one also answers
//	verbs <key> <family>_/_<family>              one key means different verb families in different contexts
//	static <key> <scope> <action> <action>       two actions list one key in one scope and mode
//	layer <key> <action>                         an app action uses a key a framework action owns
//
// A line may end with "# reason". A line the audit no longer finds is reported, so the list stays minimal.
func readAllowList(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile("testdata/keyaudit_allow.txt")
	if os.IsNotExist(err) {
		return map[string]bool{}
	}
	require.NoError(t, err)
	out := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			out[line] = true
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
