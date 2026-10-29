package query

import (
	"sort"

	"github.com/jdanielnd/crm-cli/internal/model"
)

// Class is the family of operators a field takes. It follows the field type of the registry
// (model.Fields): text and longtext and list are text, a relation to a tag is a tag, any other relation
// is a relation.
type Class string

const (
	ClassText     Class = "text"
	ClassDate     Class = "date"
	ClassChoice   Class = "choice"
	ClassNumber   Class = "number"
	ClassRelation Class = "relation"
	ClassTag      Class = "tag"
)

// operatorsByClass is the one table of which operators a class takes (owner request 2026-10-29).
var operatorsByClass = map[Class][]string{
	ClassText:     {OpIs, OpIsNot, OpContains, OpEmpty, OpNotEmpty},
	ClassDate:     {OpBefore, OpAfter, OpWithinLast, OpInNext, OpEmpty, OpNotEmpty},
	ClassChoice:   {OpIs, OpIsNot, OpIn},
	ClassNumber:   {OpEq, OpLt, OpGt, OpBetween},
	ClassRelation: {OpLinkedTo, OpNotLinkedTo, OpHasAny},
	ClassTag:      {OpHas, OpNotHas},
}

// OperatorsFor lists the operators of a field class, in the order a picker shows them.
func OperatorsFor(c Class) []string { return append([]string(nil), operatorsByClass[c]...) }

// OperatorLabel is the words for an operator.
func OperatorLabel(op string) string {
	switch op {
	case OpIs:
		return "is"
	case OpIsNot:
		return "is not"
	case OpContains:
		return "contains"
	case OpEmpty:
		return "is empty"
	case OpNotEmpty:
		return "is not empty"
	case OpBefore:
		return "before"
	case OpAfter:
		return "after"
	case OpWithinLast:
		return "within last (days)"
	case OpInNext:
		return "in next (days)"
	case OpIn:
		return "in"
	case OpEq:
		return "="
	case OpLt:
		return "<"
	case OpGt:
		return ">"
	case OpBetween:
		return "between"
	case OpLinkedTo:
		return "linked to"
	case OpNotLinkedTo:
		return "not linked to"
	case OpHasAny:
		return "has any"
	case OpHas:
		return "has"
	case OpNotHas:
		return "does not have"
	}
	return op
}

// NeedsNoValue reports whether the operator takes no value.
func NeedsNoValue(op string) bool { return op == OpEmpty || op == OpNotEmpty || op == OpHasAny }

// rel describes a link table between the rows of a kind and another kind. Every identifier here is a
// constant of this package, never user input.
type rel struct {
	table string // link table, aliased r
	owner string // column holding the id of the row of this kind
	other string // column holding the id at the far end
	live  string // extra condition on r that keeps the link live
	far   string // far table, aliased f
	label string // SQL text of the far end's name, over f
}

// multi is a text field with several rows (emails).
type multi struct{ table, col, owner string }

// Field is one field a query can name. It is built from the field-type registry (model.Fields) with a
// few computed fields added (last contacted, status, created, updated).
type Field struct {
	Key   string
	Label string
	Type  model.FieldType
	Class Class
	// Kind is the object kind at the far end of a relation field.
	Kind string
	// Choices are the fixed options of a choice field. Vocabulary names a user-managed list whose live
	// values are the options (Env.Choices), used for the deal stage.
	Choices    []model.FieldChoiceOption
	Vocabulary string
	// Birthday marks a date that is month and day only.
	Birthday bool

	expr  string // scalar SQL over t (text, date, choice, number)
	multi *multi
	rel   *rel
	// tagEntity is the entity_type of the taggings of a tag field.
	tagEntity string
}

// Sortable says the field can be a sort key.
func (f Field) Sortable() bool { return f.expr != "" && !f.Birthday }

// Groupable says the field can be the group.
func (f Field) Groupable() bool {
	return f.expr != "" || f.rel != nil || f.tagEntity != "" || f.Birthday
}

// kindDef is how one object kind is read.
type kindDef struct {
	table  string
	live   string // condition on t that keeps the row live
	label  string // SQL of the main text of a row
	sub    string // SQL of the second text
	meta   string // SQL of the right-hand text
	search []string
	// def is the default sort when the query has none.
	def    []Sort
	fields []Field
	// tagEntity is the taggings entity_type of the kind, empty for a kind without tags.
	tagEntity string
}

func textF(kind, key, label, expr string) Field {
	return Field{Key: key, Label: label, Type: model.FieldText, Class: ClassText, expr: expr}
}

func dateF(key, label, expr string) Field {
	return Field{Key: key, Label: label, Type: model.FieldDate, Class: ClassDate, expr: expr}
}

func yesNo() []model.FieldChoiceOption {
	return []model.FieldChoiceOption{{Value: "yes", Label: "yes"}, {Value: "no", Label: "no"}}
}

func choiceF(key, label, expr string, ch []model.FieldChoiceOption) Field {
	return Field{Key: key, Label: label, Type: model.FieldChoice, Class: ClassChoice, expr: expr, Choices: ch}
}

func relF(key, label, kind string, r rel) Field {
	return Field{Key: key, Label: label, Type: model.FieldRelation, Class: ClassRelation, Kind: kind, rel: &r}
}

func tagF(entity string) Field {
	return Field{Key: "tag", Label: "tags", Type: model.FieldRelation, Class: ClassTag, Kind: "tag", tagEntity: entity}
}

// spec returns the registry field of a kind, so labels, types and choices come from model.Fields.
func spec(kind, key string) model.FieldSpec {
	s, _ := model.FieldNamed(kind, key)
	return s
}

func fromSpec(kind, key, expr string) Field {
	s := spec(kind, key)
	f := Field{Key: key, Label: s.Label, Type: s.Type, expr: expr, Choices: s.Choices, Vocabulary: s.Vocabulary, Kind: s.Kind, Birthday: s.YearOptional}
	switch s.Type {
	case model.FieldDate:
		f.Class = ClassDate
	case model.FieldChoice:
		f.Class = ClassChoice
	case model.FieldNumber:
		f.Class = ClassNumber
	default:
		f.Class = ClassText
	}
	return f
}

func activityRel(source, target string) rel {
	return rel{
		table: "activity_target", owner: "r.source_id", other: "r.target_id",
		live: "r.source_kind = '" + source + "' AND r.target_kind = '" + target + "' AND r.deleted_at IS NULL",
	}
}

func farOf(r rel, table, label string) rel {
	r.far, r.label = table, label
	return r
}

const (
	personName = "TRIM(t.first_name || ' ' || COALESCE(t.last_name, ''))"
	orgPlace   = "TRIM(COALESCE(t.city, '') || ' ' || COALESCE(t.region, '') || ' ' || COALESCE(t.country, ''))"
)

// defs builds the catalog of every kind.
func defs() map[string]*kindDef {
	person := &kindDef{
		table: "people", live: "t.archived = 0 AND t.merged_into IS NULL", tagEntity: "person",
		label: personName, sub: "COALESCE(NULLIF(t.company, ''), NULLIF(t.title, ''), '')", meta: "COALESCE(t.keep_in_touch, '')",
		search: []string{personName, "t.company", "t.title", "t.email", "t.location", "t.notes"},
		def:    []Sort{{Field: "name"}},
	}
	birthday := fromSpec("person", "birthday", "")
	birthday.Class = ClassDate
	person.fields = []Field{
		textF("person", "name", "name", "LOWER("+personName+")"),
		fromSpec("person", "first", "t.first_name"),
		fromSpec("person", "last", "t.last_name"),
		fromSpec("person", "title", "t.title"),
		fromSpec("person", "company", "t.company"),
		relF("organization", "organizations", "organization", farOf(rel{table: "membership", owner: "r.person_id", other: "r.org_id", live: "r.deleted_at IS NULL"}, "organizations", "f.name")),
		fromSpec("person", "location", "t.location"),
		birthday,
		fromSpec("person", "email", ""),
		fromSpec("person", "phone", ""),
		fromSpec("person", "tier", "t.priority_tier"),
		fromSpec("person", "notes", "t.notes"),
		tagF("person"),
		choiceF("keep_in_touch", "keep in touch", "t.keep_in_touch", cadenceChoices()),
		dateF("last_contacted", "last contacted", lastContacted),
		choiceF("favorite", "favorite", "CASE WHEN t.is_favorite = 1 THEN 'yes' ELSE 'no' END", yesNo()),
		dateF("created", "created", "t.created_at"),
		dateF("updated", "updated", "t.updated_at"),
	}
	person.fields = setBirthday(person.fields)
	for i := range person.fields {
		switch person.fields[i].Key {
		case "email":
			person.fields[i].multi = &multi{table: "person_emails", col: "email", owner: "person_id"}
			person.fields[i].Class = ClassText
		case "phone":
			person.fields[i].multi = &multi{table: "person_phones", col: "phone_number", owner: "person_id"}
			person.fields[i].Class = ClassText
		}
	}

	org := &kindDef{
		table: "organizations", live: "t.archived = 0", tagEntity: "organization",
		label: "t.name", sub: "COALESCE(t.industry, '')", meta: "COALESCE(t.domain, '')",
		search: []string{"t.name", "t.domain", "t.industry", "t.notes", "t.description", orgPlace},
		def:    []Sort{{Field: "name"}},
		fields: []Field{
			fromSpec("organization", "name", "LOWER(t.name)"),
			fromSpec("organization", "domain", "t.domain"),
			fromSpec("organization", "website", "t.website"),
			fromSpec("organization", "industry", "t.industry"),
			fromSpec("organization", "size", "t.size"),
			fromSpec("organization", "location", orgPlace),
			fromSpec("organization", "linkedin", "t.linkedin_url"),
			fromSpec("organization", "description", "t.description"),
			fromSpec("organization", "notes", "t.notes"),
			relF("member", "members", "person", farOf(rel{table: "membership", owner: "r.org_id", other: "r.person_id", live: "r.deleted_at IS NULL"}, "people", personNameF)),
			tagF("organization"),
			dateF("created", "created", "t.created_at"),
			dateF("updated", "updated", "t.updated_at"),
		},
	}

	deal := &kindDef{
		table: "deals", live: "t.archived = 0", tagEntity: "deal",
		label: "t.title", sub: "t.stage", meta: "COALESCE(CAST(t.value AS TEXT), '')",
		search: []string{"t.title", "t.notes", "t.stage"},
		def:    []Sort{{Field: "title"}},
		fields: []Field{
			fromSpec("deal", "title", "LOWER(t.title)"),
			fromSpec("deal", "stage", "t.stage"),
			fromSpec("deal", "value", "t.value"),
			fromSpec("deal", "notes", "t.notes"),
			relF("organization", "organizations", "organization", farOf(rel{table: "deal_org", owner: "r.deal_id", other: "r.org_id", live: "r.deleted_at IS NULL"}, "organizations", "f.name")),
			relF("person", "people", "person", farOf(rel{table: "deal_person", owner: "r.deal_id", other: "r.person_id", live: "r.deleted_at IS NULL"}, "people", personNameF)),
			// The outcome of the stage (open, won or lost) from the deal stage list the user manages.
			choiceF("outcome", "outcome", "COALESCE((SELECT v.outcome FROM vocabulary v WHERE v.kind = 'deal_stage' AND v.name = t.stage COLLATE NOCASE LIMIT 1), 'open')",
				[]model.FieldChoiceOption{{Value: "open", Label: "open"}, {Value: "won", Label: "won"}, {Value: "lost", Label: "lost"}}),
			dateF("closed", "closed", "t.closed_at"),
			tagF("deal"),
			dateF("created", "created", "t.created_at"),
			dateF("updated", "updated", "t.updated_at"),
		},
	}

	task := &kindDef{
		table: "tasks", live: "t.archived = 0", tagEntity: "task",
		label: "t.title", sub: "COALESCE(t.recurrence, '')", meta: "COALESCE(t.due_at, '')",
		search: []string{"t.title", "t.description"},
		def:    []Sort{{Field: "due"}, {Field: "title"}},
		fields: []Field{
			fromSpec("task", "title", "LOWER(t.title)"),
			fromSpec("task", "due", "t.due_at"),
			fromSpec("task", "priority", "t.priority"),
			fromSpec("task", "description", "t.description"),
			choiceF("status", "status", "CASE WHEN t.completed = 1 THEN 'done' ELSE 'open' END",
				[]model.FieldChoiceOption{{Value: "open", Label: "open"}, {Value: "done", Label: "done"}}),
			relF("person", "people", "person", farOf(activityRel("task", "person"), "people", personNameF)),
			relF("organization", "organizations", "organization", farOf(activityRel("task", "organization"), "organizations", "f.name")),
			relF("deal", "deals", "deal", farOf(activityRel("task", "deal"), "deals", "f.title")),
			tagF("task"),
			dateF("created", "created", "t.created_at"),
			dateF("updated", "updated", "t.updated_at"),
		},
	}
	// A task's priority sorts low, medium, high, not by the letters.
	for i := range task.fields {
		if task.fields[i].Key == "priority" {
			task.fields[i].Choices = []model.FieldChoiceOption{{Value: "low", Label: "low"}, {Value: "medium", Label: "medium"}, {Value: "high", Label: "high"}}
		}
	}

	note := &kindDef{
		table: "interactions", live: "t.archived = 0", tagEntity: "interaction",
		label: "COALESCE(NULLIF(t.subject, ''), SUBSTR(REPLACE(COALESCE(t.content, ''), CHAR(10), ' '), 1, 80))", sub: "t.type", meta: "COALESCE(t.occurred_at, '')",
		search: []string{"t.subject", "t.content"},
		def:    []Sort{{Field: "date", Dir: "desc"}},
		fields: []Field{
			fromSpec(model.NoteKind, "text", "t.content"),
			textF("note", "subject", "subject", "t.subject"),
			textF("note", "type", "type", "t.type"),
			dateF("date", "date", "t.occurred_at"),
			relF("person", "people", "person", farOf(activityRel("interaction", "person"), "people", personNameF)),
			relF("organization", "organizations", "organization", farOf(activityRel("interaction", "organization"), "organizations", "f.name")),
			relF("deal", "deals", "deal", farOf(activityRel("interaction", "deal"), "deals", "f.title")),
			tagF("interaction"),
			dateF("created", "created", "t.created_at"),
			dateF("updated", "updated", "t.updated_at"),
		},
	}

	tag := &kindDef{
		table: "tags", live: "EXISTS (SELECT 1 FROM taggings g WHERE g.tag_id = t.id AND g.deleted_at IS NULL)",
		label: "t.name", sub: "''", meta: "CAST((SELECT COUNT(*) FROM taggings g WHERE g.tag_id = t.id AND g.deleted_at IS NULL) AS TEXT)",
		search: []string{"t.name"},
		def:    []Sort{{Field: "name"}},
		fields: []Field{
			fromSpec("tag", "name", "LOWER(t.name)"),
			{Key: "used", Label: "used on", Type: model.FieldNumber, Class: ClassNumber,
				expr: "(SELECT COUNT(*) FROM taggings g WHERE g.tag_id = t.id AND g.deleted_at IS NULL)"},
		},
	}
	return map[string]*kindDef{KindPerson: person, KindOrganization: org, KindDeal: deal, KindTask: task, KindNote: note, KindTag: tag}
}

// lastContacted is the later of the manual last contacted date and the latest interaction whose type
// counts as contact, the same rule as the keep-in-touch status view. It is NULL for a person never
// contacted.
const lastContacted = `NULLIF(MAX(COALESCE(t.last_contacted_at, ''), COALESCE((SELECT MAX(i.occurred_at) FROM activity_target a ` +
	`JOIN interactions i ON i.id = a.source_id AND i.archived = 0 AND COALESCE((SELECT nt.counts_as_contact FROM note_types nt WHERE nt.name = i.type), 1) = 1 ` +
	`WHERE a.source_kind = 'interaction' AND a.target_kind = 'person' AND a.target_id = t.id AND a.deleted_at IS NULL), '')), '')`

// personNameF is the name of a person over the far alias f.
const personNameF = "TRIM(f.first_name || ' ' || COALESCE(f.last_name, ''))"

func cadenceChoices() []model.FieldChoiceOption {
	var out []model.FieldChoiceOption
	for _, c := range model.KeepInTouchCadences {
		out = append(out, model.FieldChoiceOption{Value: c, Label: c})
	}
	return out
}

func setBirthday(fs []Field) []Field {
	for i := range fs {
		if fs[i].Key == "birthday" {
			fs[i].Birthday = true
			fs[i].expr = "" // computed from month and day, see compile.go
		}
	}
	return fs
}

var catalog = defs()

// Fields lists the fields of a kind, in display order. The result is a copy.
func Fields(kind string) []Field {
	d, ok := catalog[kind]
	if !ok {
		return nil
	}
	return append([]Field(nil), d.fields...)
}

// FieldOf finds one field of a kind.
func FieldOf(kind, key string) (Field, bool) {
	d, ok := catalog[kind]
	if !ok {
		return Field{}, false
	}
	for _, f := range d.fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// FieldKeys lists the field keys of a kind, sorted.
func FieldKeys(kind string) []string {
	var out []string
	for _, f := range Fields(kind) {
		out = append(out, f.Key)
	}
	sort.Strings(out)
	return out
}

// FarTable is the table at the far end of a relation field ("" for any other field). Relation values are
// the uuids of rows of this table.
func (f Field) FarTable() string {
	if f.rel == nil {
		return ""
	}
	return f.rel.far
}
