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
- [x] ✅ CRM-015: Footer shows ctrl+o options (`808431a`, `688061b`, `f3e554d`)
  - Outcome: `ctrl+o options` shows in the footer of every view that has view options (People, Organizations, Tags, saved views, Merge, Archived). Deals and Tasks show nothing. The key follows the registry.
  - Decisions: owner chose the label `options` for every view, not `sort`, and the hint before `?`. Mutation sweep on the footer condition had no survivors. Review was by the implementing agent, not independent.
  - Exceptions: a footer too narrow for the hint also drops `?` (People at 80 columns). Two tests that need the help hint, `TestFooterButtonsAndStatusTap` and `TestHelpAndFooterMentionNewKeys`, now use width 160.
- [x] ✅ CRM-016: Footer hints wrap to a second row (`80e45e6`, `f07c3d4`)
  - Outcome: one footer component, `internal/tui/footer.go`. Hints that do not fit the width wrap to a second row, whole and in order, two rows at most. Used by the main footer and the edit, prompt, confirm, key capture, names, view editor, text, help and compare footers. Body height, mouse hits on hints, the toast row and the detail pane follow the real number of hint rows. People at 80 columns shows `ctrl+o options` and `?`.
  - Decisions: owner chose every footer, two rows, and a reusable component. `bodyHeight` reaches the footer through the field `hintRowsFn` (a direct call is an initialization cycle with `viewNavs`). A terminal under 8 rows keeps one hint row so the body keeps 3 rows. `TestKeyHintsAreOnlyInTheFooter` expects 2 footer lines for the names panel. Mutation sweep killed 9 of 9 after adding `TestDetailPaneFollowsTheWrappedFooter`. Review was by the implementing agent, not independent.
  - Exceptions: popup hints stay inside their box and still cut. The prompt input line is not cut to a terminal narrower than 29 columns (existing behavior, not changed here).
- [x] ✅ CRM-017: Letter jump in Deals and Tasks (`cd6c24a`)
  - Outcome: `ctrl+up` and `ctrl+down` jump to the previous or next first letter of the row title in Deals and Tasks, through the existing `unitLetter` entry in `viewNavs`. Saved views keep their page or group jump.
  - Decisions: the letter comes from the list order, so it is useful once the list is sorted by name (CRM-020). Mutation sweep killed 8 of 9, the survivor is equivalent. Review was by the implementing agent.
  - Exceptions: in the default order (Deals newest first, Tasks soonest due) the jump moves nearly row by row.
- [x] ✅ CRM-018: Order row in view options (`8d8ed5f`, `f472658`)
  - Outcome: `ctrl+o` has an Order row (ascending or descending) next to Sort in People, Organizations and Tags, remembered per view for the session. No new key. Merge and Archived have no Order row.
  - Decisions: an order the user has not set is the natural one of the sort (name ascending, recent and count descending), so nothing changed until a user flips it. A set order is kept when Sort changes. Mutation sweep killed 14 of 14. Review was by the implementing agent.
  - Exceptions: saved views are out of scope and "Save current view" stores only the sort field. Order has no effect in the Tags duplicates filter or tree layout.
- [x] ✅ CRM-020: Name sort for Deals and Tasks (`29ef004`, `c557cfb`)
  - Outcome: Deals and Tasks have the Sort and Order rows of `ctrl+o` with a Name sort, so the letter jump (CRM-017) follows title order. Defaults are unchanged: Deals newest created first, Tasks soonest due first.
  - Decisions: Tasks sorted by due date keep undated tasks last in both directions. Footer shows `ctrl+o options` in both views because they now have options. `TestFooterShowsViewOptionsWhereTheViewHasThem` now lists Birthdays as the view without options. Mutation sweep killed 12 of 12 after adding `TestSortRowNamesTheDefaultSortOfDealsAndTasks`. Review was by the implementing agent.
  - Exceptions: none.
- [x] ✅ CRM-019: Middle name for People (`bf4fc89`)
  - Outcome: `people.middle_name` (migration 022, database version 21) is a detail field after last name, searchable, filterable as `middle`, and synced to phones through the vCard N additional names. Carried by the CLI (`--middle-name`), MCP, command bar (`set Bob: middle=...`), merge, device sync and DAV (`MapVersion` 2, so phones refetch). Existing phone middle names are backfilled by the migration.
  - Decisions: display name, rows, links, briefings and name sort stay First Last. Key `middle`, column `middle_name`. Replica `SchemaVersion` is not bumped, so older peers keep the field as unknown until they upgrade. Mutation sweep killed 44 of 45, the survivor is equivalent (DAV create path). Review was by the implementing agent.
  - Exceptions: no middle name in the new-contact line, the Dex CSV import or the `crm context` briefing. Migration down on a sync-enabled database leaves people capture off until `crm peers` Enable or Refresh runs.
