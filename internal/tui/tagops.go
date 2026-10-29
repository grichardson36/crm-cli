package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
)

// Tag rename and merge (X25). The actions are in tagview.go. "Rename tag" and "Merge tag into another"
// work from the palette (they ask which tag first). A merge shows what moves and asks y/n. Both are one undo step.

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

// runTagRename applies the new name typed in the prompt. The tags below the tag follow (RenameTree).
func (m *Model) runTagRename(value string) {
	from, to := m.tagRenaming, repo.NormalizeTagName(value)
	if to == "" || to == from {
		return
	}
	res, err := m.tagr.RenameTree(m.ctx, from, to, false)
	var exists *repo.TagExistsError
	if errors.As(err, &exists) {
		if exists.From != from {
			// A tag below it meets a tag of the same name there: offer to merge those.
			m.runTreeChange("rename", func(merge bool) (*repo.TagTreeResult, error) { return m.tagr.RenameTree(m.ctx, from, to, merge) },
				func() (*repo.TagTreeResult, error) { return m.tagr.RenameTreeDryRun(m.ctx, from, to, true) })
			return
		}
		// The name is taken: offer the merge the error talks about.
		m.setStatus(err.Error(), true)
		m.askTagMergeWith(from, exists.Existing, fmt.Sprintf("tag #%s exists. ", exists.Existing))
		return
	}
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.record(res.Label(),
		func() error { return m.tagr.UndoTree(m.ctx, res) },
		func() error { return m.tagr.RedoTree(m.ctx, res) })
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
	res, err := m.tagr.MergeTreeDryRun(m.ctx, source, target)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	detail := fmt.Sprintf("%d records move", res.Records)
	if res.Duplicates > 0 {
		detail += fmt.Sprintf(", %d already have #%s", res.Duplicates, target)
	}
	if kids := res.Renamed + res.Merged - 1; kids > 0 {
		detail += fmt.Sprintf(", %d tags below follow", kids)
	}
	m.mode = modeConfirm
	m.confirm = &confirmState{
		prompt: strings.TrimSpace(lead + fmt.Sprintf("merge #%s into #%s (%s)? y/n", source, target, detail)),
		yes:    func() { m.runTagMerge(source, target) },
	}
}

func (m *Model) runTagMerge(source, target string) {
	res, err := m.tagr.MergeTree(m.ctx, source, target)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.record(res.Label(),
		func() error { return m.tagr.UndoTree(m.ctx, res) },
		func() error { return m.tagr.RedoTree(m.ctx, res) })
	m.afterWrite()
	m.setStatus(fmt.Sprintf("tag #%s merged into #%s, %d records moved (ctrl+z undoes)", source, target, res.Records), false)
}
