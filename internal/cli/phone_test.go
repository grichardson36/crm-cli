package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhoneNormalizeFlow(t *testing.T) {
	home := t.TempDir()
	run := func(args ...string) (string, string, int) { return keysRun(t, home, args...) }

	_, stderr, code := run("person", "add", "Jane Smith", "--phone", "(415) 555-2671")
	require.Equal(t, 0, code, stderr)
	out, _, code := run("person", "phone", "add", "1", "+44 20 7946 0958", "-f", "json")
	require.Equal(t, 0, code)
	row := jsonRows(t, out)[0]
	phones := row["phones"].([]any)
	require.Len(t, phones, 2)
	first := phones[0].(map[string]any)
	assert.Equal(t, "(415) 555-2671", first["phone_number"], "typed text kept")
	assert.Equal(t, "+14155552671", first["e164"])
	assert.Equal(t, "+44 20 7946 0958", phones[1].(map[string]any)["display"], "other region shows international")

	t.Run("dry run by default then idempotent apply", func(t *testing.T) {
		out, stderr, code := run("phone", "normalize", "-f", "json")
		require.Equal(t, 0, code, stderr)
		assert.Contains(t, stderr, "Dry run")
		var rep map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &rep))
		assert.Equal(t, float64(2), rep["total"])
		assert.Equal(t, float64(0), rep["fill"], "new numbers are already normalized")
		assert.Equal(t, "US", rep["region"])
		_, stderr, code = run("phone", "normalize", "--apply")
		assert.Equal(t, 0, code)
		assert.Contains(t, stderr, "Applied")
	})

	t.Run("the region comes from config.toml", func(t *testing.T) {
		out, _, _ := run("phone", "parse", "020 7946 0958")
		assert.Contains(t, out, `"e164":""`)
		_, stderr, code := run("phone", "region", "gb")
		require.Equal(t, 0, code, stderr)
		data, err := os.ReadFile(filepath.Join(home, ".crm", "config.toml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), `region = "GB"`)
		out, _, _ = run("phone", "region")
		assert.Equal(t, "GB\n", out)
		out, _, _ = run("phone", "parse", "020 7946 0958")
		assert.Contains(t, out, `"e164":"+442079460958"`)
		_, _, code = run("phone", "region", "zz")
		assert.Equal(t, 2, code, "an unknown region is a usage error")
	})

	t.Run("undo clears the forms again", func(t *testing.T) {
		_, stderr, code := run("phone", "normalize", "--undo")
		require.Equal(t, 0, code)
		assert.Contains(t, stderr, "Cleared the E.164 form of 2")
	})
}

func TestPhoneInvalidShapeAndUnlikelyWarning(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crm.db")
	_, stderr, code := crm(t, db, "person", "add", "Bad Shape", "--phone", "hello")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "E_PHONE_INVALID")
	_, stderr, code = crm(t, db, "person", "add", "Odd Area", "--phone", "555-123-4567")
	assert.Equal(t, 0, code, "an unlikely number is stored as typed")
	assert.Contains(t, stderr, "W_PHONE_UNLIKELY")
}
