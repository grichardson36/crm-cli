# Terminal UI

A full-screen, keyboard-first interface for browsing and updating contacts. Mouse works too: click rows and tabs, scroll with the wheel.

```sh crm-example-skip
crm tui
crm tui --read-only
```

`--read-only` disables every key that changes data. Press `?` inside for the full key list. The command palette (`ctrl+k`, also `:`) searches and runs every action, so you never have to remember a key.

## Views

Switch with the number keys, or `[` and `]`.

| Key | View | Shows |
|---|---|---|
| 1 | People | everyone, searchable |
| 2 | Due | who you are due to contact (keep-in-touch) |
| 3 | Birthdays | upcoming birthdays |
| 4 | Tasks | open tasks |
| 5 | Merge | groups of probable duplicates |
| 6 | Archived | archived people |

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
- `+` create a contact.

## Edit pane

In the detail form: `enter` edits a row, `a` adds a row, `d` deletes the row (asks y/n), `y` copies the value, `o` opens a link, email or phone on this machine.

## Several people at once

`space` marks a row and `*` marks all visible. Then `c`, `f`, `T`, `F`, `p` and `#` apply to every marked person.

## Merge and archive

- Merge view: `enter` opens the side by side compare. `m` cycles the lowest confidence shown. `D` dismisses a group and `u` undoes it.
- In the compare screen: `tab` picks the other person's value for a row, `s` changes the survivor, `m` merges, `esc` leaves. Dropped people are archived and can be restored.
- `M` merges the marked people. `#` archives (asks y/n).
- Archived view: `u` restores. `X` purges after you type the name.

## Quit

`q` or `ctrl+c`.

## Related topics

`keep-in-touch`, `duplicates`, `archive`.
