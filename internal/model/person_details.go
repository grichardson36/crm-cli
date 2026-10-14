package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// KeepInTouchCadences lists valid keep-in-touch cadences. They match Dex's values.
var KeepInTouchCadences = []string{"7 days", "14 days", "1 mon", "42 days", "3 mons", "6 mons", "1 year", "never"}

// ValidCadence checks if the given keep-in-touch cadence is valid.
func ValidCadence(c string) bool {
	for _, v := range KeepInTouchCadences {
		if v == c {
			return true
		}
	}
	return false
}

// HandleKinds lists valid social handle kinds.
var HandleKinds = []string{"linkedin", "twitter", "facebook", "instagram", "telegram", "whatsapp", "tiktok", "youtube", "website", "other"}

// ValidHandleKind checks if the given handle kind is valid.
func ValidHandleKind(k string) bool {
	for _, v := range HandleKinds {
		if v == k {
			return true
		}
	}
	return false
}

// PersonEmail is one email address of a person.
type PersonEmail struct {
	Email     string  `json:"email"`
	Label     *string `json:"label"`
	IsPrimary bool    `json:"is_primary"`
}

// PersonPhone is one phone number of a person.
type PersonPhone struct {
	PhoneNumber string  `json:"phone_number"`
	Label       *string `json:"label"`
	CountryCode *string `json:"country_code"`
	IsPrimary   bool    `json:"is_primary"`
}

// PersonHandle is a social profile or website of a person.
type PersonHandle struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// DueContact is a person whose keep-in-touch cadence says it is time to reach out.
type DueContact struct {
	PersonID    int64   `json:"person_id"`
	Name        string  `json:"name"`
	KeepInTouch string  `json:"keep_in_touch"`
	LastSeen    *string `json:"last_seen"`
	DueAt       string  `json:"due_at"`
	DaysOverdue int     `json:"days_overdue"`
}

// UpcomingBirthday is a person's next birthday.
type UpcomingBirthday struct {
	PersonID  int64  `json:"person_id"`
	Name      string `json:"name"`
	Date      string `json:"date"`
	DaysUntil int    `json:"days_until"`
	Turning   *int   `json:"turning"`
}

// ParseBirthday parses "YYYY-MM-DD", "--MM-DD", or "MM-DD". The year is nil when unknown.
func ParseBirthday(s string) (month, day int, year *int, err error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(strings.TrimPrefix(s, "--"), "-")
	var m, d string
	switch len(parts) {
	case 2:
		m, d = parts[0], parts[1]
	case 3:
		m, d = parts[1], parts[2]
		y, yerr := strconv.Atoi(parts[0])
		if yerr != nil || y < 1 || y > 9999 {
			return 0, 0, nil, fmt.Errorf("invalid birthday year in %q: %w", s, ErrValidation)
		}
		year = &y
	default:
		return 0, 0, nil, fmt.Errorf("invalid birthday %q (use YYYY-MM-DD or --MM-DD): %w", s, ErrValidation)
	}
	month, merr := strconv.Atoi(m)
	day, derr := strconv.Atoi(d)
	if merr != nil || derr != nil || month < 1 || month > 12 || day < 1 {
		return 0, 0, nil, fmt.Errorf("invalid birthday %q (use YYYY-MM-DD or --MM-DD): %w", s, ErrValidation)
	}
	// Check the day against a leap year so 02-29 is allowed without a year.
	if day > time.Date(2000, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() {
		return 0, 0, nil, fmt.Errorf("invalid birthday %q: day out of range: %w", s, ErrValidation)
	}
	return month, day, year, nil
}

// PersonSummary is a lightweight row for lists and search-as-you-type.
type PersonSummary struct {
	ID          int64
	Name        string
	Company     string
	Title       string
	Email       string
	AllEmails   string
	Location    string
	Tags        string
	IsFavorite  bool
	KeepInTouch string
	UpdatedAt   string
}
