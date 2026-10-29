package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jdanielnd/crm-cli/internal/replica"
	"github.com/jdanielnd/crm-cli/internal/replica/replicasql"
)

// Client talks to a relay.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClient makes a client with a sensible timeout.
func NewClient(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: 60 * time.Second}}
}

// StatusError is a non-200 answer.
type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string { return fmt.Sprintf("relay answered %d: %s", e.Code, e.Message) }

func (c *Client) do(ctx context.Context, method, path string, in, out any, auth bool) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/"+Version+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	resp, err := h.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var er ErrorResponse
		if json.Unmarshal(data, &er) != nil || er.Error == "" {
			er.Error = strings.TrimSpace(string(data))
		}
		return &StatusError{Code: resp.StatusCode, Message: er.Error}
	}
	return json.Unmarshal(data, out)
}

// Enroll trades a one-time token for a device token.
func (c *Client) Enroll(ctx context.Context, req EnrollRequest) (EnrollResponse, error) {
	var out EnrollResponse
	err := c.do(ctx, http.MethodPost, "/enroll", req, &out, false)
	return out, err
}

// Pull asks for changes.
func (c *Client) Pull(ctx context.Context, req PullRequest) (PullResponse, error) {
	var out PullResponse
	err := c.do(ctx, http.MethodPost, "/pull", req, &out, true)
	return out, err
}

// Push sends changes.
func (c *Client) Push(ctx context.Context, req PushRequest) (PushResponse, error) {
	var out PushResponse
	err := c.do(ctx, http.MethodPost, "/push", req, &out, true)
	return out, err
}

// Status reads the relay's status.
func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	var out StatusResponse
	err := c.do(ctx, http.MethodGet, "/status", nil, &out, true)
	return out, err
}

// Report says what one exchange with a peer did.
type Report struct {
	Peer        string              `json:"peer"`
	Device      string              `json:"device"`
	Pulled      int                 `json:"pulled"`
	Applied     int                 `json:"applied"`
	Duplicates  int                 `json:"duplicates"`
	Unknown     int                 `json:"unknown"`
	Pushed      int                 `json:"pushed"`
	PushApplied int                 `json:"push_applied"`
	Rejected    []replica.Rejection `json:"rejected,omitempty"`
	OpenReviews int64               `json:"open_reviews"`
}

// Options of an exchange.
type Options struct {
	Pull, Push bool
	Full       bool // ignore this database's cursor and pull everything (a snapshot)
	Limit      int
}

// Sync pulls from a peer, then pushes what the peer lacks. A rejected change (clock drift or bad
// shape) stops that device's later changes in the same batch and is reported, never skipped.
func Sync(ctx context.Context, a *replicasql.Adapter, peer replicasql.Device, opt Options) (Report, error) {
	rep := Report{Peer: peer.Name, Device: peer.Device}
	c := NewClient(peer.URL, peer.Token)
	limit := opt.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	head := func() int64 {
		v, _ := a.Vector(ctx)
		return v[a.Device()]
	}
	var remote replica.VersionVector
	note := func(pulled, pushed bool, err error) {
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		_ = a.NoteDevice(ctx, peer.Device, remote, 0, pulled, pushed, msg)
	}
	if opt.Pull {
		cursor := replica.VersionVector{}
		if !opt.Full {
			v, err := a.Vector(ctx)
			if err != nil {
				return rep, err
			}
			cursor = v
		}
		for {
			resp, err := c.Pull(ctx, PullRequest{Cursor: cursor, Limit: limit, Head: head()})
			if err != nil {
				note(false, false, err)
				return rep, err
			}
			remote = resp.Vector
			rep.Pulled += len(resp.Changes)
			if len(resp.Changes) == 0 {
				break
			}
			res, err := a.ApplyBatch(ctx, resp.Changes)
			if err != nil {
				note(false, false, err)
				return rep, err
			}
			rep.Applied += res.Applied
			rep.Duplicates += res.Duplicates
			rep.Unknown += res.Unknown
			rep.Rejected = append(rep.Rejected, res.Rejected...)
			if opt.Full {
				cursor = resp.Cursor
			} else if cursor, err = a.Vector(ctx); err != nil {
				return rep, err
			}
			if !resp.More || len(res.Rejected) > 0 {
				// Tell the relay where this device is now. The relay uses it to decide when a delete may be purged.
				if len(res.Rejected) == 0 {
					if cur, err := a.Vector(ctx); err == nil {
						_, _ = c.Pull(ctx, PullRequest{Cursor: cur, Limit: 1, Head: head()})
					}
				}
				break
			}
		}
		note(true, false, nil)
	}
	if opt.Push {
		if remote == nil {
			st, err := c.Status(ctx)
			if err != nil {
				note(false, false, err)
				return rep, err
			}
			remote = st.Vector
		}
		for {
			cs, err := a.Pull(ctx, remote, limit)
			if err != nil {
				return rep, err
			}
			if len(cs) == 0 {
				break
			}
			mine, _ := a.Vector(ctx)
			resp, err := c.Push(ctx, PushRequest{Changes: cs, Vector: mine, Head: mine[a.Device()]})
			if err != nil {
				note(false, false, err)
				return rep, err
			}
			rep.Pushed += len(cs)
			rep.PushApplied += resp.Applied
			rep.Rejected = append(rep.Rejected, resp.Rejected...)
			next := resp.Vector
			if next == nil || !vectorAdvanced(remote, next, cs) {
				// The relay did not take the page. Stop instead of sending it again forever.
				break
			}
			remote = next
		}
		note(false, true, nil)
	}
	st, err := a.Status(ctx)
	if err == nil {
		rep.OpenReviews = st.OpenReviews
	}
	return rep, nil
}

// vectorAdvanced reports whether the relay's new vector moved past at least the first change of
// the page it was given.
func vectorAdvanced(old, now replica.VersionVector, sent []replica.Change) bool {
	for _, c := range sent {
		if now[c.ID.Device] > old[c.ID.Device] {
			return true
		}
	}
	return false
}
