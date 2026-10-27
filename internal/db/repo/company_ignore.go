package repo

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Company strings that never become an organization (owner decision 2026-10-27). The defaults below
// are built in. The user's changes live in the company_ignore table: mode 'add' ignores one more
// string, mode 'remove' turns a built-in off. A text with no letter or digit ("-", "...") is always
// ignored, it cannot name an organization.

// builtinIgnore are the default junk company strings.
var builtinIgnore = []string{
	"n/a", "na", "none", "no", "nil", "null", "-", "--", "unknown", "tbd", "not applicable", "no company",
	"self-employed", "self employed", "selfemployed", "self", "retired", "unemployed", "student",
	"freelance", "freelancer", "independent", "homemaker", "between jobs",
}

// IgnoreKey is the form an ignore entry is stored and compared in: NormalizeCompany, or the trimmed
// lower-case text when that is empty.
func IgnoreKey(text string) string {
	if n := NormalizeCompany(text); n != "" {
		return n
	}
	return strings.ToLower(strings.TrimSpace(text))
}

// IgnoreEntry is one line of the effective ignore list.
type IgnoreEntry struct {
	Text    string `json:"text"`
	Key     string `json:"key"`
	Builtin bool   `json:"builtin"`
	// Active is false for a built-in the user turned off.
	Active bool `json:"active"`
}

// IgnoreRepo manages the ignore list.
type IgnoreRepo struct{ db *sql.DB }

// NewIgnoreRepo creates an IgnoreRepo.
func NewIgnoreRepo(db *sql.DB) *IgnoreRepo { return &IgnoreRepo{db: db} }

func builtinKeys() map[string]string {
	out := map[string]string{}
	for _, t := range builtinIgnore {
		k := IgnoreKey(t)
		if _, dup := out[k]; !dup {
			out[k] = t
		}
	}
	return out
}

type ignoreRow struct{ key, text, mode string }

func (r *IgnoreRepo) rows(ctx context.Context) ([]ignoreRow, error) {
	rs, err := r.db.QueryContext(ctx, `SELECT key, text, mode FROM company_ignore ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list ignored company texts: %w", err)
	}
	defer rs.Close()
	var out []ignoreRow
	for rs.Next() {
		var x ignoreRow
		if err := rs.Scan(&x.key, &x.text, &x.mode); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rs.Err()
}

// Set returns the active ignore keys: the built-ins minus the removed ones, plus the added ones.
func (r *IgnoreRepo) Set(ctx context.Context) (map[string]bool, error) {
	set := map[string]bool{}
	for k := range builtinKeys() {
		set[k] = true
	}
	rows, err := r.rows(ctx)
	if err != nil {
		return nil, err
	}
	for _, x := range rows {
		set[x.key] = x.mode == "add"
	}
	return set, nil
}

// List is the effective list, sorted by text: built-ins (active or turned off) and the user's additions.
func (r *IgnoreRepo) List(ctx context.Context) ([]IgnoreEntry, error) {
	built := builtinKeys()
	rows, err := r.rows(ctx)
	if err != nil {
		return nil, err
	}
	state := map[string]ignoreRow{}
	for _, x := range rows {
		state[x.key] = x
	}
	var out []IgnoreEntry
	for k, t := range built {
		active := true
		if x, ok := state[k]; ok && x.mode == "remove" {
			active = false
		}
		out = append(out, IgnoreEntry{Text: t, Key: k, Builtin: true, Active: active})
	}
	for _, x := range rows {
		if _, isBuiltin := built[x.key]; isBuiltin || x.mode != "add" {
			continue
		}
		out = append(out, IgnoreEntry{Text: x.text, Key: x.key, Active: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Ignored reports whether the text must not create an organization.
func (r *IgnoreRepo) Ignored(ctx context.Context, text string) (bool, error) {
	set, err := r.Set(ctx)
	if err != nil {
		return false, err
	}
	return ignoredBy(set, text), nil
}

func ignoredBy(set map[string]bool, text string) bool {
	if NormalizeCompany(text) == "" {
		return true
	}
	return set[IgnoreKey(text)]
}

// Add ignores a text. A built-in that was turned off comes back on. It is a no-op when the text is
// already ignored. It returns whether anything changed.
func (r *IgnoreRepo) Add(ctx context.Context, text string) (bool, error) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return false, fmt.Errorf("the text to ignore is empty: %w", model.ErrValidation)
	}
	key := IgnoreKey(text)
	set, err := r.Set(ctx)
	if err != nil {
		return false, err
	}
	if set[key] {
		return false, nil
	}
	if _, isBuiltin := builtinKeys()[key]; isBuiltin {
		_, err := r.db.ExecContext(ctx, `DELETE FROM company_ignore WHERE key = ?`, key)
		return err == nil, err
	}
	return r.upsert(ctx, key, text, "add")
}

// Remove stops ignoring a text. A user addition is deleted, a built-in is turned off. It returns
// ErrNotFound when the text is not on the list.
func (r *IgnoreRepo) Remove(ctx context.Context, text string) error {
	key := IgnoreKey(text)
	set, err := r.Set(ctx)
	if err != nil {
		return err
	}
	if !set[key] {
		return fmt.Errorf("%q is not on the ignore list: %w", strings.TrimSpace(text), model.ErrNotFound)
	}
	if built, isBuiltin := builtinKeys()[key]; isBuiltin {
		_, err := r.upsert(ctx, key, built, "remove")
		return err
	}
	_, err = r.db.ExecContext(ctx, `DELETE FROM company_ignore WHERE key = ?`, key)
	return err
}

func (r *IgnoreRepo) upsert(ctx context.Context, key, text, mode string) (bool, error) {
	_, err := r.db.ExecContext(ctx, `INSERT INTO company_ignore (uuid, key, text, mode) VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET text = excluded.text, mode = excluded.mode, updated_at = datetime('now')`,
		uuid.NewString(), key, text, mode)
	if err != nil {
		return false, fmt.Errorf("save ignored company text: %w", err)
	}
	return true, nil
}
