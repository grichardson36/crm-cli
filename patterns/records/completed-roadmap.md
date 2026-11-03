---
type: roadmap
---
# Completed Roadmap

Items below are built. This record is for implementation history and links to deeper documentation.

Badges show only the gates that have evidence. All items below passed `go build`, `go vet` and `go test ./...` before merge. None has had a mutation sweep or an independent review yet, so none has 🧪, 🔍 or ✅. The next gate for each is Test.

# 2026
## Week of 2026-11-02

- [ ] 🧭 🔨 CRM-001: Theme pane (X46) (`8cdf305`, `b534952`, `e249b6c`, `3d8f11c`)
  - Outcome: one Settings row, Theme. One entry per theme with a dark and a light palette (`default`, `anthropocene`, `catppuccin`, `gruvbox`, `high-contrast`, `nord`, `solarized`, custom). Mode `auto | light | dark` with `tab` in the pane. `auto` never sets a terminal color. `enter` loads, `c` copies, `e` `d` `shift+r` on custom themes. Footer shows only usable keys. Custom files are `[dark.colors]` and `[light.colors]`. `crm theme list|show|set|init|mode|path`.
  - Decisions: no legacy aliases or old file formats. Colors section stays a separate set of kind overrides. `default` keeps its name until the app is named.
  - Exceptions: interactive color picker is on the roadmap. The forced light mode on a dark terminal and the swatches are unverified in a real terminal.
- [ ] 🧭 🔨 CRM-002: Navigation standard (X47) (`d23ca75`)
  - Outcome: [[navigation-standard]] with tests for every overlay and two-pane screen. `esc` always leaves.
- [ ] 🧭 🔨 CRM-003: Tags pane lists the tagged objects (X48) (`b157f74`)
  - Outcome: `enter` opens an object, `d` or `u` takes the tag off.
  - Exceptions: `space` and bulk verbs inside the pane are not built. The tag page has them.
- [ ] 🧭 🔨 CRM-004: Add standard (X49) (`355d29d`)
  - Outcome: [[add-standard]]. A person adds from the footer line like every kind, the form is the palette action "Person: add with the form", and every add lands on the new object.
- [ ] 🧭 🔨 CRM-005: Undo and redo history popups (`292ab9c`)
  - Outcome: `ctrl+shift+z` lists the session's changes, newest first, and undoes back to the chosen one. Redo history is a palette action.
  - Exceptions: `ctrl+shift+z` on Windows Terminal is unverified.
- [ ] 🧭 🔨 CRM-006: Reusable popup component (`1adc92a`, `b9cde4b`)
  - Outcome: [[popup-component]]. A centered box with a filter line or a title, hints in the bottom row, used by every overlay. Inline syntax helpers stay while typing. Views popup has a filter line, the hidden-line model and `shift+up/down` to hide or show.
  - Exceptions: popup dimming, light themes, short terminals and mouse clicks are unverified.
- [ ] 🧭 🔨 CRM-007: Types lists live in the Settings pane (`01a88e6`)
  - Outcome: Deal stages, Deal roles, Notes, Links and Ignored company texts open only in the right pane, with a used-by count, `tab` cycles and `shift+r` reset. "Counts as contact" is now "mark contacted". No standalone popup, key or palette action.
  - Decisions: reset never deletes custom items. `d` on a built-in ignore text turns it off.
  - Exceptions: the Links "has: anniversary" column waits for the anniversaries feature.
- [ ] 🧭 🔨 CRM-008: Settings Views and Colors panes (`b9cde4b`)
  - Outcome: Views pane manages saved views with bare `e` `d` `r`. Colors pane edits the six object kind colors with a swatch and a hex field, in Views order, reset per kind.
  - Exceptions: mouse clicks in the panes do nothing.
- [ ] 🧭 🔨 CRM-009: Long values wrap in the right pane (`d5e8469`)
  - Outcome: choice options and long read-only rows wrap instead of running off the pane.
- [ ] 🧭 🔨 CRM-010: Edit an activity's date inline (`04541c8`)
  - Outcome: a date cell on note rows, one validated update path and one undo step. CLI, MCP and repository accept and reject the same dates.
- [ ] 🧭 🔨 CRM-011: More date formats (`8caf632`)
  - Outcome: one parser accepts m/d/y, m-d-y, m/d, "Dec 25" and "25 December" forms. The first number is the month, and a first number above 12 reads day-first. Every door uses it.
  - Exceptions: CLI and MCP date fields now also accept phrases like `next friday`. `log --at`, `note --at` and `log edit --date` keep their own parser.
- [ ] 🧭 🔨 CRM-012: Toasts draw above the bottom input bar (`04541c8`)
  - Outcome: a toast never covers typed input, a confirm line or the status line.
- [ ] 🧭 🔨 CRM-013: Home heading "Follow-up" (`8e8fa0c`)
  - Outcome: the Home heading and its jump row use the name of the Follow-up screen.
- [ ] 🧭 🔨 CRM-014: Discord profile links (`8e8fa0c`)
  - Outcome: `discord` handle kind in every door, with the vCard social profile map. Migration 021 edits the stored CHECK text without rebuilding the table.
  - Exceptions: no web-search entry for Discord. The migration was tested without device sync enabled.
