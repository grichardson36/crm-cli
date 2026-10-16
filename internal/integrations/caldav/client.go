// Package caldav syncs events with a CalDAV calendar (Radicale first). It is a sync adapter: runs,
// cursor, counts and the inbox come from package sync. Events are matched by UID through
// external_ids, an ETag per resource says when the server copy changed, and caldav_resources
// remembers the state both sides last agreed on so a later run can tell who changed what.
package caldav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	wdcaldav "github.com/emersion/go-webdav/caldav"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Config says where the calendar is and how to reach it.
type Config struct {
	// URL is the calendar collection, such as https://dav.example.com/grant/personal/.
	URL      string
	User     string
	Password string
	// TwoWay also sends local changes, new events and deletions to the server. Off by default:
	// the first run would otherwise copy the whole local calendar to the server.
	TwoWay bool
	// SelfEmails are your own addresses, left out when logging meetings with attendees.
	SelfEmails []string
}

// FromEnv reads the configuration from the environment: CRM_CALDAV_URL, CRM_CALDAV_USER,
// CRM_CALDAV_PASSWORD or CRM_CALDAV_PASSWORD_FILE (the first line), CRM_CALDAV_TWO_WAY=1 and
// CRM_CALDAV_SELF (comma separated addresses). It reports false when no URL is set.
func FromEnv(getenv func(string) string, readFile func(string) ([]byte, error)) (Config, bool) {
	c := Config{URL: strings.TrimSpace(getenv("CRM_CALDAV_URL")), User: getenv("CRM_CALDAV_USER"), Password: getenv("CRM_CALDAV_PASSWORD")}
	if c.URL == "" {
		return c, false
	}
	if f := getenv("CRM_CALDAV_PASSWORD_FILE"); f != "" && c.Password == "" {
		if b, err := readFile(f); err == nil {
			c.Password = strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
		}
	}
	v := strings.ToLower(getenv("CRM_CALDAV_TWO_WAY"))
	c.TwoWay = v == "1" || v == "true" || v == "yes"
	for _, e := range strings.Split(getenv("CRM_CALDAV_SELF"), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			c.SelfEmails = append(c.SelfEmails, e)
		}
	}
	if strings.Contains(c.User, "@") {
		c.SelfEmails = append(c.SelfEmails, strings.ToLower(c.User))
	}
	return c, true
}

// Object is one calendar resource on the server.
type Object struct {
	Href string
	ETag string
	Data []byte
}

// Client talks to one calendar collection.
type Client struct {
	cfg   Config
	http  webdav.HTTPClient
	cal   *wdcaldav.Client
	base  *url.URL
	coll  string // collection path with a trailing slash
	plain *http.Client
}

// NewClient prepares a client. It makes no request.
func NewClient(cfg Config, hc *http.Client) (*Client, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("CRM_CALDAV_URL %q is not a URL: %w", cfg.URL, model.ErrValidation)
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	var wc webdav.HTTPClient = hc
	if cfg.User != "" {
		wc = webdav.HTTPClientWithBasicAuth(hc, cfg.User, cfg.Password)
	}
	base := &url.URL{Scheme: u.Scheme, Host: u.Host}
	cal, err := wdcaldav.NewClient(wc, base.String())
	if err != nil {
		return nil, fmt.Errorf("caldav client: %w", err)
	}
	coll := u.Path
	if !strings.HasSuffix(coll, "/") {
		coll += "/"
	}
	return &Client{cfg: cfg, http: wc, cal: cal, base: base, coll: coll, plain: hc}, nil
}

// ETags are kept the way List reports them, without quotes, and quoted again on the wire.
func quoteETag(s string) string {
	if strings.HasPrefix(s, `"`) || strings.HasPrefix(s, "W/") {
		return s
	}
	return `"` + s + `"`
}

func unquoteETag(s string) string { return strings.Trim(strings.TrimPrefix(s, "W/"), `"`) }

// Collection is the path of the calendar.
func (c *Client) Collection() string { return c.coll }

func encode(cal *ical.Calendar) ([]byte, error) {
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// List returns every event object of the calendar.
func (c *Client) List(ctx context.Context) ([]Object, error) {
	objs, err := c.cal.QueryCalendar(ctx, c.coll, &wdcaldav.CalendarQuery{
		CompRequest: wdcaldav.CalendarCompRequest{Name: ical.CompCalendar, AllProps: true, AllComps: true},
		CompFilter:  wdcaldav.CompFilter{Name: ical.CompCalendar, Comps: []wdcaldav.CompFilter{{Name: ical.CompEvent}}},
	})
	if err != nil {
		return nil, fmt.Errorf("list calendar: %w", err)
	}
	out := make([]Object, 0, len(objs))
	for _, o := range objs {
		if o.Data == nil {
			continue
		}
		data, err := encode(o.Data)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", o.Path, err)
		}
		out = append(out, Object{Href: o.Path, ETag: o.ETag, Data: data})
	}
	return out, nil
}

// HrefFor builds the path of a new object from its UID.
func (c *Client) HrefFor(uid string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.', r == '@':
			return r
		}
		return '_'
	}, uid)
	return path.Join(c.coll, safe+".ics")
}

func (c *Client) do(ctx context.Context, method, href string, body []byte, hdr map[string]string) (*http.Response, error) {
	u := *c.base
	u.Path = href
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if c.cfg.User != "" {
		req.SetBasicAuth(c.cfg.User, c.cfg.Password)
	}
	return c.plain.Do(req)
}

// Put writes an object. ifMatch is the ETag the server copy must still have ("" means create
// only, "*" means overwrite whatever is there). It returns the new ETag.
func (c *Client) Put(ctx context.Context, href string, data []byte, ifMatch string) (string, error) {
	hdr := map[string]string{"Content-Type": "text/calendar; charset=utf-8"}
	switch ifMatch {
	case "":
		hdr["If-None-Match"] = "*"
	case "*":
	default:
		hdr["If-Match"] = quoteETag(ifMatch)
	}
	resp, err := c.do(ctx, http.MethodPut, href, data, hdr)
	if err != nil {
		return "", fmt.Errorf("put %s: %w", href, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed {
		return "", fmt.Errorf("put %s: the server copy changed meanwhile: %w", href, model.ErrConflict)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return "", fmt.Errorf("put %s: %s %s", href, resp.Status, strings.TrimSpace(string(b)))
	}
	if et := resp.Header.Get("ETag"); et != "" {
		return unquoteETag(et), nil
	}
	o, err := c.cal.GetCalendarObject(ctx, href)
	if err != nil {
		return "", fmt.Errorf("read etag of %s: %w", href, err)
	}
	return unquoteETag(o.ETag), nil
}

// Delete removes an object if it still has the ETag.
func (c *Client) Delete(ctx context.Context, href, ifMatch string) error {
	hdr := map[string]string{}
	if ifMatch != "" {
		hdr["If-Match"] = quoteETag(ifMatch)
	}
	resp, err := c.do(ctx, http.MethodDelete, href, nil, hdr)
	if err != nil {
		return fmt.Errorf("delete %s: %w", href, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusPreconditionFailed:
		return fmt.Errorf("delete %s: the server copy changed meanwhile: %w", href, model.ErrConflict)
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode >= 200 && resp.StatusCode <= 299:
		return nil
	}
	return fmt.Errorf("delete %s: %s", href, resp.Status)
}
