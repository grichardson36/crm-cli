package sync

import (
	"context"
	"database/sql"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Overview is the data behind `crm sync status`.
type Overview struct {
	Sources      []Status `json:"sources"`
	InboxPending int      `json:"inbox_pending"`
}

// Overview returns every source's status and the pending inbox count.
func (r *Runner) Overview(ctx context.Context) (*Overview, error) {
	sources, err := r.Statuses(ctx)
	if err != nil {
		return nil, err
	}
	n, err := repo.NewSyncRepo(r.db).InboxCount(ctx, model.InboxPending)
	if err != nil {
		return nil, err
	}
	return &Overview{Sources: sources, InboxPending: n}, nil
}

// LastSync summarizes the newest finished run for `crm status`.
type LastSync struct {
	Source       string `json:"source"`
	Status       string `json:"status"`
	FinishedAt   string `json:"finished_at"`
	Created      int    `json:"created"`
	Updated      int    `json:"updated"`
	Skipped      int    `json:"skipped"`
	Errors       int    `json:"errors"`
	InboxPending int    `json:"inbox_pending"`
}

// LastSyncSummary returns the newest run across sources, or nil when nothing ever ran.
func LastSyncSummary(ctx context.Context, db *sql.DB) (*LastSync, error) {
	sr := repo.NewSyncRepo(db)
	run, err := sr.LastRun(ctx, "")
	if err != nil || run == nil {
		return nil, err
	}
	pending, err := sr.InboxCount(ctx, model.InboxPending)
	if err != nil {
		return nil, err
	}
	finished := run.StartedAt
	if run.FinishedAt != nil {
		finished = *run.FinishedAt
	}
	return &LastSync{Source: run.Source, Status: run.Status, FinishedAt: finished,
		Created: run.Created, Updated: run.Updated, Skipped: run.Skipped, Errors: run.Errors, InboxPending: pending}, nil
}
