package repo_test

import (
	"context"
	"testing"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAvailabilityDefaults(t *testing.T) {
	ar := repo.NewAvailabilityRepo(openRaw(t))
	ctx := context.Background()

	cfg, err := ar.Config(ctx)
	require.NoError(t, err)
	assert.Equal(t, model.ScheduleConfig{DefaultRule: "work", BufferMin: 10, StepMin: 15, HorizonDays: 14}, cfg)

	h, err := ar.Get(ctx, "")
	require.NoError(t, err)
	assert.True(t, h.Default)
	assert.Equal(t, "work", h.Name)
	for _, d := range []string{"mon", "tue", "wed", "thu", "fri"} {
		assert.Equal(t, []model.TimeRange{{Start: "09:00", End: "17:00"}}, h.Weekly[d], d)
	}
	assert.Empty(t, h.Weekly["sat"])
	assert.Empty(t, h.Weekly["sun"])

	_, err = ar.Get(ctx, "nope")
	assert.ErrorIs(t, err, model.ErrNotFound)
	names, err := ar.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"work"}, names)
}

func TestAvailabilitySetDaysAndOverrides(t *testing.T) {
	d := openRaw(t)
	ar := repo.NewAvailabilityRepo(d)
	ctx := context.Background()

	h, err := ar.SetDays(ctx, "", "America/Chicago", []string{"sat"}, []model.TimeRange{{Start: "10:00", End: "14:00"}})
	require.NoError(t, err)
	assert.False(t, h.Default)
	assert.Equal(t, "America/Chicago", h.TZ)
	assert.Equal(t, []model.TimeRange{{Start: "10:00", End: "14:00"}}, h.Weekly["sat"])
	assert.Len(t, h.Weekly["mon"], 1, "the built-in weekdays were stored with it")

	h, err = ar.SetDays(ctx, "", "", []string{"fri"}, nil)
	require.NoError(t, err)
	assert.Empty(t, h.Weekly["fri"], "no ranges closes the day")

	h, err = ar.SetDays(ctx, "", "", []string{"mon"}, []model.TimeRange{{Start: "13:00", End: "17:00"}, {Start: "08:00", End: "12:00"}})
	require.NoError(t, err)
	assert.Equal(t, []model.TimeRange{{Start: "08:00", End: "12:00"}, {Start: "13:00", End: "17:00"}}, h.Weekly["mon"], "sorted")

	h, err = ar.SetOverride(ctx, "", "2026-12-24", []model.TimeRange{{Start: "09:00", End: "12:00"}})
	require.NoError(t, err)
	assert.Equal(t, []model.TimeRange{{Start: "09:00", End: "12:00"}}, h.Overrides["2026-12-24"])
	h, err = ar.SetOverride(ctx, "", "2026-12-25", nil)
	require.NoError(t, err)
	assert.Empty(t, h.Overrides["2026-12-25"])
	assert.Contains(t, h.Overrides, "2026-12-25", "an empty override is a day off, not a missing one")

	h, err = ar.RemoveOverride(ctx, "", "2026-12-24")
	require.NoError(t, err)
	assert.NotContains(t, h.Overrides, "2026-12-24")
	_, err = ar.RemoveOverride(ctx, "", "2026-12-24")
	assert.ErrorIs(t, err, model.ErrNotFound)

	// A second rule is separate.
	other, err := ar.SetDays(ctx, "evenings", "UTC", []string{"mon", "tue"}, []model.TimeRange{{Start: "18:00", End: "21:00"}})
	require.NoError(t, err)
	assert.Equal(t, "evenings", other.Name)
	assert.Empty(t, other.Weekly["wed"])
	names, err := ar.List(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"evenings", "work"}, names)

	// Writes bump updated_at.
	_, err = d.Exec(`UPDATE availability_rules SET updated_at = '2000-01-01 00:00:00'`)
	require.NoError(t, err)
	_, err = ar.SetOverride(ctx, "", "2026-12-31", nil)
	require.NoError(t, err)
	var stale int
	require.NoError(t, d.QueryRow(`SELECT COUNT(*) FROM availability_rules WHERE name = 'work' AND updated_at = '2000-01-01 00:00:00'`).Scan(&stale))
	assert.Equal(t, 0, stale)
}

func TestAvailabilityValidation(t *testing.T) {
	ar := repo.NewAvailabilityRepo(openRaw(t))
	ctx := context.Background()
	bad := [][]model.TimeRange{
		{{Start: "9", End: "17:00"}},
		{{Start: "17:00", End: "09:00"}},
		{{Start: "09:00", End: "09:00"}},
		{{Start: "09:00", End: "13:00"}, {Start: "12:00", End: "15:00"}},
		{{Start: "25:00", End: "26:00"}},
		{{Start: "09:00", End: "24:30"}},
	}
	for _, r := range bad {
		_, err := ar.SetDays(ctx, "", "", []string{"mon"}, r)
		assert.ErrorIs(t, err, model.ErrValidation, "%v", r)
	}
	_, err := ar.SetDays(ctx, "", "Mars/Base", []string{"mon"}, nil)
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = ar.SetOverride(ctx, "", "Christmas", nil)
	assert.ErrorIs(t, err, model.ErrValidation)
	_, err = ar.SetDays(ctx, "", "", []string{"mon"}, []model.TimeRange{{Start: "00:00", End: "24:00"}})
	assert.NoError(t, err, "24:00 ends the day")
}

func TestScheduleConfigRoundTrip(t *testing.T) {
	ar := repo.NewAvailabilityRepo(openRaw(t))
	ctx := context.Background()
	cfg, err := ar.SetConfig(ctx, ptr("evenings"), intp(0), intp(30), intp(21))
	require.NoError(t, err)
	assert.Equal(t, model.ScheduleConfig{DefaultRule: "evenings", BufferMin: 0, StepMin: 30, HorizonDays: 21}, cfg)
	cfg, err = ar.SetConfig(ctx, nil, intp(5), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.BufferMin)
	assert.Equal(t, 30, cfg.StepMin, "unset values stay")

	for _, bad := range []struct{ b, s, h *int }{{intp(-1), nil, nil}, {nil, intp(0), nil}, {nil, nil, intp(0)}} {
		_, err = ar.SetConfig(ctx, nil, bad.b, bad.s, bad.h)
		assert.ErrorIs(t, err, model.ErrValidation)
	}
	_, err = ar.SetConfig(ctx, ptr(" "), nil, nil, nil)
	assert.ErrorIs(t, err, model.ErrValidation)
}

func TestParseDaysAndRanges(t *testing.T) {
	days := []struct {
		in   string
		want []string
		bad  bool
	}{
		{"mon", []string{"mon"}, false},
		{"mon-fri", []string{"mon", "tue", "wed", "thu", "fri"}, false},
		{"weekdays", []string{"mon", "tue", "wed", "thu", "fri"}, false},
		{"weekend", []string{"sat", "sun"}, false},
		{"all", model.WeekdayKeys, false},
		{"Mon,Wed,friday", []string{"mon", "wed", "fri"}, false},
		{"sat-mon", nil, true},
		{"funday", nil, true},
		{"", nil, true},
	}
	for _, tt := range days {
		got, err := model.ParseDays(tt.in)
		if tt.bad {
			assert.Error(t, err, tt.in)
			continue
		}
		require.NoError(t, err, tt.in)
		assert.Equal(t, tt.want, got, tt.in)
	}
	r, err := model.ParseRanges("09:00-12:00, 13:00-17:30")
	require.NoError(t, err)
	assert.Equal(t, []model.TimeRange{{Start: "09:00", End: "12:00"}, {Start: "13:00", End: "17:30"}}, r)
	for _, off := range []string{"off", "", "none"} {
		r, err = model.ParseRanges(off)
		require.NoError(t, err)
		assert.Empty(t, r)
	}
	_, err = model.ParseRanges("nine to five")
	assert.Error(t, err)
}
