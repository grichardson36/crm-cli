// Package relay is the HTTP transport of the sync contract: the relay server any enrolled device
// can run, the client, and the pull-then-push exchange between two databases. Plain net/http and
// JSON. There is no encryption layer: run it over Tailscale or a LAN (contract rule 6).
package relay

import "github.com/jdanielnd/crm-cli/internal/replica"

// Version is the protocol version, the path prefix of every endpoint.
const Version = "v1"

// DefaultLimit is the page size of pull and push.
const DefaultLimit = 1000

// MaxLimit is the largest page a relay serves or accepts.
const MaxLimit = 5000

// PullRequest asks for changes the caller's cursor has not seen. Cursor is the caller's version
// vector. Head is the highest sequence the caller has written, so the relay can tell whether it
// holds everything the caller has made.
type PullRequest struct {
	Cursor replica.VersionVector `json:"cursor"`
	Limit  int                   `json:"limit"`
	Head   int64                 `json:"head"`
}

// PullResponse carries changes in order of device and sequence. Cursor is the caller's cursor
// advanced past the changes returned, Vector is everything the relay holds. More is true when the
// page was full.
type PullResponse struct {
	Device        string                `json:"device"`
	SchemaVersion int                   `json:"schema_version"`
	Changes       []replica.Change      `json:"changes"`
	Cursor        replica.VersionVector `json:"cursor"`
	Vector        replica.VersionVector `json:"vector"`
	More          bool                  `json:"more"`
}

// PushRequest sends changes. Vector is the caller's whole version vector.
type PushRequest struct {
	Changes []replica.Change      `json:"changes"`
	Vector  replica.VersionVector `json:"vector"`
	Head    int64                 `json:"head"`
}

// PushResponse reports what the relay did with each change.
type PushResponse struct {
	Device     string                `json:"device"`
	Applied    int                   `json:"applied"`
	Duplicates int                   `json:"duplicates"`
	Unknown    int                   `json:"unknown"`
	Rejected   []replica.Rejection   `json:"rejected,omitempty"`
	Vector     replica.VersionVector `json:"vector"`
}

// EnrollRequest trades a one-time enroll token for a device token.
type EnrollRequest struct {
	Token  string `json:"token"`
	Device string `json:"device"`
	Name   string `json:"name"`
}

// EnrollResponse carries the long-lived device token and the relay's identity.
type EnrollResponse struct {
	Device        string `json:"device"`
	Name          string `json:"name"`
	Token         string `json:"token"`
	SchemaVersion int    `json:"schema_version"`
}

// StatusResponse is what GET /v1/status returns.
type StatusResponse struct {
	Device        string                `json:"device"`
	SchemaVersion int                   `json:"schema_version"`
	ReadOnly      bool                  `json:"read_only"`
	Vector        replica.VersionVector `json:"vector"`
	Changes       int64                 `json:"changes"`
	OpenReviews   int64                 `json:"open_reviews"`
	Devices       int                   `json:"devices"`
}

// ErrorResponse is the body of every error.
type ErrorResponse struct {
	Error string `json:"error"`
}
