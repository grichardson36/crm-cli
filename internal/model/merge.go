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
	// Moved counts records re-pointed to the survivor: interactions, tasks, task_people, deals, relationships.
	Moved map[string]int `json:"moved"`
}
