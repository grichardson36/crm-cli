package tui

import "github.com/jdanielnd/crm-cli/internal/model"

// dealRoleNames lists the live deal roles in pick order. The roles are edited in the list editor
// (listeditor.go, patterns/list-editor-standard).
func (m *Model) dealRoleNames() []string {
	names, _ := m.vr.Names(m.ctx, model.DealRoleKind)
	return names
}
