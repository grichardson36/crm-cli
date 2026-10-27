package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/quick"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
)

// Before a note, task or log is saved, every name in it that is not settled (it matched nobody,
// several people, or only the start of a name) is listed on one screen. Per name you connect it to
// a person, keep it as plain text, or create a new contact. Nothing is ever created by default:
// the default is to keep the text, unless exactly one person fits. Enter saves everything at once
// and esc goes back to editing.

const (
	choiceText   = -1
	choiceCreate = -2
)

type namesPanel struct {
	refs   []quick.NameRef
	choice []int // choiceText, choiceCreate, or the index of a candidate
	cur    int
	err    string
	// reopen goes back to editing the line (esc).
	reopen func()
	// reparse plans the line again with the choices.
	reparse func(map[string]quick.NameChoice) *quick.Plan
	// commit applies the settled plan and returns the command for the toast.
	commit func(*quick.Plan) tea.Cmd
}

// nameProblem is true for the problems that a names list can settle.
func nameProblem(code string) bool {
	return code == "ambiguous" || code == "not_found" || code == "unconfirmed"
}

// needsNames is true when the plan waits only for names (plus nothing else that editing must fix).
func needsNames(p *quick.Plan) bool {
	if len(p.Names) == 0 {
		return false
	}
	for _, pr := range p.Problems {
		if !nameProblem(pr.Code) && pr.Code != "no_person" {
			return false
		}
	}
	return true
}

// openNames shows the names list for a plan.
func (m *Model) openNames(p *quick.Plan, reopen func(), reparse func(map[string]quick.NameChoice) *quick.Plan, commit func(*quick.Plan) tea.Cmd) {
	np := &namesPanel{refs: p.Names, reopen: reopen, reparse: reparse, commit: commit}
	np.choice = make([]int, len(p.Names))
	for i, ref := range p.Names {
		np.choice[i] = ref.Default
		// A name settled earlier in this session is preselected again.
		if id, ok := m.nameMemory[ref.Key]; ok {
			for ci, c := range ref.Candidates {
				if c.ID == id {
					np.choice[i] = ci
				}
			}
		}
	}
	m.names = np
	m.mode = modeNames
}

func (m *Model) handleNamesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	np := m.names
	k := msg.String()
	switch k {
	case "esc":
		m.names = nil
		m.mode = modeNormal
		if np.reopen != nil {
			np.reopen()
		}
		return m, nil
	case "down", "tab":
		np.cur = clampInt(np.cur+1, 0, len(np.refs)-1)
	case "up", "shift+tab":
		np.cur = clampInt(np.cur-1, 0, len(np.refs)-1)
	case "c":
		np.choice[np.cur] = choiceCreate
	case "x":
		np.choice[np.cur] = choiceText
	case "A":
		// Apply this row's answer to every row with the same name.
		want := shortsyntax.Fold(np.refs[np.cur].Text)
		for i, r := range np.refs {
			if shortsyntax.Fold(r.Text) == want && np.choice[np.cur] < len(r.Candidates) {
				np.choice[i] = np.choice[np.cur]
			}
		}
	case "enter":
		return m, m.commitNames()
	default:
		if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			if i := int(k[0] - '1'); i < len(np.refs[np.cur].Candidates) {
				np.choice[np.cur] = i
			}
		}
	}
	np.err = ""
	return m, nil
}

// commitNames turns the answers into choices, creates the people that were asked for, replans and
// applies. The created people and the write undo together.
func (m *Model) commitNames() tea.Cmd {
	np := m.names
	m.beginGroup()
	choices := map[string]quick.NameChoice{}
	for i, ref := range np.refs {
		switch c := np.choice[i]; {
		case c == choiceText:
			choices[ref.Key] = quick.NameChoice{Action: "text"}
		case c == choiceCreate:
			p, err := m.createNamed(ref.Text)
			if err != nil {
				m.endGroup("")
				np.err = err.Error()
				return nil
			}
			choices[ref.Key] = quick.NameChoice{Action: "link", PersonID: p.ID, Name: fullName(p)}
			m.nameMemory[ref.Key] = p.ID
		default:
			cand := ref.Candidates[c]
			choices[ref.Key] = quick.NameChoice{Action: "link", PersonID: cand.ID, Name: cand.Name}
			m.nameMemory[ref.Key] = cand.ID
		}
	}
	plan := np.reparse(choices)
	switch {
	case len(plan.Problems) > 0:
		m.endGroup("")
		np.err = plan.Problems[0].Message
		m.afterWrite()
		return nil
	case !plan.Ready():
		m.endGroup("")
		np.err = strings.TrimSpace(plan.Preview)
		m.afterWrite()
		return nil
	}
	m.names = nil
	m.mode = modeNormal
	cmd := np.commit(plan)
	m.endGroup("")
	return cmd
}

// createNamed makes a contact from a typed name: the last word is the last name.
func (m *Model) createNamed(name string) (*model.Person, error) {
	parts := strings.Fields(strings.ReplaceAll(name, "_", " "))
	if len(parts) == 0 {
		return nil, fmt.Errorf("a name is required: %w", model.ErrValidation)
	}
	in := model.CreatePersonInput{FirstName: parts[0]}
	if len(parts) > 1 {
		in.FirstName = strings.Join(parts[:len(parts)-1], " ")
		last := parts[len(parts)-1]
		in.LastName = &last
	}
	return m.hCreatePerson(m.ctx, in)
}

// runLineAsk plans a prompt line and applies it. When names need a choice it opens the names
// list instead: asked is true and onDone runs after the choices, with the applied plan.
func (m *Model) runLineAsk(line string, env quick.Env, reopen func(), onDone func(*quick.Plan) tea.Cmd) (asked bool, err error) {
	plan := quick.Parse(line, env)
	if needsNames(plan) {
		reparse := func(ch map[string]quick.NameChoice) *quick.Plan {
			e := env
			e.Choices = ch
			return quick.Parse(line, e)
		}
		m.openNames(plan, reopen, reparse, func(p *quick.Plan) tea.Cmd {
			res, err := quick.Apply(m.ctx, m.qrepos(), p)
			if err != nil {
				m.setStatus(err.Error(), true)
				return m.showToast(err.Error(), true)
			}
			m.recordQuick(p.Preview, res.Changes)
			m.afterWrite()
			return onDone(p)
		})
		return true, nil
	}
	p2, err := m.runLine(line, env)
	if err != nil {
		return false, err
	}
	m.pendingCmd = onDone(p2)
	return false, nil
}

// --- drawing ---

func (m *Model) namesView() string {
	np := m.names
	st := m.st
	cut := lipgloss.NewStyle().MaxWidth(m.width)
	var lines []string
	lines = append(lines, cut.Render(st.Head.Render("Link people in this line?")+st.Dim.Render("   enter save  esc back")))
	lines = append(lines, cut.Render(st.Dim.Render("1-9 pick a person  c create a new contact  x keep as text  A same answer for the same name  up/down move")))
	for i, ref := range np.refs {
		var opts []string
		for ci, c := range ref.Candidates {
			label := fmt.Sprintf("[%d] %s", ci+1, c.Name)
			if np.choice[i] == ci {
				label = st.Name.Render("*" + label)
			}
			opts = append(opts, label)
		}
		if len(ref.Candidates) == 0 {
			opts = append(opts, st.Dim.Render("no match"))
		}
		create := "[c] create"
		if np.choice[i] == choiceCreate {
			create = st.Name.Render("*[c] create \"" + strings.ReplaceAll(ref.Text, "_", " ") + "\"")
		}
		keep := "[x] keep as text"
		if np.choice[i] == choiceText {
			keep = st.Name.Render("*[x] keep as text")
		}
		opts = append(opts, create, keep)
		marker := "  "
		if i == np.cur {
			marker = st.Name.Render("> ")
		}
		row := fmt.Sprintf("%s%d  %s  %s", marker, i+1, fit(ref.Token, 14), strings.Join(opts, "  "))
		lines = append(lines, cut.Render(row))
	}
	if np.err != "" {
		lines = append(lines, cut.Render(st.Err.Render(np.err)))
	}
	if len(lines) > m.height-1 {
		lines = lines[:max(1, m.height-1)]
	}
	return strings.Join(lines, "\n")
}
