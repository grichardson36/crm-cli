package sync

import (
	"context"
	"database/sql"
)

// FindCandidates returns the ids of active people that share an email address, phone number or
// social handle with the given identities, using the same normalization as sync runs. The ids
// are sorted. It loads the index fresh on every call, so use it for occasional lookups.
func FindCandidates(ctx context.Context, db *sql.DB, emails, phones []string, handles map[string]string) ([]int64, error) {
	idx := &matchIndex{db: db}
	return idx.candidates(ctx, emails, phones, handles)
}
