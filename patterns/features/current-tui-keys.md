---
type: feature-area
---
# Current TUI keys


BRANCH feat/keymap-a (owner decisions 2026-10-27, core gate 6, navigation units and keymap layers; not merged). Rows in the sections below that name `t`, `n`, `+`, `#`, `u`, `T`, `L`, `U`, `M`, `D`, `X`, `c`, `p`, `o`, `r`, `j`, `k`, `g`, `G`, `ctrl+t` or `ctrl+a` describe the keys before this branch: use the migration table here.

BRANCH feat/keymap-b (owner decisions 2026-10-28, keymap part B2, built on main 6fcc672, not merged). It applies the audit's clear removals and the owner's choices, and rows below that name `ctrl+n`, `ctrl+p`, `:`, `F`, `ctrl+b`, `ctrl+w`, `ctrl+l`, `ctrl+f`, `*`, `E`, `y` or a tab toggle in the compare describe the keys before this branch: use the B2 table here. What B2 changes:

- **Cleanup**: `ctrl+n`, `ctrl+p`, `:`, `q` in the compare and in text pages, `J`/`K`/`r` in Modules are gone. `F` (cadence), `ctrl+b` (next pair), `ctrl+w` (switch pane) and the modules reset are palette entries. Kept: `ctrl+e`, `m`, `d`, `y`/`n` answers in prompts, `alt+d`, the digit hints in pickers, the digits, `[ ]`, `v`, `ctrl+d`, `ctrl+u`. `ctrl+left` and `ctrl+right` are aliases of `[` and `]` (no view uses them as a jump unit, jumps stay on `ctrl+up`/`ctrl+down`).
- **Merge** lists every candidate grouped by confidence level under divider headings (the Dates day dividers, one reusable grouped list: a view declares its group field in `viewGroupFields`). `ctrl+o` is one view options overlay for every list view (People and Organizations: Sort and Order (ascending or descending, remembered per view; recently updated defaults to newest first), Merge: Minimum level, Archived: Type, Dates: Show (all, birthdays, anniversaries)). The footer shows `ctrl+o options` in every view that has options, before `?`. Hints that do not fit the width wrap to a second row ([[key-hints-in-one-place]]). It replaces `ctrl+o` sort, `ctrl+l` and `ctrl+f`.
- **Cells**: link attributes (role, from, to, primary, kind, notes, a deal's stage and value) and the note type are cells of the linked row. `left`/`right`/`tab` move across cells, `enter`/`e` edits one by its field type. `E` is gone. In the compare `tab` cycles the fields and `left`/`right` choose the side (`shift+tab` and the tab toggle are gone, `space` toggles).
- **Selection and bulk**: `ctrl+a` selects all visible, `shift+up`/`shift+down` extend (the Modules screen keeps them as reorder), `space` toggles, `*` is gone. Every verb acts on the selection, else on the highlighted row, for every object type. Delete, unlink and purge of several objects ask for the count typed back. Palette "Set field on selected" sets choice, date and number fields (tier, deal stage and value, task due and priority). Merge groups can be selected and dismissed together.
- **Settings polish (2026-10-30)**: no new key. The Keys section of Settings lists every action grouped by layer and area, `enter` rebinds. Key hints show only in the footer ([[key-hints-in-one-place]]), `o` can open a picker of the engines marked in Settings ([[web-search-engines]]).
- **Copy and open**: `ctrl+c` copies the focused field in the detail pane and otherwise quits or cancels as before (`y` is gone). Palette "Copy value" and "Copy contact as vCard" (`ctrl+shift+c` is bound but on the swallowed-chord list, the palette is the guaranteed path). `o` opens the focused thing outside the app for any object (URL, mail, phone, else a web search, template `[open] search_url` in `config.toml`).
- **Purge** is a verb of every archivable type: repo `PurgeObject`/`PurgeArchived`, `crm purge --archived [--type T] [--ids ...] [--apply]`, MCP `crm_purge_archived`, `d` and the palette bulk purge in the Archived view. Notes are listed in Archived.
- **Add picker**: tags are objects (`a g`, name prompt, the chip puts the tag on the highlighted object, `u` takes it off). Each kind declares its letter (`objDef.shortcut`), `TestAddShortcutsAreUniqueAndShownInThePicker` keeps them unique.

- **Footer words (CRM-023, 2026-11-03)**: no key changed, only the labels. `d` reads delete on every screen (person with marks, Merge, Sync inbox, the saved view editor), purge only in Archived. `enter` reads open in every object list (Tasks, Organizations and Deals were edit), compare in Merge, accept in the Sync inbox. `tab` on a list reads details. `esc` reads cancel, back or close by what it leaves. The footer also shows `f favorite` and `s snooze` on a person row and `u unlink` on a linked row of the detail pane while there is room. The table is in [[key-hints-in-one-place]].
- **Key text (CRM-025, 2026-11-04)**: no key changed, only how keys are drawn. The arrows read `↑` `↓` `←` `→` (and `↑/↓`, `←/→`, `shift+↑/↓`) in the footer, popups, modes, the keys page, the palette key column and the Settings Keys pane. Other keys keep their names. One table, `internal/tui/hintkeys.go` (`showKey`), see [[key-hints-in-one-place]]. In this note `up` and `down` name the keys for matching.
- **Esc unmarks (CRM-026, 2026-11-04)**: in a list with selected rows (`space`, `shift+up`/`shift+down`, `ctrl+a`) and focus on the list, `esc` clears every mark of the view. It is the last layer esc peels: popups, modes, prompts, the detail pane (esc goes back to the list), a search (People filter, tag search) and the tag page take esc first, so two presses clear a search and then the marks. The cursor stays. A toast says `unmarked N (ctrl+z undoes)`. It is one undo step: `ctrl+z` puts back the same marks while that list is open (ids that left the list are skipped), `ctrl+y` clears them again, and an undo from another list is refused with a message and stays on the stack. The footer shows `esc unmark` while marks exist, the last contextual hint to be dropped. With no marks `esc` does what it did (nothing in a list). Registry action `unmark`.

**Old key to new key (B2)**

| Old | New |
|---|---|
| `ctrl+n`, `ctrl+p` | `down`, `up` |
| `:` | `/`, `ctrl+k` |
| `q` in the compare and text pages | `esc` |
| `J`, `K` (Modules) | `shift+down`, `shift+up` |
| `r` (Modules) | palette "Modules: reset" |
| `F` | palette "Set keep-in-touch" |
| `ctrl+b`, `ctrl+w` (compare) | palette "Change pair", "Switch pane" |
| `ctrl+o` sort, `ctrl+l` level, `ctrl+f` archived type | `ctrl+o` view options |
| `*` | `ctrl+a` |
| `E` | cells of the linked row or note, `right` then `enter` |
| `y` | `ctrl+c`, palette "Copy value" |
| `o` open a link (detail pane only) | `o` opens anything outside the app |
| `tab`, `shift+tab` toggle (compare) | `space` toggles, `tab` next field, `left`/`right` pick a side |
| bulk tier, bulk stage | palette "Set field on selected" |
| purge of people only | `d` or palette purge for every archived type |

Views (number jumps, `[`/`]` cycle, `v` switcher): 1 Home, 2 People, 3 Due, 4 Tasks, 5 Dates, 6 Merge, 7 Archived, 8 Sync (Organizations and Deals open from the palette). Views popup (`v`): the shown views numbered 1 to 0, a line, the hidden views in gray. Typing filters by name, `enter` opens, `shift+up` and `shift+down` move a view (across the line hides or shows it, there is no hide key), `shift+r` resets the order and hidden set, `esc` closes. The popup has no edit, delete or restore key. Settings, Views (right pane of the Views row, no filter line) lists the same rows and adds `e` edit, `d` delete (asks y or n, undoable) and `r` restore for saved views. Settings, Colors (pane of the Colors row) lists person, task, organization, deal, tag and note in the Views order with a swatch and a hex field: `e` edits, `shift+r` resets the row, `esc` closes. `ctrl+k` or `/` opens the palette. `?` lists the keys live in the current view and mode, framework layer first, then app. `--read-only` blocks writes.

**Universal verbs** (framework layer, the same in every view): `a` add (one footer line for every kind, the cursor lands on the new object ([[add-standard]]); a picker of kinds, the view's own preselected, a letter jumps: `a t` task, `a n` note, `a o` organization, `a p` person, `a d` deal; links are pipe segments of the add prompt: a highlighted object is the first suggestion of the segment that takes its kind and `tab` accepts it, several objects are separated by commas, backspacing over a name is the unlink, `enter` links every name and unresolved names offer pick-or-create (branch feat/pipe-autocomplete, owner decision 2026-10-29, replaces the grayscale offer and the dropped link chip), `enter` and `e` activate (one action id, `activate`), `d` delete, `r` restore (Archived), `l` link (tags are objects), `u` unlink, `x` mark (task done and again reopens, person contacted), `f` favorite, `m` merge (selected rows or the highlighted duplicate), `space` select, `shift+up`/`shift+down` extend the selection, `ctrl+a` select all, `ctrl+z` undo, `ctrl+y` redo, `ctrl+shift+z` undo history (a popup, pick how far back; Redo history is in the palette), `ctrl+r` refresh. In Settings, Theme (entered with `enter`): `up` and `down` preview a theme, `enter` loads it, `tab` changes the mode (auto, light, dark), `c` copies a theme, `esc` closes, and on a custom theme `e` edits the name and colors (`enter` on a row edits the hex), `a` adds a theme, `d` deletes it, `shift+r` resets its colors to the theme it was copied from. The footer shows only the keys the highlighted theme can use ([[custom-themes]], [[navigation-standard]]). **Navigation**: arrows, `tab`, `shift+tab`, `ctrl+u` `ctrl+d` half page, `pgup` `pgdn` full page, `ctrl+up` `ctrl+down` jump by the view's unit (day in Birthdays, first letter in People, Organizations, Deals and Tasks, page elsewhere), `ctrl+shift+up` `ctrl+shift+down` the view's home (today in Birthdays) else first and last row, `home` `end` first and last row always. Units and homes are declared in `viewNavs` (`internal/tui/navunits.go`). **App layer**: `s` snooze, `F` cadence, `E` edit link or note type, `y` copy, `o` open a form row, `ctrl+o` sort, `ctrl+l` duplicate level, `ctrl+f` Archived type filter, compare `ctrl+e` survivor, `ctrl+b` pair, `ctrl+w` pane.

**Registry structure** (`internal/tui/keymap.go`): two layers (framework, app) in `frameworkIDs`, scopes (`app`, view types, `form`, `compare`, `view:<name>`) in `scopeTable` and `scopeByID`, lookup from the most specific scope outward with erase, key modes (normal, text). A new action starts in the app layer and the app scope. `crm keys list` shows each action's layer and scope. `TestKeyAudit`, `TestNoTwoActionsShareAKeyInOneScope` and `TestAppLayerKeepsOffFrameworkKeys` fail on a new conflict, an app action on a framework key or a stale line of `internal/tui/testdata/keyaudit_allow.txt` (one line).

**Palette-only now** (keep working, no bare key): Purge selected forever and Purge all remaining forever (Archived, typed count), Sync now, Dry-run sync, Attach inbox record, Undo dismiss, Add note (pick type), Tag, Tag task, Go to today, Scroll detail down and up, and the per-object Add, Edit and Restore actions. Their ids are unchanged, so a `[keys]` entry still binds them.

**Old key to new key**

| Old | New |
|---|---|
| `t`, `n`, `+`, `ctrl+a`, per-view `a`, `N` | `a` then a letter (`a t`, `a n`), `a` then `enter`. `N`: palette "Add note (pick type)" |
| `enter` open or edit, `e` edit, Home open, inbox accept `a`, tag rename `r` | `enter` or `e` |
| `c`, `e` contacted, `x` complete | `x` |
| `#` archive, `X` purge, `D` dismiss, `i` ignore | `d` |
| `u` restore, `u` undo dismiss | `r` (Archived), `ctrl+z`, palette |
| `L`, `U`, `T`, `T` task tag | `l`, `u`, `l`, palette |
| `M`, `m` apply, `m` tag merge | `m` |
| `m` level, `T` type filter, `o` sort, `r` reload | `ctrl+l`, `ctrl+f`, `ctrl+o`, `ctrl+r` |
| `s` sync, `d` dry run, `p` attach | palette |
| `s` stage, `p` tier (and the palette entries Set deal stage, Set priority tier) | `enter` or `e` on the stage or priority row of the record page (choice field). Bulk tier on space-selected people is gone |
| `s` survivor, `p` pair, `r` pane (compare) | `ctrl+e`, `ctrl+b`, `ctrl+w` |
| `t` note type | `E` on a note row |
| `j` `k` `g` `G` `h` `l`, compare `1` `2` | `down` `up` `home` `end` `left` `right` |
| `J` `K` | palette "Scroll detail down" and "Scroll detail up" |
| `ctrl+t`, `ctrl+up` | `ctrl+shift+up` in Dates, palette "Go to today" |
| `y` confirm in compare | `enter` |

Still open: the digits, `[ ]` and `v` are bare view-moving keys (X28, the owner decided to keep them in B2).

**List panes** (the right pane of the Settings rows Deal stages, Deal roles, Notes, Links and Ignored company texts, [[list-editor-standard]]). There is no palette entry, popup or key outside Settings. `enter` on the row enters the pane. In the pane the footer shows a key only when it works on the highlighted row: `up` / `down` select, `a` add (to the group under the cursor), `e` rename (`enter` too), `tab` cycles the outcome of a stage or toggles mark contacted of a note type (on Links `tab` moves between name and inverse), `d` delete (items in use: pick where the records go, then `y`), `shift+up` / `shift+down` move, `shift+r` reset to the built-in defaults (a confirm lists the changes), `esc` closes (`left` and `shift+tab` too), `space` / `ctrl+a` select, `ctrl+z` / `ctrl+y` undo and redo. No new bare key outside the pane. Removed with this change: the palette entries Edit deal stages, Edit deal roles, Edit note types, Edit relation kinds and Ignored company texts.
