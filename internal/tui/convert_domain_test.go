package tui

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompanyReviewShowsDomainSuggestionsApartFromNameMatches(t *testing.T) {
	m := convertModel(t)
	ctx := context.Background()
	assert.Contains(t, m.detail.View(), "acme.com", "a create shows the domain it would set")
	assert.Contains(t, m.detail.View(), "set when created")

	dom := "acme.com"
	o, err := m.orr.Create(ctx, model.CreateOrgInput{Name: "Acme Holdings", Domain: &dom})
	require.NoError(t, err)
	m.afterWrite()

	assert.Equal(t, "@ Acme Holdings", m.rows[0].meta, "a domain suggestion is marked apart from = and ~")
	detail := m.detail.View()
	assert.Contains(t, detail, "email domain")
	assert.Contains(t, detail, "domain match")
	assert.Contains(t, detail, "suggestion only")
	assert.Contains(t, detail, "no organization yet", "there is still no name match")

	// Bulk accept ignores it.
	press(m, "A")
	assert.NotEqual(t, modeConfirm, m.mode, "nothing matches exactly")

	// The link picker offers it first, and staging create shows the domain.
	press(m, "l")
	require.NotNil(t, m.ov)
	press(m, "enter")
	assert.Equal(t, o.ID, m.convStaged["acme"].OrgID)
	press(m, "up", "c")
	assert.Contains(t, m.rows[0].meta, "create (acme.com)")
}
