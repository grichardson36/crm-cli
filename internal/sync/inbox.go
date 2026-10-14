package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Inbox handles records the sync runner could not place.
type Inbox struct {
	db   *sql.DB
	repo *repo.SyncRepo
}

// NewInbox creates an Inbox.
func NewInbox(db *sql.DB) *Inbox {
	return &Inbox{db: db, repo: repo.NewSyncRepo(db)}
}

// InboxEntry is an inbox row with its payload decoded for display.
type InboxEntry struct {
	model.SyncInboxItem
	Kind       string  `json:"kind"`
	Reason     string  `json:"reason"`
	Candidates []int64 `json:"candidates,omitempty"`
	Summary    string  `json:"summary"`
}

// List returns inbox entries oldest first. An empty status means every status.
func (b *Inbox) List(ctx context.Context, status string, limit int) ([]InboxEntry, error) {
	items, err := b.repo.InboxList(ctx, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]InboxEntry, 0, len(items))
	for _, it := range items {
		out = append(out, describe(it))
	}
	return out, nil
}

// Get returns one entry.
func (b *Inbox) Get(ctx context.Context, id int64) (*InboxEntry, error) {
	it, err := b.repo.InboxGet(ctx, id)
	if err != nil {
		return nil, err
	}
	e := describe(it)
	return &e, nil
}

func describe(it *model.SyncInboxItem) InboxEntry {
	e := InboxEntry{SyncInboxItem: *it}
	var p inboxPayload
	if err := json.Unmarshal([]byte(it.Payload), &p); err != nil {
		e.Summary = "unreadable payload"
		return e
	}
	e.Kind, e.Reason, e.Candidates = p.Kind, p.Reason, p.Candidates
	switch {
	case p.Patch != nil:
		name := strDeref(p.Patch.FirstName)
		if l := strDeref(p.Patch.LastName); l != "" {
			name += " " + l
		}
		if name == "" && len(p.Patch.Emails) > 0 {
			name = p.Patch.Emails[0]
		}
		e.Summary = name
	case p.Record != nil:
		e.Summary = p.Record.Type
		if p.Record.Subject != nil {
			e.Summary += ": " + *p.Record.Subject
		}
	}
	return e
}

func (b *Inbox) pending(ctx context.Context, id int64) (*model.SyncInboxItem, *inboxPayload, error) {
	it, err := b.repo.InboxGet(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if it.Status != model.InboxPending {
		return nil, nil, fmt.Errorf("inbox item %d is already %s: %w", id, it.Status, model.ErrConflict)
	}
	var p inboxPayload
	if err := json.Unmarshal([]byte(it.Payload), &p); err != nil {
		return nil, nil, fmt.Errorf("inbox item %d has an unreadable payload: %w", id, model.ErrValidation)
	}
	return it, &p, nil
}

// Accept creates a person from a pending person record. It returns the new person id.
func (b *Inbox) Accept(ctx context.Context, id int64) (int64, error) {
	it, p, err := b.pending(ctx, id)
	if err != nil {
		return 0, err
	}
	if p.Kind != "person" || p.Patch == nil {
		return 0, fmt.Errorf("only person records can be accepted, attach interaction %d to a person instead: %w", id, model.ErrValidation)
	}
	person, err := newSink(b.db, RunOptions{AllowCreate: true}).createPerson(ctx, it.Source, it.ExternalID, *p.Patch)
	if err != nil {
		return 0, err
	}
	if err := b.repo.InboxSetStatus(ctx, id, model.InboxAccepted); err != nil {
		return 0, err
	}
	return person.ID, nil
}

// Attach links a pending record to an existing person. Person records fill that person's
// blank fields and map the external id. Interaction records are logged against the person.
func (b *Inbox) Attach(ctx context.Context, id, personID int64) error {
	it, p, err := b.pending(ctx, id)
	if err != nil {
		return err
	}
	if _, err := repo.NewPersonRepo(b.db).FindByID(ctx, personID); err != nil {
		return err
	}
	s := newSink(b.db, RunOptions{})
	switch {
	case p.Kind == "person" && p.Patch != nil:
		if _, err := s.applyPatch(ctx, personID, *p.Patch, false); err != nil {
			return err
		}
		if _, err := s.ext.Upsert(ctx, repo.ExternalIDInput{Entity: entityPerson, LocalID: personID,
			Source: it.Source, ExternalID: it.ExternalID, ETag: etagPtr(p.Patch.ETag)}); err != nil {
			return err
		}
	case p.Kind == "interaction" && p.Record != nil:
		if _, err := s.createInteraction(ctx, it.Source, it.ExternalID, *p.Record, []int64{personID}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("inbox item %d has an unknown kind %q: %w", id, p.Kind, model.ErrValidation)
	}
	return b.repo.InboxSetStatus(ctx, id, model.InboxAttached)
}

// Ignore dismisses a pending record. Later runs leave it dismissed.
func (b *Inbox) Ignore(ctx context.Context, id int64) error {
	return b.repo.InboxSetStatus(ctx, id, model.InboxIgnored)
}
