package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// FakeRecord is one scripted record for the fake adapter.
type FakeRecord struct {
	Seq         int                `json:"seq"`
	Kind        string             `json:"kind"` // "person" or "interaction"
	ExternalID  string             `json:"external_id"`
	Person      *PersonPatch       `json:"person,omitempty"`
	Interaction *InteractionRecord `json:"interaction,omitempty"`
}

// FakeAdapter replays scripted records. It exists for tests only and talks to no network.
// It is registered by DefaultRegistry only when CRM_SYNC_FAKE_FILE points at a JSON array of
// FakeRecord. The cursor is the highest seq already delivered.
type FakeAdapter struct {
	Path    string
	Records []FakeRecord
	// FailAfter stops Pull with an error after that many records when greater than zero.
	FailAfter int
}

// Name implements Adapter.
func (f *FakeAdapter) Name() string { return "fake" }

// Pull implements Adapter.
func (f *FakeAdapter) Pull(ctx context.Context, since string, sink Sink) (string, error) {
	records := f.Records
	if f.Path != "" {
		raw, err := os.ReadFile(f.Path)
		if err != nil {
			return since, fmt.Errorf("read fake records: %w", err)
		}
		if err := json.Unmarshal(raw, &records); err != nil {
			return since, fmt.Errorf("parse fake records: %w", err)
		}
	}
	after, _ := strconv.Atoi(since)
	next, sent := after, 0
	for _, r := range records {
		if r.Seq <= after {
			continue
		}
		if err := ctx.Err(); err != nil {
			return since, err
		}
		if f.FailAfter > 0 && sent >= f.FailAfter {
			return since, fmt.Errorf("fake adapter: simulated failure after %d records", sent)
		}
		sent++
		// Sink errors are counted by the sink. The adapter keeps going so one bad record
		// does not hide the rest.
		switch r.Kind {
		case "person":
			if r.Person != nil {
				_, _ = sink.UpsertPerson(ctx, f.Name(), r.ExternalID, *r.Person)
			}
		case "interaction":
			if r.Interaction != nil {
				_, _ = sink.LogInteraction(ctx, f.Name(), r.ExternalID, *r.Interaction)
			}
		}
		if r.Seq > next {
			next = r.Seq
		}
	}
	return strconv.Itoa(next), nil
}
