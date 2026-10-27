package sync_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
)

// Device sync applies records through the repositories, so a record the shared validation
// framework rejects is counted as a record error and creates nothing.
func TestSyncApplyUsesTheValidationFramework(t *testing.T) {
	e := newEnv(t, personRec(1, "bad1", crmsync.PersonPatch{FirstName: ptr("Zed"), Emails: []string{"not-an-email"}}))
	res := e.run(crmsync.RunOptions{AllowCreate: true})
	assert.Equal(t, 1, res.Errors)
	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 0, e.count(`SELECT COUNT(*) FROM people`))
}

func TestSyncApplyCleansText(t *testing.T) {
	e := newEnv(t, personRec(1, "ok1", crmsync.PersonPatch{FirstName: ptr("  Zed "), Company: ptr("  Acme   Co ")}))
	res := e.run(crmsync.RunOptions{AllowCreate: true})
	assert.Equal(t, 1, res.Created)
	var first, company string
	assert.NoError(t, e.db.QueryRow(`SELECT first_name, company FROM people`).Scan(&first, &company))
	assert.Equal(t, "Zed", first)
	assert.Equal(t, "Acme Co", company)
}
