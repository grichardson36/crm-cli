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

// The importer keeps every row and reports what the shared validation framework found.
func TestImportDexReportsValidationFindings(t *testing.T) {
	d, err := db.Open(":memory:")
	require.NoError(t, err)
	defer d.Close()
	damaged := "Caf" + string(rune(0xFFFD)) + " Roma"
	csv := "id,first_name,last_name,company,dex_emails\n" +
		"a1,Jane,Doe,\"" + damaged + "\",nope\n" +
		"a2,Bob,Roe,+1 555 010 0100,bob@x.com\n" +
		"a3,Cy,Poe,  Spaced   Co  ,cy@x.com\n"
	path := filepath.Join(t.TempDir(), "c.csv")
	require.NoError(t, os.WriteFile(path, []byte(csv), 0o600))

	rep, err := importer.ImportDex(context.Background(), d, path, "", importer.DexOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, rep.ContactsImported, "no row is dropped")

	codes := map[string]bool{}
	for _, i := range rep.Issues {
		codes[i.Code] = true
	}
	assert.True(t, codes["W_ENCODING_DAMAGED"])
	assert.True(t, codes["E_EMAIL_INVALID"])
	var sug []string
	for _, s := range rep.Suggestions {
		sug = append(sug, s.Code)
	}
	assert.Contains(t, sug, "S_PHONE_IN_TEXT_FIELD")

	var company string
	require.NoError(t, d.QueryRow(`SELECT company FROM people WHERE first_name = 'Cy'`).Scan(&company))
	assert.Equal(t, "Spaced Co", company, "text is sanitized")
	require.NoError(t, d.QueryRow(`SELECT company FROM people WHERE first_name = 'Jane'`).Scan(&company))
	assert.Equal(t, damaged, company, "a damaged encoding is flagged and never rewritten")
}
