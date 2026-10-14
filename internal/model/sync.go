package model

// UpdateInteractionInput holds optional fields for editing an interaction.
// An empty string clears Subject, Content and Direction.
type UpdateInteractionInput struct {
	Type       *string
	Subject    *string
	Content    *string
	Direction  *string
	OccurredAt *string
}

// ExternalID links a local row to a record in an external system.
type ExternalID struct {
	ID         int64   `json:"id"`
	UUID       string  `json:"uuid"`
	Entity     string  `json:"entity"`
	LocalID    int64   `json:"local_id"`
	LocalUUID  string  `json:"local_uuid"`
	Source     string  `json:"source"`
	ExternalID string  `json:"external_id"`
	ETag       *string `json:"etag"`
	LastSeenAt *string `json:"last_seen_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// SyncRun is one execution of a sync source.
type SyncRun struct {
	ID         int64   `json:"id"`
	UUID       string  `json:"uuid"`
	Source     string  `json:"source"`
	StartedAt  string  `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	Status     string  `json:"status"`
	Created    int     `json:"created"`
	Updated    int     `json:"updated"`
	Skipped    int     `json:"skipped"`
	Errors     int     `json:"errors"`
	Note       *string `json:"note"`
}

// SyncCursor is the resume position for a source.
type SyncCursor struct {
	Source    string  `json:"source"`
	Cursor    *string `json:"cursor"`
	UpdatedAt string  `json:"updated_at"`
}

// Sync run statuses.
const (
	SyncRunning = "running"
	SyncOK      = "ok"
	SyncPartial = "partial"
	SyncFailed  = "failed"
)

// Sync inbox statuses.
const (
	InboxPending  = "pending"
	InboxAccepted = "accepted"
	InboxAttached = "attached"
	InboxIgnored  = "ignored"
)

// SyncInboxItem is an incoming record that matched no local row.
type SyncInboxItem struct {
	ID         int64  `json:"id"`
	UUID       string `json:"uuid"`
	Source     string `json:"source"`
	ExternalID string `json:"external_id"`
	Payload    string `json:"payload"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}
