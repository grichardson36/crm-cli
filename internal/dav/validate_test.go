package dav_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// An inbound card goes through the repositories, so the shared validation framework rejects it
// with a 400 and the code in the body, and nothing is created.
func TestPutRejectsWhatTheValidationFrameworkRejects(t *testing.T) {
	e := newEnv(t, false)
	long := strings.Repeat("A", 600)
	card := "BEGIN:VCARD\nVERSION:3.0\nUID:bad-1\nN:Quinn;" + long + ";;;\nFN:" + long + " Quinn\nEND:VCARD\n"
	resp := e.do("PUT", bookPath+"bad-1.vcf", strings.ReplaceAll(card, "\n", "\r\n"),
		map[string]string{"Content-Type": vcardHdr, "If-None-Match": "*"}, true)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, bodyOf(t, resp))
	var n int
	assert.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM people`).Scan(&n))
	assert.Equal(t, 0, n)
}
