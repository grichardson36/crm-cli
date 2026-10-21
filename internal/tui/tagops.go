package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
)

// Tag rename and merge (X25). Registry actions in the Tags group: "Rename tag" and "Merge tag into
// another" work from the palette (they ask which tag first) and, with their keys r and m, on a tag row
// in the bar. A merge shows what moves and asks y/n. Both are one undo step.

func tagOpsActions() []action {
	inRow := func(m *Model) bool { return m.barTag != "" }
	return []action{
		{id: "tag-rename", title: "Rename tag", group: "Tags", keys: []string{"r"}, keyIf: inRow, tagVerb: true, write: true, footLabel: "rename",
			desc: "give a tag a new name. A name that exists offers a merge instead. Same as crm tag rename",
			run:  func(m *Model) tea.Cmd { m.askTagRename(m.barTag); return nil }},
		{id: "tag-merge", title: "Merge tag into another", group: "Tags", keys: []string{"m"}, keyIf: inRow, tagVerb: true, write: true, footLabel: "merge",
			desc: "move every record of this tag to another tag and remove this one (y/n). Same as crm tag merge",
			run:  func(m *Model) tea.Cmd { m.askTagMerge(m.barTag); return nil }},
	}
}

// pickTag lists every tag (except the one given) and runs onPick with the name.
func (m *Model) pickTag(title, except string, onPick func(name string)) {
	tags, err := m.tagr.FindAll(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	var items []overlayItem
	for _, t := range tags {
		if t.Name == except {
			continue
		}
		items = append(items, overlayItem{id: "tag:" + t.Name, label: "#" + t.Name, value: t.Name})
	}
	if len(items) == 0 {
		m.setStatus("there is no tag to pick", true)
		return
	}
	m.openPicker(title, items, func(m *Model, it overlayItem) { onPick(it.value) })
}

func (m *Model) askTagRename(name string) {
	if name == "" {
		m.pickTag("Rename tag", "", func(n string) { m.askTagRename(n) })
		return
	}
	m.tagRenaming = name
	m.startInput("tag-rename", "rename #"+name+" to: ", name)
}

// runTagRename applies the new name typed in the prompt.
func (m *Model) runTagRename(value string) {
	from, to := m.tagRenaming, repo.NormalizeTagName(value)
	if to == "" || to == from {
		return
	}
	err := m.tagr.Rename(m.ctx, from, to)
	var exists *repo.TagExistsError
	if errors.As(err, &exists) {
		// The name is taken: offer the merge the error talks about.
		m.setStatus(err.Error(), true)
		m.askTagMergeWith(from, exists.Existing, fmt.Sprintf("tag #%s exists. ", exists.Existing))
		return
	}
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.record("rename tag #"+from+" to #"+to,
		func() error { return m.tagr.Rename(m.ctx, to, from) },
		func() error { return m.tagr.Rename(m.ctx, from, to) })
	m.afterWrite()
	m.setStatus("tag #"+from+" renamed to #"+to+" (ctrl+z undoes)", false)
}

func (m *Model) askTagMerge(source string) {
	if source == "" {
		m.pickTag("Merge which tag", "", func(n string) { m.askTagMerge(n) })
		return
	}
	m.pickTag("Merge #"+source+" into", source, func(target string) { m.confirmTagMerge(source, target) })
}

// confirmTagMerge shows what the merge moves and asks.
func (m *Model) confirmTagMerge(source, target string) { m.askTagMergeWith(source, target, "") }

// askTagMergeWith is confirmTagMerge with a lead sentence in front of the question.
func (m *Model) askTagMergeWith(source, target, lead string) {
	res, err := m.tagr.MergeDryRun(m.ctx, source, target)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	dups := 0
	for _, n := range res.Duplicates {
		dups += n
	}
	detail := fmt.Sprintf("%d records move", res.Total)
	if dups > 0 {
		detail += fmt.Sprintf(", %d already have #%s", dups, target)
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: strings.TrimSpace(lead + fmt.Sprintf("merge #%s into #%s (%s)? y/n", source, target, detail)),
		yes:    func() { m.runTagMerge(source, target) },
	}
}

func (m *Model) runTagMerge(source, target string) {
	res, err := m.tagr.Merge(m.ctx, source, target)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.record(res.Label(),
		func() error { return m.tagr.UndoMerge(m.ctx, res) },
		func() error { return m.tagr.RedoMerge(m.ctx, res) })
	m.afterWrite()
	m.setStatus(fmt.Sprintf("tag #%s merged into #%s, %d records moved (ctrl+z undoes)", source, target, res.Total), false)
}
