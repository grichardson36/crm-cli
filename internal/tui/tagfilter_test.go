package tui

import (
	"testing"

	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestFilterTermsQuotedTag(t *testing.T) {
	assert.Equal(t, []string{"#university of florida (uf)/rinker", "bob"}, filterTerms(`#"University of Florida (UF)/Rinker" Bob`))
	assert.Equal(t, []string{"#uf", "x"}, filterTerms("#uf  x"))
}

func TestPeopleTagFilterSiblings(t *testing.T) {
	p := model.PersonSummary{Name: "Ann", Tags: "work/client"}
	assert.True(t, matches(p, []string{"#work"}))
	assert.True(t, matches(p, []string{"#work/client"}))
	assert.False(t, matches(p, []string{"#work/partner"}))
	assert.False(t, matches(p, []string{"#work/employee"}))
}

func TestPeopleTagFilterUsesTagRule(t *testing.T) {
	p := model.PersonSummary{Name: "Ann", Tags: "work/clients, University of Florida (UF)/Rinker"}
	assert.True(t, matches(p, []string{"#work"}))
	assert.True(t, matches(p, []string{"#work/clients"}))
	assert.True(t, matches(p, []string{"#university of florida (uf)"}))
	assert.False(t, matches(p, []string{"#uf"}))
	assert.False(t, matches(p, []string{"#clients"}))
}
