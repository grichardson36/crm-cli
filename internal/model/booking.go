package model

import (
	"fmt"
	"regexp"
	"strings"
)

// Booking statuses.
const (
	BookingRequested   = "requested"
	BookingConfirmed   = "confirmed"
	BookingCancelled   = "cancelled"
	BookingRescheduled = "rescheduled"
)

// BookingQuestion is a question the invitee answers.
type BookingQuestion struct {
	Label    string `json:"label"`
	Required bool   `json:"required,omitempty"`
}

// BookingLink says which times may be offered to people who want to meet.
type BookingLink struct {
	ID               int64             `json:"id"`
	UUID             string            `json:"uuid"`
	Name             string            `json:"name"`
	Title            string            `json:"title"`
	Slug             string            `json:"slug"`
	Description      string            `json:"description,omitempty"`
	HostEmail        string            `json:"host_email,omitempty"`
	Durations        []int             `json:"durations"`
	Schedule         string            `json:"schedule,omitempty"`
	HorizonDays      int               `json:"horizon_days"`
	StartAfter       *string           `json:"start_after,omitempty"`
	MinNoticeMin     int               `json:"min_notice_min"`
	SlotStepMin      int               `json:"slot_step_min"`
	BufferBeforeMin  int               `json:"buffer_before_min"`
	BufferAfterMin   int               `json:"buffer_after_min"`
	MaxPerDay        *int              `json:"max_per_day,omitempty"`
	MaxPerWeek       *int              `json:"max_per_week,omitempty"`
	MaxPerMonth      *int              `json:"max_per_month,omitempty"`
	BusyTasks        bool              `json:"busy_tasks,omitempty"`
	LocationKind     string            `json:"location_kind,omitempty"`
	Location         string            `json:"location,omitempty"`
	Questions        []BookingQuestion `json:"questions,omitempty"`
	RemindersMin     []int             `json:"reminders_min,omitempty"`
	FollowupAfterMin *int              `json:"followup_after_min,omitempty"`
	MaxUses          *int              `json:"max_uses,omitempty"`
	ExpiresAt        *string           `json:"expires_at,omitempty"`
	Active           bool              `json:"active"`
	Archived         bool              `json:"-"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

// Booking is one booked meeting.
type Booking struct {
	ID           int64             `json:"id"`
	UUID         string            `json:"uuid"`
	LinkID       int64             `json:"link_id"`
	Link         string            `json:"link,omitempty"`
	EventID      *int64            `json:"event_id,omitempty"`
	PersonID     *int64            `json:"person_id,omitempty"`
	InviteeName  string            `json:"invitee_name,omitempty"`
	Email        string            `json:"invitee_email"`
	TZ           string            `json:"invitee_tz,omitempty"`
	Answers      map[string]string `json:"answers,omitempty"`
	Start        string            `json:"start"`
	End          string            `json:"end"`
	Status       string            `json:"status"`
	Reminded     []int             `json:"reminded_min,omitempty"`
	FollowupSent bool              `json:"followup_sent,omitempty"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
}

// CreateBookingLinkInput holds the settings of a new link. Zero values take the defaults.
type CreateBookingLinkInput struct {
	Name             string
	Title            string
	Slug             string
	Description      string
	HostEmail        string
	Durations        []int
	Schedule         string
	HorizonDays      int
	StartAfter       *string
	MinNoticeMin     *int
	SlotStepMin      int
	BufferBeforeMin  int
	BufferAfterMin   int
	MaxPerDay        *int
	MaxPerWeek       *int
	MaxPerMonth      *int
	BusyTasks        bool
	LocationKind     string
	Location         string
	Questions        []BookingQuestion
	RemindersMin     []int
	FollowupAfterMin *int
	MaxUses          *int
	ExpiresAt        *string
}

var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Slugify turns a name into a URL tail.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	return b.String()
}

// Validate fills defaults and checks the input.
func (in *CreateBookingLinkInput) Validate() error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return fmt.Errorf("a booking link needs a name: %w", ErrValidation)
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		in.Title = in.Name
	}
	if in.Slug == "" {
		in.Slug = Slugify(in.Name)
	}
	if !slugRE.MatchString(in.Slug) {
		return fmt.Errorf("slug %q must be lowercase letters, digits and dashes: %w", in.Slug, ErrValidation)
	}
	if len(in.Durations) == 0 {
		in.Durations = []int{30}
	}
	for _, d := range in.Durations {
		if d <= 0 || d > 24*60 {
			return fmt.Errorf("duration %d min is out of range: %w", d, ErrValidation)
		}
	}
	if in.HorizonDays == 0 {
		in.HorizonDays = 14
	}
	if in.SlotStepMin == 0 {
		in.SlotStepMin = 30
	}
	if in.HorizonDays < 0 || in.SlotStepMin < 0 || in.BufferBeforeMin < 0 || in.BufferAfterMin < 0 {
		return fmt.Errorf("horizon, step and buffers cannot be negative: %w", ErrValidation)
	}
	if in.MinNoticeMin != nil && *in.MinNoticeMin < 0 {
		return fmt.Errorf("notice cannot be negative: %w", ErrValidation)
	}
	for _, p := range []*int{in.MaxPerDay, in.MaxPerWeek, in.MaxPerMonth, in.MaxUses} {
		if p != nil && *p <= 0 {
			return fmt.Errorf("limits must be positive: %w", ErrValidation)
		}
	}
	switch in.LocationKind {
	case "", "phone", "in_person", "url":
	default:
		return fmt.Errorf("location kind %q must be phone, in_person or url: %w", in.LocationKind, ErrValidation)
	}
	if in.LocationKind == "" && in.Location != "" {
		in.LocationKind = "url"
		if !strings.Contains(in.Location, "://") {
			in.LocationKind = "in_person"
		}
	}
	for _, m := range in.RemindersMin {
		if m <= 0 {
			return fmt.Errorf("reminders are minutes before the start and must be positive: %w", ErrValidation)
		}
	}
	in.HostEmail = strings.TrimSpace(in.HostEmail)
	if in.HostEmail != "" && !strings.Contains(in.HostEmail, "@") {
		return fmt.Errorf("host email %q is not an address: %w", in.HostEmail, ErrValidation)
	}
	return nil
}
