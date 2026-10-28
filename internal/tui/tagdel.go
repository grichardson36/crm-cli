package tui

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
		m.askDeleteTag(it.value) // the same question and the same delete as the Tags view (tagview.go)
	})
}

// deleteTag deletes one tag: the same code as the Tags view (tagview.go deleteTags).
func (m *Model) deleteTag(name string) { m.deleteTags([]string{name}) }
