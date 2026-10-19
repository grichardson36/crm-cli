package tui

import (
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Record pages (patterns/one-record-page): every object opens Identity, Now and Context. Context is
// the links, then the timeline. People, organizations and deals build their rows here and the
// detail pane draws them the same way.

// alwaysShow says which link sections appear on a page even when empty, as the place to add the
// first link. The others appear when they have rows (advanced objects stay off the main path).
func alwaysShow(src, other string) bool {
	switch {
	case src == "person" && other == "organization":
		return true
	case src == "organization" && other == "person":
		return true
	case src == "deal" && (other == "organization" || other == "person"):
		return true
	}
	return false
}

// linkSuffix is the dim text after a linked row: role, dates, primary flag, deal stage.
func (m *Model) linkSuffix(li model.LinkedObject) string {
	var parts []string
	if li.Role != nil && *li.Role != "" {
		parts = append(parts, *li.Role)
	}
	if d := linkDates(li); d != "" {
		parts = append(parts, d)
	}
	if !li.Current {
		parts = append(parts, "past")
	}
	if li.Primary {
		parts = append(parts, "primary")
	}
	if li.Kind == "deal" {
		if d, err := m.dlr.FindByID(m.ctx, li.ID); err == nil {
			parts = append(parts, d.Stage)
			if d.Value != nil {
				parts = append(parts, money(d.Value))
			}
		}
	}
	if li.Notes != nil && *li.Notes != "" {
		parts = append(parts, cleanCell(*li.Notes))
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, " · ")
}

// linkRows builds the rows of one link section of src's page. Past memberships come after current
// ones and are drawn dim, with their dates.
func (m *Model) linkRows(src repo.Ref, other, head, label string) []formRow {
	items, err := m.links.Linked(m.ctx, src, other)
	if err != nil {
		return nil
	}
	return m.rowsFromLinks(items, src.Kind, other, head, label)
}

func (m *Model) rowsFromLinks(items []model.LinkedObject, srcKind, other, head, label string) []formRow {
	if len(items) == 0 {
		if !alwaysShow(srcKind, other) {
			return nil
		}
		// An empty section is one dim row without a heading, so a bare page stays short.
		return []formRow{{kind: rkLinked, label: label, empty: true, other: other}}
	}
	var rows []formRow
	for i, li := range items {
		li := li
		r := formRow{kind: rkLinked, label: label, value: li.Name, id: li.ID, suffix: m.linkSuffix(li), li: &li, other: other, dim: !li.Current}
		if li.Label != "" {
			r.label = li.Label
		}
		if i == 0 {
			r.head = head
		}
		rows = append(rows, r)
	}
	return rows
}

// infoRow is a read-only labelled row.
func infoRow(label, value string) formRow { return formRow{kind: rkInfo, label: label, value: value} }

// timelineRows lists the notes attached to an object, newest first, as note rows under a heading.
func (m *Model) timelineRows(f model.InteractionFilters) []formRow {
	f.Limit = 8
	ints, err := m.ir.FindAll(m.ctx, f)
	if err != nil || len(ints) == 0 {
		return nil
	}
	var rows []formRow
	for i, it := range ints {
		date := it.OccurredAt
		if len(date) > 10 {
			date = date[:10]
		}
		val := it.Type
		if it.Subject != nil && *it.Subject != "" {
			val += ": " + *it.Subject
		}
		r := formRow{kind: rkNote, label: date, key: val, value: str(it.Content), id: it.ID, typ: it.Type}
		if i == 0 {
			r.head = "Recent"
		}
		rows = append(rows, r)
	}
	return rows
}

// taskRows lists open tasks linked to an object.
func (m *Model) taskRows(f model.TaskFilters) []formRow {
	tasks, err := m.tr.FindAll(m.ctx, f)
	if err != nil || len(tasks) == 0 {
		return nil
	}
	var rows []formRow
	for i, t := range tasks {
		r := formRow{kind: rkTask, label: "task", value: t.Title, id: t.ID}
		if t.DueAt != nil {
			r.suffix = "  due " + *t.DueAt
		}
		if i == 0 {
			r.head = "Open tasks"
		}
		rows = append(rows, r)
	}
	return rows
}

// buildOrgForm is the record page of an organization.
func (m *Model) buildOrgForm(id int64) []formRow {
	m.formTitle = ""
	o, err := m.orr.FindByID(m.ctx, id)
	if err != nil {
		return []formRow{infoRow("organization", err.Error())}
	}
	ref := repo.Ref{Kind: "organization", ID: id}
	m.formTitle = fmt.Sprintf("%s  #%d", cleanCell(o.Name), o.ID)

	// Identity
	var rows []formRow
	first := true
	for _, kv := range [][2]string{{"domain", str(o.Domain)}, {"industry", str(o.Industry)}, {"notes", cleanCell(str(o.Notes))}} {
		if kv[1] == "" {
			continue
		}
		r := infoRow(kv[0], kv[1])
		if first {
			r.head, first = "Identity", false
		}
		rows = append(rows, r)
	}
	// Now: what is open around this organization.
	var now []formRow
	if deals, err := m.dlr.FindAll(m.ctx, model.DealFilters{OrgID: &id, ExcludeClosed: true}); err == nil && len(deals) > 0 {
		now = append(now, infoRow("open deals", fmt.Sprint(len(deals))))
	}
	if tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{OrgID: &id, Limit: 1}); err == nil && len(tasks) > 0 {
		v := tasks[0].Title
		if tasks[0].DueAt != nil {
			v += "  due " + *tasks[0].DueAt
		}
		now = append(now, infoRow("next task", v))
	}
	if len(now) > 0 {
		now[0].head = "Now"
		rows = append(rows, now...)
	}

	// Context: links, then the timeline.
	rows = append(rows, m.linkRows(ref, "person", "Members", "member")...)
	orgs, _ := m.links.Linked(m.ctx, ref, "organization")
	var parents, children, others []model.LinkedObject
	for _, li := range orgs {
		switch li.Label {
		case "parent":
			parents = append(parents, li)
		case "child":
			children = append(children, li)
		default:
			others = append(others, li)
		}
	}
	rows = append(rows, m.rowsFromLinks(parents, "organization", "organization", "Parent organizations", "parent")...)
	rows = append(rows, m.rowsFromLinks(children, "organization", "organization", "Child organizations", "child")...)
	rows = append(rows, m.rowsFromLinks(others, "organization", "organization", "Related organizations", "related")...)
	rows = append(rows, m.linkRows(ref, "deal", "Deals", "deal")...)
	rows = append(rows, m.taskRows(model.TaskFilters{OrgID: &id})...)
	rows = append(rows, m.timelineRows(model.InteractionFilters{OrgID: &id})...)
	return rows
}

// buildDealForm is the record page of a deal.
func (m *Model) buildDealForm(id int64) []formRow {
	m.formTitle = ""
	d, err := m.dlr.FindByID(m.ctx, id)
	if err != nil {
		return []formRow{infoRow("deal", err.Error())}
	}
	ref := repo.Ref{Kind: "deal", ID: id}
	m.formTitle = fmt.Sprintf("%s  #%d", cleanCell(d.Title), d.ID)

	rows := []formRow{infoRow("stage", d.Stage)}
	rows[0].head = "Identity"
	if d.Value != nil {
		rows = append(rows, infoRow("value", money(d.Value)))
	}
	if d.Notes != nil && *d.Notes != "" {
		rows = append(rows, infoRow("notes", cleanCell(*d.Notes)))
	}
	// Now
	var now []formRow
	if d.ClosedAt != nil {
		now = append(now, infoRow("closed", *d.ClosedAt))
	}
	if tasks, err := m.tr.FindAll(m.ctx, model.TaskFilters{DealID: &id, Limit: 1}); err == nil && len(tasks) > 0 {
		v := tasks[0].Title
		if tasks[0].DueAt != nil {
			v += "  due " + *tasks[0].DueAt
		}
		now = append(now, infoRow("next task", v))
	}
	if len(now) > 0 {
		now[0].head = "Now"
		rows = append(rows, now...)
	}
	rows = append(rows, m.linkRows(ref, "organization", "Organizations", "organization")...)
	rows = append(rows, m.linkRows(ref, "person", "People", "person")...)
	rows = append(rows, m.taskRows(model.TaskFilters{DealID: &id})...)
	rows = append(rows, m.timelineRows(model.InteractionFilters{DealID: &id})...)
	return rows
}

// onObjectPage is true while the detail pane shows an organization or a deal.
func (m *Model) onObjectPage() bool { return m.view == viewOrgs || m.view == viewDeals }

// objectPageEdit handles Enter on a row of an organization or deal page. Linked rows open the
// other object, task rows open the task, the rest is read-only here.
func (m *Model) objectPageEdit(r formRow) {
	switch r.kind {
	case rkLinked:
		m.pendingCmd = m.openLinkRow(r)
	case rkTask:
		m.openObject("task", r.id)
	case rkNote:
		m.setStatus("notes are edited on their person's page, n adds a note here", false)
	default:
		m.setStatus("enter on the list edits this record, this row is read-only", true)
	}
}

// startObjectNote adds a note to the organization or deal under the cursor.
func (m *Model) startObjectNote() {
	ref := m.pageRef()
	if ref.Kind == "person" {
		return
	}
	m.startInput("note-object", m.currentNoteType()+" on "+m.refName(ref)+": ", "")
	m.pendingCmd = m.input.Focus()
}

// saveObjectNote logs a note attached to the organization or deal and records it for undo.
func (m *Model) saveObjectNote(text string) {
	ref := m.pageRef()
	typ := m.currentNoteType()
	in := model.CreateInteractionInput{Type: typ, Content: &text}
	if ref.Kind == "organization" {
		in.OrgIDs = []int64{ref.ID}
	} else if ref.Kind == "deal" {
		in.DealIDs = []int64{ref.ID}
	} else {
		return
	}
	it, err := m.ir.Create(m.ctx, in)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	id := it.ID
	m.record("add "+typ+" to "+m.refName(ref),
		func() error { return m.ir.Archive(m.ctx, id) },
		func() error { return m.ir.Restore(m.ctx, id) })
	m.afterWrite()
	m.setStatus(typ+" added to "+m.refName(ref)+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(typ+" added · ctrl+z undoes", false)
}
