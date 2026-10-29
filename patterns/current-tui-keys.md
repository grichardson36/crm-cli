---
type: feature-area
---
# Current TUI keys

Moved from FEATURES.md on 2026-10-29 (section Current TUI keys). The rows that use these keys are in [[features-tui-keys-and-modules]]. Patterns: [[universal-verbs]], [[navigation-units-and-homes]], [[keymap-layers-and-scopes]], [[ctrl-moves-the-view-bare-moves-the-object]], [[keys-are-rebindable]]. Open key conflicts: [[open-discrepancies]].

BRANCH feat/keymap-a (owner decisions 2026-10-27, patterns universal-verbs, navigation-units-and-homes, keymap-layers-and-scopes; not merged). Rows in the sections below that name `t`, `n`, `+`, `#`, `u`, `T`, `L`, `U`, `M`, `D`, `X`, `c`, `p`, `o`, `r`, `j`, `k`, `g`, `G`, `ctrl+t` or `ctrl+a` describe the keys before this branch: use the migration table here.

BRANCH feat/keymap-b (owner decisions 2026-10-28, keymap part B2, built on main 6fcc672, not merged). It applies the audit's clear removals and the owner's choices, and rows below that name `ctrl+n`, `ctrl+p`, `:`, `F`, `ctrl+b`, `ctrl+w`, `ctrl+l`, `ctrl+f`, `*`, `E`, `y` or a tab toggle in the compare describe the keys before this branch: use the B2 table here. What B2 changes:

- **Cleanup**: `ctrl+n`, `ctrl+p`, `:`, `q` in the compare and in text pages, `J`/`K`/`r` in Modules are gone. `F` (cadence), `ctrl+b` (next pair), `ctrl+w` (switch pane) and the modules reset are palette entries. Kept: `ctrl+e`, `m`, `d`, `y`/`n` answers in prompts, `alt+d`, the digit hints in pickers, the digits, `[ ]`, `v`, `ctrl+d`, `ctrl+u`. `ctrl+left` and `ctrl+right` are aliases of `[` and `]` (no view uses them as a jump unit, jumps stay on `ctrl+up`/`ctrl+down`).
- **Merge** lists every candidate grouped by confidence level under divider headings (the Birthdays day dividers, one reusable grouped list: a view declares its group field in `viewGroupFields`). `ctrl+o` is one view options overlay for every list view (People and Organizations: Sort, Merge: Minimum level, Archived: Type) and replaces `ctrl+o` sort, `ctrl+l` and `ctrl+f`.
- **Cells**: link attributes (role, from, to, primary, kind, notes, a deal's stage and value) and the note type are cells of the linked row. `left`/`right`/`tab` move across cells, `enter`/`e` edits one by its field type. `E` is gone. In the compare `tab` cycles the fields and `left`/`right` choose the side (`shift+tab` and the tab toggle are gone, `space` toggles).
- **Selection and bulk**: `ctrl+a` selects all visible, `shift+up`/`shift+down` extend (the Modules screen keeps them as reorder), `space` toggles, `*` is gone. Every verb acts on the selection, else on the highlighted row, for every object type. Delete, unlink and purge of several objects ask for the count typed back. Palette "Set field on selected" sets choice, date and number fields (tier, deal stage and value, task due and priority). Merge groups can be selected and dismissed together.
- **Copy and open**: `ctrl+c` copies the focused field in the detail pane and otherwise quits or cancels as before (`y` is gone). Palette "Copy value" and "Copy contact as vCard" (`ctrl+shift+c` is bound but on the swallowed-chord list, the palette is the guaranteed path). `o` opens the focused thing outside the app for any object (URL, mail, phone, else a web search, template `[open] search_url` in `config.toml`).
- **Purge** is a verb of every archivable type: repo `PurgeObject`/`PurgeArchived`, `crm purge --archived [--type T] [--ids ...] [--apply]`, MCP `crm_purge_archived`, `d` and the palette bulk purge in the Archived view. Notes are listed in Archived.
- **Add picker**: tags are objects (`a g`, name prompt, the chip puts the tag on the highlighted object, `u` takes it off). Each kind declares its letter (`objDef.shortcut`), `TestAddShortcutsAreUniqueAndShownInThePicker` keeps them unique.

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

Views (number jumps, `[`/`]` cycle, `v` Views screen): the number key is the place among the views that are shown. The default arrangement is 1 Home, 2 People, 3 Follow-up, 4 Tasks, 5 Organizations, 6 Deals, 7 Tags, 8 Birthdays, 9 Merge, 0 Settings. Archived and Sync are hidden by default (Settings rows, the bar, the Hidden group of the Views screen), and a `[modules]` table you wrote keeps them where it puts them. The text below that names 7 Archived and 8 Sync describes the old numbers.

**Views screen (`v`, branch feat/settings-views, [[views-screen]])**: one screen that lists every view, shown ones first, hidden ones under a Hidden divider. `enter` goes to the view, a number key goes to a shown view, `x` shows or hides (Settings refuses), `shift+up` / `shift+down` reorder inside the group, `esc` closes. It replaces the Modules screen: the palette entry "Modules" is gone, "Modules: reset" stays. **Settings view (key 9, [[settings-view]])**: a grouped list, `enter` / `e` edits a field setting in place or opens the screen behind the row. No new key. `ctrl+k` or `/` opens the palette. `?` lists the keys live in the current view and mode, framework layer first, then app. `--read-only` blocks writes.

**Universal verbs** (framework layer, the same in every view): `a` add (a picker of kinds, the view's own preselected, a letter jumps: `a t` task, `a n` note, `a o` organization, `a p` person, `a d` deal; links are pipe segments of the add prompt: a highlighted object is the first suggestion of the segment that takes its kind and `tab` accepts it, several objects are separated by commas, backspacing over a name is the unlink, `enter` links every name and unresolved names offer pick-or-create (branch feat/pipe-autocomplete, owner decision 2026-10-29, replaces the grayscale offer and the dropped link chip), `enter` and `e` activate (one action id, `activate`), `d` delete, `r` restore (Archived), `l` link (tags are objects), `u` unlink, `x` mark (task done and again reopens, person contacted), `f` favorite, `m` merge (selected rows or the highlighted duplicate), `space` select, `shift+up`/`shift+down` extend the selection, `ctrl+a` select all, `ctrl+z` undo, `ctrl+y` redo, `ctrl+r` refresh. **Navigation**: arrows, `tab`, `shift+tab`, `ctrl+u` `ctrl+d` half page, `pgup` `pgdn` full page, `ctrl+up` `ctrl+down` jump by the view's unit (day in Birthdays, first letter in People and Organizations, page elsewhere), `ctrl+shift+up` `ctrl+shift+down` the view's home (today in Birthdays) else first and last row, `home` `end` first and last row always. Units and homes are declared in `viewNavs` (`internal/tui/navunits.go`). **App layer**: `s` snooze, `F` cadence, `E` edit link or note type, `y` copy, `o` open a form row, `ctrl+o` sort, `ctrl+l` duplicate level, `ctrl+f` Archived type filter, compare `ctrl+e` survivor, `ctrl+b` pair, `ctrl+w` pane.

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
| `ctrl+t`, `ctrl+up` | `ctrl+shift+up` in Birthdays, palette "Go to today" |
| `y` confirm in compare | `enter` |

Still open: the digits, `[ ]` and `v` are bare view-moving keys (X28, the owner decided to keep them in B2).

## Tags view (BRANCH feat/tags-module, owner request 2026-10-30)

No tag-specific key. The universal verbs act on the selected tags, else the highlighted one ([[bulk-actions-always]]), and the rows of a tag in the command bar run the same actions. Details: [[features-tags-module]].

| Key | Tags list | Tag page (after `enter`) |
|---|---|---|
| `enter` | open the tag page | open the highlighted object |
| `tab`, then `enter` or `e` | edit the name in place, rename everywhere, a name that exists asks to merge | |
| `esc` | clear the search | back to the list |
| `d` | delete (untag everywhere, or remove an unused name), several ask the count typed back | |
| `m` | merge the selected tags into the highlighted one, nothing selected asks which tag | |
| `l` | give the tag to an object of any kind (all selected tags) | link another object to the tag |
| `u` | take the tag off an object that has it | unlink the highlighted or selected objects |
| `a g` | add a tag | |
| `space`, `ctrl+a`, `shift+up`/`shift+down` | select tags | select objects |
| `ctrl+o` | Sort (name, number of objects), Filter (in use, unused, possible case duplicates, all) | |
| `/` | find a tag by name | |

Palette: "Tags: merge case duplicates", "Tags: delete unused tags", "Rename tag", "Merge tag into another", "Delete tag" (a picker, same code).
