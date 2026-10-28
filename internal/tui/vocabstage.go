package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Manage deal stages (palette, no key): the TUI path of `crm deal stages`, mirroring Manage deal
// roles. A picker lists the stages in pipeline order with outcome and deal count. The first row adds a
// stage. A stage opens its actions: rename, set outcome, move up, move down, remove. Each change is on
// the undo stack, except removing a stage, which needs a replacement when deals are in it.

// fieldSpec is the declared field of a kind with the options of its vocabulary set to the live values.
func (m *Model) fieldSpec(kind, key string) (model.FieldSpec, bool) {
	spec, ok := model.FieldNamed(kind, key)
	if ok && spec.Vocabulary != "" {
		if names, err := m.vr.Names(m.ctx, spec.Vocabulary); err == nil {
			spec = spec.WithVocabulary(names)
		}
	}
	return spec, ok
}

// fieldsOf lists the declared fields of a kind, vocabulary fields with their live options.
func (m *Model) fieldsOf(kind string) []model.FieldSpec {
	specs := append([]model.FieldSpec(nil), model.FieldsOf(kind)...)
	for i, s := range specs {
		if s.Vocabulary != "" {
			if names, err := m.vr.Names(m.ctx, s.Vocabulary); err == nil {
				specs[i] = s.WithVocabulary(names)
			}
		}
	}
	return specs
}

// dealStageNames lists the live deal stages in pipeline order.
func (m *Model) dealStageNames() []string {
	names, _ := m.vr.Names(m.ctx, model.DealStageKind)
	return names
}

// resolveStage checks a typed stage against the list and returns the listed spelling. Empty stays
// empty (a new deal then gets the first open stage).
func (m *Model) resolveStage(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	name, _, err := m.vr.Resolve(m.ctx, "deal", "stage", text, false)
	return name, err
}

// stageClosed reports whether a stage closes a deal (outcome won or lost).
func (m *Model) stageClosed(stage string) bool {
	list, err := m.vr.List(m.ctx, model.DealStageKind, true)
	if err != nil {
		return false
	}
	for _, e := range list {
		if strings.EqualFold(e.Name, stage) {
			return model.OutcomeClosed(e.Outcome)
		}
	}
	return false
}

// openDealStages shows the deal stages.
func (m *Model) openDealStages() tea.Cmd {
	list, err := m.vr.List(m.ctx, model.DealStageKind, false)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	items := []overlayItem{{id: "+add", label: "+ Add a stage", desc: "type a name, for example qualified"}}
	for i, e := range list {
		use := "no deals"
		if e.InUse > 0 {
			use = fmt.Sprintf("%d deal(s)", e.InUse)
		}
		items = append(items, overlayItem{id: "stage:" + e.Name, label: fmt.Sprintf("%d  %s  [%s]", i+1, e.Name, e.Outcome), value: e.Name,
			desc: use + " · enter: rename, outcome, move or remove"})
	}
	o := m.newOverlay("Deal stages", items, false)
	o.onPick = func(m *Model, it overlayItem) {
		if it.id == "+add" {
			m.startInput("vocab-stage-add", "new deal stage: ", "")
			m.pendingCmd = m.input.Focus()
			return
		}
		m.pendingCmd = m.openDealStageActions(it.value)
	}
	return m.showOverlay(o)
}

// openDealStageActions lists what can be done with one stage.
func (m *Model) openDealStageActions(name string) tea.Cmd {
	items := []overlayItem{
		{id: "rename", label: "Rename", desc: "rewrites the deals in it"},
		{id: "outcome", label: "Set outcome", desc: "open, won or lost: decides what closes a deal"},
		{id: "up", label: "Move up"},
		{id: "down", label: "Move down"},
		{id: "remove", label: "Remove", desc: "deals in it move to another stage"},
	}
	o := m.newOverlay("Deal stage "+name, items, false)
	o.onPick = func(m *Model, it overlayItem) {
		switch it.id {
		case "rename":
			m.vocabName = name
			m.startInput("vocab-stage-rename", fmt.Sprintf("rename %s to: ", name), name)
			m.pendingCmd = m.input.Focus()
		case "outcome":
			m.pendingCmd = m.pickStageOutcome(name)
		case "up", "down":
			m.moveDealStage(name, it.id == "up")
		case "remove":
			m.removeDealStage(name)
		}
	}
	return m.showOverlay(o)
}

// addDealStage is the enter of the "new deal stage" prompt.
func (m *Model) addDealStage(value string) {
	m.beginGroup()
	err := m.createVocabValue(model.DealStageKind, value)
	m.endGroup("")
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus("added deal stage "+model.CleanVocabName(value)+", open (ctrl+z undoes)", false)
	m.pendingCmd = m.openDealStages()
}

// renameDealStage is the enter of the rename prompt. The deals in the stage follow.
func (m *Model) renameDealStage(to string) {
	from := m.vocabName
	m.vocabName = ""
	res, err := m.vr.Rename(m.ctx, model.DealStageKind, from, to)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.record("rename deal stage "+res.Old+" to "+res.New,
		func() error { _, err := m.vr.Rename(m.ctx, model.DealStageKind, res.New, res.Old); return err },
		func() error { _, err := m.vr.Rename(m.ctx, model.DealStageKind, res.Old, res.New); return err })
	m.afterWrite()
	m.setStatus(fmt.Sprintf("renamed deal stage %s to %s, %d deal(s) updated (ctrl+z undoes)", res.Old, res.New, res.Rows), false)
	m.pendingCmd = m.openDealStages()
}

// pickStageOutcome asks for the outcome of a stage.
func (m *Model) pickStageOutcome(name string) tea.Cmd {
	items := []overlayItem{
		{id: "open", label: "open", value: "open", desc: "the deal is in the pipeline"},
		{id: "won", label: "won", value: "won", desc: "the deal is closed and won"},
		{id: "lost", label: "lost", value: "lost", desc: "the deal is closed and lost"},
	}
	o := m.newOverlay("Outcome of "+name, items, false)
	o.onPick = func(m *Model, it overlayItem) { m.setStageOutcome(name, it.value) }
	return m.showOverlay(o)
}

// setStageOutcome changes the outcome of a stage as one undo step.
func (m *Model) setStageOutcome(name, outcome string) {
	res, err := m.vr.SetOutcome(m.ctx, model.DealStageKind, name, outcome)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if res.Old != res.New {
		m.record("set outcome of deal stage "+res.Name+" to "+res.New,
			func() error { _, err := m.vr.SetOutcome(m.ctx, model.DealStageKind, res.Name, res.Old); return err },
			func() error { _, err := m.vr.SetOutcome(m.ctx, model.DealStageKind, res.Name, res.New); return err })
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("deal stage %s is now %s (ctrl+z undoes)", res.Name, res.New), false)
	m.pendingCmd = m.openDealStages()
}

// moveDealStage moves a stage one place up or down.
func (m *Model) moveDealStage(name string, up bool) {
	before := m.dealStageNames()
	pos := -1
	for i, n := range before {
		if n == name {
			pos = i + 1
		}
	}
	to := pos + 1
	if up {
		to = pos - 1
	}
	if pos < 0 || to < 1 || to > len(before) {
		m.setStatus(name+" is already there", true)
		return
	}
	if err := m.vr.Move(m.ctx, model.DealStageKind, name, to); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	after := m.dealStageNames()
	m.record("move deal stage "+name,
		func() error { return m.vr.Reorder(m.ctx, model.DealStageKind, before) },
		func() error { return m.vr.Reorder(m.ctx, model.DealStageKind, after) })
	m.setStatus("moved deal stage "+name+" (ctrl+z undoes)", false)
	m.pendingCmd = m.openDealStages()
}

// removeDealStage removes a stage. Deals in it move to a stage picked from the list.
func (m *Model) removeDealStage(name string) {
	list, err := m.vr.List(m.ctx, model.DealStageKind, false)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	used := 0
	var others []overlayItem
	for _, e := range list {
		if strings.EqualFold(e.Name, name) {
			used = e.InUse
			continue
		}
		others = append(others, overlayItem{id: "to:" + e.Name, label: e.Name, value: e.Name})
	}
	finish := func(replace string) {
		res, err := m.vr.Remove(m.ctx, model.DealStageKind, name, replace, false)
		if err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWrite()
		msg := "removed deal stage " + res.Name
		if res.Moved > 0 {
			msg += fmt.Sprintf(", %d deal(s) moved to %s", res.Moved, res.ReplacedBy)
		}
		m.setStatus(msg+" (add it again to undo)", false)
		m.pendingCmd = m.openDealStages()
	}
	if used == 0 {
		m.mode = modeConfirm
		m.confirm = &confirmState{prompt: fmt.Sprintf("remove deal stage %s? y/n", name), yes: func() { finish("") }}
		return
	}
	m.openPicker(fmt.Sprintf("%d deal(s) are in %s: move them to", used, name), others, func(m *Model, it overlayItem) { finish(it.value) })
}

// dealStagesAction is the palette entry.
func dealStagesAction() action {
	return action{id: "deal-stages", title: "Manage deal stages", group: "Contact",
		desc:  "the stages of the deal pipeline (lead, proposal, won, ...): add, rename, set the outcome (open, won or lost), reorder and remove. The outcome decides what closes a deal. At least one stage stays open and one is won and one lost",
		write: true, run: func(m *Model) tea.Cmd { return m.openDealStages() }}
}
