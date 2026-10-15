# Terminal UI keys

Every key in the terminal UI, grouped by task. The palette (`ctrl+k`) runs any of these by name. `?` inside lists the keys of the current view.

## Moving

- `j` / `k` or arrows: next and previous row. `g` / `G`: first and last. `ctrl+d` / `ctrl+u`: half page.
- `tab` or `l`: move into the detail form. `esc`, `h` or `tab`: back to the list.
- `J` / `K`: scroll the detail pane.
- `/` search by name, company, email, location and tags. `#tag` filters by tag. `esc` clears.
- `o` toggles sort between name and recently updated. `r` reloads.

## Contact keys

- `c` mark contacted now (resets the keep-in-touch clock). `s` snooze for N days.
- `n` add a note of the last used type. `N` pick the type first.
- `t` add a task: `title | optional due date`. `x` complete the selected task.
- `f` toggle favorite. `T` add a tag, `-name` removes it. `F` set the cadence. `p` set the priority tier.
- `+` create a contact. `space` marks a row and `*` marks all visible. Then `c`, `f`, `T`, `F`, `p` and `#` apply to every marked person.

## Edit pane

In the detail form: `enter` edits a row, `a` adds a row, `d` deletes the row (asks y/n), `y` copies the value, `o` opens a link, email or phone on this machine.

## Merge and archive

- Merge view: `enter` opens the side by side compare. `m` cycles the lowest confidence shown. `D` dismisses a group and `u` undoes it.
- In the compare screen: `tab` picks the other person's value for a row, `s` changes the survivor, `m` merges, `esc` leaves. Dropped people are archived and can be restored.
- `M` merges the marked people. `#` archives (asks y/n).
- Archived view: `u` restores. `X` purges after you type the name.

## Notes, Sync and Home

- Note row in the detail form: `enter` edits the text, `t` changes the type, `d` deletes (asks y/n). "Restore last deleted note" is in the palette.
- Sync view: `s` runs a sync for the selected source, `d` does a dry run. In the inbox: `a` accepts (creates a person), `p` attaches to an existing person, `i` ignores.
- Home: `enter` on an item opens that person in People with the detail form focused.

See also: tui, duplicates, archive, notes, sync.
