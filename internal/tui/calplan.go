package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdanielnd/crm-cli/internal/cal/tz"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/schedule"
)

// Plan overlay: the planner's proposal as a list. Pick a row to accept it, reject it or lock it
// at the proposed time; pick the first row to apply.

type planUI struct {
	res    *schedule.PlanResult
	reject map[int64]bool
	lock   map[int64]bool
}

func (m *Model) openPlan() tea.Cmd {
	res, err := schedule.PlanTasks(m.ctx, m.db, schedule.PlanOptions{Now: m.now()})
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	if len(res.Decisions) == 0 {
		m.setStatus("no auto-scheduled tasks: A on a task turns it on, S sets a duration", false)
		return nil
	}
	m.showPlan(&planUI{res: res, reject: map[int64]bool{}, lock: map[int64]bool{}})
	return nil
}

func (m *Model) showPlan(p *planUI) {
	var items []overlayItem
	changes := 0
	for _, d := range p.res.Decisions {
		if p.reject[d.TaskID] {
			continue
		}
		switch d.Decision {
		case schedule.Placed, schedule.Late:
			if len(d.Blocks) > 0 && d.PrevStart != d.Blocks[0].Start {
				changes++
			}
		case schedule.Unfit, "skipped":
			if d.PrevStart != "" {
				changes++
			}
		}
	}
	s := p.res.Summary
	items = append(items, overlayItem{id: "apply", label: fmt.Sprintf("Apply %d change(s)", changes),
		desc: fmt.Sprintf("%d placed, %d late, %d unfit, %d without a duration. Locked tasks and events never move", s.Placed, s.Late, s.Unfit, s.Skipped), value: "apply"})
	for _, d := range p.res.Decisions {
		mark := "  "
		switch {
		case p.reject[d.TaskID]:
			mark = "x "
		case p.lock[d.TaskID]:
			mark = "L "
		}
		when := d.Decision
		if len(d.Blocks) > 0 {
			st, _ := tz.ParseInstant(d.Blocks[0].Start)
			en, _ := tz.ParseInstant(d.Blocks[len(d.Blocks)-1].End)
			when = fmt.Sprintf("%s-%s %s", st.In(m.loc).Format("Mon 2 Jan 15:04"), en.In(m.loc).Format("15:04"), d.Decision)
			if len(d.Blocks) > 1 {
				when += fmt.Sprintf(" in %d blocks", len(d.Blocks))
			}
		} else if d.Reason != "" {
			when += " (" + d.Reason + ")"
		}
		desc := ""
		if len(d.Why) > 0 {
			desc = d.Why[0]
		}
		items = append(items, overlayItem{id: fmt.Sprint(d.TaskID), label: mark + cleanCell(d.Title) + "  " + when, desc: desc, value: fmt.Sprint(d.TaskID)})
	}
	m.openPicker("Plan (enter on a task: accept, reject or lock; first row applies)", items, func(m *Model, it overlayItem) {
		if it.value == "apply" {
			m.applyPlan(p)
			return
		}
		var id int64
		_, _ = fmt.Sscan(it.value, &id)
		rows := []overlayItem{
			{id: "accept", label: "Accept", desc: "apply this row's proposal", value: "accept"},
			{id: "reject", label: "Reject", desc: "leave the task as it is", value: "reject"},
			{id: "lock", label: "Accept and lock", desc: "place it at the proposed time and never move it", value: "lock"},
		}
		m.openPicker(it.label, rows, func(m *Model, pick overlayItem) {
			delete(p.reject, id)
			delete(p.lock, id)
			switch pick.value {
			case "reject":
				p.reject[id] = true
			case "lock":
				p.lock[id] = true
			}
			m.showPlan(p)
		})
	})
}

func (m *Model) applyPlan(p *planUI) {
	j, err := schedule.Apply(m.ctx, m.db, p.res, schedule.ApplyOptions{Reject: p.reject, Lock: p.lock})
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("plan applied to %d task(s) (palette: Undo last plan)", len(j.Entries)), false)
}

func (m *Model) undoPlan() {
	j, err := repo.NewPlanRepo(m.db).Undo(m.ctx)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.afterWrite()
	m.setStatus(fmt.Sprintf("plan #%d undone (%d task(s) restored)", j.ID, len(j.Entries)), false)
}
