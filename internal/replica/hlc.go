// Package replica is the device-to-device sync core: hybrid logical clock, field-change records,
// version vectors, per-field conflict policies and the apply engine. It is pure Go and talks to a
// database only through the Store and Projector interfaces. See patterns/sync-contract.md.
package replica

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MaxDrift is the largest accepted gap between a clock and the wall clock. Actual Budget uses the
// same five minutes ("Allow 5 minutes of clock drift" in packages/crdt/crdt/timestamp.ts).
const MaxDrift = 5 * time.Minute

// MaxCounter is the largest logical counter of one millisecond (four hex digits).
const MaxCounter = 0xFFFF

const (
	hlcTimeLen = 24 // 2006-01-02T15:04:05.000Z
	hlcLen     = hlcTimeLen + 1 + 4 + 1 + 16
	hlcLayout  = "2006-01-02T15:04:05.000"
)

// Errors returned by the clock.
var (
	ErrDrift      = errors.New("clock drift is above the maximum")
	ErrBadHLC     = errors.New("malformed hybrid logical clock")
	ErrCounterMax = errors.New("hybrid logical clock counter overflow")
)

// HLC is a hybrid logical clock reading. Its string form is fixed width and sorts as a string:
// "2026-10-28T12:00:00.000Z-00AF-0123456789abcdef" (UTC millis, 4 hex digit counter, 16 hex node).
type HLC struct {
	Time    time.Time
	Counter int
	Node    string
}

// String formats the reading.
func (h HLC) String() string {
	return FormatHLC(h.Time, h.Counter, h.Node)
}

// FormatHLC builds the fixed-width string.
func FormatHLC(t time.Time, counter int, device string) string {
	return fmt.Sprintf("%s-%04X-%s", t.UTC().Format(hlcLayout)+"Z", counter, NodeID(device))
}

// NodeID reduces a device id to the 16 lowercase hex characters used in clock strings.
func NodeID(device string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(device) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
		}
	}
	s := b.String()
	for len(s) < 16 {
		s += "0"
	}
	return s[:16]
}

// ParseHLC parses the fixed-width string form.
func ParseHLC(s string) (HLC, error) {
	if len(s) != hlcLen || s[hlcTimeLen] != '-' || s[hlcTimeLen+5] != '-' || s[hlcTimeLen-1] != 'Z' {
		return HLC{}, fmt.Errorf("%w: %q", ErrBadHLC, s)
	}
	t, err := time.Parse(hlcLayout, s[:hlcTimeLen-1])
	if err != nil {
		return HLC{}, fmt.Errorf("%w: %q", ErrBadHLC, s)
	}
	n, err := strconv.ParseUint(s[hlcTimeLen+1:hlcTimeLen+5], 16, 32)
	if err != nil {
		return HLC{}, fmt.Errorf("%w: %q", ErrBadHLC, s)
	}
	return HLC{Time: t.UTC(), Counter: int(n), Node: s[hlcTimeLen+6:]}, nil
}

// CheckDrift rejects a clock string whose time is more than MaxDrift ahead of now.
func CheckDrift(hlc string, now time.Time) error {
	h, err := ParseHLC(hlc)
	if err != nil {
		return err
	}
	if h.Time.Sub(now.UTC()) > MaxDrift {
		return fmt.Errorf("%w: %s is %s ahead", ErrDrift, hlc, h.Time.Sub(now.UTC()).Round(time.Second))
	}
	return nil
}

// Clock issues and merges hybrid logical clock readings for one node.
type Clock struct {
	mu   sync.Mutex
	now  func() time.Time
	node string
	last time.Time
	cnt  int
}

// NewClock makes a clock for the device. A nil now uses the wall clock.
func NewClock(device string, now func() time.Time) *Clock {
	if now == nil {
		now = time.Now
	}
	return &Clock{now: now, node: device}
}

// Restore sets the last reading, for a clock that was saved to disk.
func (c *Clock) Restore(last time.Time, counter int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last, c.cnt = last.UTC().Truncate(time.Millisecond), counter
}

// Last returns the saved reading.
func (c *Clock) Last() (time.Time, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last, c.cnt
}

func (c *Clock) wall() time.Time { return c.now().UTC().Truncate(time.Millisecond) }

// Now issues a reading for a local event. It fails when the clock is already more than MaxDrift
// ahead of the wall clock, which means the wall clock moved back or a peer sent a far future time.
func (c *Clock) Now() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.wall()
	if c.last.Sub(w) > MaxDrift {
		return "", fmt.Errorf("%w: clock is %s ahead of the wall clock", ErrDrift, c.last.Sub(w).Round(time.Second))
	}
	if w.After(c.last) {
		c.last, c.cnt = w, 0
	} else {
		c.cnt++
		if c.cnt > MaxCounter {
			return "", ErrCounterMax
		}
	}
	return FormatHLC(c.last, c.cnt, c.node), nil
}

// Recv merges a reading from another node and returns the new local reading. It rejects a remote
// reading more than MaxDrift ahead of the wall clock.
func (c *Clock) Recv(remote string) (string, error) {
	r, err := ParseHLC(remote)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.wall()
	if r.Time.Sub(w) > MaxDrift {
		return "", fmt.Errorf("%w: %s is %s ahead", ErrDrift, remote, r.Time.Sub(w).Round(time.Second))
	}
	t := w
	if c.last.After(t) {
		t = c.last
	}
	if r.Time.After(t) {
		t = r.Time
	}
	switch {
	case t.Equal(c.last) && t.Equal(r.Time):
		c.cnt = max(c.cnt, r.Counter) + 1
	case t.Equal(c.last):
		c.cnt++
	case t.Equal(r.Time):
		c.cnt = r.Counter + 1
	default:
		c.cnt = 0
	}
	if c.cnt > MaxCounter {
		return "", ErrCounterMax
	}
	c.last = t
	return FormatHLC(c.last, c.cnt, c.node), nil
}
