package tui

import (
	"strings"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/require"
)

// The key audit. For every context (a view with the list focused, the person detail form, the bar
// results on each kind of row) it asks the registry which action each key would run, exactly as
// handleKey does, and lists every action that answers to a key, so a key with two answers shows up.
// Run it to regenerate the tables in patterns/audit-keybindings-2026-10.md:
//
//	KEYS_AUDIT_OUT=audit.md go test ./internal/tui -run TestKeyAudit
//
// Without KEYS_AUDIT_OUT it only checks that the audit runs and that no key has two answers where
// the owner rule forbids it (see knownTwoAnswers).

type auditCtx struct {
	name string
	// answers maps a key to every action that would run for it in this context, in registry order.
	answers map[string][]string
}

func auditAnswers(m *Model, bar bool, tag bool) map[string][]string {
	out := map[string][]string{}
	for i := range m.actions {
		a := &m.actions[i]
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
				if a.barOnly {
					continue
				}
			}
			out[k] = append(out[k], a.id)
		}
	}
	return out
}

func auditContexts(t *testing.T) []auditCtx {
	var ctxs []auditCtx
	m, _ := newTestModel(t)
	ctx := m.ctx
	_, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme"})
	require.NoError(t, err)
	_, err = m.dlr.Create(ctx, model.CreateDealInput{Title: "Big deal", Stage: "lead"})
	require.NoError(t, err)
	_, err = m.tr.Create(ctx, model.CreateTaskInput{Title: "Call", Priority: "medium"})
	require.NoError(t, err)
	arch, err := m.pr.Create(ctx, model.CreatePersonInput{FirstName: "Old"})
	require.NoError(t, err)
	require.NoError(t, m.pr.Archive(ctx, arch.ID))
	m.afterWrite()

	for _, v := range []viewKind{viewHome, viewPeople, viewDue, viewTasks, viewOrgs, viewDeals, viewBirthdays, viewArchived, viewSync, viewConvert} {
		m.switchView(v)
		m.focus = focusList
		m.settle()
		ctxs = append(ctxs, auditCtx{name: viewNames[v] + " (list)", answers: auditAnswers(m, false, false)})
	}
	m.switchView(viewPeople)
	m.focus = focusDetail
	ctxs = append(ctxs, auditCtx{name: "Person form (detail)", answers: auditAnswers(m, false, false)})
	m.focus = focusList

	mm, _ := dupeModel(t)
	mm.switchView(viewMerge)
	mm.settle()
	ctxs = append(ctxs, auditCtx{name: "Merge (list)", answers: auditAnswers(mm, false, false)})

	// Bar results: the verbs of each kind of row.
	for _, tc := range []struct{ name, kind string }{{"Bar: person", "person"}, {"Bar: organization", "org"}, {"Bar: deal", "deal"}, {"Bar: task", "task"}, {"Bar: tag", "tag"}} {
		tg := barTarget{kind: tc.kind, id: 1, name: "friend"}
		saved := m.enterBarContext(tg)
		ctxs = append(ctxs, auditCtx{name: tc.name, answers: auditAnswers(m, true, tc.kind == "tag")})
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
