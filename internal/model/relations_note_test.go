package model

import "testing"

// Notes are a registered object: they take part in the link registry like the others.
func TestNoteIsARegisteredObjectKind(t *testing.T) {
	if !ValidObjectKind(NoteKind) {
		t.Fatal("note is not an object kind")
	}
	tests := []struct {
		kind  string
		other string
		want  string
	}{
		{"person", NoteKind, "interaction_person"},
		{"organization", NoteKind, "interaction_organization"},
		{"deal", NoteKind, "interaction_deal"},
		{NoteKind, "person", "interaction_person"},
	}
	for _, tt := range tests {
		t.Run(tt.kind+"-"+tt.other, func(t *testing.T) {
			rel, ok := RelationBetween(tt.kind, tt.other)
			if !ok || rel.Name != tt.want {
				t.Fatalf("relation = %q, %v; want %q", rel.Name, ok, tt.want)
			}
			found := false
			for _, k := range RelatedKinds(tt.kind) {
				found = found || k == tt.other
			}
			if !found {
				t.Fatalf("%s does not list %s as linkable", tt.kind, tt.other)
			}
		})
	}
}
