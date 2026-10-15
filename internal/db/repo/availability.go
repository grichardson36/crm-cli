package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// AvailabilityRepo stores working hours and the scheduling defaults.
type AvailabilityRepo struct {
	db *sql.DB
}

// NewAvailabilityRepo creates a new AvailabilityRepo.
func NewAvailabilityRepo(db *sql.DB) *AvailabilityRepo { return &AvailabilityRepo{db: db} }

const (
	metaDefaultRule = "schedule.default_rule"
	metaBufferMin   = "schedule.buffer_min"
	metaStepMin     = "schedule.step_min"
	metaHorizonDays = "schedule.horizon_days"
)

// Config returns the scheduling defaults, with built-in values for anything not set.
func (r *AvailabilityRepo) Config(ctx context.Context) (model.ScheduleConfig, error) {
	c := model.DefaultScheduleConfig()
	rows, err := r.db.QueryContext(ctx, `SELECT key, value FROM meta WHERE key LIKE 'schedule.%'`)
	if err != nil {
		return c, fmt.Errorf("read schedule config: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			return c, err
		}
		n, _ := strconv.Atoi(v.String)
		switch k {
		case metaDefaultRule:
			if v.String != "" {
				c.DefaultRule = v.String
			}
		case metaBufferMin:
			c.BufferMin = n
		case metaStepMin:
			if n > 0 {
				c.StepMin = n
			}
		case metaHorizonDays:
			if n > 0 {
				c.HorizonDays = n
			}
		}
	}
	return c, rows.Err()
}

// SetConfig changes the given defaults. Nil leaves a value alone.
func (r *AvailabilityRepo) SetConfig(ctx context.Context, rule *string, buffer, step, horizon *int) (model.ScheduleConfig, error) {
	set := func(key string, v any) error {
		_, err := r.db.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, fmt.Sprint(v))
		return err
	}
	if rule != nil {
		name := strings.TrimSpace(*rule)
		if name == "" {
			return model.ScheduleConfig{}, fmt.Errorf("the default rule needs a name: %w", model.ErrValidation)
		}
		if err := set(metaDefaultRule, name); err != nil {
			return model.ScheduleConfig{}, err
		}
	}
	for _, f := range []struct {
		key  string
		p    *int
		min  int
		what string
	}{{metaBufferMin, buffer, 0, "buffer"}, {metaStepMin, step, 1, "step"}, {metaHorizonDays, horizon, 1, "horizon"}} {
		if f.p == nil {
			continue
		}
		if *f.p < f.min {
			return model.ScheduleConfig{}, fmt.Errorf("%s must be at least %d: %w", f.what, f.min, model.ErrValidation)
		}
		if err := set(f.key, *f.p); err != nil {
			return model.ScheduleConfig{}, err
		}
	}
	return r.Config(ctx)
}

func defaultHours(name string) *model.WorkingHours {
	h := &model.WorkingHours{Name: name, TZ: tz.Name(tz.Device()), Weekly: map[string][]model.TimeRange{}, Default: true}
	for _, d := range model.WeekdayKeys[:5] {
		h.Weekly[d] = []model.TimeRange{{Start: "09:00", End: "17:00"}}
	}
	return h
}

type weeklyJSON map[string][][2]string

func toJSON(m map[string][]model.TimeRange) (string, error) {
	w := weeklyJSON{}
	for k, rs := range m {
		pairs := make([][2]string, len(rs))
		for i, r := range rs {
			pairs[i] = [2]string{r.Start, r.End}
		}
		w[k] = pairs
	}
	b, err := json.Marshal(w)
	return string(b), err
}

func fromJSON(s string) (map[string][]model.TimeRange, error) {
	var w weeklyJSON
	if err := json.Unmarshal([]byte(s), &w); err != nil {
		return nil, fmt.Errorf("bad hours data: %w", err)
	}
	out := map[string][]model.TimeRange{}
	for k, pairs := range w {
		rs := make([]model.TimeRange, len(pairs))
		for i, p := range pairs {
			rs[i] = model.TimeRange{Start: p[0], End: p[1]}
		}
		out[k] = rs
	}
	return out, nil
}

// Get returns the working hours of a rule. An empty name is the default rule. A rule that was
// never stored gives Monday to Friday 09:00-17:00 in this device's zone.
func (r *AvailabilityRepo) Get(ctx context.Context, name string) (*model.WorkingHours, error) {
	if strings.TrimSpace(name) == "" {
		c, err := r.Config(ctx)
		if err != nil {
			return nil, err
		}
		name = c.DefaultRule
	}
	var id int64
	h := &model.WorkingHours{Name: name}
	var weekly string
	err := r.db.QueryRowContext(ctx, `SELECT id, uuid, tz, weekly_json FROM availability_rules WHERE name = ? AND archived = 0`, name).Scan(&id, &h.UUID, &h.TZ, &weekly)
	if errors.Is(err, sql.ErrNoRows) {
		if name != model.DefaultScheduleConfig().DefaultRule {
			return nil, fmt.Errorf("working hours %q: %w", name, model.ErrNotFound)
		}
		return defaultHours(name), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read working hours: %w", err)
	}
	if h.Weekly, err = fromJSON(weekly); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT date, intervals_json FROM availability_overrides WHERE rule_id = ? ORDER BY date`, id)
	if err != nil {
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	defer rows.Close()
	h.Overrides = map[string][]model.TimeRange{}
	for rows.Next() {
		var date, js string
		if err := rows.Scan(&date, &js); err != nil {
			return nil, err
		}
		var pairs [][2]string
		if err := json.Unmarshal([]byte(js), &pairs); err != nil {
			return nil, fmt.Errorf("bad override data: %w", err)
		}
		rs := make([]model.TimeRange, len(pairs))
		for i, p := range pairs {
			rs[i] = model.TimeRange{Start: p[0], End: p[1]}
		}
		h.Overrides[date] = rs
	}
	return h, rows.Err()
}

// List returns the stored rule names, plus the default if it was never stored.
func (r *AvailabilityRepo) List(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT name FROM availability_rules WHERE archived = 0 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list working hours: %w", err)
	}
	defer rows.Close()
	var out []string
	have := false
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		have = have || n == model.DefaultScheduleConfig().DefaultRule
		out = append(out, n)
	}
	if !have {
		out = append([]string{model.DefaultScheduleConfig().DefaultRule}, out...)
	}
	return out, rows.Err()
}

// save writes a rule, creating it on first use.
func (r *AvailabilityRepo) save(ctx context.Context, h *model.WorkingHours) error {
	js, err := toJSON(h.Weekly)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO availability_rules (uuid, name, tz, weekly_json) VALUES (?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET tz = excluded.tz, weekly_json = excluded.weekly_json, archived = 0, updated_at = datetime('now')`,
		uuid.NewString(), h.Name, h.TZ, js)
	if err != nil {
		return fmt.Errorf("save working hours: %w", err)
	}
	return nil
}

func (r *AvailabilityRepo) ruleID(ctx context.Context, name string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM availability_rules WHERE name = ? AND archived = 0`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("working hours %q: %w", name, model.ErrNotFound)
	}
	return id, err
}

// SetDays sets the hours of some weekdays. Empty ranges close those days. tzName, when given,
// changes the zone of the rule.
func (r *AvailabilityRepo) SetDays(ctx context.Context, name, tzName string, days []string, ranges []model.TimeRange) (*model.WorkingHours, error) {
	if strings.TrimSpace(name) == "" {
		c, err := r.Config(ctx)
		if err != nil {
			return nil, err
		}
		name = c.DefaultRule
	}
	clean, err := model.CleanRanges(ranges)
	if err != nil {
		return nil, err
	}
	h, err := r.Get(ctx, name)
	if errors.Is(err, model.ErrNotFound) {
		h = &model.WorkingHours{Name: name, TZ: tz.Name(tz.Device()), Weekly: map[string][]model.TimeRange{}}
	} else if err != nil {
		return nil, err
	}
	if strings.TrimSpace(tzName) != "" {
		loc, err := tz.Load(tzName)
		if err != nil {
			return nil, fmt.Errorf("%v: %w", err, model.ErrValidation)
		}
		h.TZ = tz.Name(loc)
	}
	for _, d := range days {
		if len(clean) == 0 {
			delete(h.Weekly, d)
		} else {
			h.Weekly[d] = clean
		}
	}
	if err := r.save(ctx, h); err != nil {
		return nil, err
	}
	return r.Get(ctx, name)
}

// SetOverride sets the hours of one date. Empty ranges make it a day off.
func (r *AvailabilityRepo) SetOverride(ctx context.Context, name, date string, ranges []model.TimeRange) (*model.WorkingHours, error) {
	if _, err := tz.ParseDate(date); err != nil {
		return nil, fmt.Errorf("date must look like 2026-12-24: %w", model.ErrValidation)
	}
	clean, err := model.CleanRanges(ranges)
	if err != nil {
		return nil, err
	}
	h, err := r.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := r.save(ctx, h); err != nil { // make sure the rule exists
		return nil, err
	}
	id, err := r.ruleID(ctx, h.Name)
	if err != nil {
		return nil, err
	}
	pairs := make([][2]string, len(clean))
	for i, c := range clean {
		pairs[i] = [2]string{c.Start, c.End}
	}
	js, _ := json.Marshal(pairs)
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO availability_overrides (rule_id, date, intervals_json) VALUES (?, ?, ?)
		 ON CONFLICT(rule_id, date) DO UPDATE SET intervals_json = excluded.intervals_json`, id, date, string(js)); err != nil {
		return nil, fmt.Errorf("save override: %w", err)
	}
	if err := r.touch(ctx, id); err != nil {
		return nil, err
	}
	return r.Get(ctx, h.Name)
}

// RemoveOverride deletes the override of a date.
func (r *AvailabilityRepo) RemoveOverride(ctx context.Context, name, date string) (*model.WorkingHours, error) {
	h, err := r.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	id, err := r.ruleID(ctx, h.Name)
	if err != nil {
		return nil, fmt.Errorf("no override for %s: %w", date, model.ErrNotFound)
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM availability_overrides WHERE rule_id = ? AND date = ?`, id, date)
	if err != nil {
		return nil, fmt.Errorf("remove override: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("no override for %s: %w", date, model.ErrNotFound)
	}
	if err := r.touch(ctx, id); err != nil {
		return nil, err
	}
	return r.Get(ctx, h.Name)
}

func (r *AvailabilityRepo) touch(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE availability_rules SET updated_at = datetime('now') WHERE id = ?`, id)
	return err
}
