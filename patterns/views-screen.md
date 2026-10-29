---
type: pattern
scale: screen
status: decided by owner 2026-10-29, built in feat/settings-views
---

# One Views screen lists, opens, shows, hides and orders every view

**Problem.** Two screens did one job. `v` listed the views that were on and jumped to one. A separate Modules screen turned views on or off and ordered them. The user had to know both, and a hidden view was invisible in the list that was supposed to show every view.

**Therefore.** `v` opens one screen, the Views screen. It lists every view as a grouped list ([[grouped-lists-with-dividers]]): the views that are shown first, in tab order with their number key, then a divider named Hidden and the hidden views under it.

| Key | Does |
|---|---|
| `enter` | goes to the view under the cursor, shown or hidden ([[universal-verbs]]) |
| a number key | goes to that shown view |
| `x` | shows or hides the view, the universal mark ([[universal-verbs]]) |
| `shift+up`, `shift+down` | move the view inside its group, the scoped keys the Modules screen already had ([[keymap-layers-and-scopes]]) |
| `esc` | closes |

No new key. The separate Modules screen and its palette entry are gone ([[one-abstraction-many-contexts]]). The palette entry "Modules: reset" stays, and so do `crm modules` and the MCP tools `crm_modules_*`.

**Never hidden.** The Views screen is not a module, it is the `v` key, so no arrangement can remove it. Settings is a module with `Always` set: it cannot be turned off from the screen, the CLI, MCP or a hand-edited `config.toml` (the file is read, the entry is reported and ignored). The user can therefore never lock themselves out of the way back ([[settings-view]]).

**Reorder stays inside the group.** A shown view never jumps across the divider, so `shift+down` on the last shown view does nothing. Showing a view puts it back at its place in the order.

**Defaults.** Archived and Sync are hidden by default (`defaultHidden` in `internal/modules`), so a new setup has nine tabs: Home, People, Follow-up, Tasks, Organizations, Deals, Birthdays, Merge, Settings. They stay reachable from Settings ("Open Archived", "Open Sync"), from the bar (type `archived` or `sync`) and from the Hidden group of this screen. A `[modules]` table that is already in `config.toml` is respected as written: a hand-written table with no `hidden` list hides nothing.

Built (M31): `internal/tui/modules.go` (`openViews`, `viewItems`, `shiftModule`), divider rows in the overlay (`overlayItem.divider`, drawn with `ruleLine` like the Birthdays day dividers), `internal/modules` (`Always`, `defaultHidden`). Tests in `views_screen_test.go` and `modules_test.go`.

**Because.** Owner request 2026-10-29: one screen that does all three jobs, with the hidden views under a divider, and Archived and Sync moved under Settings so the tabs stay short.

**Larger patterns.** [[modules-are-user-arranged]], [[one-abstraction-many-contexts]]

**Smaller patterns.** [[settings-view]], [[saved-views]], [[grouped-lists-with-dividers]]

**Sources.** Owner request 2026-10-29. `internal/tui/modules.go`, `internal/tui/overlay.go`, `internal/modules/modules.go`
