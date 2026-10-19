package model

// MergeFieldChange describes one scalar field whose value on the survivor changes in a merge.
type MergeFieldChange struct {
	Field string `json:"field"`
	// From is the person whose value is used.
	From int64 `json:"from"`
	Old  any   `json:"old"`
	New  any   `json:"new"`
}

// MergeResult reports what a merge did, or would do when DryRun is true.
type MergeResult struct {
	KeepID  int64              `json:"keep_id"`
	DropIDs []int64            `json:"drop_ids"`
	DryRun  bool               `json:"dry_run"`
	Fields  []MergeFieldChange `json:"fields"`
	// Added counts attribute rows copied to the survivor: emails, phones, handles, tags, custom_fields.
	Added map[string]int `json:"added"`
	// Moved counts records re-pointed to the survivor: interactions, tasks, deals (deal links), relationships (person links), memberships, duplicates_dropped, aliases.
	Moved map[string]int `json:"moved"`
	// Removed counts survivor rows deleted because MergeOptions.Exclude named them. Absent when none.
	Removed map[string]int `json:"removed,omitempty"`
}

// PersonField is one custom field value of a person.
type PersonField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MergeOptions refines a merge beyond per-field choices. Both fields are optional.
type MergeOptions struct {
	// Exclude lists list items that must not be in the result. Keys: "email", "phone" (the address
	// or number), "tag" (tag name), "handle" (link kind), "custom_field" (field name). Matching items
	// are not copied from the dropped people and are removed from the survivor.
	Exclude map[string][]string
	// CustomFrom picks the person whose value a custom field takes. Without an entry the survivor's
	// value wins and a field only the dropped people have is copied.
	CustomFrom map[string]int64
}
