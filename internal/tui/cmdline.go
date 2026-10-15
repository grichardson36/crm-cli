package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jdanielnd/crm-cli/internal/model"
	"github.com/jdanielnd/crm-cli/internal/quick"
	"github.com/jdanielnd/crm-cli/internal/shortsyntax"
	"github.com/mattn/go-runewidth"
)

// The command bar is the palette with a command line in it. Type a verb and a space (n, t, log,
// set, tag, snooze) and the palette shows what the line will do, live. Tab completes, Enter runs.
// The parsing, the preview and the writes all come from internal/quick, the same code crm do uses.

// cmdState is the state of the command line inside the palette overlay.
type cmdState struct {
	plan       *quick.Plan
	comp       quick.Completion
	sel        int
	confirming bool
	msg        string
}

// cycleState remembers the last Tab insertion so Tab again moves to the next candidate.
type cycleState struct {
	start, end int
	cands      []string
	idx        int
	after      string
}

// markStyle picks the style of a highlighted range from the active theme.
func (m *Model) markStyle(mk quick.Mark) lipgloss.Style {
	if mk.Bad {
		return m.st.Err.Underline(true)
	}
	switch mk.Kind {
	case "verb", "subject", "person":
		return m.st.Name
	case "tag":
		if mk.New {
			return m.st.Mark.Underline(true)
		}
		return m.st.Mark
	case "due", "deadline":
		return m.st.Ok
	case "duration", "cadence", "priority", "location", "recurrence":
		return m.st.Warn
	case "field", "escape":
		return m.st.Dim.Bold(true)
	}
	return lipgloss.NewStyle()
}

// quickEnv describes the current database to the quick package. Index, tags and types are cached
// until the next reload.
func (m *Model) quickEnv() quick.Env { return m.quickEnvFor(m.cmdPerson) }

func (m *Model) quickEnvFor(person int64) quick.Env {
	if m.qix == nil {
		m.qix = quick.NewIndex(m.people)
	}
	if m.qtags == nil {
		m.qtags = []string{}
		if tags, err := m.tagr.FindAll(m.ctx); err == nil {
			for _, t := range tags {
				m.qtags = append(m.qtags, t.Name)
			}
		}
	}
	if m.qtypes == nil {
		m.qtypes = m.noteTypeNames()
	}
	return quick.Env{
		Now: m.now(), Index: m.qix, Tags: m.qtags, NoteTypes: m.qtypes, Context: person, NoSyntax: m.noSyntax,
		LoadPerson: func(id int64) (*model.Person, error) {
			p, err := m.pr.FindByID(m.ctx, id)
			if err != nil {
				return nil, err
			}
			return p, m.pr.LoadDetails(m.ctx, p)
		},
	}
}

func (m *Model) qrepos() quick.Repos {
	return quick.Repos{People: m.pr, Interactions: m.ir, Tasks: m.tr, Tags: m.tagr}
}

// palettePerson is the person the command bar acts on without a name: the selected one.
func (m *Model) palettePerson() int64 {
	if m.view == viewArchived || m.view == viewMerge || m.view == viewSync {
		return 0
	}
	return m.curPerson()
}

// matchesActionTitle is true when the typed text is the start of an action title, such as
// "set keep". Then the palette keeps searching actions instead of reading a command.
func (m *Model) matchesActionTitle(val string) bool {
	v := strings.ToLower(val)
	for i := range m.actions {
		if strings.HasPrefix(strings.ToLower(m.actions[i].title), v) {
			return true
		}
	}
	return false
}

// bareVerb is true for exactly a verb and a space ("snooze "), which always opens the command
// line even when an action title starts the same way.
func bareVerb(val string) bool {
	if !strings.HasSuffix(val, " ") {
		return false
	}
	word := strings.TrimSpace(val)
	if strings.ContainsAny(word, " \t") {
		return false
	}
	_, ok := quick.VerbOf(word)
	return ok
}

// byteCursor is the cursor of the palette input as a byte offset into its value.
func (o *overlay) byteCursor() int {
	runes := []rune(o.filter.Value())
	pos := o.filter.Position()
	if pos > len(runes) {
		pos = len(runes)
	}
	return len(string(runes[:pos]))
}

// syncCmd re-reads the palette text: it switches the overlay in or out of command mode and
// refreshes the plan and the completions.
func (m *Model) syncCmd() {
	o := m.ov
	if o == nil || !o.palette {
		return
	}
	val := o.filter.Value()
	if !quick.IsCommand(val) || (m.matchesActionTitle(val) && !bareVerb(val)) {
		if o.cmd != nil {
			o.cmd = nil
			o.refilter()
		}
		return
	}
	env := m.quickEnv()
	env.Dismissed = o.dismissed
	plan := quick.Parse(val, env)
	comp := quick.Complete(val, o.byteCursor(), env)
	if o.cmd == nil {
		o.cmd = &cmdState{}
	}
	o.cmd.plan, o.cmd.comp, o.cmd.sel, o.cmd.confirming, o.cmd.msg = plan, comp, 0, false, ""
}

// verbCompletion completes a half-typed verb: "ta" becomes "task ".
func verbCompletion(val string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(val))
	if v == "" || strings.ContainsAny(v, " \t") {
		return "", false
	}
	for _, verb := range quick.Verbs {
		if strings.HasPrefix(verb, v) {
			return verb + " ", true
		}
	}
	return "", false
}

func (o *overlay) setLine(s string, bytePos int) {
	o.filter.SetValue(s)
	o.filter.SetCursor(utf8.RuneCountInString(s[:min(bytePos, len(s))]))
}

// cmdKey handles keys that mean something in command mode. ok is false when the generic overlay
// handling should run.
func (m *Model) cmdKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	o := m.ov
	k := msg.String()
	if o.cmd == nil {
		if k == "tab" {
			if v, ok := verbCompletion(o.filter.Value()); ok {
				o.setLine(v, len(v))
				o.cycle = nil
				m.syncCmd()
				return m, nil, true
			}
		}
		return m, nil, false
	}
	c := o.cmd
	if c.confirming {
		switch k {
		case "enter":
			return m, m.cmdRun(), true
		case "esc":
			c.confirming = false
		}
		return m, nil, true
	}
	switch k {
	case "enter":
		return m, m.cmdEnter(), true
	case "tab":
		m.cmdTab(1)
		return m, nil, true
	case "shift+tab":
		m.cmdTab(-1)
		return m, nil, true
	case "alt+d":
		return m, m.dismissChip(), true
	case "up", "ctrl+p":
		c.sel = max(0, c.sel-1)
		return m, nil, true
	case "down", "ctrl+n":
		c.sel = clampInt(c.sel+1, 0, max(0, len(c.comp.Candidates)-1))
		return m, nil, true
	case "pgup", "pgdown":
		return m, nil, true
	}
	return m, nil, false
}

// cmdTab inserts the selected completion, or the next one when Tab is pressed again.
func (m *Model) cmdTab(step int) {
	o := m.ov
	c := o.cmd
	val := o.filter.Value()
	if o.cycle != nil && o.cycle.after == val && len(o.cycle.cands) > 0 {
		n := len(o.cycle.cands)
		o.cycle.idx = (o.cycle.idx + step + n) % n
	} else {
		if len(c.comp.Candidates) == 0 {
			c.msg = "nothing to complete here"
			return
		}
		o.cycle = &cycleState{start: c.comp.Start, end: c.comp.End, cands: c.comp.Candidates, idx: clampInt(c.sel, 0, len(c.comp.Candidates)-1)}
	}
	cy := o.cycle
	cand := cy.cands[cy.idx]
	end := min(cy.end, len(val))
	next := val[:min(cy.start, len(val))] + cand + val[end:]
	cy.end = cy.start + len(cand)
	cy.after = next
	o.setLine(next, cy.end)
	// Refresh the plan for the new text, but keep the candidates being cycled.
	keep := c.comp
	m.syncCmd()
	if o.cmd != nil {
		o.cmd.comp = keep
		o.cmd.sel = cy.idx
	}
}

// cmdEnter runs the line, asks for confirmation first when it changes several fields, or says
// why it cannot run.
func (m *Model) cmdEnter() tea.Cmd {
	c := m.ov.cmd
	p := c.plan
	switch {
	case m.readOnly:
		c.msg = "read-only: this line would change data"
	case needsNames(p):
		return m.askNamesFromBar()
	case len(p.Problems) > 0:
		c.msg = p.Problems[0].Message
	case !p.Ready():
		c.msg = "nothing to do yet"
	case p.NeedsConfirm && !c.confirming:
		c.confirming = true
	default:
		return m.cmdRun()
	}
	return nil
}

// cmdRun applies the plan, closes the palette and remembers the changes for Undo.
func (m *Model) cmdRun() tea.Cmd {
	c := m.ov.cmd
	p := c.plan
	res, err := quick.Apply(m.ctx, m.qrepos(), p)
	if err != nil {
		c.msg, c.confirming = err.Error(), false
		return nil
	}
	m.closeOverlay()
	m.recordQuick(p.Preview, res.Changes)
	m.afterWrite()
	m.setStatus("done: "+p.Preview, false)
	return m.showToast("Done: "+p.Preview, false)
}

func (m *Model) toggleSyntax() {
	m.noSyntax = !m.noSyntax
	if m.noSyntax {
		m.setStatus("short syntax off: #, @, !, ~, by, remind, every and dates stay plain text", false)
	} else {
		m.setStatus("short syntax on", false)
	}
}

// runLine plans and applies a line built by a prompt (the note and task prompts). Problems and
// failures come back as errors. The changes go on the undo stack.
func (m *Model) runLine(line string, env quick.Env) (*quick.Plan, error) {
	plan := quick.Parse(line, env)
	if len(plan.Problems) > 0 {
		return plan, fmt.Errorf("%s: %w", plan.Problems[0].Message, quick.ErrorFor(plan.Problems[0].Code))
	}
	if !plan.Ready() {
		return plan, fmt.Errorf("%s: %w", strings.TrimSpace(plan.Preview), model.ErrValidation)
	}
	res, err := quick.Apply(m.ctx, m.qrepos(), plan)
	if err != nil {
		return plan, err
	}
	m.recordQuick(plan.Preview, res.Changes)
	return plan, nil
}

// extras words what a prompt line did besides its main write: ", #deal, reminder 2026-10-16".
func extras(plan *quick.Plan) string {
	var parts []string
	for i, c := range plan.Changes {
		if i == 0 {
			continue
		}
		switch {
		case c.Op == "tag":
			parts = append(parts, "#"+c.New)
		case c.Entity == "task":
			parts = append(parts, "reminder "+c.New)
		case c.Op == "update":
			parts = append(parts, c.Field+" "+c.New)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return ", " + strings.Join(parts, ", ")
}

// normalizeDue reads a typed due date such as fri or 2026-12-01 into its stored form. Text that
// is not a date is kept as typed, like before.
func (m *Model) normalizeDue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || m.noSyntax {
		return raw
	}
	// "by <date>" takes every date form, so the text after the pipe may be fri, 2w or 2026-12-01.
	res := shortsyntax.Parse("by "+raw, shortsyntax.Options{Now: m.now()})
	if res.Deadline != nil && res.Title == "" {
		return res.Deadline.String()
	}
	return raw
}

// --- drawing ---

func (m *Model) cmdInputLine(o *overlay) string {
	val := o.filter.Value()
	marks := o.cmd.plan.Marks
	cursor := o.byteCursor()
	type cell struct {
		s     string
		style lipgloss.Style
		w     int
	}
	var cells []cell
	cursorIdx := -1
	for bi, r := range val {
		st := lipgloss.NewStyle()
		for _, mk := range marks {
			if bi >= mk.Start && bi < mk.End {
				st = m.markStyle(mk)
			}
		}
		if bi == cursor {
			cursorIdx = len(cells)
			st = st.Reverse(true)
		}
		cells = append(cells, cell{string(r), st, runewidth.RuneWidth(r)})
	}
	if cursor >= len(val) {
		cursorIdx = len(cells)
		cells = append(cells, cell{" ", lipgloss.NewStyle().Reverse(true), 1})
	}
	room := m.width - 3
	total := 0
	for _, c := range cells {
		total += c.w
	}
	skip := 0
	for total > room && skip < cursorIdx {
		total -= cells[skip].w
		skip++
	}
	var b strings.Builder
	b.WriteString(m.st.Dim.Render("> "))
	for _, c := range cells[skip:] {
		b.WriteString(c.style.Render(c.s))
	}
	return b.String()
}

// cmdView draws the palette in command mode.
func (m *Model) cmdView() string {
	o := m.ov
	c := o.cmd
	p := c.plan
	cut := lipgloss.NewStyle().MaxWidth(m.width)
	var lines []string
	lines = append(lines, cut.Render(m.st.Head.Render("Command bar")+m.st.Dim.Render("   enter run  tab complete  esc close")))
	lines = append(lines, cut.Render(m.cmdInputLine(o)))

	preview := p.Preview
	style := m.st.Name
	switch {
	case m.readOnly:
		preview, style = "read-only: "+preview, m.st.Err
	case len(p.Problems) > 0:
		style = m.st.Err
	case !p.Ready():
		style = m.st.Dim
	}
	preview = runewidth.Truncate(preview, 3*max(1, m.width)-3, "…")
	lines = append(lines, lipgloss.NewStyle().Width(max(1, m.width)).Render(style.Render(preview)))

	if c.confirming {
		lines = append(lines, cut.Render(m.st.Head.Render("Confirm "+p.Subject.Name)))
		for _, ch := range p.Changes {
			switch ch.Op {
			case "update":
				lines = append(lines, cut.Render(fmt.Sprintf("  %s  %s -> %s", fit(ch.Field, 10), m.st.Dim.Render(orNone(ch.Old)), m.st.Name.Render(orNone(ch.New)))))
			default:
				lines = append(lines, cut.Render("  "+ch.Describe()))
			}
		}
		lines = append(lines, cut.Render(m.st.Dim.Render("enter apply  esc back  (ctrl+z undoes it afterwards)")))
		return strings.Join(lines, "\n")
	}
	if c.msg != "" {
		lines = append(lines, cut.Render(m.st.Err.Render(c.msg)))
	}
	room := max(0, m.height-strings.Count(strings.Join(lines, "\n"), "\n")-2)
	var rest []string
	for i, cand := range c.comp.Candidates {
		line := "  " + cand
		if i == c.sel {
			rest = append(rest, cut.Render(m.st.Sel.Render(fit(line, m.width))))
		} else {
			rest = append(rest, cut.Render(line))
		}
	}
	if len(p.Problems) > 0 {
		for _, cand := range p.Problems[0].Candidates {
			rest = append(rest, cut.Render(m.st.Dim.Render("  "+cand.Name)))
		}
	}
	for _, w := range p.Warnings {
		rest = append(rest, cut.Render(m.st.Dim.Render("  note: "+w)))
	}
	if len(rest) == 0 {
		rest = append(rest, cut.Render(m.st.Dim.Render("  verbs: n note, t task, log, set, tag, snooze   tokens: #tag @Name !high ~30m by fri remind fri every 3m")))
	}
	if len(rest) > room {
		rest = rest[:room]
	}
	lines = append(lines, rest...)
	return strings.Join(lines, "\n")
}

func orNone(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}

// dismissChip turns the highlighted chip under (or just before) the cursor back into plain text.
// Parsing then leaves that text alone, so a word like "sat" in a sentence is not a date.
func (m *Model) dismissChip() tea.Cmd {
	o := m.ov
	val := o.filter.Value()
	cur := o.byteCursor()
	best := -1
	for i, mk := range o.cmd.plan.Marks {
		switch mk.Kind {
		case "verb", "subject", "field", "escape":
			continue
		}
		if mk.Start <= cur && cur <= mk.End {
			best = i
			break
		}
		if mk.End <= cur {
			best = i
		}
	}
	if best < 0 {
		return m.showToast("No chip to turn back into text here", true)
	}
	mk := o.cmd.plan.Marks[best]
	text := val[mk.Start:mk.End]
	o.dismissed = append(o.dismissed, text)
	m.syncCmd()
	return m.showToast("Kept as plain text: "+text, false)
}
// askNamesFromBar lists the unsettled names of the command line. After the choices the line runs
// as usual (a multi-field set still asks to confirm).
func (m *Model) askNamesFromBar() tea.Cmd {
	o := m.ov
	c := o.cmd
	line := o.filter.Value()
	dismissed := o.dismissed
	reparse := func(ch map[string]quick.NameChoice) *quick.Plan {
		env := m.quickEnv()
		env.Dismissed, env.Choices = dismissed, ch
		return quick.Parse(line, env)
	}
	m.openNames(c.plan, func() { m.mode = modeOverlay }, reparse, func(p *quick.Plan) tea.Cmd {
		m.mode = modeOverlay
		c.plan = p
		if p.NeedsConfirm {
			c.confirming = true
			return nil
		}
		return m.cmdRun()
	})
	return nil
}