package model

// DavState is what the CardDAV server remembers about one person's contact on the phone.
type DavState struct {
	PersonID   int64   `json:"person_id"`
	BaseJSON   *string `json:"-"`
	ETag       *string `json:"etag"`
	ExtraVCard *string `json:"-"`
	// RawVCard is the last vCard text the phone and the server agreed on, byte for byte. RawSource is
	// "device" when it is the phone's own PUT and "server" when the CRM rendered it.
	RawVCard  *string `json:"-"`
	RawSource *string `json:"-"`
	SyncedAt  *string `json:"synced_at"`
}

// DavIssue is a phone contact the server could not use, flagged for review. A vCard that cannot be
// read is skipped and recorded here, it is never read as an emptied contact.
type DavIssue struct {
	ID         int64   `json:"id"`
	UUID       string  `json:"uuid"`
	DavID      string  `json:"dav_id"`
	PersonID   *int64  `json:"person_id"`
	Kind       string  `json:"kind"`
	Message    string  `json:"message"`
	RawVCard   *string `json:"raw_vcard,omitempty"`
	CreatedAt  string  `json:"created_at"`
	ResolvedAt *string `json:"resolved_at"`
}

// Sync issue kinds.
const DavIssueUnreadable = "unreadable_vcard"

// SyncConflict is one field where the phone and the CRM both changed, or a link of a phone
// contact to an existing person.
type SyncConflict struct {
	ID          int64   `json:"id"`
	UUID        string  `json:"uuid"`
	PersonID    int64   `json:"person_id"`
	PersonName  string  `json:"person_name,omitempty"`
	Field       string  `json:"field"`
	CRMValue    *string `json:"crm_value"`
	DeviceValue *string `json:"device_value"`
	Winner      string  `json:"winner"`
	ResolvedAt  *string `json:"resolved_at"`
	Undone      bool    `json:"undone"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// Sync conflict winners.
const (
	ConflictWinnerCRM    = "crm"
	ConflictWinnerDevice = "device"
	ConflictWinnerLink   = "link"
)

// DavStatus summarizes the CardDAV server's state in the database.
type DavStatus struct {
	ContactsExposed int     `json:"contacts_exposed"`
	DeviceCreated   int     `json:"device_created"`
	SyncedToDevice  int     `json:"synced_to_device"`
	LastRequestAt   *string `json:"last_request_at"`
	LastGetAt       *string `json:"last_get_at"`
	LastPutAt       *string `json:"last_put_at"`
	Conflicts       int     `json:"conflicts"`
	OpenConflicts   int     `json:"open_conflicts"`
	OpenIssues      int     `json:"open_issues"`
}
