package model

// Person represents a contact in the CRM.
type Person struct {
	ID        int64   `json:"id"`
	UUID      string  `json:"uuid"`
	FirstName string  `json:"first_name"`
	LastName  *string `json:"last_name"`
	Email     *string `json:"email"`
	Phone     *string `json:"phone"`
	Title     *string `json:"title"`
	Company   *string `json:"company"`
	Location  *string `json:"location"`
	Notes     *string `json:"notes"`
	Summary   *string `json:"summary"`
	OrgID     *int64  `json:"org_id"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`

	BirthdayMonth           *int    `json:"birthday_month"`
	BirthdayDay             *int    `json:"birthday_day"`
	BirthdayYear            *int    `json:"birthday_year"`
	KeepInTouch             *string `json:"keep_in_touch"`
	KeepInTouchSnoozedUntil *string `json:"keep_in_touch_snoozed_until"`
	LastContactedAt         *string `json:"last_contacted_at"`
	IsFavorite              bool    `json:"is_favorite"`
	PriorityTier            *int    `json:"priority_tier"`

	// Filled only by PersonRepo.LoadDetails.
	Emails  []PersonEmail  `json:"emails,omitempty"`
	Phones  []PersonPhone  `json:"phones,omitempty"`
	Handles []PersonHandle `json:"handles,omitempty"`
}

// CreatePersonInput holds fields for creating a person.
type CreatePersonInput struct {
	FirstName string
	LastName  *string
	Email     *string
	Phone     *string
	Title     *string
	Company   *string
	Location  *string
	Notes     *string
	OrgID     *int64
}

// UpdatePersonInput holds optional fields for updating a person.
// Pointer fields: nil = don't change, non-nil = set to this value.
type UpdatePersonInput struct {
	FirstName *string
	LastName  *string
	Email     *string
	Phone     *string
	Title     *string
	Company   *string
	Location  *string
	Notes     *string
	Summary   *string
	OrgID     *int64

	// Birthday is "YYYY-MM-DD" or "--MM-DD"; an empty string clears it.
	Birthday *string
	// KeepInTouch is one of KeepInTouchCadences; an empty string clears it.
	KeepInTouch *string
	// SnoozedUntil hides the person from the due list until then; an empty string clears it.
	SnoozedUntil    *string
	LastContactedAt *string
	IsFavorite      *bool
	// PriorityTier 0 clears the tier.
	PriorityTier *int
}

// PersonFilters holds optional filters for listing people.
type PersonFilters struct {
	Tag   *string
	OrgID *int64
	Limit int
	// Archived lists archived people instead of active ones.
	Archived bool
}

// NoteType is a user-defined interaction type (call, email, standup, ...).
type NoteType struct {
	ID              int64  `json:"id"`
	UUID            string `json:"uuid"`
	Name            string `json:"name"`
	CountsAsContact bool   `json:"counts_as_contact"`
	Archived        bool   `json:"archived"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// Organization represents a company or group.
type Organization struct {
	ID        int64   `json:"id"`
	UUID      string  `json:"uuid"`
	Name      string  `json:"name"`
	Domain    *string `json:"domain"`
	Industry  *string `json:"industry"`
	Notes     *string `json:"notes"`
	Summary   *string `json:"summary"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// CreateOrgInput holds fields for creating an organization.
type CreateOrgInput struct {
	Name     string
	Domain   *string
	Industry *string
	Notes    *string
}

// UpdateOrgInput holds optional fields for updating an organization.
type UpdateOrgInput struct {
	Name     *string
	Domain   *string
	Industry *string
	Notes    *string
	Summary  *string
}

// Interaction represents an activity log entry.
type Interaction struct {
	ID         int64   `json:"id"`
	UUID       string  `json:"uuid"`
	Type       string  `json:"type"`
	Subject    *string `json:"subject"`
	Content    *string `json:"content"`
	Direction  *string `json:"direction"`
	OccurredAt string  `json:"occurred_at"`
	PersonIDs  []int64 `json:"person_ids"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// CreateInteractionInput holds fields for creating an interaction.
type CreateInteractionInput struct {
	Type       string
	Subject    *string
	Content    *string
	Direction  *string
	OccurredAt *string
	PersonIDs  []int64
}

// InteractionFilters holds optional filters for listing interactions.
type InteractionFilters struct {
	PersonID *int64
	Type     *string
	Limit    int
}

// Deal represents a sales opportunity.
type Deal struct {
	ID        int64    `json:"id"`
	UUID      string   `json:"uuid"`
	Title     string   `json:"title"`
	Value     *float64 `json:"value"`
	Stage     string   `json:"stage"`
	PersonID  *int64   `json:"person_id"`
	OrgID     *int64   `json:"org_id"`
	Notes     *string  `json:"notes"`
	ClosedAt  *string  `json:"closed_at"`
	Archived  bool     `json:"-"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// CreateDealInput holds fields for creating a deal.
type CreateDealInput struct {
	Title    string
	Value    *float64
	Stage    string
	PersonID *int64
	OrgID    *int64
	Notes    *string
}

// UpdateDealInput holds optional fields for updating a deal.
type UpdateDealInput struct {
	Title    *string
	Value    *float64
	Stage    *string
	PersonID *int64
	OrgID    *int64
	Notes    *string
	ClosedAt *string
}

// DealFilters holds optional filters for listing deals.
type DealFilters struct {
	Stage         *string
	PersonID      *int64
	OrgID         *int64
	ExcludeClosed bool
	Limit         int
}

// PipelineStage represents a summary of deals in a stage.
type PipelineStage struct {
	Stage      string  `json:"stage"`
	Count      int     `json:"count"`
	TotalValue float64 `json:"total_value"`
}

// Tag represents a label that can be applied to any entity.
type Tag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Task represents a follow-up or to-do item.
type Task struct {
	Recurrence  *string `json:"recurrence"`
	PersonIDs   []int64 `json:"person_ids,omitempty"`
	NextTaskID  *int64  `json:"next_task_id,omitempty"`
	ID          int64   `json:"id"`
	UUID        string  `json:"uuid"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	PersonID    *int64  `json:"person_id"`
	DealID      *int64  `json:"deal_id"`
	DueAt       *string `json:"due_at"`
	Priority    string  `json:"priority"`
	Completed   bool    `json:"completed"`
	CompletedAt *string `json:"completed_at"`
	Archived    bool    `json:"-"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`

	// Scheduling fields. DueAt stays the soft target; DeadlineAt is the date the scheduler plans against.
	DurationMin     *int    `json:"duration_min"`
	EarliestStart   *string `json:"earliest_start"`
	DeadlineAt      *string `json:"deadline_at"`
	DeadlineHard    bool    `json:"deadline_hard"`
	MinChunkMin     *int    `json:"min_chunk_min"`
	AutoSchedule    bool    `json:"auto_schedule"`
	ScheduledStart  *string `json:"scheduled_start"`
	ScheduledLocked bool    `json:"scheduled_locked"`
	ScheduleName    string  `json:"schedule_name"`
}

// TaskSchedule holds the optional scheduling fields of a task for create and update. Nil means
// unchanged. For numbers and text, 0 and the empty string clear the value.
type TaskSchedule struct {
	DurationMin    *int
	EarliestStart  *string
	DeadlineAt     *string
	DeadlineHard   *bool
	MinChunkMin    *int
	AutoSchedule   *bool
	ScheduledStart *string // setting a time by hand also locks it, unless ScheduledLocked says otherwise
	// ScheduledLocked keeps the scheduler away from the placed time.
	ScheduledLocked *bool
	ScheduleName    *string
}

// CreateTaskInput holds fields for creating a task.
type CreateTaskInput struct {
	Title       string
	Description *string
	PersonID    *int64
	DealID      *int64
	DueAt       *string
	Priority    string
	// PersonIDs links the task to several people. PersonID is added to it.
	PersonIDs  []int64
	Recurrence *string
	Schedule   TaskSchedule
}

// UpdateTaskInput holds optional fields for updating a task.
type UpdateTaskInput struct {
	Title       *string
	Description *string
	PersonID    *int64
	DealID      *int64
	DueAt       *string
	Priority    *string
	Recurrence  *string
	Schedule    TaskSchedule
}

// TaskFilters holds optional filters for listing tasks.
type TaskFilters struct {
	PersonID         *int64
	DealID           *int64
	Overdue          bool
	IncludeCompleted bool
	Limit            int
}

// Relationship represents a person-to-person link.
type Relationship struct {
	ID              int64   `json:"id"`
	PersonID        int64   `json:"person_id"`
	RelatedPersonID int64   `json:"related_person_id"`
	Type            string  `json:"type"`
	Notes           *string `json:"notes"`
	CreatedAt       string  `json:"created_at"`
}

// Valid entity types for tagging.
var EntityTypes = []string{"person", "organization", "deal", "interaction", "event"}

// ValidEntityType checks if the given type is valid.
func ValidEntityType(t string) bool {
	for _, et := range EntityTypes {
		if et == t {
			return true
		}
	}
	return false
}

// InteractionTypes lists the seed note types. The note_types table is the source of truth.
var InteractionTypes = []string{"call", "email", "meeting", "note", "message"}

// ValidInteractionType checks if the given type is valid.
func ValidInteractionType(t string) bool {
	for _, it := range InteractionTypes {
		if it == t {
			return true
		}
	}
	return false
}

// InteractionDirections lists valid interaction directions.
var InteractionDirections = []string{"inbound", "outbound"}

// ValidInteractionDirection checks if the given direction is valid.
func ValidInteractionDirection(d string) bool {
	for _, id := range InteractionDirections {
		if id == d {
			return true
		}
	}
	return false
}

// Deal stages
var DealStages = []string{"lead", "prospect", "proposal", "negotiation", "won", "lost"}

// ValidDealStage checks if the given stage is valid.
func ValidDealStage(s string) bool {
	for _, ds := range DealStages {
		if ds == s {
			return true
		}
	}
	return false
}

// Priority levels
var Priorities = []string{"low", "medium", "high", "asap"}

// ValidPriority checks if the given priority is valid.
func ValidPriority(p string) bool {
	for _, pr := range Priorities {
		if pr == p {
			return true
		}
	}
	return false
}

// Relationship types
var RelationshipTypes = []string{"colleague", "friend", "manager", "mentor", "referred-by"}

// ValidRelationshipType checks if the given type is valid.
func ValidRelationshipType(t string) bool {
	for _, rt := range RelationshipTypes {
		if rt == t {
			return true
		}
	}
	return false
}
