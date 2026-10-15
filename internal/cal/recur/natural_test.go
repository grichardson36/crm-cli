package recur

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNatural(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"daily", "FREQ=DAILY", false},
		{"every day", "FREQ=DAILY", false},
		{"every weekday", "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", false},
		{"weekdays", "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", false},
		{"weekly", "FREQ=WEEKLY", false},
		{"every week", "FREQ=WEEKLY", false},
		{"biweekly", "FREQ=WEEKLY;INTERVAL=2", false},
		{"every other week", "FREQ=WEEKLY;INTERVAL=2", false},
		{"monthly", "FREQ=MONTHLY", false},
		{"every month", "FREQ=MONTHLY", false},
		{"yearly", "FREQ=YEARLY", false},
		{"every year", "FREQ=YEARLY", false},
		{"every 2w", "FREQ=WEEKLY;INTERVAL=2", false},
		{"every 3d", "FREQ=DAILY;INTERVAL=3", false},
		{"every 2mo", "FREQ=MONTHLY;INTERVAL=2", false},
		{"every 1y", "FREQ=YEARLY", false},
		{"every 2 weeks", "FREQ=WEEKLY;INTERVAL=2", false},
		{"3 days", "FREQ=DAILY;INTERVAL=3", false},
		{"every fri", "FREQ=WEEKLY;BYDAY=FR", false},
		{"every Friday", "FREQ=WEEKLY;BYDAY=FR", false},
		{"mon,wed,fri", "FREQ=WEEKLY;BYDAY=MO,WE,FR", false},
		{"every mon and wed", "FREQ=WEEKLY;BYDAY=MO,WE", false},
		{"every 1st mon", "FREQ=MONTHLY;BYDAY=1MO", false},
		{"every last fri", "FREQ=MONTHLY;BYDAY=-1FR", false},
		{"2nd tue", "FREQ=MONTHLY;BYDAY=2TU", false},
		{"  Every  FRI  ", "FREQ=WEEKLY;BYDAY=FR", false},
		{"FREQ=WEEKLY;BYDAY=MO", "FREQ=WEEKLY;BYDAY=MO", false},
		{"rrule:freq=daily;count=3", "FREQ=DAILY;COUNT=3", false},
		{"FREQ=DAILY;COUNT=3;UNTIL=20261231", "", true},
		{"", "", true},
		{"whenever", "", true},
		{"every 0w", "", true},
		{"every blursday", "", true},
		{"every fri and blursday", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Natural(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.NoError(t, Validate(got), "every phrase compiles to a valid rule")
		})
	}
}

func TestFromTaskRecurrence(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"daily", "FREQ=DAILY"},
		{"weekly", "FREQ=WEEKLY"},
		{"monthly", "FREQ=MONTHLY"},
		{"quarterly", "FREQ=MONTHLY;INTERVAL=3"},
		{"yearly", "FREQ=YEARLY"},
		{"7 days", "FREQ=WEEKLY"},
		{"10 days", "FREQ=DAILY;INTERVAL=10"},
		{"2 weeks", "FREQ=WEEKLY;INTERVAL=2"},
		{"1 mon", "FREQ=MONTHLY"},
		{"3 mons", "FREQ=MONTHLY;INTERVAL=3"},
		{"1 year", "FREQ=YEARLY"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := FromTaskRecurrence(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.NoError(t, Validate(got))
		})
	}
	_, err := FromTaskRecurrence("whenever")
	assert.Error(t, err)
}

func TestDescribe(t *testing.T) {
	tests := []struct {
		rule string
		want string
	}{
		{"FREQ=DAILY", "every day"},
		{"FREQ=WEEKLY;BYDAY=MO,WE", "every week on Mon, Wed"},
		{"FREQ=WEEKLY;INTERVAL=2", "every 2 weeks"},
		{"FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", "every weekday"},
		{"FREQ=MONTHLY;BYDAY=-1FR", "every month on last Fri"},
		{"FREQ=MONTHLY;BYDAY=1MO", "every month on 1 Mon"},
		{"FREQ=DAILY;COUNT=5", "every day, 5 times"},
		{"FREQ=WEEKLY;UNTIL=20261231T000000Z", "every week until 2026-12-31"},
		{"not a rule", "not a rule"},
	}
	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			assert.Equal(t, tt.want, Describe(tt.rule))
		})
	}
}
