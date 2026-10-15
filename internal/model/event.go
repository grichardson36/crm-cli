package model

import (
	"fmt"
	"time"
)

// Event statuses.
const (
	EventConfirmed = "confirmed"
	EventTentative = "tentative"
	EventCancelled = "cancelled"
)

// EventStatuses lists the valid event statuses.
var EventStatuses = []string{EventConfirmed, EventTentative, EventCancelled}

// ValidEventStatus checks an event status.
func ValidEventStatus(s string) bool {
	for _, x := range EventStatuses {
		if x == s {
			return true
		}
	}
	return false
}

// Attendee roles and RSVP answers.
var (
	EventRoles = []string{"organizer", "attendee"}
	EventRSVPs = []string{"needs-action", "accepted", "declined", "tentative"}
)

func inList(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ValidateEventRole checks an attendee role.
func ValidateEventRole(s string) error {
	if !inList(EventRoles, s) {
		return fmt.Errorf("invalid role %q (must be organizer or attendee): %w", s, ErrValidation)
	}
	return nil
}

// ValidateEventRSVP checks an RSVP answer.
func ValidateEventRSVP(s string) error {
	if !inList(EventRSVPs, s) {
		return fmt.Errorf("invalid rsvp %q (must be one of needs-action, accepted, declined, tentative): %w", s, ErrValidation)
	}
	return nil
}

// Edit scopes for a repeating event.
const (
	ScopeAll       = "all"
	ScopeThis      = "this"
	ScopeFollowing = "following"
)

// ValidScope checks an edit scope. The empty scope means all.
func ValidScope(s string) bool {
	return s == "" || s == ScopeAll || s == ScopeThis || s == ScopeFollowing
}

// Event is a calendar entry: one event, a repeating series, or one replaced instance of a series.
type Event struct {
	ID    int64  `json:"id"`
	UUID  string `json:"uuid"`
	Title string `json:"title"`

	Notes    *string `json:"notes"`
	Location *string `json:"location"`

	AllDay bool `json:"all_day"`
	// StartUTC and EndUTC are RFC 3339 UTC instants of a timed event.
	StartUTC *string `json:"start_utc"`
	EndUTC   *string `json:"end_utc"`
	// StartDate and EndDate are the dates of an all-day event. EndDate is exclusive.
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
	// TZ is the IANA zone of the author's wall clock.
	TZ *string `json:"tz"`
	// RRule is the RFC 5545 recurrence rule body, empty for a single event.
	RRule *string `json:"rrule"`

	RecurrenceParentID *int64  `json:"recurrence_parent_id,omitempty"`
	RecurrenceID       *string `json:"recurrence_id,omitempty"`

	Status   string `json:"status"`
	Busy     bool   `json:"busy"`
	Source   string `json:"source"`
	Floating bool   `json:"floating,omitempty"`

	Exdates   []string        `json:"exdates,omitempty"`
	Attendees []EventAttendee `json:"attendees,omitempty"`
	Guests    []EventGuest    `json:"guests,omitempty"`
	Reminders []EventReminder `json:"reminders,omitempty"`
	Tags      []string        `json:"tags,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Archived  bool   `json:"-"`
}

// EventAttendee links an event to a person.
type EventAttendee struct {
	PersonID int64  `json:"person_id"`
	Name     string `json:"name,omitempty"`
	Role     string `json:"role"`
	RSVP     string `json:"rsvp"`
}

// EventGuest is an attendee who is not a contact: the address and name the invitation carried.
// No contact is created from it.
type EventGuest struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	Role  string `json:"role"`
	RSVP  string `json:"rsvp"`
}

// EventReminder asks for a notice some minutes before the start. Delivery is outside the program.
type EventReminder struct {
	MinutesBefore int    `json:"minutes_before"`
	Method        string `json:"method"`
}

// EventInstance is one occurrence of an event in a time window, as lists and calendar views show it.
type EventInstance struct {
	EventID  int64  `json:"event_id"`
	UUID     string `json:"uuid"`
	SeriesID int64  `json:"series_id"`
	Title    string `json:"title"`
	Location string `json:"location,omitempty"`
	Status   string `json:"status"`
	Busy     bool   `json:"busy"`
	AllDay   bool   `json:"all_day"`
	// Start and End are UTC instants for a timed event. They are empty for an all-day event, which
	// carries StartDate and EndDate (exclusive) instead.
	Start     string `json:"start,omitempty"`
	End       string `json:"end,omitempty"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
	TZ        string `json:"tz,omitempty"`
	Recurring bool   `json:"recurring"`
	// Occurrence is the original start of the instance in its series (an instant or a date). Pass it
	// as the scope anchor when editing or deleting one instance.
	Occurrence string `json:"occurrence,omitempty"`
	Attendees  int    `json:"attendees"`
}

// CreateEventInput holds the fields for a new event.
type CreateEventInput struct {
	Title    string
	Notes    *string
	Location *string
	AllDay   bool
	// Start and End are instants for a timed event. For an all-day event they are midnight UTC of the
	// first day and of the day after the last day. A zero End defaults to one hour (timed) or one day.
	Start, End time.Time
	// TZ is the IANA zone name. Empty means the device zone.
	TZ       string
	RRule    string
	Exdates  []time.Time
	Status   string
	Busy     *bool
	Source   string
	Floating bool

	Attendees []EventAttendee
	Guests    []EventGuest
	Reminders []EventReminder
	Tags      []string
}

// UpdateEventInput holds optional changes. Nil means unchanged.
type UpdateEventInput struct {
	Title    *string
	Notes    *string
	Location *string
	AllDay   *bool
	// Start moves the event. Without End the duration is kept.
	Start *time.Time
	End   *time.Time
	TZ    *string
	// RRule replaces the rule. An empty string makes the event a single event.
	RRule  *string
	Status *string
	Busy   *bool
}

// EventFilters selects events for a listing.
type EventFilters struct {
	// From and To bound the window [From, To). Both are required.
	From, To time.Time
	// Loc is the viewer's zone. It decides which dates an all-day event covers in the window.
	Loc *time.Location
	// PersonID keeps events with this attendee.
	PersonID *int64
	// Tag keeps events with this tag.
	Tag string
	// Query keeps events matching this full text query.
	Query string
	// IncludeCancelled shows cancelled events.
	IncludeCancelled bool
	// Limit caps the instances returned. 0 means the default cap.
	Limit int
}
