package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/model"
	crmsync "github.com/jdanielnd/crm-cli/internal/sync"
	"github.com/mattn/go-runewidth"
)

// Sync view: sources (adapters, last run, counts, cursor) and the inbox of unmatched records.

// syncItem is what a Sync view row stands for: a source or an inbox record.
type syncItem struct {
	source *crmsync.Status
	entry  *crmsync.InboxEntry
}

// syncDoneMsg reports a finished background sync run.
type syncDoneMsg struct {
	source string
	dry    bool
	res    *crmsync.RunResult
	err    error
}

func selectedSync(m *Model) *syncItem {
	if m.view != viewSync || m.focus != focusList {
		return nil
	}
	r, ok := m.selected()
	if !ok {
		return nil
	}
	return r.sync
}

func whenSource(m *Model) bool {
	it := selectedSync(m)
	return it != nil && it.source != nil && it.source.Registered
}

func whenInboxRow(m *Model) bool {
	it := selectedSync(m)
	return it != nil && it.entry != nil
}

// sourceLine is the one line status of a source: "ok 5m ago" or "never run".
func sourceLine(s crmsync.Status, now time.Time) string {
	if s.LastRun == nil {
		return "never run"
	}
	when := s.LastRun.StartedAt
	if s.LastRun.FinishedAt != nil {
		when = *s.LastRun.FinishedAt
	}
	return s.LastRun.Status + " " + ago(when, now)
}

// syncRows lists the sources, then the pending inbox records.
func (m *Model) syncRows() ([]row, error) {
	ov, err := m.runner.Overview(m.ctx)
	if err != nil {
		return nil, err
	}
	inbox, err := m.inbox.List(m.ctx, model.InboxPending, 500)
	if err != nil {
		return nil, err
	}
	var rows []row
	now := m.now()
	if len(ov.Sources) > 0 {
		rows = append(rows, row{title: "Sources", meta: strconv.Itoa(len(ov.Sources)), skip: true})
		for i := range ov.Sources {
			s := ov.Sources[i]
			r := row{title: s.Source, sub: sourceLine(s, now), sync: &syncItem{source: &s}}
			if !s.Registered {
				r.title += " (not connected)"
			}
			if s.LastRun != nil {
				r.meta = fmt.Sprintf("+%d ~%d =%d !%d", s.LastRun.Created, s.LastRun.Updated, s.LastRun.Skipped, s.LastRun.Errors)
			}
			rows = append(rows, r)
		}
	}
	if len(ov.Sources) > 0 || len(inbox) > 0 {
		meta := strconv.Itoa(len(inbox))
		if len(inbox) == 0 {
			meta = "empty"
		}
		rows = append(rows, row{title: "Inbox", meta: meta, skip: true})
		for i := range inbox {
			e := inbox[i]
			title := cleanCell(e.Summary)
			if title == "" {
				title = "(unnamed " + e.Kind + ")"
			}
			rows = append(rows, row{title: title, sub: e.Source + " " + e.Reason, meta: e.Kind, sync: &syncItem{entry: &e}})
		}
	}
	return rows, nil
}

// syncItemDetail is the detail pane text for the selected Sync row.
func (m *Model) syncItemDetail(r row) string {
	w := max(1, m.detail.Width)
	line := func(s string) string { return runewidth.Truncate(s, w, "…") }
	kv := func(k, v string) string { return dimStyle.Render(fit(k, labelWidth)) + line(v) + "\n" }
	var b strings.Builder
	it := r.sync
	switch {
	case it == nil:
		return ""
	case it.source != nil:
		s := it.source
		b.WriteString(nameStyle.Render(line(s.Source)) + "\n\n")
		if !s.Registered {
			b.WriteString(errStyle.Render(line("not connected in this build, history only")) + "\n")
		}
		cursor := s.Cursor
		if cursor == "" {
			cursor = "none (first run pulls everything)"
		}
		if s.LastRun == nil {
			b.WriteString(kv("last run", "never"))
		} else {
			lr := s.LastRun
			b.WriteString(kv("last run", sourceLine(*s, m.now())))
			b.WriteString(kv("started", lr.StartedAt))
			b.WriteString(kv("created", strconv.Itoa(lr.Created)))
			b.WriteString(kv("updated", strconv.Itoa(lr.Updated)))
			b.WriteString(kv("skipped", strconv.Itoa(lr.Skipped)))
			b.WriteString(kv("errors", strconv.Itoa(lr.Errors)))
			if lr.Note != nil && *lr.Note != "" {
				b.WriteString(kv("note", *lr.Note))
			}
		}
		b.WriteString(kv("cursor", cursor))
		if s.Registered {
			b.WriteString("\n" + dimStyle.Render(line("s syncs now, d previews without writing")))
		}
	case it.entry != nil:
		e := it.entry
		b.WriteString(nameStyle.Render(line(cleanCell(e.Summary))) + "\n\n")
		b.WriteString(kv("source", e.Source))
		b.WriteString(kv("record", e.Kind))
		b.WriteString(kv("external id", e.ExternalID))
		b.WriteString(kv("why", e.Reason))
		b.WriteString(kv("received", e.CreatedAt))
		for _, id := range e.Candidates {
			b.WriteString(kv("candidate", fmt.Sprintf("#%d %s", id, m.nameOf(id))))
		}
		b.WriteString(inboxPayloadLines(e.Payload, kv))
		b.WriteString("\n" + dimStyle.Render(line("a creates a person, p attaches to one, i ignores")))
	}
	return strings.TrimRight(b.String(), "\n")
}

// inboxPayloadLines shows the useful parts of an inbox payload.
func inboxPayloadLines(payload string, kv func(k, v string) string) string {
	var p struct {
		Patch  *crmsync.PersonPatch       `json:"patch"`
		Record *crmsync.InteractionRecord `json:"record"`
	}
	if json.Unmarshal([]byte(payload), &p) != nil {
		return ""
	}
	var b strings.Builder
	add := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			b.WriteString(kv(k, v))
		}
	}
	if pt := p.Patch; pt != nil {
		add("title", str(pt.Title))
		add("company", str(pt.Company))
		add("location", str(pt.Location))
		for _, e := range pt.Emails {
			add("email", e)
		}
		for _, ph := range pt.Phones {
			add("phone", ph)
		}
	}
	if rc := p.Record; rc != nil {
		add("type", rc.Type)
		add("subject", str(rc.Subject))
		add("content", firstLine(str(rc.Content)))
		add("occurred", rc.OccurredAt)
		for _, who := range rc.People {
			add("with", strings.Join(append(append([]string{}, who.Emails...), who.Phones...), ", "))
		}
	}
	return b.String()
}

// --- running a source ---

// startSync runs a source in the background. The UI keeps working and shows a spinner.
func (m *Model) startSync(source string, dry bool) tea.Cmd {
	if m.busy != "" {
		m.setStatus("a sync is already running: "+m.busy, true)
		return nil
	}
	if _, err := m.registry.Get(source); err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	m.busy = "syncing " + source
	if dry {
		m.busy = "dry-run " + source
	}
	m.setStatus("", false)
	runner, ctx := m.runner, m.ctx
	work := func() tea.Msg {
		res, err := runner.Run(ctx, source, crmsync.RunOptions{DryRun: dry})
		return syncDoneMsg{source: source, dry: dry, res: res, err: err}
	}
	return tea.Batch(work, m.spin.Tick)
}

// finishSync shows the result line and refreshes the screen.
func (m *Model) finishSync(msg syncDoneMsg) {
	m.busy = ""
	if msg.err != nil {
		m.setStatus(fmt.Sprintf("sync %s: %v", msg.source, msg.err), true)
		return
	}
	res := msg.res
	line := fmt.Sprintf("%s: %s, %d created, %d updated, %d skipped, %d in inbox, %d errors",
		res.Source, res.Status, res.Created, res.Updated, res.Skipped, res.Inbox, res.Errors)
	if msg.dry {
		line = "dry run " + line + " (nothing written)"
	}
	if res.Status == model.SyncFailed && res.Note != "" {
		line += ": " + res.Note
	}
	if !msg.dry {
		m.afterWrite()
	}
	m.setStatus(line, res.Status == model.SyncFailed || res.Status == model.SyncPartial)
}

func (m *Model) selectedSource() string {
	if it := selectedSync(m); it != nil && it.source != nil {
		return it.source.Source
	}
	return ""
}

// --- inbox ---

func (m *Model) selectedEntry() *crmsync.InboxEntry {
	if it := selectedSync(m); it != nil {
		return it.entry
	}
	return nil
}

func (m *Model) acceptInbox() {
	e := m.selectedEntry()
	if e == nil {
		return
	}
	pid, err := m.inbox.Accept(m.ctx, e.ID)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("inbox #%d accepted as person #%d", e.ID, pid), false)
}

func (m *Model) ignoreInbox() {
	e := m.selectedEntry()
	if e == nil {
		return
	}
	if err := m.inbox.Ignore(m.ctx, e.ID); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("inbox #%d ignored", e.ID), false)
}

// attachInbox opens a person search. Candidates the matcher found come first.
func (m *Model) attachInbox() {
	e := m.selectedEntry()
	if e == nil {
		return
	}
	isCand := map[int64]bool{}
	for _, id := range e.Candidates {
		isCand[id] = true
	}
	var items []overlayItem
	add := func(p model.PersonSummary, cand bool) {
		desc := cleanCell(p.Company)
		if desc == "" {
			desc = cleanCell(p.Email)
		}
		hint := ""
		if cand {
			hint = "match"
		}
		items = append(items, overlayItem{id: strconv.FormatInt(p.ID, 10), label: cleanCell(p.Name), hint: hint, desc: desc, value: strconv.FormatInt(p.ID, 10)})
	}
	for _, p := range m.people {
		if isCand[p.ID] {
			add(p, true)
		}
	}
	for _, p := range m.people {
		if !isCand[p.ID] {
			add(p, false)
		}
	}
	entryID := e.ID
	m.openPicker(fmt.Sprintf("Attach inbox #%d to", entryID), items, func(m *Model, it overlayItem) {
		pid, _ := strconv.ParseInt(it.value, 10, 64)
		if err := m.inbox.Attach(m.ctx, entryID, pid); err != nil {
			m.setStatus(err.Error(), true)
			return
		}
		m.afterWrite()
		m.setStatus(fmt.Sprintf("inbox #%d attached to %s", entryID, it.label), false)
	})
}

// openInbox shows the Sync view with the cursor on the first pending inbox record.
func (m *Model) openInbox() {
	m.switchView(viewSync)
	m.focus = focusList
	for i, r := range m.rows {
		if r.sync != nil && r.sync.entry != nil {
			m.cursor = i
			m.clamp()
			m.refreshDetail(false)
			return
		}
	}
	m.setStatus("the inbox is empty", false)
}
