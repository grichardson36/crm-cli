package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/validate"
)

// The four lists the list editor edits (listeditor.go). Each is one listDesc: the words, the cells,
// the storage calls and the removal rules. The storage calls are the repository functions the CLI and
// MCP use, wrapped to record one undo step each.

func (m *Model) listDescFor(id string) *listDesc {
	switch id {
	case "deal-stages":
		return m.dealStagesDesc()
	case "deal-roles":
		return m.dealRolesDesc()
	case "note-types":
		return m.noteTypesDesc()
	case "relation-kinds":
		return m.relationKindsDesc()
	case "company-ignore":
		return m.companyIgnoreDesc()
	}
	return nil
}

var listYesNo = []model.FieldChoiceOption{{Value: "yes", Label: "yes"}, {Value: "no", Label: "no"}}

// defaultNoteTypes are the note types a new database starts with (migration 004), in the order a reset restores.
var defaultNoteTypes = []struct {
	name    string
	contact bool
}{{"call", true}, {"email", true}, {"meeting", true}, {"message", true}, {"note", false}}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func itemNames(ts []listItem) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.name
	}
	return out
}

// --- vocabularies: deal stages and deal roles ---

// vocabListDesc is the part of a descriptor that deal stages and deal roles share: both live in the
// vocabulary table (repo.VocabRepo).
func (m *Model) vocabListDesc(kind, id, title, usedBy string) *listDesc {
	vs, _ := model.VocabularyNamed(kind)
	d := &listDesc{id: id, title: title, noun: vs.Label, plural: vs.Label + "s", usedBy: usedBy}
	d.load = func(m *Model) ([]listItem, error) {
		list, err := m.vr.List(m.ctx, kind, false)
		if err != nil {
			return nil, err
		}
		out := make([]listItem, len(list))
		for i, e := range list {
			out[i] = listItem{name: e.Name, inUse: e.InUse}
			if vs.Outcomes {
				out[i].cells = []string{e.Outcome}
			}
		}
		return out, nil
	}
	d.add = func(m *Model, _, name string) error {
		m.beginGroup()
		err := m.createVocabValue(kind, name)
		m.endGroup("")
		return err
	}
	d.rename = func(m *Model, it listItem, to string) (string, error) {
		res, err := m.vr.Rename(m.ctx, kind, it.name, to)
		if err != nil {
			return "", err
		}
		m.record("rename "+vs.Label+" "+res.Old+" to "+res.New,
			func() error { _, err := m.vr.Rename(m.ctx, kind, res.New, res.Old); return err },
			func() error { _, err := m.vr.Rename(m.ctx, kind, res.Old, res.New); return err })
		return fmt.Sprintf("renamed %s %s to %s, %d %s updated", vs.Label, res.Old, res.New, res.Rows, usedBy), nil
	}
	d.reorder = func(m *Model, _ string, names []string) error {
		before, _ := m.vr.Names(m.ctx, kind)
		if err := m.vr.Reorder(m.ctx, kind, names); err != nil {
			return err
		}
		m.record("move "+vs.Label,
			func() error { return m.vr.Reorder(m.ctx, kind, before) },
			func() error { return m.vr.Reorder(m.ctx, kind, names) })
		return nil
	}
	d.remove = func(m *Model, ts []listItem, dest listDest) (string, error) {
		rem, err := m.vr.RemoveMany(m.ctx, kind, itemNames(ts), repo.VocabRemoveAction{ReplaceWith: dest.to, Blank: dest.blank})
		if err != nil {
			return "", err
		}
		m.record("remove "+vs.Label+" "+namesOf(ts),
			func() error { return m.vr.UndoRemoval(m.ctx, rem) },
			func() error { return m.vr.RedoRemoval(m.ctx, rem) })
		msg := "removed " + vs.Label + " " + namesOf(ts)
		switch {
		case rem.Moved > 0:
			msg += fmt.Sprintf(", %d %s moved to %s", rem.Moved, usedBy, rem.ReplacedBy)
		case rem.Blanked > 0:
			msg += fmt.Sprintf(", %d %s now blank", rem.Blanked, usedBy)
		}
		return msg, nil
	}
	return d
}

func (m *Model) dealRolesDesc() *listDesc {
	d := m.vocabListDesc(model.DealRoleKind, "deal-roles", "Deal roles", "links")
	d.defaults = func(*Model) []listItem {
		vs, _ := model.VocabularyNamed(model.DealRoleKind)
		var out []listItem
		for _, name := range vs.Seeds {
			out = append(out, listItem{name: name})
		}
		return out
	}
	d.removeOpts = func(*Model, []listItem, []listItem) (listRemoveOpts, error) {
		return listRemoveOpts{blank: true, blankLabel: "Leave the role blank on those links"}, nil
	}
	return d
}

func (m *Model) dealStagesDesc() *listDesc {
	d := m.vocabListDesc(model.DealStageKind, "deal-stages", "Deal stages", "deals")
	d.cells = []listCellDesc{{id: "outcome", label: "outcome", cycle: true, spec: model.FieldSpec{Key: "outcome", Label: "outcome", Type: model.FieldChoice,
		Choices: []model.FieldChoiceOption{{Value: model.OutcomeOpen, Label: "open"}, {Value: model.OutcomeWon, Label: "won"}, {Value: model.OutcomeLost, Label: "lost"}}}}}
	d.defaults = func(*Model) []listItem {
		vs, _ := model.VocabularyNamed(model.DealStageKind)
		var out []listItem
		for _, name := range vs.Seeds {
			outcome := model.OutcomeOpen
			if o, ok := vs.SeedOutcomes[name]; ok {
				outcome = o
			}
			out = append(out, listItem{name: name, cells: []string{outcome}})
		}
		return out
	}
	d.setCell = func(m *Model, it listItem, _ string, value string) error {
		res, err := m.vr.SetOutcome(m.ctx, model.DealStageKind, it.name, value)
		if err != nil {
			return stageRuleError(err)
		}
		if res.Old != res.New {
			m.record("set outcome of deal stage "+res.Name+" to "+res.New,
				func() error { _, err := m.vr.SetOutcome(m.ctx, model.DealStageKind, res.Name, res.Old); return err },
				func() error { _, err := m.vr.SetOutcome(m.ctx, model.DealStageKind, res.Name, res.New); return err })
		}
		return nil
	}
	// A deal always holds a listed stage, so there is no blank. The rule at least one open, one won
	// and one lost stage must remain blocks a removal that would break it.
	d.removeOpts = func(_ *Model, targets, all []listItem) (listRemoveOpts, error) {
		var after []validate.StageState
		for _, it := range all {
			if !listHas(targets, it) {
				after = append(after, validate.StageState{Name: it.name, Outcome: it.cells[0]})
			}
		}
		if err := validate.StageRules(after); err != nil {
			return listRemoveOpts{}, stageRuleError(err)
		}
		return listRemoveOpts{}, nil
	}
	return d
}

func listHas(ts []listItem, it listItem) bool {
	for _, t := range ts {
		if t.id() == it.id() {
			return true
		}
	}
	return false
}

// stageRuleError words the stage rules for the list editor. Other errors pass through.
func stageRuleError(err error) error {
	ve, ok := validate.As(err)
	if !ok || len(ve.Issues) == 0 {
		return err
	}
	switch ve.Issues[0].Code {
	case validate.CodeStageNoOpen:
		return fmt.Errorf("at least one open, one won and one lost stage must remain: this would leave no open stage")
	case validate.CodeStageNoWon:
		return fmt.Errorf("at least one open, one won and one lost stage must remain: this would leave no won stage")
	case validate.CodeStageNoLost:
		return fmt.Errorf("at least one open, one won and one lost stage must remain: this would leave no lost stage")
	}
	return err
}

// --- note types ---

func (m *Model) noteTypesDesc() *listDesc {
	d := &listDesc{id: "note-types", title: "Notes", noun: "note type", plural: "note types", usedBy: "notes"}
	d.cells = []listCellDesc{{id: "contact", label: "mark contacted", cycle: true, spec: model.FieldSpec{Key: "contact", Label: "mark contacted", Type: model.FieldChoice, Choices: listYesNo}}}
	d.defaults = func(*Model) []listItem {
		var out []listItem
		for _, t := range defaultNoteTypes {
			out = append(out, listItem{name: t.name, cells: []string{yesNo(t.contact)}})
		}
		return out
	}
	d.load = func(m *Model) ([]listItem, error) {
		list, err := m.ntr.List(m.ctx, false)
		if err != nil {
			return nil, err
		}
		counts, err := m.ntr.Counts(m.ctx)
		if err != nil {
			return nil, err
		}
		out := make([]listItem, len(list))
		for i, t := range list {
			out[i] = listItem{name: t.Name, cells: []string{yesNo(t.CountsAsContact)}, inUse: counts[strings.ToLower(t.Name)]}
			if repo.IsProtectedNoteType(t.Name) {
				out[i].locked = "default type, stays"
			}
		}
		return out, nil
	}
	d.add = func(m *Model, _, name string) error {
		t, err := m.ntr.Add(m.ctx, name, false)
		if err != nil {
			return err
		}
		m.record("add note type "+t.Name,
			func() error { return m.ntr.Archive(m.ctx, t.Name) },
			func() error { _, err := m.ntr.Add(m.ctx, t.Name, t.CountsAsContact); return err })
		return nil
	}
	d.rename = func(m *Model, it listItem, to string) (string, error) {
		res, err := m.ntr.Rename(m.ctx, it.name, to)
		if err != nil {
			return "", err
		}
		m.record("rename note type "+res.Old+" to "+res.New,
			func() error { _, err := m.ntr.Rename(m.ctx, res.New, res.Old); return err },
			func() error { _, err := m.ntr.Rename(m.ctx, res.Old, res.New); return err })
		return fmt.Sprintf("renamed note type %s to %s, %d notes updated", res.Old, res.New, res.Rows), nil
	}
	d.setCell = func(m *Model, it listItem, _ string, value string) error {
		before, after := it.cells[0] == "yes", value == "yes"
		if before == after {
			return nil
		}
		if err := m.ntr.SetCountsAsContact(m.ctx, it.name, after); err != nil {
			return err
		}
		m.record("note type "+it.name+" mark contacted: "+value,
			func() error { return m.ntr.SetCountsAsContact(m.ctx, it.name, before) },
			func() error { return m.ntr.SetCountsAsContact(m.ctx, it.name, after) })
		return nil
	}
	d.reorder = func(m *Model, _ string, names []string) error {
		before := m.noteTypeNames()
		if err := m.ntr.Reorder(m.ctx, names); err != nil {
			return err
		}
		m.record("move note type",
			func() error { return m.ntr.Reorder(m.ctx, before) },
			func() error { return m.ntr.Reorder(m.ctx, names) })
		return nil
	}
	// Notes always hold a type (the column is required), so there is no blank. Leaving the type on
	// the old notes is what archiving always did.
	d.removeOpts = func(*Model, []listItem, []listItem) (listRemoveOpts, error) {
		return listRemoveOpts{keep: true, keepLabel: "Keep the type on those notes (the type is archived)"}, nil
	}
	d.remove = func(m *Model, ts []listItem, dest listDest) (string, error) {
		rem, err := m.ntr.RemoveMany(m.ctx, itemNames(ts), dest.to)
		if err != nil {
			return "", err
		}
		m.record("remove note type "+namesOf(ts),
			func() error { return m.ntr.UndoRemoval(m.ctx, rem) },
			func() error { return m.ntr.RedoRemoval(m.ctx, rem) })
		msg := "removed note type " + namesOf(ts)
		switch {
		case rem.Moved > 0:
			msg += fmt.Sprintf(", %d notes moved to %s", rem.Moved, rem.ReplaceWith)
		case rem.Kept > 0:
			msg += fmt.Sprintf(", %d notes keep the type", rem.Kept)
		}
		return msg, nil
	}
	return d
}

// --- relation kinds ---

func (m *Model) relationKindsDesc() *listDesc {
	kr := repo.NewKindRepo(m.db)
	d := &listDesc{id: "relation-kinds", title: "Links", noun: "kind", plural: "kinds", usedBy: "links",
		groups: []listGroup{{key: "person", title: "People"}, {key: "organization", title: "Organizations"}}}
	// A name equal to its inverse means mutual.
	d.cells = []listCellDesc{
		{id: "inverse", label: "inverse", spec: model.FieldSpec{Key: "inverse", Label: "inverse", Type: model.FieldText, Max: 80}},
	}
	d.defaults = func(m *Model) []listItem {
		var out []listItem
		for _, typ := range []string{"person", "organization"} {
			list, err := kr.List(m.ctx, typ)
			if err != nil {
				continue
			}
			var built []*model.RelationKind
			for _, k := range list {
				if k.Builtin {
					built = append(built, k)
				}
			}
			sort.Slice(built, func(i, j int) bool { return built[i].ID < built[j].ID })
			for _, k := range built {
				out = append(out, listItem{name: k.Name, group: typ})
			}
		}
		return out
	}
	d.load = func(m *Model) ([]listItem, error) {
		var out []listItem
		for _, typ := range []string{"person", "organization"} {
			list, err := kr.List(m.ctx, typ)
			if err != nil {
				return nil, err
			}
			counts, err := kr.Counts(m.ctx, typ)
			if err != nil {
				return nil, err
			}
			for _, k := range list {
				inv := k.InverseName
				if k.Symmetric || inv == "" {
					inv = k.Name
				}
				it := listItem{name: k.Name, group: typ, cells: []string{inv}, inUse: counts[k.ID]}
				if k.Builtin {
					it.locked = "built-in, stays"
				}
				out = append(out, it)
			}
		}
		return out, nil
	}
	namesIn := func(typ string) []string {
		list, _ := kr.List(m.ctx, typ)
		out := make([]string, len(list))
		for i, k := range list {
			out[i] = k.Name
		}
		return out
	}
	d.add = func(m *Model, typ, name string) error {
		k, err := kr.AddKind(m.ctx, typ, name, "", false)
		if err != nil {
			return err
		}
		var rem *repo.KindRemoval
		m.record("add "+typ+" kind "+k.Name,
			func() (err error) { rem, err = kr.RemoveMany(m.ctx, typ, []string{k.Name}, repo.KindRemoveAction{}); return },
			func() error {
				if rem == nil {
					return nil
				}
				return kr.UndoRemoval(m.ctx, rem)
			})
		return nil
	}
	update := func(m *Model, typ, name string, p repo.KindPatch) (*model.RelationKind, error) {
		return kr.Update(m.ctx, typ, name, p)
	}
	d.rename = func(m *Model, it listItem, to string) (string, error) {
		old, _, err := kr.Resolve(m.ctx, it.group, it.name)
		if err != nil {
			return "", err
		}
		oldName, oldInv := old.Name, old.InverseName
		if old.Symmetric {
			oldInv = ""
		}
		k, err := update(m, it.group, it.name, repo.KindPatch{Name: &to})
		if err != nil {
			return "", err
		}
		m.record("rename kind "+oldName+" to "+k.Name,
			func() error {
				_, err := update(m, it.group, k.Name, repo.KindPatch{Name: &oldName, Inverse: &oldInv})
				return err
			},
			func() error { _, err := update(m, it.group, oldName, repo.KindPatch{Name: &to}); return err })
		return fmt.Sprintf("renamed %s kind %s to %s", it.group, oldName, k.Name), nil
	}
	d.setCell = func(m *Model, it listItem, _, value string) error {
		old, _, err := kr.Resolve(m.ctx, it.group, it.name)
		if err != nil {
			return err
		}
		prev := old.InverseName
		if old.Symmetric {
			prev = ""
		}
		next := value
		if _, err := update(m, it.group, it.name, repo.KindPatch{Inverse: &next}); err != nil {
			return err
		}
		m.record("inverse of kind "+it.name,
			func() error { _, err := update(m, it.group, it.name, repo.KindPatch{Inverse: &prev}); return err },
			func() error { _, err := update(m, it.group, it.name, repo.KindPatch{Inverse: &next}); return err })
		return nil
	}
	d.reorder = func(m *Model, typ string, names []string) error {
		before := namesIn(typ)
		if err := kr.Reorder(m.ctx, typ, names); err != nil {
			return err
		}
		m.record("move kind",
			func() error { return kr.Reorder(m.ctx, typ, before) },
			func() error { return kr.Reorder(m.ctx, typ, names) })
		return nil
	}
	// A kind that links use moves them to another kind or unlinks them. Built-in kinds stay.
	d.removeOpts = func(*Model, []listItem, []listItem) (listRemoveOpts, error) {
		return listRemoveOpts{entirely: true, entirelyLb: "Unlink them (the links are deleted, ctrl+z brings them back)"}, nil
	}
	d.remove = func(m *Model, ts []listItem, dest listDest) (string, error) {
		typ := ts[0].group
		act := repo.KindRemoveAction{MoveTo: dest.to, Unlink: dest.entirely}
		rem, err := kr.RemoveMany(m.ctx, typ, itemNames(ts), act)
		if err != nil {
			return "", err
		}
		m.record("remove "+typ+" kind "+namesOf(ts),
			func() error { return kr.UndoRemoval(m.ctx, rem) },
			func() error { return kr.RedoRemoval(m.ctx, rem) })
		msg := "removed " + typ + " kind " + namesOf(ts)
		switch {
		case rem.Moved > 0:
			msg += fmt.Sprintf(", %d links moved to %s", rem.Moved, dest.to)
		case rem.Unlinked > 0:
			msg += fmt.Sprintf(", %d links unlinked", rem.Unlinked)
		}
		return msg, nil
	}
	return d
}

// --- ignored company texts ---

// companyIgnoreDesc is the list of company texts that never create an organization. A built-in text
// cannot be renamed and removing it turns it off. An added text is deleted.
func (m *Model) companyIgnoreDesc() *listDesc {
	ig := repo.NewIgnoreRepo(m.db)
	d := &listDesc{id: "company-ignore", title: "Ignored company texts", noun: "text", plural: "texts", nameLabel: "text", noUsed: true}
	d.cells = []listCellDesc{{id: "state", label: "state", readonly: true, spec: model.FieldSpec{Key: "state", Label: "state", Type: model.FieldText}}}
	d.load = func(m *Model) ([]listItem, error) {
		list, err := ig.List(m.ctx)
		if err != nil {
			return nil, err
		}
		out := make([]listItem, len(list))
		for i, e := range list {
			state := "ignored"
			if !e.Active {
				state = "off"
			}
			out[i] = listItem{name: cleanCell(e.Text), cells: []string{state}, noRename: e.Builtin}
			if !e.Active {
				out[i].locked = "off"
			}
		}
		return out, nil
	}
	set := func(text string, on bool) error {
		if on {
			_, err := ig.Add(m.ctx, text)
			return err
		}
		return ig.Remove(m.ctx, text)
	}
	d.add = func(m *Model, _, name string) error {
		changed, err := ig.Add(m.ctx, name)
		if err != nil {
			return err
		}
		if !changed {
			return fmt.Errorf("%s is already ignored", model.CleanVocabName(name))
		}
		m.record("ignore company text "+name,
			func() error { return ig.Remove(m.ctx, name) },
			func() error { _, err := ig.Add(m.ctx, name); return err })
		return nil
	}
	d.rename = func(m *Model, it listItem, to string) (string, error) {
		on, err := ig.Ignored(m.ctx, to)
		if err != nil {
			return "", err
		}
		if on {
			return "", fmt.Errorf("%s is already ignored", model.CleanVocabName(to))
		}
		m.beginGroup()
		defer m.endGroup("rename ignored company text " + it.name)
		if err := set(it.name, false); err != nil {
			return "", err
		}
		m.record("stop ignoring company text "+it.name,
			func() error { return set(it.name, true) }, func() error { return set(it.name, false) })
		if err := set(to, true); err != nil {
			return "", err
		}
		m.record("ignore company text "+to,
			func() error { return set(to, false) }, func() error { return set(to, true) })
		return fmt.Sprintf("renamed %s to %s", it.name, model.CleanVocabName(to)), nil
	}
	d.removeOpts = func(*Model, []listItem, []listItem) (listRemoveOpts, error) { return listRemoveOpts{}, nil }
	d.remove = func(m *Model, ts []listItem, _ listDest) (string, error) {
		m.beginGroup()
		defer m.endGroup("remove ignored company text " + namesOf(ts))
		for _, t := range ts {
			t := t
			if err := set(t.name, false); err != nil {
				return "", err
			}
			m.record("stop ignoring company text "+t.name,
				func() error { return set(t.name, true) }, func() error { return set(t.name, false) })
		}
		return "removed text " + namesOf(ts), nil
	}
	return d
}
