// Package integrations assembles the sync adapters a build offers. Each adapter lives in its own
// package below this one.
package integrations

import (
	"os"

	"github.com/jdanielnd/crm-cli/internal/integrations/caldav"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
)

// DefaultRegistry returns the adapters available given the environment: the test fake when
// CRM_SYNC_FAKE_FILE is set, and CalDAV when CRM_CALDAV_URL is set.
func DefaultRegistry(getenv func(string) string) *crmsync.Registry {
	reg := crmsync.DefaultRegistry(getenv)
	if cfg, ok := caldav.FromEnv(getenv, os.ReadFile); ok {
		reg.Add(&caldav.Adapter{Cfg: cfg})
	}
	return reg
}
