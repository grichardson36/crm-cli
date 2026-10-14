package cli_test

import (
	"bufio"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDavServeRefusesWithoutCredentials(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	_, stderr, code := crm(t, db, "dav", "serve")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "without credentials")

	_, stderr, code = crm(t, db, "dav", "serve", "--password-env", "CRM_TEST_UNSET_PASSWORD_VAR")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "CRM_TEST_UNSET_PASSWORD_VAR")

	empty := filepath.Join(t.TempDir(), "pw")
	require.NoError(t, os.WriteFile(empty, []byte("\n"), 0o600))
	_, _, code = crm(t, db, "dav", "serve", "--password-file", empty)
	assert.Equal(t, 2, code)

	_, _, code = crm(t, db, "dav", "serve", "--password-file", empty, "--password-env", "X")
	assert.Equal(t, 2, code)

	_, _, code = crm(t, db, "dav", "serve", "--user", "bad user!", "--password-env", "X")
	assert.Equal(t, 2, code)
}

func TestDavServeRefusesNonLoopback(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	pw := filepath.Join(t.TempDir(), "pw")
	require.NoError(t, os.WriteFile(pw, []byte("hunter2-hunter2\n"), 0o600))
	for _, addr := range []string{"0.0.0.0:0", ":0", "192.168.1.5:5232"} {
		_, stderr, code := crm(t, db, "dav", "serve", "--addr", addr, "--password-file", pw)
		assert.Equal(t, 2, code, addr)
		assert.Contains(t, stderr, "--allow-remote", addr)
	}
}

func TestDavInfoStatusAndConflictsCommands(t *testing.T) {
	db := seedTemplate(t)

	stdout, _, code := crm(t, db, "dav", "info", "--base-url", "https://contacts.example.com", "--user", "grant", "-f", "json")
	require.Equal(t, 0, code)
	var info struct {
		BaseURL        string              `json:"base_url"`
		User           string              `json:"user"`
		AddressBookURL string              `json:"address_book_url"`
		DiscoveryURL   string              `json:"discovery_url"`
		Steps          map[string][]string `json:"steps"`
	}
	jsonOut(t, stdout, &info)
	assert.Equal(t, "https://contacts.example.com", info.BaseURL)
	assert.Equal(t, "https://contacts.example.com/dav/grant/contacts/", info.AddressBookURL)
	assert.Equal(t, "https://contacts.example.com/.well-known/carddav", info.DiscoveryURL)
	assert.NotEmpty(t, info.Steps["android"])
	assert.NotEmpty(t, info.Steps["ios"])
	assert.NotEmpty(t, info.Steps["macos"])

	stdout, _, code = crm(t, db, "dav", "info", "-f", "table")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "DAVx5")
	assert.Contains(t, stdout, "Login with URL and user name")

	stdout, _, code = crm(t, db, "dav", "status", "-f", "json")
	require.Equal(t, 0, code)
	var st map[string]any
	jsonOut(t, stdout, &st)
	assert.EqualValues(t, 3, st["contacts_exposed"], "Old Contact is archived")
	assert.EqualValues(t, 0, st["conflicts"])

	stdout, _, code = crm(t, db, "dav", "conflicts", "-f", "json")
	require.Equal(t, 0, code)
	var list []map[string]any
	jsonOut(t, stdout, &list)
	assert.Empty(t, list)

	_, _, code = crm(t, db, "dav", "conflicts", "show", "99")
	assert.Equal(t, 3, code)
	_, _, code = crm(t, db, "dav", "conflicts", "undo", "99")
	assert.Equal(t, 3, code)
	_, _, code = crm(t, db, "dav", "conflicts", "show", "abc")
	assert.Equal(t, 2, code)
}

var listenLine = regexp.MustCompile(`listening on http://(127\.0\.0\.1:\d+)`)

// startDav runs `crm dav serve` on a random loopback port and returns its address.
func startDav(t *testing.T, db string, extra ...string) string {
	t.Helper()
	pw := filepath.Join(t.TempDir(), "pw")
	require.NoError(t, os.WriteFile(pw, []byte("hunter2-hunter2\n"), 0o600))
	args := append([]string{"--db", db, "dav", "serve", "--addr", "127.0.0.1:0", "--user", "grant", "--password-file", pw}, extra...)
	cmd := exec.Command(binaryPath, args...)
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if m := listenLine.FindStringSubmatch(sc.Text()); m != nil {
				found <- m[1]
			}
		}
	}()
	select {
	case addr := <-found:
		return addr
	case <-time.After(15 * time.Second):
		t.Fatal("server did not report its address")
		return ""
	}
}

func TestDavServeEndToEnd(t *testing.T) {
	db := seedTemplate(t)
	addr := startDav(t, db)
	base := "http://" + addr

	do := func(method, path, body string, auth bool, hdr map[string]string) *http.Response {
		req, err := http.NewRequest(method, base+path, strings.NewReader(body))
		require.NoError(t, err)
		if auth {
			req.SetBasicAuth("grant", "hunter2-hunter2")
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}

	assert.Equal(t, http.StatusUnauthorized, do("PROPFIND", "/dav/", "", false, map[string]string{"Depth": "0"}).StatusCode)
	resp := do("PROPFIND", "/dav/grant/contacts/", "", true, map[string]string{"Depth": "1"})
	assert.Equal(t, http.StatusMultiStatus, resp.StatusCode)

	// Jane, Bob and Janet are active, Old Contact is archived.
	listBody := new(strings.Builder)
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		listBody.WriteString(sc.Text())
	}
	assert.Equal(t, 3, strings.Count(listBody.String(), ".vcf</"), listBody.String())

	// A new contact from the phone shows up in the CRM through the CLI.
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:phone-9\r\nN:Newcomer;Nia;;;\r\nFN:Nia Newcomer\r\nEMAIL:nia@example.com\r\nEND:VCARD\r\n"
	resp = do("PUT", "/dav/grant/contacts/phone-9.vcf", card, true, map[string]string{"Content-Type": "text/vcard", "If-None-Match": "*"})
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	stdout, _, code := crm(t, db, "person", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "Nia")
	stdout, _, code = crm(t, db, "dav", "status", "-f", "json")
	require.Equal(t, 0, code)
	var st map[string]any
	jsonOut(t, stdout, &st)
	assert.EqualValues(t, 4, st["contacts_exposed"])
	assert.EqualValues(t, 1, st["device_created"])
	assert.NotNil(t, st["last_put_at"])
}

func TestDavServeReadOnly(t *testing.T) {
	db := seedTemplate(t)
	addr := startDav(t, db, "--read-only")
	req, err := http.NewRequest("PUT", "http://"+addr+"/dav/grant/contacts/x.vcf", strings.NewReader("BEGIN:VCARD\r\nVERSION:3.0\r\nFN:X\r\nEND:VCARD\r\n"))
	require.NoError(t, err)
	req.SetBasicAuth("grant", "hunter2-hunter2")
	req.Header.Set("Content-Type", "text/vcard")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
