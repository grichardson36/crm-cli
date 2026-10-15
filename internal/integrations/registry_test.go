package integrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRegistryOffersCalDAVOnlyWhenConfigured(t *testing.T) {
	none := DefaultRegistry(func(string) string { return "" })
	assert.Empty(t, none.Names())

	env := map[string]string{"CRM_CALDAV_URL": "https://dav.example.com/grant/personal/", "CRM_CALDAV_USER": "grant"}
	with := DefaultRegistry(func(k string) string { return env[k] })
	assert.Equal(t, []string{"caldav"}, with.Names())
	a, err := with.Get("caldav")
	assert.NoError(t, err)
	assert.Equal(t, "caldav", a.Name())

	env["CRM_SYNC_FAKE_FILE"] = "x.json"
	assert.Equal(t, []string{"caldav", "fake"}, DefaultRegistry(func(k string) string { return env[k] }).Names())
}
