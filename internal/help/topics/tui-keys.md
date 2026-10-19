# Terminal UI keys

Every key in the terminal UI. The palette (`ctrl+k`) runs any of these by name, as "Verb object" (Add organization). `?` inside lists the keys of the current view.

## Standard keys (same in every object view)

- `a` add (`+` also). `enter` edit (`e` also in Organizations and Deals). `d` delete, asks y/n. `u` restore. `L` link. `U` unlink. `space` mark. `ctrl+z` undo, `ctrl+y` redo.
- The object is the one under the cursor: a person, a task, or in the Organizations and Deals views (palette) an organization or a deal. Restore brings back the object you deleted last in this session. In the Archived view `u` restores the archived object, `t` filters by type.
- `L` and `U` ask which relation (organization, person, deal, task, tag), then pick or create in one step, then the kind or the role/dates line. `E` edits a linked row. See links.

## Moving

- `j` / `k` or arrows: next and previous row. `g` / `G`: first and last. `ctrl+d` / `ctrl+u`: half page. `J` / `K`: scroll the detail pane.
- `tab` or `l`: move into the detail form. `esc`, `h` or `tab`: back to the list.
- `/`, `ctrl+k` or `:` opens the bar: find people, orgs, deals, tasks, notes, actions and settings. `#tag` filters by tag, `esc` clears it. `=` lists settings.
- `o` toggles sort. `r` reloads. Rebind keys: see `keys`.

## Person keys

- `c` mark contacted. `s` snooze. `n` add a note, `N` pick the type first. `t` add a task for the person: `title | optional due date`. `x` complete a task.
- `f` favorite. `T` tag (`-name` removes). `F` cadence. `p` priority tier. `*` marks all visible, then `c`, `f`, `T`, `F`, `p`, `d` apply to every marked person.
- `s` sets a deal stage in the Deals view. Linked rows (organizations, related people, deals, members) in the detail form: `enter` opens (or links when empty), `d` unlinks, `E` edits the role and dates. `n` adds a note on an organization or deal page.

## Edit pane

In the detail form: `enter` edits a row, `a` adds a row, `d` deletes the row (asks y/n), `y` copies the value, `o` opens a link, email or phone. A note row: `t` changes the type.

## Merge, archive, notes, sync, home

- Merge view: `enter` compare, `m` level, `D` dismiss, `u` undo dismiss. `M` merges marked people. Compare: `left`/`right`, `space`, `s`, `p`, `r`, `enter`, `esc`.
- In the Organizations view `space` marks a row and `M` merges the marked organizations (see orgs). Company review (palette "Company review", or C in Organizations): c create, l link, e rename, s skip, x clear, A accept exact matches, enter applies the staged batch. See convert.
- Archived view lists people, organizations, deals and tasks: `T` filters by type, `u` restores, `X` purges a person after you type the name.
- Sync view: `s` runs a source, `d` dry run. Inbox: `a` accepts, `p` attaches, `i` ignores. Home: `enter` opens the person.

See also: tui, keys, links, duplicates, archive, notes, sync, orgs.
