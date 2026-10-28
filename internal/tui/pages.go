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
	kc := m.kindChoices()
	for i, li := range items {
		li := li
		r := formRow{kind: rkLinked, label: label, value: li.Name, id: li.ID, suffix: m.linkSuffix(li), li: &li, other: other, dim: !li.Current}
		if cells := m.linkCells(li, kc); len(cells) > 0 {
			r.cells, r.suffix = cells, ""
		}
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
// Every record page builds them here, with one dim placeholder row when there are none, so Enter on
// it adds the first note.
func (m *Model) timelineRows(f model.InteractionFilters) []formRow {
	f.Limit = 8
	ints, err := m.ir.FindAll(m.ctx, f)
	if err != nil || len(ints) == 0 {
		return []formRow{{kind: rkNote, label: "note", empty: true, head: "Recent"}}
	}
	var rows []formRow
	cells := m.noteCells("")
	for i, it := range ints {
		val := it.Type
		if it.Subject != nil && *it.Subject != "" {
			val += ": " + *it.Subject
		}
		r := formRow{kind: rkNote, label: shortDate(it.OccurredAt), key: val, value: str(it.Content), id: it.ID, typ: it.Type}
		r.cells = []rowCell{{key: "type", label: "type", value: it.Type, spec: cells[0].spec}}
		if i == 0 {
			r.head = "Recent"
		}
		rows = append(rows, r)
	}
	return rows
}

// taskRows lists open tasks linked to an object, with one dim placeholder row when there are none.
func (m *Model) taskRows(f model.TaskFilters) []formRow {
	tasks, err := m.tr.FindAll(m.ctx, f)
	if err != nil || len(tasks) == 0 {
		return []formRow{{kind: rkTask, label: "task", empty: true, head: "Open tasks"}}
	}
	var rows []formRow
	for i, t := range tasks {
		r := formRow{kind: rkTask, label: "task", value: t.Title, id: t.ID}
		if t.DueAt != nil {
			r.suffix = "  due " + *t.DueAt
		}
		for _, tg := range t.Tags {
			r.suffix += "  #" + tg
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
	// Identity: every declared field is a row, so an empty one can be filled in place (model.Fields).
	var rows []formRow
	values := map[string]string{"name": o.Name, "domain": str(o.Domain), "website": str(o.Website), "industry": str(o.Industry), "size": str(o.Size),
		"location": o.Location(), "linkedin": str(o.LinkedinURL), "description": str(o.Description), "notes": str(o.Notes)}
	for _, spec := range model.FieldsOf("organization") {
		if spec.Type == model.FieldRelation {
			continue
		}
		r := specRow(ref, spec, values[spec.Key])
		if len(rows) == 0 {
			r.head = "Identity"
		}
		rows = append(rows, r)
	}
	if lp := str(o.LogoPath); lp != "" {
		rows = append(rows, infoRow("logo", lp))
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
	for i, c := range m.similarOrgs()[id] {
		if i == 3 {
			break
		}
		now = append(now, infoRow("similar name", fmt.Sprintf("%s #%d (%s), space marks, M merges", cleanCell(c.name), c.id, c.reason)))
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

	values := map[string]string{"title": d.Title, "stage": d.Stage, "value": plainNumber(d.Value), "notes": str(d.Notes)}
	var rows []formRow
	for _, spec := range m.fieldsOf("deal") {
		if spec.Type == model.FieldRelation {
			continue
		}
		r := specRow(ref, spec, values[spec.Key])
		if len(rows) == 0 {
			r.head = "Identity"
		}
		rows = append(rows, r)
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

// objectPageEdit handles Enter on a row of an organization or deal page that is not a task or note
// (those edit inline like a person's, see startEdit). Linked rows open the other object.
func (m *Model) objectPageEdit(r formRow) {
	switch r.kind {
	case rkLinked:
		m.pendingCmd = m.openLinkRow(r)
	default:
		m.setStatus("enter on the list edits this record, this row is read-only", true)
	}
}

// startObjectNote adds a note to the organization or deal under the cursor. Tab in the prompt
// changes the type, as it does for a person's note.
func (m *Model) startObjectNote() {
	ref := m.pageRef()
	if ref.Kind == "person" {
		return
	}
	m.noteType = m.currentNoteType()
	m.startInput("note-object", m.objectNotePrompt(ref), "")
	m.pendingCmd = m.input.Focus()
}

func (m *Model) objectNotePrompt(ref repo.Ref) string {
	return m.noteType + " on " + m.refName(ref) + ": "
}

// saveObjectNote adds a note attached to the organization or deal and records it for undo.
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
	if _, err := m.hCreateNote(in); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus(typ+" added to "+m.refName(ref)+" (ctrl+z undoes)", false)
	m.pendingCmd = m.showToast(typ+" added · ctrl+z undoes", false)
}
