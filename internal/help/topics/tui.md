# Terminal UI

A full-screen, keyboard-first interface for browsing and updating contacts. Mouse works too: click rows and tabs, scroll with the wheel.

```sh crm-example-skip
crm tui
crm tui --read-only
```

`--read-only` disables every key that changes data. Press `?` inside for the full key list. The bar (`ctrl+k`, `:` or `/`) is the one box. Type to find a person, org, deal, task or note, run an action, check or change a setting (`theme`, `keys`, `syntax`), or enter a command (`n Sam called`). Prefixes narrow it: `>` actions, `@` people, `#` tags, `=` settings, `?` help. An empty box lists what you used recently. A name with no match offers to create the person or add a note.

## Views

Jump with the number keys, cycle with `[` and `]`, or press `v` for the view switcher. The tab bar shows Home, People, Due and Tasks, with the rest under More.

| Key | View | Shows |
|---|---|---|
| 1 | Home | what needs attention: who is due, birthdays, tasks, recent notes, sync status |
| 2 | People | everyone |
| 3 | Due | who you are due to contact (keep-in-touch) |
| 4 | Tasks | open tasks |
| 5 | Birthdays | upcoming birthdays |
| 6 | Merge | groups of probable duplicates |
| 7 | Archived | archived people |
| 8 | Sync | sync sources and the inbox of unmatched records |

## Palette actions that mirror CLI commands

These work in `--read-only` mode. Each opens a scrollable page (`j` / `k`, `g` / `G`, `esc` goes back).

- "Help: topics" lists topics with a filter. "Help: search" lists matching lines.
- "Doctor" runs the checks of `crm doctor` with the fix line for each problem.
- "Where" shows the open database. "Domains" lists the databases in the data directory.
- "DAV: info", "DAV: conflicts" and "DAV: status" show the phone sync details, logged conflicts and counts. They never start the server (`crm help dav`).

All keys, editing, merge, archive, notes, sync and Home are in `crm help tui-keys`.

See also: short-syntax, tui-keys, theme, keep-in-touch, duplicates, archive.
