package importer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db"
	"github.com/jdanielnd/crm-cli/internal/importer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contactsCSV = `full_name,first_name,last_name,job_title,company,description,birthday,birthday_year,linkedin,website,starred,frequency,frequency_text,last_seen_at,created_at,updated_at,dex_email,dex_emails,dex_phone,dex_phones,dex_groups,dex_tags,Discord,id
Jane Doe,Jane,Doe,CTO,Acme,Met at conf,2022-03-14,,https://linkedin.com/in/jane,https://jane.example,true,Every month,1 mon,2026-02-10T09:00:00.000Z,2025-01-01T00:00:00.000Z,2025-01-02T00:00:00.000Z,jane@acme.com,"jane@acme.com,jane@home.com",'+15551234567,"'+15551234567,'+15557654321","LinkedIn Connections,Acme",Investor,jd#1,11111111-1111-1111-1111-111111111111
,,,,,,'--07-04,,,,false,Don't keep in touch,,,2025-01-01T00:00:00.000Z,2025-01-01T00:00:00.000Z,solo@example.com,solo@example.com,,,,,,22222222-2222-2222-2222-222222222222
Bob Real,Bob,Real,,,,1980-12-25,1980,,,false,,,,2025-01-01T00:00:00.000Z,2025-01-01T00:00:00.000Z,,,,,,,,33333333-3333-3333-3333-333333333333
`

const notesCSV = `id,event_time,contact_names,contact_emails,note_type,basic_note,note,contact_ids
n1,2026-03-01T10:00:00.000Z,Jane Doe,jane@acme.com,Meeting,Talked roadmap,<p>Talked roadmap</p>,11111111-1111-1111-1111-111111111111
n2,2026-03-02T10:00:00.000Z,Jane Doe,,Life Event,New baby,<p>New baby</p>,"11111111-1111-1111-1111-111111111111, 33333333-3333-3333-3333-333333333333 ,99999999-9999-9999-9999-999999999999"
`

func writeFiles(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	c := filepath.Join(dir, "contacts.csv")
	n := filepath.Join(dir, "notes.csv")
	require.NoError(t, os.WriteFile(c, []byte(contactsCSV), 0o600))
	require.NoError(t, os.WriteFile(n, []byte(notesCSV), 0o600))
	return c, n
}

func TestImportDex(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	contacts, notes := writeFiles(t)

	rep, err := importer.ImportDex(context.Background(), d, contacts, notes, importer.DexOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, rep.ContactsImported)
	assert.Equal(t, 3, rep.Emails, "jane has two, solo has one")
	assert.Equal(t, 2, rep.Phones)
	assert.Equal(t, 2, rep.NotesImported)
	assert.Equal(t, 3, rep.NoteLinks)
	assert.Equal(t, 1, rep.NoteLinksMissing)

	one := func(q string, args ...any) string {
		var s string
		require.NoError(t, d.QueryRow(q, args...).Scan(&s))
		return s
	}

	// Phones lose the formula-guard apostrophe, and the first is primary.
	assert.Equal(t, "+15551234567", one(`SELECT phone FROM people WHERE first_name='Jane'`))
	assert.Equal(t, "+15557654321", one(`SELECT phone_number FROM person_phones WHERE is_primary = 0`))

	// Birthday year is only trusted when the explicit column is filled.
	assert.Equal(t, "3-14-", one(`SELECT birthday_month || '-' || birthday_day || '-' || COALESCE(birthday_year, '') FROM people WHERE first_name='Jane'`))
	assert.Equal(t, "7-4-", one(`SELECT birthday_month || '-' || birthday_day || '-' || COALESCE(birthday_year, '') FROM people WHERE email='solo@example.com'`))
	assert.Equal(t, "12-25-1980", one(`SELECT birthday_month || '-' || birthday_day || '-' || birthday_year FROM people WHERE first_name='Bob'`))

	// Cadence, favorite, last seen, timestamps.
	assert.Equal(t, "1 mon", one(`SELECT keep_in_touch FROM people WHERE first_name='Jane'`))
	assert.Equal(t, "never", one(`SELECT keep_in_touch FROM people WHERE email='solo@example.com'`))
	assert.Equal(t, "2026-02-10 09:00:00", one(`SELECT last_contacted_at FROM people WHERE first_name='Jane'`))
	assert.Equal(t, "1", one(`SELECT is_favorite FROM people WHERE first_name='Jane'`))
	assert.Equal(t, "2025-01-01 00:00:00", one(`SELECT created_at FROM people WHERE first_name='Jane'`))

	// A contact with no name falls back to its email.
	assert.Equal(t, "solo@example.com", one(`SELECT first_name FROM people WHERE email='solo@example.com'`))

	// Handles, tags (groups and Dex tags), custom fields.
	assert.Equal(t, "https://linkedin.com/in/jane", one(`SELECT value FROM person_handles WHERE kind='linkedin'`))
	assert.Equal(t, "https://jane.example", one(`SELECT value FROM person_handles WHERE kind='website'`))
	assert.Equal(t, "3", one(`SELECT COUNT(*) FROM taggings WHERE entity_id = 1`))
	assert.Equal(t, "jd#1", one(`SELECT field_value FROM custom_fields WHERE field_name='Discord'`))

	// Notes: types map, unknown types become the subject, and links resolve.
	assert.Equal(t, "meeting", one(`SELECT type FROM interactions WHERE uuid='dex:n1'`))
	assert.Equal(t, "note", one(`SELECT type FROM interactions WHERE uuid='dex:n2'`))
	assert.Equal(t, "Life Event", one(`SELECT subject FROM interactions WHERE uuid='dex:n2'`))
	assert.Equal(t, "New baby", one(`SELECT content FROM interactions WHERE uuid='dex:n2'`))
}

func TestImportDex_IdempotentAndDryRun(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	contacts, notes := writeFiles(t)

	rep, err := importer.ImportDex(context.Background(), d, contacts, notes, importer.DexOptions{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, 3, rep.ContactsImported)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&n))
	assert.Equal(t, 0, n, "dry run writes nothing")

	_, err = importer.ImportDex(context.Background(), d, contacts, notes, importer.DexOptions{})
	require.NoError(t, err)
	rep, err = importer.ImportDex(context.Background(), d, contacts, notes, importer.DexOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, rep.ContactsImported)
	assert.Equal(t, 3, rep.ContactsSkipped)
	assert.Equal(t, 0, rep.NotesImported)
	assert.Equal(t, 2, rep.NotesSkipped)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&n))
	assert.Equal(t, 3, n)

	// The mapping lives in external_ids (source dex). The custom field stays.
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM external_ids WHERE source = 'dex' AND entity = 'person'`).Scan(&n))
	assert.Equal(t, 3, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM custom_fields WHERE field_name = 'dex_id'`).Scan(&n))
	assert.Equal(t, 3, n)
	var local int64
	require.NoError(t, d.QueryRow(`SELECT e.local_id FROM external_ids e JOIN people p ON p.id = e.local_id AND p.uuid = e.local_uuid
		WHERE e.external_id = '11111111-1111-1111-1111-111111111111'`).Scan(&local))
	var first string
	require.NoError(t, d.QueryRow(`SELECT first_name FROM people WHERE id = ?`, local).Scan(&first))
	assert.Equal(t, "Jane", first)
}

func TestImportDex_RejectsOtherCSV(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	path := filepath.Join(t.TempDir(), "other.csv")
	require.NoError(t, os.WriteFile(path, []byte("a,b\n1,2\n"), 0o600))
	_, err = importer.ImportDex(context.Background(), d, path, "", importer.DexOptions{})
	assert.Error(t, err)
}

func TestImportDex_SkipTags(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	contacts, _ := writeFiles(t)

	_, err = importer.ImportDex(context.Background(), d, contacts, "", importer.DexOptions{SkipTags: []string{"linkedin connections"}})
	require.NoError(t, err)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM tags WHERE name = 'LinkedIn Connections'`).Scan(&n))
	assert.Equal(t, 0, n)
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM tags WHERE name = 'Acme'`).Scan(&n))
	assert.Equal(t, 1, n)
}

const relatedCSV = `first_name,last_name,id,dex_related_contacts
Jane,Doe,11111111-1111-1111-1111-111111111111,"Bob Real, 33333333-3333-3333-3333-333333333333, Nobody Known"
Bob,Real,33333333-3333-3333-3333-333333333333,Jane Doe
Cy,Poe,44444444-4444-4444-4444-444444444444,
`

func TestImportDexLinksRelatedContacts(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	path := filepath.Join(t.TempDir(), "contacts.csv")
	require.NoError(t, os.WriteFile(path, []byte(relatedCSV), 0o600))

	rep, err := importer.ImportDex(context.Background(), d, path, "", importer.DexOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, rep.ContactsImported)
	// Jane lists Bob by name and by id (one pair), Bob lists Jane back (same pair), one name matches nobody.
	assert.Equal(t, 1, rep.RelatedLinks)
	assert.Equal(t, 1, rep.RelatedUnresolved)
	var n int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM person_relation pr JOIN relation_kind k ON k.id = pr.kind_id WHERE k.name = 'related' AND pr.a_id < pr.b_id`).Scan(&n))
	assert.Equal(t, 1, n)

	// Importing again links nothing new.
	rep, err = importer.ImportDex(context.Background(), d, path, "", importer.DexOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, rep.RelatedLinks)
}
