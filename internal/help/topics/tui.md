# Terminal UI

A full-screen, keyboard-first interface for browsing and updating contacts. Mouse works too: click rows and tabs, scroll with the wheel.

```sh crm-example-skip
crm tui
crm tui --read-only
```

`--read-only` disables every key that changes data. Press `?` inside for the full key list. The command palette (`ctrl+k`, also `:`) searches and runs every action, so you never have to remember a key.

## Views

Jump with the number keys, cycle with `[` and `]`, or press `v` for the view switcher. The tab bar shows Home, People, Due and Tasks, with the rest under More.

| Key | View | Shows |
|---|---|---|
| 1 | Home | what needs attention: who is due, birthdays, tasks, recent notes, sync status |
| 2 | People | everyone, searchable |
| 3 | Due | who you are due to contact (keep-in-touch) |
| 4 | Tasks | open tasks |
| 5 | Birthdays | upcoming birthdays |
| 6 | Merge | groups of probable duplicates |
| 7 | Archived | archived people |
| 8 | Sync | sync sources and the inbox of unmatched records |
| 9 | Calendar | events, birthdays, keep-in-touch dates and tasks by day |

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

## Notes

In the detail form, on a note row: `enter` edits the text, `t` changes the type, `d` deletes (asks y/n). "Restore last deleted note" is in the palette.

## Sync view

`s` runs a sync for the selected source, `d` does a dry run. In the inbox: `a` accepts (creates a person), `p` attaches to an existing person, `i` ignores.

## Home

`enter` on an item opens that person in People with the detail form focused.

## Help, doctor and where from the palette

These palette actions mirror the CLI commands and work in `--read-only` mode. Each opens a scrollable page: `j` / `k`, `g` / `G`, `esc` goes back.

- "Help: topics" lists the topics with a filter, `enter` opens one. "Help: search" asks for a word and lists the matching lines.
- "Doctor" runs the checks of `crm doctor` on the open database, with the fix line for each problem.
- "Where" shows the open database: path, domain, schema version, device id, size. "Domains" lists the databases in the data directory.

## Calendar

Opens on the Agenda (two weeks by day). `a` agenda, `d` day, `w` week grid, `m` month grid (a narrow terminal shows the agenda instead). `t` today, `[` and `]` step a week (day in Day view, month in Month view), so they do not change view here. In the grids `h` and `l` move a day, in Month `j` and `k` move a week and `enter` opens the day.

- `+` adds an event as one line: `Lunch | fri 12:30 1h | weekly`. The preview shows what will be created. `enter` edits the event under the cursor, `D` deletes it. A repeating event asks first: this, this and following, or all.
- `b` birthdays, `u` keep-in-touch dates, `T` tasks switch overlays. Enter on one opens the person.
- The palette adds attendees, reminders, tags, status, busy, go to date, show times in another zone, import and export ICS files, and restore the last deleted event.

## Quit

`q` or `ctrl+c`.

## Related topics

`keep-in-touch`, `duplicates`, `archive`.
