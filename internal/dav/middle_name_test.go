package dav_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/jdanielnd/crm-cli/internal/dav"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CRM-019, vCard door. The middle name is the additional names component (the third part) of N.
// The prefix and suffix stay aside and travel unchanged.

func TestMiddleName_ParseReadsAdditionalNames(t *testing.T) {
	c, extra := dav.Parse(parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:Smith;Jane;Marie;;\nFN:Jane Marie Smith\nEND:VCARD\n"))
	assert.Equal(t, "Jane", c.First)
	assert.Equal(t, "Smith", c.Last)
	assert.Equal(t, "Marie", c.Middle)
	assert.Nil(t, extra.Get(vcard.FieldName), "nothing left to keep aside")

	c, extra = dav.Parse(parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:Smith;Jane;Marie;Dr.;PhD\nEND:VCARD\n"))
	assert.Equal(t, "Marie", c.Middle)
	require.NotNil(t, extra.Get(vcard.FieldName), "prefix and suffix are still kept aside")

	c, extra = dav.Parse(parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:Smith;Jane;;;PhD\nEND:VCARD\n"))
	assert.Equal(t, "", c.Middle)
	require.NotNil(t, extra.Get(vcard.FieldName), "a suffix alone is kept aside too")

	c, _ = dav.Parse(parseCard(t, "BEGIN:VCARD\nVERSION:3.0\nN:Smith;Jane;;;\nEND:VCARD\n"))
	assert.Equal(t, "", c.Middle)
}

func TestMiddleName_MappingChangeMakesPhonesRefetch(t *testing.T) {
	assert.Equal(t, "2", dav.MapVersion, "the card layout changed with the middle name, the version is part of every ETag")
}

func TestMiddleName_RenderWritesAdditionalNamesKeepingPrefixAndSuffix(t *testing.T) {
	p := &model.Person{FirstName: "Jane", LastName: ptr("Smith"), MiddleName: ptr("Marie")}
	card := dav.Render(p, nil, "uid-1")
	assert.Equal(t, "Smith;Jane;Marie;;", card.Value(vcard.FieldName))
	assert.Equal(t, "Jane Marie Smith", card.Value(vcard.FieldFormattedName))

	extra := vcard.Card{}
	extra.Set(vcard.FieldName, &vcard.Field{Value: "Old;Old;Stale;Dr.;PhD"})
	card = dav.Render(p, extra, "uid-1")
	assert.Equal(t, "Smith;Jane;Marie;Dr.;PhD", card.Value(vcard.FieldName), "the CRM value replaces the stale one, prefix and suffix stay")
	assert.Equal(t, "Dr. Jane Marie Smith PhD", card.Value(vcard.FieldFormattedName))

	card = dav.Render(&model.Person{FirstName: "Jane", LastName: ptr("Smith")}, extra, "uid-1")
	assert.Equal(t, "Smith;Jane;;Dr.;PhD", card.Value(vcard.FieldName), "no middle name in the CRM, none on the card")
}

func TestMiddleName_PhoneToCRMAndBack(t *testing.T) {
	ctx := context.Background()
	t.Run("a new phone contact keeps its middle name", func(t *testing.T) {
		e := newEnv(t, false)
		resp := e.putCard("mid-1", "BEGIN:VCARD\nVERSION:3.0\nUID:mid-1\nN:Smith;Jane;Marie;;\nFN:Jane Marie Smith\nEND:VCARD\n", nil)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		p := e.personOf("mid-1")
		require.NotNil(t, p.MiddleName)
		assert.Equal(t, "Marie", *p.MiddleName)
		assert.Equal(t, "Jane", p.FirstName)
		got, _ := e.getCard("mid-1")
		assert.Contains(t, got, "N:Smith;Jane;Marie;;")
	})

	t.Run("a phone edit sets, changes and clears it", func(t *testing.T) {
		e := newEnv(t, false)
		p, err := e.people.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith")})
		require.NoError(t, err)
		card, etag := e.pull(p)
		card.Set(vcard.FieldName, &vcard.Field{Value: "Smith;Jane;Marie;;"})
		_, err = e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		assert.Equal(t, "Marie", *e.reload(p.ID).MiddleName)
		assert.Empty(t, e.conflicts())

		card, etag = e.pull(e.reload(p.ID))
		card.Set(vcard.FieldName, &vcard.Field{Value: "Smith;Jane;Lou;;"})
		_, err = e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		assert.Equal(t, "Lou", *e.reload(p.ID).MiddleName)

		card, etag = e.pull(e.reload(p.ID))
		card.Set(vcard.FieldName, &vcard.Field{Value: "Smith;Jane;;;"})
		_, err = e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		got := e.reload(p.ID).MiddleName
		assert.True(t, got == nil || *got == "", "the phone cleared it")
	})

	t.Run("a CRM edit reaches the phone and touches only N and FN", func(t *testing.T) {
		e := newEnv(t, false)
		card := "BEGIN:VCARD\nVERSION:3.0\nPRODID:-//Apple Inc.//iPhone OS 17.5//EN\nUID:mid-2\nN:Smith;Jane;;Dr.;\nFN:Dr. Jane Smith\nTITLE:Engineer\nX-ABUID:keep-me\nEND:VCARD\n"
		require.Equal(t, http.StatusCreated, e.putCard("mid-2", card, nil).StatusCode)
		p := e.personOf("mid-2")
		_, err := e.people.Update(ctx, p.ID, model.UpdatePersonInput{MiddleName: ptr("Marie")})
		require.NoError(t, err)
		e.bump(p.ID)
		got, _ := e.getCard("mid-2")
		onlyOld, onlyNew := diff(crlf(card), got)
		assert.ElementsMatch(t, []string{"N:Smith;Jane;;Dr.;", "FN:Dr. Jane Smith"}, onlyOld)
		assert.ElementsMatch(t, []string{"N:Smith;Jane;Marie;Dr.;", "FN:Dr. Jane Marie Smith"}, onlyNew, strings.Join(onlyNew, "|"))
	})

	t.Run("both sides changed it, the CRM value is kept, the phone value is stored and can be restored", func(t *testing.T) {
		e := newEnv(t, false)
		p, err := e.people.Create(ctx, model.CreatePersonInput{FirstName: "Jane", LastName: ptr("Smith"), MiddleName: ptr("Ann")})
		require.NoError(t, err)
		card, etag := e.pull(p)
		_, err = e.people.Update(ctx, p.ID, model.UpdatePersonInput{MiddleName: ptr("Beth")})
		require.NoError(t, err)
		e.clock = time.Now().Add(time.Hour)
		card.Set(vcard.FieldName, &vcard.Field{Value: "Smith;Jane;Cara;;"})
		_, err = e.push(p.UUID, card, etagHeader(etag))
		require.NoError(t, err)
		assert.Equal(t, "Beth", *e.reload(p.ID).MiddleName)
		cs := e.conflicts()
		require.Len(t, cs, 1)
		assert.Equal(t, "middle_name", cs[0].Field)
		assert.Equal(t, "Beth", *cs[0].CRMValue)
		assert.Equal(t, "Cara", *cs[0].DeviceValue)
		_, err = dav.UndoConflict(ctx, e.db, cs[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "Cara", *e.reload(p.ID).MiddleName)
	})

	t.Run("an unchanged card with a middle name changes nothing", func(t *testing.T) {
		e := newEnv(t, false)
		card := "BEGIN:VCARD\nVERSION:3.0\nUID:mid-3\nN:Smith;Jane;Marie;Dr.;PhD\nFN:Dr. Jane Marie Smith PhD\nEND:VCARD\n"
		require.Equal(t, http.StatusCreated, e.putCard("mid-3", card, nil).StatusCode)
		before := e.personOf("mid-3").UpdatedAt
		got, etag := e.getCard("mid-3")
		assert.Equal(t, crlf(card), got)
		resp := e.putCard("mid-3", card, map[string]string{"If-Match": etagHeader(etag)})
		assert.Less(t, resp.StatusCode, 300)
		assert.Equal(t, before, e.personOf("mid-3").UpdatedAt)
		assert.Empty(t, e.conflicts())
	})
}
