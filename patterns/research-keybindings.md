---
type: research
---
# Research: keybindings that stay intuitive across modules

Date 2026-10-21. Trigger: owner asked for a full audit of our keys and for inspiration from Obsidian, herdr and other programs. Companion: [[ctrl-moves-the-view-bare-moves-the-object]], [[same-key-same-verb]]. Evidence tags: doc (first-party page read), search (summary only), general (background knowledge, not re-verified).

## What the inspirations do

- K1. **herdr (doc, herdr.dev/docs/keyboard).** Reserves one key, the prefix (default ctrl+b, rebindable). Every action is prefix plus a mnemonic letter: new tab `c`, next/previous `n`/`p`, split `v` and `-`, zoom `z`, close `x`, rename `shift+t`. The shifted letter is the destructive or heavier twin (`x` close pane, `shift+x` close tab). Navigation uses `h j k l` plus arrow aliases. Modes (go-to picker, resize, copy) each own their keys and Esc leaves. `prefix+?` lists active bindings and `/` filters them. Every action accepts a list of chords, so a prefix form and a direct form coexist. Chords that other layers swallow (ctrl+j is Enter, plain alt on macOS) are documented, and `ctrl+alt` is the recommended direct family.
- K2. **Obsidian (doc for customization, general for defaults).** Few defaults, only for the commonest commands. Every command is reachable from the command palette, which shows its hotkey, and any command can be given one or several hotkeys in Settings, with filter to show only bound commands. The defaults people rely on are one-letter ctrl chords with a mnemonic (palette, quick switcher "open", new note, find). Conflict detection and the exact default table were not on the pages read.
- K3. **GitHub Primer and GitLab design guidance (search).** Single-character shortcuts are the commonest accessibility failure (speech input, motor issues). A shortcut is an enhancement, never the only route. Provide a setting to turn them off or add a modifier. Show a shortcut next to its action, one shortcut per action in the display. Scope shortcuts by context and generate the help from the bindings so it cannot drift.
- K4. **lazygit and k9s (search, secondary).** Persistent panels with a footer that shows the focused panel's actions. Same letters for the same verb across panels, `?` for the key list. (general) vim-style `hjkl`, `/` search, `:` command, `esc` back.
- K5. **Superhuman, Linear, Gmail (general, not researched).** Single letters act on the selected item (`e` archive, `h` snooze, `c` compose). `cmd+k` opens the command palette. `g` then a letter goes to a place. Not yet verified from first-party docs.

## Principles that follow

1. **Two classes.** Bare keys act on the selected object. Modified keys act on the interface. Our owner rule matches Obsidian, Superhuman and herdr (which uses a prefix as its modifier).
2. **One letter, one verb, everywhere.** A key is free only if no view uses it differently (herdr's shift form for the heavier twin is a good pattern for delete/purge).
3. **Mnemonic and paired.** `a` add and `ctrl+a` add from anywhere. `t` task and `ctrl+t` today is the weaker pair, so the registry must display it.
4. **Go-to is a verb, not a letter per view.** Tab keys and `[` `]` rotate views (owner arrangement), a go-to picker or the palette jumps by name.
5. **Everything has a palette entry showing its key, and `?` lists the active keys**, filtered by context. Footer shows the focused object's verbs.
6. **Every action takes a list of chords** (herdr), so a defaults set and a user set can coexist. Rebinding is validated and conflicts are reported at load, never silently shadowed.
7. **Terminal limits are documented.** ctrl+i, ctrl+m, ctrl+h, ctrl+[ and ctrl+j are Tab, Enter, Backspace, Esc and newline. Shift, plus and ctrl with symbols are unreliable. `ctrl+alt` chords are sometimes claimed by the desktop.
8. **Accessibility switch.** Offer a "modifier for single-letter shortcuts" setting later (Primer).

## Audit plan (done in the keys-audit branch, results in [[audit-keybindings-2026-10]])

- A1. List every key per view and mode and flag conflicts, bare keys that navigate the view, and ctrl keys that act on objects.
- A2. Compare against K1 to K5: which verbs and letters inspirations use for the same job (add, delete, archive, snooze, search, go-to, help, undo) and flag where we differ without a reason.
- A3. Proposed new registry rules: load-time conflict check, chord lists per action, `?` key list, one footer rule.
- A4. Owner decision list for each conflict, before any key is changed.

## To research further

Obsidian default hotkey table, Superhuman shortcut philosophy, Linear and Gmail shortcut sets, helix and kakoune selection-first model, Zellij and tmux mode design, Things and Todoist shortcuts for tasks.

## Sources
- herdr keyboard doc: https://herdr.dev/docs/keyboard/
- Obsidian hotkeys help: https://obsidian.md/help/hotkeys
- GitHub Primer keyboard shortcuts: https://primer.style/accessibility/patterns/keyboard-shortcuts
- GitLab Pajamas keyboard shortcuts: https://design.gitlab.com/patterns/keyboard-shortcuts
