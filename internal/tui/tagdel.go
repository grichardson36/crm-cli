package tui

import "fmt"

// Delete a tag everywhere (same as crm tag delete). The undo puts the tag back on the records
// that had it.

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
	held := map[string][]int64{}
	for _, e := range taggableEntities {
		if ids, err := m.tagr.GetEntities(m.ctx, name, e); err == nil && len(ids) > 0 {
			held[e] = ids
		}
	}
	if err := m.tagr.Delete(m.ctx, name); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.record("delete tag #"+name,
		func() error {
			for e, ids := range held {
				for _, id := range ids {
					_ = m.tagr.Apply(m.ctx, e, id, name)
				}
			}
			return nil
		},
		func() error { return m.tagr.Delete(m.ctx, name) })
	m.afterWrite()
	m.setStatus("tag #"+name+" deleted (ctrl+z undoes)", false)
}
