package repo_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/phone"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func phoneDB(t *testing.T) (*repo.PersonRepo, *sql.DB) {
	t.Helper()
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close(); phone.SetDefaultRegion("") })
	return repo.NewPersonRepo(d), d
}

func TestAddPhoneStoresTypedAndE164(t *testing.T) {
	r, _ := phoneDB(t)
	ctx := context.Background()
	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane", Phone: ptr("(415) 555-2671")})
	require.NoError(t, err)

	phones, err := r.Phones(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, phones, 1)
	assert.Equal(t, "(415) 555-2671", phones[0].PhoneNumber, "the typed text is kept")
	assert.Equal(t, "+14155552671", phones[0].E164)
	assert.Equal(t, "(415) 555-2671", phones[0].Display, "home region shows national format")

	t.Run("same number in another format is the same number", func(t *testing.T) {
		require.NoError(t, r.AddPhone(ctx, p.ID, "+1 415-555-2671", ptr("Mobile"), false))
		phones, err := r.Phones(ctx, p.ID)
		require.NoError(t, err)
		require.Len(t, phones, 1)
		assert.Equal(t, "(415) 555-2671", phones[0].PhoneNumber, "the row keeps the text it was typed with")
		assert.Equal(t, "Mobile", *phones[0].Label)
	})
	t.Run("a number from another region shows international", func(t *testing.T) {
		require.NoError(t, r.AddPhone(ctx, p.ID, "+44 20 7946 0958", nil, false))
		phones, _ := r.Phones(ctx, p.ID)
		require.Len(t, phones, 2)
		assert.Equal(t, "+44 20 7946 0958", phones[1].Display)
		assert.Equal(t, "+442079460958", phones[1].E164)
	})
	t.Run("removing by another format removes the number", func(t *testing.T) {
		require.NoError(t, r.RemovePhone(ctx, p.ID, "415.555.2671"))
		phones, _ := r.Phones(ctx, p.ID)
		require.Len(t, phones, 1)
		assert.Equal(t, "+442079460958", phones[0].E164)
	})
	t.Run("text that cannot be parsed is stored as typed", func(t *testing.T) {
		require.NoError(t, r.AddPhone(ctx, p.ID, "555-0100", nil, false))
		phones, _ := r.Phones(ctx, p.ID)
		require.Len(t, phones, 2)
		assert.Equal(t, "555-0100", phones[1].PhoneNumber)
		assert.Empty(t, phones[1].E164)
		assert.Equal(t, "555-0100", phones[1].Display)
	})
}

func TestAddPhoneUsesTheDefaultRegion(t *testing.T) {
	r, _ := phoneDB(t)
	ctx := context.Background()
	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Ann"})
	require.NoError(t, err)

	require.NoError(t, r.AddPhone(ctx, p.ID, "020 7946 0958", nil, false))
	phones, _ := r.Phones(ctx, p.ID)
	assert.Empty(t, phones[0].E164, "no country code and the US region: not a US number")

	require.True(t, phone.SetDefaultRegion("GB"))
	require.NoError(t, r.AddPhone(ctx, p.ID, "020 7946 0958", nil, false))
	phones, _ = r.Phones(ctx, p.ID)
	require.Len(t, phones, 1, "same typed text, one row")
	assert.Equal(t, "+442079460958", phones[0].E164)
	assert.Equal(t, "020 7946 0958", phones[0].Display, "national format in the home region")
}

func TestNormalizePhonesBackfill(t *testing.T) {
	r, d := phoneDB(t)
	ctx := context.Background()
	p, err := r.Create(ctx, model.CreatePersonInput{FirstName: "Jane"})
	require.NoError(t, err)
	// Rows as they were before migration 015: typed text only.
	for _, n := range []string{"(415) 555-2671", "+44 20 7946 0958", "555-0100", "555-123-4567", "415.555.2671"} {
		_, err := d.Exec(`INSERT INTO person_phones (person_id, phone_number) VALUES (?, ?)`, p.ID, n)
		require.NoError(t, err)
	}

	dry, err := r.NormalizePhones(ctx, false, 10)
	require.NoError(t, err)
	assert.Equal(t, 5, dry.Total)
	assert.Equal(t, 4, dry.Fill)
	assert.Equal(t, 1, dry.Unparseable)
	assert.Equal(t, 2, dry.Unlikely, "555-0100 and the 555 area code do not exist")
	assert.Equal(t, 1, dry.Duplicates, "two formats of one number on one person")
	assert.False(t, dry.Applied)
	var filled int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM person_phones WHERE e164 IS NOT NULL`).Scan(&filled))
	assert.Zero(t, filled, "a dry run writes nothing")

	got, err := r.NormalizePhones(ctx, true, 0)
	require.NoError(t, err)
	assert.True(t, got.Applied)
	assert.Equal(t, 4, got.Fill)
	var typed string
	require.NoError(t, d.QueryRow(`SELECT phone_number FROM person_phones WHERE e164 = '+442079460958'`).Scan(&typed))
	assert.Equal(t, "+44 20 7946 0958", typed, "typed text is never changed")

	again, err := r.NormalizePhones(ctx, true, 0)
	require.NoError(t, err)
	assert.Zero(t, again.Fill, "idempotent")
	assert.Equal(t, 4, again.Current-1, "four rows hold a current E.164, the unparseable one has none")

	n, err := r.ClearPhoneE164(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(4), n)
	redo, err := r.NormalizePhones(ctx, false, 0)
	require.NoError(t, err)
	assert.Equal(t, 4, redo.Fill, "undo puts the backfill back to the dry-run state")

	t.Run("a region change refills", func(t *testing.T) {
		phone.SetDefaultRegion("GB")
		rep, err := r.NormalizePhones(ctx, false, 0)
		require.NoError(t, err)
		assert.Equal(t, "GB", rep.Region)
	})
}
