package tui

import (
	"strings"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Deal stages are edited in the list editor (listeditor.go). What stays here are the helpers that
// read the live list of stages.

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
