package book

import (
	"fmt"
	"time"

	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/cal/when"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// LinkSpec is a booking link as typed: lengths, notice and the like in text, numbers where 0
// means "not set". The CLI, MCP and TUI all build links from it.
type LinkSpec struct {
	Name, Title, Slug, Description, Email string
	Lens, Rule, Notice, Followup          string
	From, Expires                         string
	Where, WhereKind                      string
	Horizon, Step                         int
	Before, After, Both                   int
	MaxDay, MaxWeek, MaxMonth, Uses       int
	BusyTasks                             bool
	Questions, Reminds                    []string
}

func minutesOf(flag, text string) (*int, error) {
	if text == "" {
		return nil, nil
	}
	d, err := when.ParseLength(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %v: %w", flag, err, model.ErrValidation)
	}
	n := int(d / time.Minute)
	return &n, nil
}

func intPtr(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// Input converts the spec to a validated create input. Times without a zone are read in loc.
func (s LinkSpec) Input(now time.Time, loc *time.Location) (model.CreateBookingLinkInput, error) {
	in := model.CreateBookingLinkInput{
		Name: s.Name, Title: s.Title, Slug: s.Slug, Description: s.Description, HostEmail: s.Email, Schedule: s.Rule,
		HorizonDays: s.Horizon, SlotStepMin: s.Step, BufferBeforeMin: s.Before, BufferAfterMin: s.After, BusyTasks: s.BusyTasks,
		Location: s.Where, LocationKind: s.WhereKind, Questions: Questions(s.Questions),
		MaxPerDay: intPtr(s.MaxDay), MaxPerWeek: intPtr(s.MaxWeek), MaxPerMonth: intPtr(s.MaxMonth), MaxUses: intPtr(s.Uses),
	}
	var err error
	if s.Lens != "" {
		if in.Durations, err = Lengths(s.Lens); err != nil {
			return in, fmt.Errorf("len: %v: %w", err, model.ErrValidation)
		}
	}
	if in.RemindersMin, err = Reminders(s.Reminds); err != nil {
		return in, fmt.Errorf("remind: %v: %w", err, model.ErrValidation)
	}
	if s.Both != 0 {
		in.BufferBeforeMin, in.BufferAfterMin = s.Both, s.Both
	}
	if in.MinNoticeMin, err = minutesOf("notice", s.Notice); err != nil {
		return in, err
	}
	if in.FollowupAfterMin, err = minutesOf("followup", s.Followup); err != nil {
		return in, err
	}
	if s.From != "" {
		if _, err := tz.ParseDate(s.From); err != nil {
			return in, fmt.Errorf("from needs a date like 2026-11-02: %w", model.ErrValidation)
		}
		from := s.From
		in.StartAfter = &from
	}
	if s.Expires != "" {
		r, err := when.Parse(s.Expires, now, loc)
		if err != nil {
			return in, fmt.Errorf("expires: %v: %w", err, model.ErrValidation)
		}
		e := tz.FormatInstant(r.Start)
		in.ExpiresAt = &e
	}
	return in, in.Validate()
}
