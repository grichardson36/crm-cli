package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Default relation kinds. A link made without --kind uses them.
const (
	DefaultPersonKind = "related"
	DefaultOrgKind    = "parent"
)

// RelationKind is a named, optionally directional kind of person-to-person or org-to-org link.
// A row (a, b, kind) reads "a is Name b". Seen from b, a is shown as Name. Seen from a, b is shown
// as InverseName. A symmetric kind has the same name both ways.
type RelationKind struct {
	ID          int64  `json:"id"`
	UUID        string `json:"uuid"`
	ObjectType  string `json:"object_type"`
	Name        string `json:"name"`
	InverseName string `json:"inverse_name"`
	Symmetric   bool   `json:"symmetric"`
	// Phrase kinds are verbs (referred-by). Their labels read from the viewing side: the page of a
	// shows b as Name, the page of b shows a as InverseName. Other kinds show the other person's role.
	Phrase  bool `json:"phrase"`
	Builtin bool `json:"builtin"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// LinkAttrs are the attributes a link carries. A nil field means "not given": on a new link it is
// left empty, on an existing link it is left unchanged. An empty string clears a text or date field.
type LinkAttrs struct {
	Role      *string `json:"role,omitempty"`
	StartedOn *string `json:"from,omitempty"`
	EndedOn   *string `json:"to,omitempty"`
	Primary   *bool   `json:"primary,omitempty"`
	// Kind is a relation kind name (person and organization relations).
	Kind  *string `json:"kind,omitempty"`
	Notes *string `json:"notes,omitempty"`
	// CreateRole lets a deal link take a role that is not on the deal role list: it is added to the
	// list while the link is saved (--create, create in MCP). It is a switch, not stored.
	CreateRole bool `json:"-"`
}

// Empty reports whether no attribute is set.
func (a LinkAttrs) Empty() bool {
	return a.Role == nil && a.StartedOn == nil && a.EndedOn == nil && a.Primary == nil && a.Kind == nil && a.Notes == nil
}

// LinkedObject is one link seen from one end: the object at the other end and the link's attributes.
// Record pages and `show` list them.
type LinkedObject struct {
	// LinkID and Relation (the storage table) identify the stored link.
	LinkID   int64  `json:"link_id"`
	Relation string `json:"relation"`
	// Kind, ID and Name describe the object at the other end.
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Label is the relation kind as the viewer sees it (parent, child, friend). Empty for links without a kind.
	Label     string  `json:"label,omitempty"`
	Role      *string `json:"role,omitempty"`
	StartedOn *string `json:"from,omitempty"`
	EndedOn   *string `json:"to,omitempty"`
	Primary   bool    `json:"primary,omitempty"`
	Notes     *string `json:"notes,omitempty"`
	// Current is false for a membership that has ended.
	Current bool `json:"current"`
	// Incoming is true when the link is stored with the other object as "a" (it points at the viewer).
	Incoming bool `json:"-"`
}

// Dates are stored as ISO text, a full day, a month or only a year (2021, 2021-03, 2021-03-15).
var linkDateRE = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2})?)?$`)

// NormalizeLinkDate checks a link date. "" is allowed and means cleared. "today" and "now" give
// today's date.
func NormalizeLinkDate(s string, now time.Time) (string, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "":
		return "", nil
	case "today", "now":
		return now.Format("2006-01-02"), nil
	}
	if !linkDateRE.MatchString(s) {
		return "", fmt.Errorf("invalid date %q (use YYYY, YYYY-MM or YYYY-MM-DD): %w", s, ErrValidation)
	}
	layout := map[int]string{4: "2006", 7: "2006-01", 10: "2006-01-02"}[len(s)]
	if _, err := time.Parse(layout, s); err != nil {
		return "", fmt.Errorf("invalid date %q: %w", s, ErrValidation)
	}
	return s, nil
}

// CurrentOn reports whether a link with these dates is current on the day. A link with no end is
// current. An end on or after the day counts as current.
func CurrentOn(endedOn *string, day string) bool {
	return endedOn == nil || *endedOn == "" || *endedOn >= day[:min(len(*endedOn), len(day))]
}
