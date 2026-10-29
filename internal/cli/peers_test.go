package cli_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeersInitStatusAndRefusals(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	_, stderr, code := crm(t, db, "person", "add", "Jane Smith")
	require.Equal(t, 0, code, stderr)

	out, _, code := crm(t, db, "peers", "status", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, `"enabled":false`)

	_, stderr, code = crm(t, db, "peers", "list")
	assert.Equal(t, 2, code, "sync must be on first")
	assert.Contains(t, stderr, "peers init")

	out, stderr, code = crm(t, db, "peers", "init", "--name", "laptop", "-f", "json")
	require.Equal(t, 0, code, stderr)
	var res struct {
		Device   string `json:"device"`
		Baseline int    `json:"baseline_changes"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	assert.NotEmpty(t, res.Device)
	assert.Greater(t, res.Baseline, 0)

	// Rows are untouched and the person still lists.
	out, _, code = crm(t, db, "person", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "Jane")

	// A later write is captured.
	_, _, code = crm(t, db, "person", "edit", "1", "--title", "CTO")
	require.Equal(t, 0, code)
	out, _, _ = crm(t, db, "peers", "status", "-f", "json")
	assert.Contains(t, out, `"enabled":true`)
	assert.Contains(t, out, "laptop")

	_, stderr, code = crm(t, db, "peers", "serve", "--addr", "0.0.0.0:7499")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "--allow-public")
	_, stderr, code = crm(t, db, "peers", "serve", "--addr", "8.8.8.8:7499")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "public address")

	_, stderr, code = crm(t, db, "peers", "sync")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "no relay added")
	_, _, code = crm(t, db, "peers", "resolve", "1")
	assert.Equal(t, 2, code, "needs --keep or --delete")
	out, _, code = crm(t, db, "peers", "review", "-f", "json")
	require.Equal(t, 0, code)
	assert.Equal(t, "[]", strings.TrimSpace(out))
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func TestPeersSyncBetweenTwoDatabasesThroughServe(t *testing.T) {
	dir := t.TempDir()
	relayDB, laptop, phone := filepath.Join(dir, "relay.db"), filepath.Join(dir, "laptop.db"), filepath.Join(dir, "phone.db")
	for _, p := range []string{relayDB, laptop, phone} {
		_, stderr, code := crm(t, p, "peers", "init")
		require.Equal(t, 0, code, stderr)
	}
	addr := freeAddr(t)
	srv := exec.Command(binaryPath, "--db", relayDB, "peers", "serve", "--addr", addr)
	var serr strings.Builder
	srv.Stderr = &serr
	require.NoError(t, srv.Start())
	t.Cleanup(func() { _ = srv.Process.Kill(); _ = srv.Wait() })
	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
		}
		return err == nil
	}, 20*time.Second, 100*time.Millisecond, serr.String())
	url := "http://" + addr

	enroll := func(label string) string {
		out, stderr, code := crm(t, relayDB, "peers", "enroll", "--label", label, "-f", "json")
		require.Equal(t, 0, code, stderr)
		var r struct {
			Token string `json:"token"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &r))
		return r.Token
	}
	_, stderr, code := crm(t, laptop, "peers", "add", "home", url, "--token", enroll("laptop"), "--as", "laptop")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, phone, "peers", "add", "home", url, "--token", enroll("phone"), "--as", "phone")
	require.Equal(t, 0, code, stderr)
	// A token works once.
	_, _, code = crm(t, phone, "peers", "add", "again", url, "--token", "crmenr_nope")
	assert.Equal(t, 4, code)

	_, stderr, code = crm(t, laptop, "person", "add", "Ada Lovelace", "--email", "ada@example.com")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, laptop, "peers", "sync")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, phone, "peers", "sync")
	require.Equal(t, 0, code, stderr)
	out, _, code := crm(t, phone, "person", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "Ada")

	out, _, code = crm(t, phone, "peers", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "home")
	out, _, code = crm(t, relayDB, "peers", "list", "-f", "json")
	require.Equal(t, 0, code)
	assert.Contains(t, out, "laptop")
	assert.Contains(t, out, "phone")

	_, stderr, code = crm(t, relayDB, "peers", "retire", "phone")
	require.Equal(t, 0, code, stderr)
	_, stderr, code = crm(t, phone, "peers", "sync")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, fmt.Sprint(stderr), "401")
}
