# Terminal UI keys

The palette (`ctrl+k`) runs any action by name, as "Verb object" (Add organization). `?` lists the keys that work in the current view and mode, grouped by layer: the framework (the same in every view) and the app (specific to this CRM).

## Universal verbs (same in every view)

- `a` add. A picker of kinds opens with the view's own kind preselected, so `a` then `enter` adds the view's own object. Each kind shows its first letter and typing it jumps there: `a t` task, `a n` note, `a o` organization, `a p` person, `a d` deal. A highlighted object (or tag) is shown dimmed after the cursor as the link of the new object, `tab` accepts it. When nothing is highlighted or the two kinds have no link the dim text says so.
- If an object is highlighted when you add a task or a note, the prompt shows its name dimmed after the cursor (`tab: link Bob Baker`). `tab` links the new task or note to it. Nothing is linked until you press `tab`.
- `enter` and `e` are one action, activate: open the highlighted thing further, or edit the field under the cursor. Rename is just this.
- `d` delete (soft, `ctrl+z` undoes). `r` restores in the Archived view, where `d` purges a person after you type the name.
- `l` link the highlighted object to another (a tag is an object, so tagging is linking). `u` unlink.
- `x` mark, one meaning per type: a task is done (again reopens it), a person is contacted. `f` favorite.
- `space` selects rows, `*` selects all. `m` merges the selected rows (people, organizations), or opens the compare for the highlighted duplicate group.
- `ctrl+z` undo, `ctrl+y` redo, `ctrl+r` refresh. No bare `z` or `u` for undo, no separate rename key.

## Moving

- Arrow keys, `tab` and `shift+tab` (`h j k l g G` and the `1`/`2` compare aliases are gone). `ctrl+u` / `ctrl+d` half a page, `pgup` / `pgdown` a full page, `home` / `end` first and last row in every view.
- `ctrl+up` / `ctrl+down` jump by the unit the view declares: a day in Birthdays, the next first letter in People and Organizations, a page elsewhere.
- `ctrl+shift+up` / `ctrl+shift+down` go to the view's home destination (today in Birthdays), else the first and last row. Some terminals do not report these chords, and `home` and `end` always work.
- `tab` moves into the detail form, `esc` or `tab` back. `/`, `ctrl+k` or `:` opens the bar. `ctrl+o` toggles the People sort. `ctrl+l` cycles the duplicate level in Merge. `ctrl+f` filters the Archived view by type. `ctrl+t` and `ctrl+up` as go-to-today are retired: "Go to today" is a palette entry.

## App keys and palette entries

- `s` snooze. `F` cadence. Purge selected forever, Purge all remaining forever (Archived, type the count back, for example `purge 37`), Sync now, Dry-run sync, Attach inbox record, Add note (pick type), Tag and Undo dismiss are palette entries. To change a note type press `tab` in the note prompt, or `E` on a note row.
- With rows selected, `d`, `f`, `x`, `m` and the palette's Tag and Set keep-in-touch apply to every selected person.

## Edit pane (detail form)

`enter` or `e` edits a row in place by its field type: text, number and date ("next friday" works) in the row, longtext in the row (`alt+enter` is a new line), choice (deal stage, priority) with `up`/`down` or a letter, relation rows with a typeahead (`enter` picks, the last row is Create 'X', `tab` completes). `enter` or `tab` saves, `esc` cancels. `u` unlinks the focused linked row. `a` adds a row. `d` deletes the row (asks y/n). `y` copies the value, `o` opens a link, email or phone. `E`, the heavier twin of `e`, edits the role and dates of a linked row or picks the type of a note row. While you type in a prompt such as `title | due`, the rest of the line shows dimmed after the cursor. The hint is never submitted.

## Merge, archive, sync, bar

- Merge view: `enter`, `e` or `m` compare, `d` dismiss the group. Compare: `left`/`right` pick, `space` toggle, `ctrl+e` survivor, `ctrl+b` next pair, `ctrl+w` switch pane, `d` dismiss, `m` or `enter` merge, `esc` or `q` back.
- Organizations: `space` marks, `m` merges the marked. Archived view lists people, organizations, deals and tasks. Sync inbox: `enter` or `e` accepts a record, `d` ignores it. Home: `enter` opens the person.
- Bar (`ctrl+k`): `tab` on a person, organization, deal, task or tag row moves the focus to the results, where the universal verbs act on that row (`x`, `s`, `f`, `e`, `d`, `l`, `u`, `ctrl+z`) and `a` opens the add picker with the row's kind preselected. A tag row has `d` delete, `l` give, `u` take off, `e` rename, `m` merge into another tag, `enter` opens its people.
- `crm keys set` warns about chords terminals swallow: `ctrl+i`, `ctrl+m`, `ctrl+h`, `ctrl+[`, `ctrl+j` and the `ctrl+shift` arrows. A `[keys]` entry for an action or key that is gone is reported, never a crash.

See also: tui, keys, links, duplicates, archive, notes, sync, orgs.
