package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jdanielnd/crm-cli/internal/keys"
)

// Rebindable keys. The action registry in actions.go holds the default keys. A [keys] table in
// ~/.crm/config.toml (see internal/keys) replaces the keys of the actions it names. The palette,
// the footer and the help screen read the effective keys from the same registry, so they follow.

// ActionInfo describes one action for the keys command.
type ActionInfo struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Group string   `json:"group"`
	Layer string   `json:"layer"`
	Scope string   `json:"scope"`
	Keys  []string `json:"keys"`
	Desc  string   `json:"description"`
	Write bool     `json:"changes_data"`
}

// DefaultActions lists every action with its default keys. Actions without a key can still be
// given one.
func DefaultActions() []ActionInfo {
	var out []ActionInfo
	for _, a := range buildActions() {
		out = append(out, ActionInfo{ID: a.id, Title: a.title, Group: a.group, Layer: a.layerOf(), Scope: a.scopeOf(), Keys: append([]string{}, a.keys...), Desc: a.desc, Write: a.write})
	}
	return out
}

// sameContext says whether two actions can be live in one effective scope: they have the same key mode
// and their scopes are the same or one inherits from the other (keymap.go). An app scope action is live in
// every view, so it collides with a key used anywhere. Two view types that never show together
// (the person list and the Sync inbox) do not.
func sameContext(a, b *action) bool {
	if a.mode != b.mode || !scopesOverlap(a.scopeOf(), b.scopeOf()) {
		return false
	}
	// Only actions marked inCompare are live on the compare screen, so the others never meet its keys.
	if a.scopeOf() == "compare" && b.scopeOf() != "compare" && !b.inCompare {
		return false
	}
	if b.scopeOf() == "compare" && a.scopeOf() != "compare" && !a.inCompare {
		return false
	}
	return true
}

// ResolveOverrides applies the overrides to a fresh action list and returns the effective keys by
// action id, with a message for every override that was skipped: an unknown action id, or a key
// already used by another action in the same context. Only overrides are checked, so the
// built-in keys that share a letter across screens never produce a message.
func ResolveOverrides(o keys.Overrides) (map[string][]string, []string) {
	actions := buildActions()
	problems := applyKeyOverrides(actions, o)
	eff := map[string][]string{}
	for i := range actions {
		eff[actions[i].id] = append([]string{}, actions[i].keys...)
	}
	return eff, problems
}

// applyKeyOverrides replaces the keys of the named actions in place and returns the problems.
func applyKeyOverrides(actions []action, o keys.Overrides) []string {
	ids := make([]string, 0, len(o))
	for id := range o {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	byID := map[string]*action{}
	for i := range actions {
		byID[actions[i].id] = &actions[i]
	}
	var problems []string
	overridden := map[string]bool{}
	for _, id := range ids {
		a, ok := byID[id]
		if !ok {
			problems = append(problems, fmt.Sprintf("keys.%s: no such action. Run `crm keys list` to see the ids%s", id, suggest(id, byID)))
			continue
		}
		// The keys that action would now hold must be free in its context.
		saved := a.keys
		a.keys = nil
		clash := ""
		for _, k := range o[id] {
			if a.hasKeyIn(saved, k) {
				continue // a key the action already has by default is not a new claim
			}
			for i := range actions {
				b := &actions[i]
				if b.id == id || !b.hasKey(k) || !sameContext(a, b) {
					continue
				}
				clash = fmt.Sprintf("keys.%s: %s is already used by %q (%s). Choose another key or rebind that action first", id, keys.Display(k), b.title, b.id)
				break
			}
			if clash != "" {
				break
			}
		}
		if clash != "" {
			a.keys = saved
			problems = append(problems, clash)
			continue
		}
		a.keys = append([]string{}, o[id]...)
		overridden[id] = true
		if len(a.keys) > 0 {
			a.hint = keys.Display(a.keys[0])
		}
	}
	return problems
}

func suggest(id string, byID map[string]*action) string {
	var near []string
	for other := range byID {
		if strings.Contains(other, id) || strings.Contains(id, other) {
			near = append(near, other)
		}
	}
	sort.Strings(near)
	if len(near) == 0 {
		return ""
	}
	if len(near) > 4 {
		near = near[:4]
	}
	return " (did you mean " + strings.Join(near, ", ") + "?)"
}

// StaleOverrides reports the entries of the [keys] table that the current action list cannot honor,
// so the user can clean the file: an action id that no longer exists, or a key another action now
// owns in the same scope. It never fails. The override file format is unchanged.
func StaleOverrides(o keys.Overrides) []string {
	_, problems := ResolveOverrides(o)
	return problems
}

// ConflictFor reports why keys cannot be bound to the action id given the current overrides, or "".
func ConflictFor(o keys.Overrides, id string, ks []string) string {
	trial := keys.Overrides{}
	for k, v := range o {
		trial[k] = v
	}
	trial[id] = ks
	_, problems := ResolveOverrides(trial)
	for _, p := range problems {
		if strings.HasPrefix(p, "keys."+id+":") {
			return strings.TrimPrefix(p, "keys."+id+": ")
		}
	}
	return ""
}

// applyKeys installs overrides for this session.
func (m *Model) applyKeys(o keys.Overrides) []string {
	m.keyOverrides = o
	m.actions = buildActions()
	m.keyProblems = applyKeyOverrides(m.actions, o)
	m.patchModuleActions()
	return m.keyProblems
}

// applyStartupKeys reads config.toml. A problem never stops the program: it says so in the status line.
func (m *Model) applyStartupKeys() {
	if m.env.DataDir == "" {
		return
	}
	o, problems := keys.Load(m.env.DataDir)
	problems = append(problems, m.applyKeys(o)...)
	if len(problems) > 0 {
		msg := "keys: " + problems[0]
		if len(problems) > 1 {
			msg += fmt.Sprintf(" (+%d more, see `crm keys list`)", len(problems)-1)
		}
		m.setStatus(msg, true)
	}
}

func (m *Model) saveKeys() error {
	if m.env.DataDir == "" {
		return fmt.Errorf("no data directory, the binding lasts for this session only")
	}
	return keys.Save(m.env.DataDir, m.keyOverrides)
}

func joinKeys(ks []string) string {
	parts := make([]string, len(ks))
	for i, k := range ks {
		parts[i] = keys.Display(k)
	}
	return strings.Join(parts, ", ")
}

// --- Keys: customize ---

type captureState struct {
	id string
}

// openKeysCustomize lists the actions; picking one waits for the new key.
func (m *Model) openKeysCustomize() {
	items := make([]overlayItem, 0, len(m.actions))
	for i := range m.actions {
		a := &m.actions[i]
		it := overlayItem{id: a.id, label: a.title, hint: joinKeys(a.keys), desc: a.group + ": " + a.desc}
		if _, custom := m.keyOverrides[a.id]; custom {
			it.desc = "(custom) " + it.desc
		}
		items = append(items, it)
	}
	m.openPicker("Keys: choose the action to rebind", items, func(m *Model, it overlayItem) {
		m.startCapture(it.id)
	})
}

func (m *Model) startCapture(id string) {
	m.capture = &captureState{id: id}
	m.mode = modeCapture
	m.setStatus("", false)
}

func (m *Model) captureTitle() string {
	if a := m.actionByID(m.capture.id); a != nil {
		return a.title
	}
	return m.capture.id
}

// handleCaptureKey takes the next key press as the new key for the action.
func (m *Model) handleCaptureKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := keyName(msg)
	if k == "esc" {
		m.mode, m.capture = modeNormal, nil
		return m, m.showToast("Keys: cancelled", false)
	}
	norm, err := keys.Normalize(k)
	if err != nil {
		return m, m.showToast("Keys: "+err.Error(), true)
	}
	id := m.capture.id
	if why := ConflictFor(m.keyOverrides, id, []string{norm}); why != "" {
		return m, m.showToast("Keys: "+why, true)
	}
	a := m.actionByID(id)
	old := "no key"
	if a != nil && len(a.keys) > 0 {
		old = joinKeys(a.keys)
	}
	title := m.captureTitle()
	m.capture = nil
	m.mode = modeConfirm
	warn := ""
	if why := SwallowedChordWarning(norm); why != "" {
		warn = " warning: " + why + "."
	}
	m.confirm = &confirmState{
		prompt: fmt.Sprintf("bind %s to %s (now %s)?%s y/n", title, keys.Display(norm), old, warn),
		yes: func() {
			o := keys.Overrides{}
			for k, v := range m.keyOverrides {
				o[k] = v
			}
			o[id] = []string{norm}
			m.applyKeys(o)
			msg := fmt.Sprintf("%s is now %s", title, keys.Display(norm))
			if err := m.saveKeys(); err != nil {
				m.pendingCmd = m.showToast("Keys: "+msg+", but "+err.Error(), true)
				return
			}
			m.pendingCmd = m.showToast("Keys: "+msg, false)
		},
	}
	return m, nil
}

// openKeysReset lists the custom bindings and "all".
func (m *Model) openKeysReset() {
	if len(m.keyOverrides) == 0 {
		m.pendingCmd = m.showToast("Keys: no custom keys to reset", false)
		return
	}
	items := []overlayItem{{id: "*", label: "All actions", desc: "remove every custom key"}}
	ids := make([]string, 0, len(m.keyOverrides))
	for id := range m.keyOverrides {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		title := id
		if a := m.actionByID(id); a != nil {
			title = a.title
		}
		items = append(items, overlayItem{id: id, label: title, hint: joinKeys(m.keyOverrides[id]), desc: "reset to the default key"})
	}
	m.openPicker("Keys: reset", items, func(m *Model, it overlayItem) {
		o := keys.Overrides{}
		if it.id != "*" {
			for k, v := range m.keyOverrides {
				if k != it.id {
					o[k] = v
				}
			}
		}
		m.applyKeys(o)
		what := "all keys reset to the defaults"
		if it.id != "*" {
			what = it.label + " is back to its default key"
		}
		if err := m.saveKeys(); err != nil {
			m.pendingCmd = m.showToast("Keys: "+what+", but "+err.Error(), true)
			return
		}
		m.pendingCmd = m.showToast("Keys: "+what, false)
	})
}

// takePending returns the command a helper left in pendingCmd and clears it.
func (m *Model) takePending() tea.Cmd {
	c := m.pendingCmd
	m.pendingCmd = nil
	return c
}
