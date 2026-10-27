package tui

import "fmt"

// Delete a tag everywhere (same as crm tag delete): every object that carries it is untagged as one
// undo step, and undo puts the same taggings back. A tag is only a name on objects (X26).

var taggableEntities = []string{"person", "organization", "deal", "interaction", "task"}

// openTagDelete lists the tags and asks y/n before deleting the one picked.
func (m *Model) openTagDelete() {
	tags, err := m.tagr.FindAll(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	if len(tags) == 0 {
		m.setStatus("there are no tags", true)
		return
	}
	items := make([]overlayItem, 0, len(tags))
	for _, t := range tags {
		items = append(items, overlayItem{id: "tag:" + t.Name, label: "#" + t.Name, value: t.Name})
	}
	m.openPicker("Delete tag", items, func(m *Model, it overlayItem) {
		name := it.value
		n := 0
		for _, e := range taggableEntities {
			ids, _ := m.tagr.GetEntities(m.ctx, name, e)
			n += len(ids)
		}
		m.mode = modeConfirm
		m.confirm = &confirmState{
			prompt: fmt.Sprintf("delete tag #%s from %d records? y/n", name, n),
			yes:    func() { m.deleteTag(name) },
		}
	})
}

func (m *Model) deleteTag(name string) {
	res, err := m.tagr.DeleteTag(m.ctx, name)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	// One step: untagging every object undoes and redoes as the same taggings.
	m.record(res.Label(),
		func() error { return m.tagr.UndoDelete(m.ctx, res) },
		func() error { return m.tagr.RedoDelete(m.ctx, res) })
	m.afterWrite()
	m.setStatus("tag #"+name+" deleted from every record (ctrl+z undoes)", false)
}
