package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/jdanielnd/crm-cli/internal/dav"
	"github.com/jdanielnd/crm-cli/internal/db/repo"
	"github.com/jdanielnd/crm-cli/internal/model"
)

// Read-only DAV views: the palette twins of `crm dav info`, `crm dav conflicts` and
// `crm dav status`. None of them starts the server or changes data, so they work with --read-only.
// Undoing a conflict stays a CLI action (`crm dav conflicts undo ID`).

// --- info ---

// showDavInfo shows the connection details and phone setup steps. The user and base URL come
// from CRM_DAV_USER and CRM_DAV_BASE_URL, like the CLI without flags.
func (m *Model) showDavInfo() {
	user := os.Getenv("CRM_DAV_USER")
	if user == "" || !dav.ValidUser(user) {
		user = dav.DefaultUser
	}
	book := dav.BookNameFromDB(m.ctx, m.db)
	if m.env.DBPath != "" {
		book = dav.BookNameFor(m.env.DBPath)
	}
	i := dav.BuildInfo(os.Getenv("CRM_DAV_BASE_URL"), user, book)
	var b strings.Builder
	line := func(k, v string) { fmt.Fprintf(&b, "%-15s %s\n", k+":", v) }
	line("server", i.BaseURL)
	line("user", i.User)
	line("password", i.Password)
	line("address book", i.AddressBook)
	line("book URL", i.AddressBookURL)
	line("discovery URL", i.DiscoveryURL)
	for _, p := range []struct{ key, title string }{{"android", "Android (DAVx5)"}, {"ios", "iOS"}, {"macos", "macOS"}} {
		fmt.Fprintf(&b, "\n%s\n", p.title)
		for n, step := range i.Steps[p.key] {
			fmt.Fprintf(&b, "  %d. %s\n", n+1, step)
		}
	}
	b.WriteString("\n")
	for _, n := range i.Notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	b.WriteString("\nThis view does not start the server. Run crm dav serve in a terminal.\n")
	m.openText("DAV info", b.String(), nil)
}

// --- status ---

func (m *Model) showDavStatus() {
	st, err := repo.NewDavRepo(m.db).Status(m.ctx)
	if err != nil {
		m.setStatus("dav status: "+err.Error(), true)
		return
	}
	at := func(p *string) string {
		if p == nil {
			return "never"
		}
		return *p + " UTC"
	}
	var b strings.Builder
	line := func(k, v string) { fmt.Fprintf(&b, "%-23s %s\n", k+":", v) }
	line("contacts exposed", fmt.Sprint(st.ContactsExposed))
	line("fetched by a device", fmt.Sprint(st.SyncedToDevice))
	line("created from a device", fmt.Sprint(st.DeviceCreated))
	line("last request", at(st.LastRequestAt))
	line("last contact served", at(st.LastGetAt))
	line("last device edit", at(st.LastPutAt))
	line("conflicts", fmt.Sprintf("%d (%d open)", st.Conflicts, st.OpenConflicts))
	m.openText("DAV status", b.String(), nil)
}

// --- conflicts ---

func conflictValue(p *string) string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return "(empty)"
	}
	return cleanCell(*p)
}

// showDavConflicts lists the logged conflicts, newest first, with undone ones marked. Enter
// opens one with both values.
func (m *Model) showDavConflicts() {
	cs, err := repo.NewDavRepo(m.db).Conflicts(m.ctx, true, 500)
	if err != nil {
		m.setStatus("dav conflicts: "+err.Error(), true)
		return
	}
	if len(cs) == 0 {
		m.setStatus("no DAV conflicts logged", false)
		return
	}
	items := make([]overlayItem, len(cs))
	for i, c := range cs {
		label := fmt.Sprintf("#%d %s: %s", c.ID, c.PersonName, c.Field)
		if c.Undone {
			label += " (undone)"
		}
		items[i] = overlayItem{
			id: fmt.Sprint(c.ID), label: label, value: fmt.Sprint(i),
			desc: fmt.Sprintf("CRM %s | device %s | %s won", conflictValue(c.CRMValue), conflictValue(c.DeviceValue), c.Winner),
		}
	}
	m.openPicker(fmt.Sprintf("DAV conflicts (%d)", len(cs)), items, func(m *Model, it overlayItem) {
		var idx int
		_, _ = fmt.Sscan(it.value, &idx)
		if idx < 0 || idx >= len(cs) {
			return
		}
		m.openText(fmt.Sprintf("DAV conflict #%d", cs[idx].ID), davConflictText(cs[idx]), func(m *Model) { m.showDavConflicts() })
	})
}

func davConflictText(c model.SyncConflict) string {
	var b strings.Builder
	line := func(k, v string) { fmt.Fprintf(&b, "%-10s %s\n", k+":", v) }
	line("id", fmt.Sprint(c.ID))
	line("person", fmt.Sprintf("%s (#%d)", c.PersonName, c.PersonID))
	line("field", c.Field)
	line("CRM", conflictValue(c.CRMValue))
	line("device", conflictValue(c.DeviceValue))
	line("winner", c.Winner)
	line("undone", fmt.Sprint(c.Undone))
	line("when", c.CreatedAt)
	if !c.Undone && c.Winner != model.ConflictWinnerLink {
		fmt.Fprintf(&b, "\nTo restore the losing value run: crm dav conflicts undo %d\n", c.ID)
	}
	return b.String()
}
