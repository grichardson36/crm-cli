# Terminal UI keys

The palette (`ctrl+k` or `/`) runs any action by name, as "Verb object" (Add organization). `?` lists the keys that work in the current view and mode, grouped by layer: the framework (the same in every view) and the app (specific to this CRM).

## Universal verbs (same in every view)

- `a` add. A picker of kinds opens with the view's own kind preselected, so `a` then `enter` adds the view's own object. Each kind shows its letter and typing it jumps there: `a t` task, `a n` note, `a o` organization, `a p` person, `a d` deal, `a g` tag. Each kind declares its letter in the object registry (the first letter, or the next free letter of its name). Links are pipe segments: the add prompt of a task, note, organization, deal, person or tag is a line whose last segments name the objects to link (`people`, `organizations`, `deals`, `tasks`, `notes`, `tags`, a tag's `on`), several objects separated by commas. A highlighted object (or tag) is the first suggestion of the segment that takes its kind, `tab` accepts it. Names that are not objects yet offer pick-or-create when the line is entered. A segment is checked as soon as `|` closes it and a popup helps while you type (crm help tui-prompts). A new tag is the prompt `name | on`: an existing name picks that tag, a new name creates it, and the objects in `on` get it, so it shows up in lists.
- `enter` and `e` are one action, activate: open the highlighted thing further inside the app, or edit the field under the cursor. Rename is just this.
- `d` delete (soft, `ctrl+z` undoes). `r` restores in the Archived view, where `d` purges (any type) after you type the name.
- `l` link the highlighted object to another (a tag is an object, so tagging is linking). `u` unlink. `x` mark, one meaning per type: a task is done (again reopens it), a person is contacted. `f` favorite. `m` merges the selected rows (people, organizations), or opens the compare for the highlighted duplicate group.
- `ctrl+z` undo, `ctrl+y` redo, `ctrl+r` refresh. No bare `z` or `u` for undo, no separate rename key.
- `o` opens the focused thing outside the app: a link opens the browser, mail app or dialer, anything else a web search. `ctrl+c` copies the focused field in the detail pane and everywhere else quits or cancels as before. Selection and bulk actions: see tui-selection.

## Moving

- Arrow keys, `tab` and `shift+tab` (`h j k l g G` and `ctrl+n`, `ctrl+p` are gone). `ctrl+u` / `ctrl+d` half a page, `pgup` / `pgdown` a full page, `home` / `end` first and last row in every view.
- `ctrl+up` / `ctrl+down` jump by the unit the view declares: a day in Birthdays, a confidence level in Merge, the next first letter in People and Organizations, a page elsewhere. `ctrl+shift+up` / `ctrl+shift+down` go to the home destination of the view (today in Birthdays), else the first and last row. Some terminals do not report these chords, and `home` and `end` always work.
- `tab` moves into the detail form, `esc` or `tab` back. Views: `1` to `9` and `0`, `[` and `]` (also `ctrl+left` and `ctrl+right`), `v` for the picker.
- `ctrl+o` opens the view options, one overlay for every list view: Sort in People and Organizations, Minimum level in Merge, Type in Archived. `left` and `right` change a value. `ctrl+t` and `ctrl+up` as go-to-today are retired: "Go to today" is a palette entry.

## App keys and palette entries

- `s` snooze. Set keep-in-touch (`F` before), Modules: reset, Change pair and Switch pane (Merge compare), Purge selected forever, Purge all remaining forever (Archived, any type, type the count back, for example `purge 37`), Sync now, Dry-run sync, Attach inbox record, Add note (pick type), Tag, Undo dismiss and Set field on selected are palette entries.

## Edit pane (detail form)

`enter` or `e` edits a row in place by its field type: text, number and date ("next friday" works) in the row, longtext in the row (`alt+enter` is a new line), choice (deal stage, priority) with `up`/`down` or a letter, relation rows with a typeahead (`enter` picks, the last row is Create 'X', `tab` completes). `enter` or `tab` saves, `esc` cancels. `u` unlinks the focused linked row. `a` adds a row. `d` deletes the row (asks y/n).

A relation field shows its linked rows as a small table whose cells (role, dates, primary, kind, notes, note type) are edited in place: `right`, `left` and `tab` move across them, `enter` edits (see tui-selection). The deal role of a deal link is a pick-or-create list: type to filter, a new name offers `Create role X` (see deal-roles). `E` is gone. While you type in a prompt such as `title | due`, the rest of the line shows dimmed after the cursor. The hint is never submitted.

## Merge, archive, sync, bar

- Merge view: every candidate is listed, grouped by confidence level (HIGH, MEDIUM, LOW, FUZZY) under divider headings. `enter`, `e` or `m` compare, `d` dismiss the group (all selected groups). `ctrl+o` sets the minimum level. Compare: `left`/`right` keep that candidate's value, `tab` moves to the next field (from the last to the first), `space` toggles a value or a list item, `ctrl+e` survivor, `d` dismiss, `m` or `enter` merge, `esc` back. `q` no longer leaves the compare.
- Organizations: `space` selects, `m` merges the selected. Archived view lists people, organizations, deals, tasks and notes. Sync inbox: `enter` or `e` accepts a record, `d` ignores it. Home: `enter` opens the person.
- Bar (`ctrl+k`): `tab` on a person, organization, deal, task or tag row moves the focus to the results, where the universal verbs act on that row (`x`, `s`, `f`, `e`, `d`, `l`, `u`, `ctrl+z`) and `a` opens the add picker with the row's kind preselected. A tag row has `d` delete, `l` give, `u` take off, `e` rename, `m` merge into another tag, `enter` opens its people.
- `crm keys set` warns about chords terminals swallow: `ctrl+i`, `ctrl+m`, `ctrl+h`, `ctrl+[`, `ctrl+j`, `ctrl+shift+c` and the `ctrl+shift` arrows. A `[keys]` entry for an action or key that is gone is reported, never a crash.

See also: tui-prompts, tui-selection, tui-key-changes, tui, keys, links, duplicates, archive, purge, notes, sync, orgs.
