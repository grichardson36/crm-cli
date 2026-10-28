package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	sg "github.com/jdanielnd/crm-cli/internal/suggest"
)

// Links are pipe segments (owner decision 2026-10-29, replacing the grayscale tab-to-accept offer of the
// add picker). A link segment holds object names separated by commas. Nothing is linked while the text
// is typed: backspacing over a name removes that link before it is saved. On enter every name is
// resolved to an object, an unresolved name offers pick-or-create, and the links are written through the
// existing link path (afterCreate for a new object, the deal sync for an edit).

// pendingLink is one link to write: an object that exists (ref), an object to create first (create),
// or a tag.
type pendingLink struct {
	ref     repo.Ref
	create  string
	tag     string
	primary bool
	label   string
}

type resolveState int

const (
	unresolved resolveState = iota
	resolved
	ambiguous
)

// resolution is the outcome of reading one name.
type resolution struct {
	state   resolveState
	link    pendingLink
	matches []Suggestion // the objects the name could be, for the picker
}

// resKey identifies a name in the answers remembered for the line being saved.
func resKey(kinds []string, text string) string {
	return strings.Join(kinds, ",") + ":" + strings.ToLower(strings.TrimSpace(text))
}

// autoResolve reads one name of a link segment. An answer given before (accepted from the popup or picked
// at enter) wins. Then an id, an exact name, and a name that only one object fits. A tag name is the tag.
func (m *Model) autoResolve(s *pipeSession, seg pipeSeg, text string) resolution {
	hash := strings.HasPrefix(strings.TrimSpace(text), "#")
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "#"))
	if l, ok := m.pipeRes[resKey(seg.Kinds, text)]; ok {
		return resolution{state: resolved, link: l}
	}
	if hash && segTakes(seg, tagKind) && text != "" { // #name is a tag, new or not
		for _, it := range m.pipeCandidates(s, tagKind) {
			if strings.EqualFold(it.label, text) {
				text = it.label
			}
		}
		return resolution{state: resolved, link: pendingLink{tag: text, label: "#" + text}}
	}
	if len(seg.Kinds) == 1 && seg.Kinds[0] == tagKind {
		for _, it := range m.pipeCandidates(s, tagKind) {
			if strings.EqualFold(it.label, text) {
				return resolution{state: resolved, link: pendingLink{tag: it.label, label: "#" + it.label}}
			}
		}
		return resolution{state: resolved, link: pendingLink{tag: text, label: "#" + text}}
	}
	type cand struct {
		kind string
		it   overlayItem
	}
	var all []cand
	for _, k := range seg.Kinds {
		if k == tagKind {
			continue // tags are named with a #, or match below when nothing else does
		}
		for _, it := range m.pipeCandidates(s, k) {
			all = append(all, cand{k, it})
		}
	}
	row := func(c cand) Suggestion {
		return Suggestion{Label: c.it.label, Desc: c.it.desc, Ref: repo.Ref{Kind: c.kind, ID: c.it.ref}}
	}
	one := func(cs []cand) resolution {
		return resolution{state: resolved, link: pendingLink{ref: repo.Ref{Kind: cs[0].kind, ID: cs[0].it.ref}, label: cs[0].it.label}}
	}
	many := func(cs []cand) resolution {
		r := resolution{state: ambiguous}
		for _, c := range cs {
			r.matches = append(r.matches, row(c))
		}
		return r
	}
	if id, err := strconv.ParseInt(text, 10, 64); err == nil && len(seg.Kinds) == 1 {
		for _, c := range all {
			if c.it.ref == id {
				return one([]cand{c})
			}
		}
	}
	var exact, partial []cand
	low := strings.ToLower(text)
	words := strings.Fields(low)
	for _, c := range all {
		name := strings.ToLower(c.it.label)
		switch {
		case name == low:
			exact = append(exact, c)
		case sg.Score(name, low, words, m.aliasTable()) >= 0:
			partial = append(partial, c)
		}
	}
	for _, list := range [][]cand{exact, partial} {
		switch len(list) {
		case 0:
		case 1:
			return one(list)
		default:
			return many(list)
		}
	}
	if segTakes(seg, tagKind) { // a plain word that names an existing tag and nothing else
		for _, it := range m.pipeCandidates(s, tagKind) {
			if strings.EqualFold(it.label, text) {
				return resolution{state: resolved, link: pendingLink{tag: it.label, label: "#" + it.label}}
			}
		}
	}
	res := resolution{state: unresolved}
	var rows []Suggestion
	for _, c := range all {
		rows = append(rows, row(c))
	}
	res.matches = sg.Rank(rows, text, func(r Suggestion) string { return r.Label }, sg.Options{Aliases: m.aliasTable(), Limit: acMaxRows})
	return res
}

// pipePrepare checks and resolves a pipe line before its save code runs. It returns true when the save
// can go on, with the links to write in m.pipeLinks. It returns false when it showed an error or opened a
// picker: the picker runs the prompt again with the answers it collected. reopen puts the line back
// into its input with the text kept.
func (m *Model) pipePrepare(kind, value string, reopen func()) bool {
	pl := m.pipeFor(kind)
	m.pipeLinks, m.pipeObj = nil, ""
	if pl == nil {
		return true
	}
	if _, issue, ok := m.pipeFirstError(pl, value, -1); ok {
		msg := issueText(issue)
		m.setStatus(msg, true)
		m.pendingCmd = m.showToast(msg, true)
		reopen()
		return false
	}
	s := &pipeSession{kind: kind, pl: pl, cand: map[string][]overlayItem{}}
	var links []pendingLink
	seen := map[string]bool{}
	firstOrg := true
	for _, t := range pl.tokens(value) {
		seg := pl.segs[t.Seg]
		res := m.autoResolve(s, seg, t.Text)
		if res.state != resolved {
			m.pickLinkToken(kind, value, seg, t.Text, res, reopen)
			return false
		}
		l := res.link
		key := fmt.Sprintf("%s/%d/%s/%s", l.ref.Kind, l.ref.ID, strings.ToLower(l.tag), strings.ToLower(l.create))
		if seen[key] {
			continue
		}
		seen[key] = true
		if pl.obj == "deal" && l.ref.Kind == "organization" && firstOrg {
			l.primary, firstOrg = true, false
		}
		links = append(links, l)
	}
	m.pipeLinks, m.pipeObj = links, pl.obj
	return true
}

// pickLinkToken opens the pick-or-create picker for a name that is not one object yet. The answer is
// remembered and the prompt runs again, so every unresolved name is asked in turn.
func (m *Model) pickLinkToken(kind, value string, seg pipeSeg, text string, res resolution, reopen func()) {
	single := len(seg.Kinds) == 1
	var items []overlayItem
	for _, r := range res.matches {
		items = append(items, overlayItem{id: "pick", label: r.Label, desc: r.Desc, ref: r.Ref.ID, value: r.Ref.Kind})
	}
	word := linkWord(seg.Kinds[0])
	title := fmt.Sprintf("%s: %q", word, text)
	if !single {
		title = fmt.Sprintf("Which one is %q", text)
	}
	if !single { // one related segment: the picker also offers to create the name as any kind the segment takes
		title = fmt.Sprintf("Which one is %q", text)
		if len(items) == 0 {
			title = fmt.Sprintf("%q is new", text)
		}
		for _, k := range seg.Kinds {
			if k == tagKind {
				items = append(items, overlayItem{id: "tag", label: fmt.Sprintf("Tag #%s", text), desc: "put the tag on it", value: tagKind})
			} else if o := objectByKind(k); o != nil && o.create != nil {
				items = append(items, overlayItem{id: "create", label: fmt.Sprintf("Create %s %q", relationLabel(k), text), desc: "make it and link it", value: k})
			}
		}
	}
	key := resKey(seg.Kinds, text)
	again := func(m *Model) { m.runPrompt(kind, value) }
	m.openPicker(title, items, func(m *Model, it overlayItem) {
		if it.id == "tag" {
			m.pipeRes[key] = pendingLink{tag: text, label: "#" + text}
			again(m)
			return
		}
		if it.id == "create" {
			ckind := seg.Kinds[0]
			if !single {
				ckind = it.value
			}
			o := objectByKind(ckind)
			if o != nil && o.kind == "organization" && !m.orgCreateAnyway {
				if dup, reason, err := m.orr.FindDuplicate(m.ctx, text, nil); err == nil && dup != nil {
					m.askOrgDuplicate(dup, reason, func(d *model.Organization) {
						m.pipeRes[key] = pendingLink{ref: repo.Ref{Kind: "organization", ID: d.ID}, label: d.Name}
						again(m)
					}, func() {
						m.pipeRes[key] = pendingLink{ref: repo.Ref{Kind: "organization"}, create: text, label: text}
						again(m)
					})
					m.ov.onCancel = func(m *Model) { m.closeOverlay(); reopen() }
					return
				}
			}
			m.pipeRes[key] = pendingLink{ref: repo.Ref{Kind: ckind}, create: text, label: text}
		} else {
			m.pipeRes[key] = pendingLink{ref: repo.Ref{Kind: it.value, ID: it.ref}, label: it.label}
		}
		again(m)
	})
	m.ov.onCancel = func(m *Model) { m.closeOverlay(); reopen() }
	if single && seg.Kinds[0] != tagKind {
		m.ov.create = func(string) (overlayItem, bool) {
			return overlayItem{id: "create", label: fmt.Sprintf("Create %s %q", relationLabel(seg.Kinds[0]), text), desc: "make it and link it", value: text}, true
		}
		m.ov.refilter()
	}
}

// materialize turns a pending link into an object that exists, creating it when the name was new.
func (m *Model) materialize(l pendingLink) (repo.Ref, error) {
	if l.ref.ID != 0 || l.create == "" {
		return l.ref, nil
	}
	o := objectByKind(l.ref.Kind)
	if o == nil || o.create == nil {
		return repo.Ref{}, fmt.Errorf("cannot create %s %q", l.ref.Kind, l.create)
	}
	id, err := o.create(m, l.create)
	if err != nil {
		return repo.Ref{}, err
	}
	return repo.Ref{Kind: l.ref.Kind, ID: id}, nil
}

// applyLink writes one pending link from src through the existing link path.
func (m *Model) applyLink(src repo.Ref, l pendingLink) error {
	if l.tag != "" {
		return m.hTagApply(m.ctx, src.Kind, src.ID, l.tag)
	}
	target, err := m.materialize(l)
	if err != nil {
		return err
	}
	if target == src {
		return nil
	}
	var attrs model.LinkAttrs
	if l.primary {
		yes := true
		attrs.Primary = &yes
	}
	return m.recordLinkChange("link "+m.refName(src)+" to "+m.refName(target), src, target, func() error {
		_, e := m.links.Link(m.ctx, src, target, attrs)
		return e
	})
}

// afterCreate writes the links of the pipe line that made a new object (owner decision 2026-10-29). The
// create and the links are one undo step. A failed link leaves the object created and says so.
func (m *Model) afterCreate(kind string, id int64) {
	if len(m.pipeLinks) == 0 || (m.pipeObj != "" && m.pipeObj != kind) {
		return
	}
	links := m.pipeLinks
	m.pipeLinks, m.pipeObj = nil, ""
	// Fold the entry the create just recorded into the same step as the links.
	var created *histEntry
	if m.grpDepth == 0 && len(m.hist) > 0 {
		e := m.hist[len(m.hist)-1]
		m.hist = m.hist[:len(m.hist)-1]
		created = &e
	}
	m.beginGroup()
	if created != nil {
		m.grp = append(m.grp, *created)
	}
	ref := repo.Ref{Kind: kind, ID: id}
	var names []string
	var firstErr error
	for _, l := range links {
		if err := m.applyLink(ref, l); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("created, but could not link to %s: %w", l.label, err)
		}
		names = append(names, l.label)
	}
	m.endGroup("add " + relationLabel(kind) + " and link it to " + strings.Join(names, ", "))
	m.pipeRes = map[string]pendingLink{}
	if firstErr != nil {
		m.setStatus(firstErr.Error(), true)
	}
}

// syncLinks makes the objects of one kind linked to src the ones in want: it links the new ones and
// unlinks the ones that were taken out of the line. It is the save of a link segment on an edit.
func (m *Model) syncLinks(src repo.Ref, kind string, want []pendingLink) error {
	cur, err := m.links.Linked(m.ctx, src, kind)
	if err != nil {
		return err
	}
	have := map[int64]bool{}
	for _, li := range cur {
		have[li.ID] = true
	}
	keep := map[int64]bool{}
	for _, l := range want {
		if l.ref.Kind != kind {
			continue
		}
		target, err := m.materialize(l)
		if err != nil {
			return err
		}
		keep[target.ID] = true
		if have[target.ID] {
			continue
		}
		l.ref = target
		if err := m.applyLink(src, l); err != nil {
			return err
		}
	}
	for id := range have {
		if !keep[id] {
			if err := m.unlinkRecorded(src, repo.Ref{Kind: kind, ID: id}, 0); err != nil {
				return err
			}
		}
	}
	return nil
}
