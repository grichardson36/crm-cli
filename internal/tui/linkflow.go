package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// The Link form. It is generated from the relation registry (model.Relations): the target comes
// from the pick-or-create picker, a kind from a picker that shows the inverse label, and the other
// attributes (role, dates, primary, notes) on one pipe-separated line. Nothing is written until the
// last step, so Esc anywhere leaves no trace. Create plus link is one undo step.

// linkDraft is a link in the making.
type linkDraft struct {
	src    repo.Ref
	other  string
	rel    model.Relation
	target repo.Ref // ID 0 until the target is created
	create string   // text to create the target from, "" when an existing object was picked
	attrs  model.LinkAttrs
	// editID is the stored link being edited, 0 for a new link. addNew adds another link for a
	// pair that is already linked (a second stay at one organization).
	editID   int64
	editRel  string
	addNew   bool
	existing *model.LinkedObject
	prompted bool
}

func (d *linkDraft) targetName(m *Model) string {
	if d.target.ID != 0 {
		return m.refName(d.target)
	}
	return d.create
}

// linkAttrsPrompt is the prompt of the attribute line: "role | from | to | primary | notes: ".
func linkAttrsPrompt(rel model.Relation) string {
	var labels []string
	for _, k := range rel.AskAttrs() {
		labels = append(labels, strings.ToLower(model.AttrSpecs[k].Label))
	}
	return "Link (" + strings.Join(labels, " | ") + "): "
}

// beginLinkDraft starts the form after the target was picked or typed.
func (m *Model) beginLinkDraft(src repo.Ref, other string, it overlayItem) tea.Cmd {
	rel, ok := model.RelationBetween(src.Kind, other)
	if !ok {
		m.setStatus(src.Kind+" cannot link to "+other, true)
		return nil
	}
	d := &linkDraft{src: src, other: other, rel: rel, target: repo.Ref{Kind: other, ID: it.ref}}
	if it.id == "create" {
		d.create, d.target.ID = it.value, 0
	}
	m.draft = d
	if d.target.ID == 0 {
		return m.nextLinkStep()
	}
	if d.src.Kind == d.target.Kind && d.src.ID == d.target.ID {
		m.draft = nil
		m.setStatus("an object cannot be linked to itself", true)
		return nil
	}
	// A pair may be linked under several kinds (friend and colleague), so a kind relation asks its
	// kind first. Link then keeps one link per kind. Edit link (E) changes an existing one.
	if rel.Kinded {
		return m.nextLinkStep()
	}
	existing := m.linksBetween(src, d.target)
	if len(existing) == 0 {
		return m.nextLinkStep()
	}
	// Already linked: edit that link, or add another one where a pair may repeat.
	if len(existing) == 1 && rel.Unique {
		m.useExisting(d, existing[0])
		return m.nextLinkStep()
	}
	var items []overlayItem
	for i := range existing {
		e := existing[i]
		items = append(items, overlayItem{id: fmt.Sprintf("edit-%d", e.LinkID), label: "Edit: " + linkSummary(e), value: fmt.Sprint(i), ref: e.LinkID})
	}
	if !rel.Unique {
		items = append(items, overlayItem{id: "add-new", label: "Add another link", desc: "for example a second stay at the same organization"})
	}
	m.openPicker(m.refName(src)+" is already linked to "+d.targetName(m), items, func(m *Model, it overlayItem) {
		if it.id == "add-new" {
			d.addNew = true
		} else {
			var idx int
			fmt.Sscan(it.value, &idx)
			m.useExisting(d, existing[idx])
		}
		m.pendingCmd = m.nextLinkStep()
	})
	return nil
}

func (m *Model) useExisting(d *linkDraft, e model.LinkedObject) {
	d.editID, d.editRel, d.existing = e.LinkID, e.Relation, &e
	d.attrs.Kind = nil
}

// linksBetween lists the live links from a to b.
func (m *Model) linksBetween(a, b repo.Ref) []model.LinkedObject {
	items, err := m.links.Linked(m.ctx, a, b.Kind)
	if err != nil {
		return nil
	}
	var out []model.LinkedObject
	for _, it := range items {
		if it.ID == b.ID {
			out = append(out, it)
		}
	}
	return out
}

// editLink edits the attributes of one stored link of src.
func (m *Model) editLink(src repo.Ref, li model.LinkedObject) tea.Cmd {
	rel, ok := model.RelationNamed(li.Relation)
	if !ok {
		return nil
	}
	if len(rel.AskAttrs()) == 0 && !rel.Kinded {
		m.setStatus("this link has no attributes to edit", true)
		return nil
	}
	d := &linkDraft{src: src, other: li.Kind, rel: rel, target: repo.Ref{Kind: li.Kind, ID: li.ID}}
	m.useExisting(d, li)
	m.draft = d
	if rel.Kinded {
		return m.pickKind(d)
	}
	return m.nextLinkStep()
}

// nextLinkStep asks the next thing the form needs, or writes the link.
func (m *Model) nextLinkStep() tea.Cmd {
	d := m.draft
	if d == nil {
		return nil
	}
	if d.rel.Kinded && d.attrs.Kind == nil && d.existing == nil {
		return m.pickKind(d)
	}
	if len(d.rel.AskAttrs()) > 0 && !d.prompted {
		return m.askLinkAttrs(d)
	}
	return m.finishLink(d)
}

// pickKind lists the relation kinds with both labels, so the direction is clear before it is saved.
func (m *Model) pickKind(d *linkDraft) tea.Cmd {
	kinds, err := repo.NewKindRepo(m.db).List(m.ctx, d.rel.A)
	if err != nil {
		m.setStatus(err.Error(), true)
		m.draft = nil
		return nil
	}
	srcName, tgtName := m.refName(d.src), d.targetName(m)
	var items []overlayItem
	for _, k := range kinds {
		desc := "mutual"
		if !k.Symmetric {
			desc = fmt.Sprintf("%s is %s · %s is %s", srcName, k.Name, tgtName, k.InverseName)
			if k.Phrase {
				desc = fmt.Sprintf("%s %s %s · %s %s %s", srcName, k.Name, tgtName, tgtName, k.InverseName, srcName)
			}
		}
		it := overlayItem{id: "kind-" + k.Name, label: k.Name, desc: desc, value: k.Name}
		if d.existing != nil && strings.EqualFold(d.existing.Label, k.Name) || d.existing != nil && strings.EqualFold(d.existing.Label, k.InverseName) {
			it.desc += " (current)"
		}
		items = append(items, it)
	}
	m.openPicker("Kind of link between "+srcName+" and "+tgtName, items, func(m *Model, it overlayItem) {
		name := it.value
		d.attrs.Kind = &name
		m.pendingCmd = m.nextLinkStep()
	})
	return nil
}

// askLinkAttrs opens the one-line attribute prompt, filled from the link being edited.
func (m *Model) askLinkAttrs(d *linkDraft) tea.Cmd {
	var vals []string
	for _, k := range d.rel.AskAttrs() {
		v := ""
		if e := d.existing; e != nil {
			switch k {
			case model.AttrRole:
				v = str(e.Role)
			case model.AttrFrom:
				v = str(e.StartedOn)
			case model.AttrTo:
				v = str(e.EndedOn)
			case model.AttrPrimary:
				if e.Primary {
					v = "yes"
				}
			case model.AttrNotes:
				v = cleanCell(str(e.Notes))
			}
		}
		vals = append(vals, v)
	}
	m.startInput("link-attrs", linkAttrsPrompt(d.rel), strings.TrimRight(strings.Join(vals, " | "), " |"))
	if d.rel.HasAttr(model.AttrRole) {
		if roles, err := m.links.Roles(m.ctx, d.rel.Name, 6); err == nil && len(roles) > 0 {
			m.setStatus("roles in use: "+strings.Join(roles, ", ")+" (tab completes)", false)
		}
	}
	return m.input.Focus()
}

// completeRole is Tab in the attribute prompt: it completes the first field from the roles in use.
func (m *Model) completeRole() {
	d := m.draft
	if d == nil || !d.rel.HasAttr(model.AttrRole) || len(d.rel.AskAttrs()) == 0 || d.rel.AskAttrs()[0] != model.AttrRole {
		return
	}
	first, rest, _ := strings.Cut(m.input.Value(), "|")
	typed := strings.TrimSpace(first)
	roles, err := m.links.Roles(m.ctx, d.rel.Name, 50)
	if err != nil {
		return
	}
	for _, r := range roles {
		if strings.HasPrefix(strings.ToLower(r), strings.ToLower(typed)) && !strings.EqualFold(r, typed) {
			v := r
			if strings.Contains(m.input.Value(), "|") {
				v += " |" + rest
			}
			m.input.SetValue(v)
			m.input.CursorEnd()
			return
		}
	}
}

// runLinkAttrs reads the attribute line and goes on.
func (m *Model) runLinkAttrs(line string) {
	d := m.draft
	if d == nil {
		return
	}
	keys := d.rel.AskAttrs()
	parts := fields(line, len(keys))
	for i, k := range keys {
		v := parts[i]
		text := func() *string {
			if v == "" {
				if d.existing != nil {
					e := ""
					return &e
				}
				return nil
			}
			return &v
		}
		switch k {
		case model.AttrRole:
			d.attrs.Role = text()
		case model.AttrFrom:
			d.attrs.StartedOn = text()
		case model.AttrTo:
			d.attrs.EndedOn = text()
		case model.AttrNotes:
			d.attrs.Notes = text()
		case model.AttrPrimary:
			switch strings.ToLower(v) {
			case "y", "yes", "true", "1", "primary":
				t := true
				d.attrs.Primary = &t
			case "n", "no", "false", "0":
				f := false
				d.attrs.Primary = &f
			}
		}
	}
	d.prompted = true
	m.pendingCmd = m.nextLinkStep()
}

// finishLink writes the link. Creating the target and linking it are one undo step.
func (m *Model) finishLink(d *linkDraft) tea.Cmd {
	m.draft = nil
	m.beginGroup()
	defer m.afterWrite()
	src := d.src
	name := m.refName(src)
	if d.target.ID == 0 {
		o := objectByKind(d.other)
		id, err := o.create(m, d.create)
		if err != nil {
			m.endGroup("")
			m.setStatus(err.Error(), true)
			return m.showToast(err.Error(), true)
		}
		d.target.ID = id
	}
	targetName := m.refName(d.target)
	var res repo.LinkResult
	label := "link " + name + " to " + targetName
	if d.editID != 0 {
		label = "edit link of " + name + " and " + targetName
	}
	err := m.recordLinkChange(label, src, d.target, func() error {
		var err error
		switch {
		case d.editID != 0:
			res, err = m.links.Update(m.ctx, d.editRel, d.editID, d.attrs)
		case d.addNew:
			res, err = m.links.LinkNew(m.ctx, src, d.target, d.attrs)
		default:
			res, err = m.links.Link(m.ctx, src, d.target, d.attrs)
		}
		return err
	})
	m.endGroup(label)
	switch {
	case err != nil:
		m.setStatus(err.Error(), true)
		return m.showToast(err.Error(), true)
	case !res.Changed:
		msg := name + " is already linked to " + targetName
		m.setStatus(msg, false)
		return m.showToast(msg, false)
	}
	verb := "linked " + name + " to " + targetName
	if res.Updated && !res.Created {
		verb = "saved the link of " + name + " and " + targetName
	}
	m.setStatus(verb+" (ctrl+z undoes)", false)
	return m.showToast(verb+" · ctrl+z undoes", false)
}

// recordLinkChange runs op and records it for undo and redo. It snapshots the stored link rows
// the change can touch before and after, so undo puts back exactly what was there: attributes,
// primary flags and deleted state. Undo or redo is refused when those rows changed since.
func (m *Model) recordLinkChange(label string, a, b repo.Ref, op func() error) error {
	before, err := m.links.Snapshot(m.ctx, a, b)
	if err != nil {
		return err
	}
	if err := op(); err != nil {
		return err
	}
	after, err := m.links.Snapshot(m.ctx, a, b)
	if err != nil {
		return err
	}
	if repo.SameRecords(before, after) {
		return nil
	}
	undoSet := repo.UndoSet(before, after)
	guard := func(expected []repo.LinkRecord) error {
		cur, err := m.links.Snapshot(m.ctx, a, b)
		if err != nil {
			return err
		}
		if !repo.SameRecords(cur, expected) {
			return staleError{m.refName(a) + " and " + m.refName(b) + " links changed since"}
		}
		return nil
	}
	m.record(label,
		func() error {
			if err := guard(after); err != nil {
				return err
			}
			return m.links.Apply(m.ctx, undoSet)
		},
		func() error {
			if err := guard(undoSet); err != nil {
				return err
			}
			return m.links.Apply(m.ctx, after)
		})
	return nil
}

// unlinkRecorded removes one stored link (linkID) or, with 0, every link between a and b.
func (m *Model) unlinkRecorded(a, b repo.Ref, linkID int64) error {
	label := "unlink " + m.refName(a) + " from " + m.refName(b)
	return m.recordLinkChange(label, a, b, func() error {
		_, err := m.links.Unlink(m.ctx, a, b, repo.UnlinkOptions{LinkID: linkID})
		return err
	})
}

// linkSummary describes a link in a list: the label or role, the dates and the primary flag.
func linkSummary(li model.LinkedObject) string {
	var parts []string
	if li.Label != "" {
		parts = append(parts, li.Label)
	}
	if li.Role != nil && *li.Role != "" {
		parts = append(parts, *li.Role)
	}
	if d := linkDates(li); d != "" {
		parts = append(parts, d)
	}
	if li.Primary {
		parts = append(parts, "primary")
	}
	name := cleanCell(li.Name)
	if len(parts) == 0 {
		return name
	}
	return name + " (" + strings.Join(parts, ", ") + ")"
}

// linkDates formats the dates of a link: 2019-2021, 2019- (current since), -2021.
func linkDates(li model.LinkedObject) string {
	from, to := str(li.StartedOn), str(li.EndedOn)
	switch {
	case from == "" && to == "":
		return ""
	case to == "":
		return "since " + from
	case from == "":
		return "until " + to
	}
	return from + " to " + to
}
