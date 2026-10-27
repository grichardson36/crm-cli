package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// PurgeKinds are the object kinds that can be archived and so purged. Purge is a verb of every
// object type (patterns/one-abstraction-many-contexts), not of people only.
var PurgeKinds = []string{"person", "organization", "deal", "task", "interaction"}

// IsPurgeKind says whether kind is an object kind with a purge.
func IsPurgeKind(kind string) bool {
	for _, k := range PurgeKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// PurgeArchivedOptions selects what a bulk purge removes. Kind limits it to one object kind ("" is every
// kind), IDs limits it to those archived objects (empty means every archived object of the kind) and
// needs a Kind because ids are per kind. Apply false only reports.
type PurgeArchivedOptions struct {
	Kind  string
	IDs   []int64
	Apply bool
}

// PurgeSkip is a requested id that was not purged and why.
type PurgeSkip struct {
	Kind   string `json:"kind,omitempty"`
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
}

// PurgeItem is one object in a bulk purge.
type PurgeItem struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// PurgeArchivedResult is the plan (Apply false) or the outcome (Apply true) of a bulk purge.
type PurgeArchivedResult struct {
	Applied bool        `json:"applied"`
	Count   int         `json:"count"`
	Items   []PurgeItem `json:"items"`
	Skipped []PurgeSkip `json:"skipped,omitempty"`
	Purged  int         `json:"purged"`
	Errors  []PurgeSkip `json:"errors,omitempty"`
}

// NormalizePurgeKind maps the words users type (people, orgs, notes) to an object kind.
func NormalizePurgeKind(s string) string {
	switch s {
	case "people", "persons":
		return "person"
	case "org", "orgs", "organizations":
		return "organization"
	case "deals":
		return "deal"
	case "tasks":
		return "task"
	case "note", "notes", "interactions":
		return "interaction"
	}
	return s
}

// PurgeArchived permanently deletes archived objects, each one in its own transaction exactly as
// PurgeObject does (dependents removed, a tombstone recorded for every deleted synced row). Without
// Apply nothing is written and the result lists what would go. An object that fails stays and is
// listed in Errors, the rest still purge.
func PurgeArchived(ctx context.Context, db *sql.DB, opts PurgeArchivedOptions) (PurgeArchivedResult, error) {
	res := PurgeArchivedResult{Applied: opts.Apply}
	kind := NormalizePurgeKind(opts.Kind)
	if kind != "" && !IsPurgeKind(kind) {
		return res, fmt.Errorf("unknown type %q, use one of person, organization, deal, task, note: %w", opts.Kind, model.ErrValidation)
	}
	if len(opts.IDs) > 0 && kind == "" {
		return res, fmt.Errorf("ids need a type, ids are per type: %w", model.ErrValidation)
	}
	items, err := ListArchived(ctx, db, kind)
	if err != nil {
		return res, err
	}
	byKey := map[string]string{}
	var all []PurgeItem
	for _, it := range items {
		byKey[fmt.Sprintf("%s:%d", it.Kind, it.ID)] = it.Name
		all = append(all, PurgeItem{Kind: it.Kind, ID: it.ID, Name: it.Name})
	}
	want := all
	if len(opts.IDs) > 0 {
		want = nil
		seen := map[int64]bool{}
		for _, id := range opts.IDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			name, ok := byKey[fmt.Sprintf("%s:%d", kind, id)]
			if !ok {
				res.Skipped = append(res.Skipped, PurgeSkip{Kind: kind, ID: id, Reason: "not an archived " + kind})
				continue
			}
			want = append(want, PurgeItem{Kind: kind, ID: id, Name: name})
		}
	}
	res.Items = want
	res.Count = len(want)
	if !opts.Apply {
		return res, nil
	}
	for _, it := range want {
		if err := PurgeObject(ctx, db, it.Kind, it.ID); err != nil {
			res.Errors = append(res.Errors, PurgeSkip{Kind: it.Kind, ID: it.ID, Reason: err.Error()})
			continue
		}
		res.Purged++
	}
	return res, nil
}

// PurgeObject permanently deletes one archived object and its dependent rows in one transaction. A
// tombstone is recorded for every deleted synced row. An object that is not archived is refused.
//
// Dependents by kind. Organizations: memberships, organization links, deal links, task and note
// links, taggings and custom fields. Deals: person and organization links, task and note links,
// taggings and custom fields. Tasks and notes: their links and taggings. People: see PersonRepo.Purge.
func PurgeObject(ctx context.Context, db *sql.DB, kind string, id int64) error {
	kind = NormalizePurgeKind(kind)
	if kind == "person" {
		return NewPersonRepo(db).Purge(ctx, id)
	}
	table, ok := map[string]string{"organization": "organizations", "deal": "deals", "task": "tasks", "interaction": "interactions"}[kind]
	if !ok {
		return fmt.Errorf("unknown object %q: %w", kind, model.ErrValidation)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var archived int
	err = tx.QueryRowContext(ctx, "SELECT archived FROM "+table+" WHERE id = ?", id).Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s %d: %w", kind, id, model.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("find %s %d: %w", kind, id, err)
	}
	if archived == 0 {
		return fmt.Errorf("%s %d is not archived, archive it first: %w", kind, id, model.ErrConflict)
	}
	device, err := DeviceID(ctx, tx)
	if err != nil {
		return err
	}
	tombstone := func(tbl, where string, args ...any) error {
		_, err := tx.ExecContext(ctx,
			"INSERT OR REPLACE INTO tombstones (table_name, uuid, device_id) SELECT '"+tbl+"', uuid, ? FROM "+tbl+" WHERE "+where,
			append([]any{device}, args...)...)
		if err != nil {
			return fmt.Errorf("tombstone %s: %w", tbl, err)
		}
		return nil
	}
	exec := func(query string, args ...any) error {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("purge %s %d: %w", kind, id, err)
		}
		return nil
	}
	type step struct {
		table, where string
		twice        bool
	}
	var links []step
	switch kind {
	case "organization":
		links = []step{{"membership", "org_id = ?", false}, {"org_relation", "a_id = ? OR b_id = ?", true}, {"deal_org", "org_id = ?", false},
			{"activity_target", "target_kind = 'organization' AND target_id = ?", false}}
	case "deal":
		links = []step{{"deal_person", "deal_id = ?", false}, {"deal_org", "deal_id = ?", false},
			{"activity_target", "target_kind = 'deal' AND target_id = ?", false}}
	case "task":
		links = []step{{"activity_target", "source_kind = 'task' AND source_id = ?", false}}
	case "interaction":
		links = []step{{"activity_target", "source_kind = 'interaction' AND source_id = ?", false}}
	}
	for _, l := range links {
		args := []any{id}
		if l.twice {
			args = []any{id, id}
		}
		if err := tombstone(l.table, l.where, args...); err != nil {
			return err
		}
	}
	if err := tombstone("taggings", "entity_type = ? AND entity_id = ?", kind, id); err != nil {
		return err
	}
	if err := exec("DELETE FROM taggings WHERE entity_type = ? AND entity_id = ?", kind, id); err != nil {
		return err
	}
	if err := exec("DELETE FROM custom_fields WHERE entity_type = ? AND entity_id = ?", kind, id); err != nil {
		return err
	}
	if err := exec("DELETE FROM external_ids WHERE entity = ? AND local_id = ?", kind, id); err != nil {
		return err
	}
	if err := exec("DELETE FROM field_versions WHERE entity = ? AND local_id = ?", kind, id); err != nil {
		return err
	}
	if kind == "organization" {
		if err := exec("DELETE FROM merge_alias WHERE kind = 'organization' AND (old_id = ? OR survivor_id = ?)", id, id); err != nil {
			return err
		}
	}
	if err := tombstone(table, "id = ?", id); err != nil {
		return err
	}
	if err := exec("DELETE FROM "+table+" WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}
