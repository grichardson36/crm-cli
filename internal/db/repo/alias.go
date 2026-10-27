package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/alias"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// AliasEntry is one line of the effective alias list.
type AliasEntry struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	Builtin   bool   `json:"builtin"`
	// Active is false for a built-in the user turned off.
	Active bool `json:"active"`
}

// AliasRepo manages the user's additions to the built-in name aliases (table name_aliases).
type AliasRepo struct{ db *sql.DB }

// NewAliasRepo creates an AliasRepo.
func NewAliasRepo(db *sql.DB) *AliasRepo { return &AliasRepo{db: db} }

type aliasRow struct{ alias, canonical, mode string }

func (r *AliasRepo) rows(ctx context.Context) ([]aliasRow, error) {
	rs, err := r.db.QueryContext(ctx, `SELECT alias, canonical, mode FROM name_aliases ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list name aliases: %w", err)
	}
	defer rs.Close()
	var out []aliasRow
	for rs.Next() {
		var x aliasRow
		if err := rs.Scan(&x.alias, &x.canonical, &x.mode); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rs.Err()
}

// Table is the effective alias table: the built-ins minus the removed ones plus the additions. A
// database without the table (older schema) gives the built-in set.
func (r *AliasRepo) Table(ctx context.Context) (*alias.Table, error) {
	rows, err := r.rows(ctx)
	if err != nil {
		return alias.Default(), err
	}
	if len(rows) == 0 {
		return alias.Default(), nil
	}
	added, removed := map[string]string{}, map[string]bool{}
	for _, x := range rows {
		if x.mode == "add" {
			added[x.alias] = x.canonical
		} else {
			removed[x.alias] = true
		}
	}
	return alias.NewTable(added, removed), nil
}

// List is the effective list sorted by alias, built-ins (active or turned off) and additions.
func (r *AliasRepo) List(ctx context.Context) ([]AliasEntry, error) {
	rows, err := r.rows(ctx)
	if err != nil {
		return nil, err
	}
	state := map[string]aliasRow{}
	for _, x := range rows {
		state[x.alias] = x
	}
	var out []AliasEntry
	for a, c := range alias.Builtin() {
		e := AliasEntry{Alias: a, Canonical: c, Builtin: true, Active: true}
		if x, ok := state[a]; ok {
			if x.mode == "remove" {
				e.Active = false
			} else {
				e.Canonical = x.canonical // the user pointed a built-in elsewhere
			}
		}
		out = append(out, e)
	}
	for _, x := range rows {
		if _, isBuiltin := alias.IsBuiltin(x.alias); isBuiltin || x.mode != "add" {
			continue
		}
		out = append(out, AliasEntry{Alias: x.alias, Canonical: x.canonical, Active: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out, nil
}

// Add makes alias a nickname of canonical. It returns whether anything changed. An alias that is
// already a nickname of that name is a no-op. A name that others use as their full name cannot
// become a nickname, and the pair must be two different single words.
func (r *AliasRepo) Add(ctx context.Context, name, canonical string) (bool, error) {
	a, c := alias.Key(name), alias.Key(canonical)
	if a == "" || c == "" {
		return false, fmt.Errorf("an alias and its full name are each one word of letters: %w", model.ErrValidation)
	}
	if a == c {
		return false, fmt.Errorf("%q cannot be an alias of itself: %w", a, model.ErrValidation)
	}
	tbl, err := r.Table(ctx)
	if err != nil {
		return false, err
	}
	if root := tbl.Canonical(c); root != c {
		c = root // point at the full name, never at another nickname
	}
	for _, p := range tbl.Names() {
		if p[1] == a {
			return false, fmt.Errorf("%q is the full name of %q, it cannot also be a nickname: %w", a, p[0], model.ErrConflict)
		}
	}
	if tbl.Canonical(a) == c && a != c {
		if _, active := activeAlias(tbl, a); active {
			return false, nil
		}
	}
	if built, ok := alias.IsBuiltin(a); ok && built == c {
		_, err := r.db.ExecContext(ctx, `DELETE FROM name_aliases WHERE alias = ?`, a)
		return err == nil, err
	}
	return true, r.upsert(ctx, a, c, "add")
}

func activeAlias(t *alias.Table, a string) (string, bool) {
	c := t.Canonical(a)
	return c, c != a
}

// Remove stops a nickname. An addition is deleted, a built-in is turned off. ErrNotFound when the
// word is not an active alias.
func (r *AliasRepo) Remove(ctx context.Context, name string) error {
	a := alias.Key(name)
	tbl, err := r.Table(ctx)
	if err != nil {
		return err
	}
	if _, active := activeAlias(tbl, a); a == "" || !active {
		return fmt.Errorf("%q is not an alias: %w", name, model.ErrNotFound)
	}
	if built, isBuiltin := alias.IsBuiltin(a); isBuiltin {
		return r.upsert(ctx, a, built, "remove")
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM name_aliases WHERE alias = ?`, a)
	return err
}

func (r *AliasRepo) upsert(ctx context.Context, a, c, mode string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO name_aliases (uuid, alias, canonical, mode) VALUES (?, ?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET canonical = excluded.canonical, mode = excluded.mode, updated_at = datetime('now')`,
		uuid.NewString(), a, c, mode)
	if err != nil {
		return fmt.Errorf("save name alias: %w", err)
	}
	return nil
}

// PersonFTSQuery builds the people_fts MATCH text for a search. Each word is expanded through the
// alias table (Zach also matches Zachary, Rich also matches Richard). When no word has an alias the
// text is the single quoted prefix phrase search always used, so plain queries behave as before.
// Otherwise the words are ANDed and each one is the OR of its names.
func PersonFTSQuery(query string, tbl *alias.Table) string {
	quote := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` + "*" }
	words := strings.Fields(query)
	expanded := false
	groups := make([][]string, len(words))
	for i, w := range words {
		groups[i] = tbl.Expand(w)
		if len(groups[i]) > 1 {
			expanded = true
		}
		groups[i][0] = w // keep the typed spelling
	}
	if !expanded {
		return quote(query)
	}
	parts := make([]string, len(groups))
	for i, g := range groups {
		qs := make([]string, len(g))
		for j, n := range g {
			qs[j] = quote(n)
		}
		parts[i] = "(" + strings.Join(qs, " OR ") + ")"
	}
	return strings.Join(parts, " AND ")
}
