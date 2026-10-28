package tui

import (
	"fmt"
	"sort"
	"strings"
)

// The keymap structure (patterns/keymap-layers-and-scopes).
//
// Two layers. The framework layer holds the verbs every view shares (add, activate, delete, restore,
// link, unlink, mark, favorite, merge, undo, redo, refresh, multi-select), navigation, help, the
// palette and view switching. The app layer holds what is specific to this CRM (snooze, cadence,
// birthdays, the deal pipeline, sync, merge compare). The app layer may add actions but may not use a
// key the framework layer owns (TestAppLayerKeepsOffFrameworkKeys).
//
// Scopes say where an action's keys are live: "app" (everywhere), a view type ("type:person",
// "type:task", "form", "compare", ...) or one view ("view:birthdays"). A key is looked up from the most
// specific scope of the current context outward (scopeChain) and the first enabled action wins. A scope
// can erase an inherited key so the lookup stops there. Two actions conflict only when they share a key
// in the same scope and the same mode. Text-entry keys (the footer prompt) are a separate mode, so a
// key typed into a field never conflicts with a bare key of the list.

// layer names. A layer is a property of the action id, kept in one table below.
const (
	layerFramework = "framework"
	layerApp       = "app"
)

// keyMode is the input mode a key belongs to. Normal keys run in lists, forms and the compare screen.
// Text keys run while a footer prompt is open.
type keyMode int

const (
	keyNormal keyMode = iota
	keyText
)

func (k keyMode) String() string {
	if k == keyText {
		return "text"
	}
	return "normal"
}

// scopeDef is one named scope. Parent is where the lookup continues. Erase lists inherited keys that
// stop the lookup here, so a key owned by a parent scope does nothing in this one.
type scopeDef struct {
	parent string
	erase  []string
}

const scopeApp = "app"

// scopeTable declares every scope. A view scope ("view:<name>") is added to the chain for each view
// and needs no entry unless it erases keys or has a parent other than its view type.
var scopeTable = map[string]scopeDef{
	"app":               {},
	"form":              {parent: scopeApp},
	"compare":           {parent: scopeApp},
	"cells":             {parent: "form"},
	"type:person":       {parent: scopeApp},
	"type:task":         {parent: scopeApp},
	"type:organization": {parent: scopeApp},
	"type:deal":         {parent: scopeApp},
	"type:dupes":        {parent: scopeApp},
	"type:archive":      {parent: scopeApp},
	"type:inbox":        {parent: scopeApp},
	"type:tag":          {parent: scopeApp},
}

// viewScopeType maps a view to its view type scope.
func viewScopeType(v viewKind) string {
	switch v {
	case viewTasks:
		return "type:task"
	case viewOrgs:
		return "type:organization"
	case viewDeals:
		return "type:deal"
	case viewMerge:
		return "type:dupes"
	case viewArchived:
		return "type:archive"
	case viewSync:
		return "type:inbox"
	case viewTags:
		return "type:tag"
	}
	return "type:person"
}

func viewScopeName(v viewKind) string {
	return "view:" + strings.ToLower(strings.ReplaceAll(viewNames[v], " ", "-"))
}

// scopeChain lists the scopes of the current context from the most specific outward.
func (m *Model) scopeChain() []string {
	if m.cmp != nil {
		return []string{"compare", scopeApp}
	}
	var chain []string
	if m.focus == focusDetail {
		if whenCellRow(m) {
			chain = append(chain, "cells") // left, right and tab move across the cells of this row
		}
		chain = append(chain, "form")
	}
	if m.barTag != "" && viewScopeType(m.view) != "type:tag" {
		chain = append(chain, "type:tag")
	}
	return append(chain, viewScopeName(m.view), viewScopeType(m.view), scopeApp)
}

// scopeErases says whether the scope removes the inherited key.
func scopeErases(scope, key string) bool {
	for _, k := range scopeTable[scope].erase {
		if k == key {
			return true
		}
	}
	return false
}

// scopeAncestors returns the scope and its parents, nearest first.
func scopeAncestors(scope string) []string {
	out := []string{scope}
	for i := 0; i < 8; i++ {
		p := scopeTable[scope].parent
		if p == "" {
			if strings.HasPrefix(scope, "view:") && scope != scopeApp {
				// a view scope that was never declared inherits from the app
				out = append(out, scopeApp)
			}
			break
		}
		out = append(out, p)
		scope = p
	}
	return out
}

// scopeOf is the scope of an action ("app" unless it declares one).
func (a *action) scopeOf() string {
	if a.scope == "" {
		return scopeApp
	}
	return a.scope
}

// layerOf is the layer of an action.
func (a *action) layerOf() string {
	if a.layer == "" {
		return layerApp
	}
	return a.layer
}

// inScopes says whether the action's scope is part of the chain.
func inScopes(a *action, chain []string) bool {
	s := a.scopeOf()
	for _, c := range chain {
		if c == s {
			return true
		}
	}
	return false
}

// lookupKey finds the action a key runs in the current context and mode: the first enabled action
// of the most specific scope, then the next scope outward. A scope that erases the key ends the search.
func (m *Model) lookupKey(key string, mode keyMode) *action {
	return m.lookupKeyWith(key, mode, func(a *action, key string) bool {
		return a.enabled(m) && (a.keyIf == nil || a.keyIf(m)) && (a.keyFor == nil || a.keyFor(m, key))
	})
}

// lookupKeyWith is lookupKey with the enabling test left to the caller (the bar borrows a context
// and filters by object kind).
func (m *Model) lookupKeyWith(key string, mode keyMode, ok func(a *action, key string) bool) *action {
	for _, sc := range m.scopeChain() {
		for i := range m.actions {
			a := &m.actions[i]
			if a.scopeOf() != sc || a.mode != mode || !a.hasKey(key) {
				continue
			}
			if ok(a, key) {
				return a
			}
		}
		if scopeErases(sc, key) {
			return nil
		}
	}
	return nil
}

// annotateActions fills in the layer and scope of every action from the tables below. One place
// decides both, so a new action starts as app layer in the app scope and is added here only when it
// belongs to the framework or to a narrower scope.
func annotateActions(acts []action) {
	for i := range acts {
		a := &acts[i]
		if a.layer == "" {
			if frameworkIDs[a.id] || strings.HasPrefix(a.id, "view-") {
				a.layer = layerFramework
			} else {
				a.layer = layerApp
			}
		}
		if a.scope == "" {
			if s, ok := scopeByID[a.id]; ok {
				a.scope = s
			}
		}
	}
}

// frameworkIDs are the actions of the framework layer: the universal verbs, navigation, help, the
// palette and view switching. Their keys belong to the framework.
var frameworkIDs = map[string]bool{
	// verbs
	"add-any": true, "add": true, "activate": true, "mark-state": true, "merge": true, "favorite": true,
	"archive": true, "org-delete": true, "deal-delete": true, "task-delete": true, "delete": true, "dismiss": true,
	"purge": true, "copy": true, "inbox-ignore": true, "tags-delete": true, "tag-back": true, "cmp-dismiss": true, "tag-merge": true, "cmp-apply": true,
	"restore": true, "link": true, "unlink": true, "tag-link": true, "tag-unlink": true,
	"mark": true, "select-all": true, "extend-down": true, "extend-up": true,
	"undo": true, "redo": true, "reload": true,
	// navigation
	"down": true, "up": true, "half-down": true, "half-up": true, "page-down": true, "page-up": true,
	"jump-down": true, "jump-up": true, "top": true, "bottom": true, "home-top": true, "home-bottom": true,
	"focus-detail": true, "back": true, "cell-next": true, "cell-prev": true,
	// app chrome
	"palette": true, "options": true, "help": true, "quit": true, "goto-view": true, "next-view": true, "prev-view": true, "clear-search": true,
	// merge compare screen
	"cmp-left": true, "cmp-right": true, "cmp-pick": true, "cmp-field": true, "cmp-confirm": true, "cmp-back": true,
	// text-entry mode
	"prompt-tab": true, "prompt-tab-back": true,
}

// scopeByID narrows the scope of an action. Everything not listed is in the app scope.
var scopeByID = map[string]string{
	// People (Home, People, Follow-up, Birthdays)
	"favorite": "type:person", "snooze": "type:person", "cadence": "type:person", "contacted": "type:person",
	"tag": "type:person", "note-pick": "type:person",
	"archive": "type:person", "restore": "type:archive", "person-edit": "type:person", "new": "type:person",
	// Tasks, organizations, deals
	"task-delete": "type:task", "task-edit": "type:task", "task": "type:task", "task-restore": "type:task",
	"org-delete": "type:organization", "org-edit": "type:organization", "org-restore": "type:organization", "org-add": "type:organization",
	"deal-delete": "type:deal", "deal-edit": "type:deal", "deal-restore": "type:deal", "deal-add": "type:deal",
	// Merge, Archived, Sync
	"dismiss": "type:dupes", "undo-dismiss": "type:dupes", "compare": "type:dupes",
	"purge": "type:archive", "purge-selected": "type:archive", "purge-all": "type:archive",
	"sync-now": "type:inbox", "sync-dry": "type:inbox", "inbox-accept": "type:inbox", "inbox-attach": "type:inbox", "inbox-ignore": "type:inbox",
	// Tags in the bar
	"tags-delete": "type:tag", "tag-link": "type:tag", "tag-unlink": "type:tag", "tag-rename": "type:tag", "tag-merge": "type:tag",
	"tag-open": "type:tag", "tag-back": "type:tag", "tag-merge-marked": "type:tag",
	// The detail form
	"delete": "form", "add": "form", "copy": "form", "back": "form", "cell-next": "cells", "cell-prev": "cells",
	// The compare screen
	"cmp-left": "compare", "cmp-right": "compare", "cmp-pick": "compare", "cmp-field": "compare", "cmp-survivor": "compare",
	"cmp-pair": "compare", "cmp-view": "compare", "cmp-confirm": "compare", "cmp-apply": "compare", "cmp-back": "compare", "cmp-dismiss": "compare",
}

// textKeyIDs are the actions that run in text-entry mode.
var textKeyIDs = map[string]bool{"prompt-tab": true, "prompt-tab-back": true}

// keyConflict is two actions that share a key in one scope and mode.
type keyConflict struct {
	key, scope, mode string
	a, b             string
}

func (c keyConflict) String() string {
	return fmt.Sprintf("%s in scope %s (%s mode): %s and %s", c.key, c.scope, c.mode, c.a, c.b)
}

// staticConflicts lists every pair of actions that share a key in the same scope and mode. Keys the
// action list gives to an action twice are ignored.
func staticConflicts(acts []action) []keyConflict {
	type slot struct {
		key, scope string
		mode       keyMode
	}
	seen := map[slot]string{}
	var out []keyConflict
	for i := range acts {
		a := &acts[i]
		for _, k := range a.keys {
			s := slot{k, a.scopeOf(), a.mode}
			if prev, ok := seen[s]; ok && prev != a.id {
				out = append(out, keyConflict{key: k, scope: s.scope, mode: a.mode.String(), a: prev, b: a.id})
				continue
			}
			seen[s] = a.id
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// scopesOverlap says whether the two scopes can both be live in one context: they are the same, or
// one is an ancestor of the other.
func scopesOverlap(a, b string) bool {
	for _, s := range scopeAncestors(a) {
		if s == b {
			return true
		}
	}
	for _, s := range scopeAncestors(b) {
		if s == a {
			return true
		}
	}
	return false
}

// frameworkKeys is the set of keys the framework layer owns, by mode.
func frameworkKeys(acts []action) map[keyMode]map[string]string {
	out := map[keyMode]map[string]string{keyNormal: {}, keyText: {}}
	for i := range acts {
		a := &acts[i]
		if a.layerOf() != layerFramework {
			continue
		}
		for _, k := range a.keys {
			if _, ok := out[a.mode][k]; !ok {
				out[a.mode][k] = a.id
			}
		}
	}
	return out
}

// swallowedChords are key chords many terminals, multiplexers or the operating system consume or
// rewrite before the program sees them. ctrl+i is tab, ctrl+m is enter, ctrl+h is backspace,
// ctrl+[ is esc, ctrl+j is a newline. The ctrl+shift arrows are not reported by every terminal.
var swallowedChords = map[string]string{
	"ctrl+i":           "ctrl+i is the same byte as tab",
	"ctrl+m":           "ctrl+m is the same byte as enter",
	"ctrl+h":           "ctrl+h is the same byte as backspace in many terminals",
	"ctrl+[":           "ctrl+[ is the same byte as esc",
	"ctrl+j":           "ctrl+j is the same byte as a newline",
	"ctrl+shift+up":    "many terminals do not report ctrl+shift+up, use home as the fallback",
	"ctrl+shift+down":  "many terminals do not report ctrl+shift+down, use end as the fallback",
	"ctrl+shift+c":     "terminals such as Windows Terminal keep ctrl+shift+c for their own copy, use the palette entry Copy contact as vCard",
	"ctrl+shift+left":  "many terminals do not report ctrl+shift+left",
	"ctrl+shift+right": "many terminals do not report ctrl+shift+right",
}

// SwallowedChordWarning returns why a terminal may never deliver the key, or "".
func SwallowedChordWarning(key string) string {
	if why, ok := swallowedChords[key]; ok {
		return why
	}
	return ""
}
