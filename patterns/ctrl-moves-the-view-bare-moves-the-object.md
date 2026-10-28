---
type: pattern
scale: interaction
status: decided (owner 2026-10-21), details open
---

# Ctrl keys move the view, bare keys move the object

**Problem.** The same key means different things in different modules (`t` was add-task everywhere and "today" in Birthdays). Users cannot predict what a key will do.

**Therefore.** Two key classes. Bare keys act on the objects in the current view (add, edit, delete, mark, link, snooze) and mean the same thing in every module ([[same-key-same-verb]]). Ctrl keys act on the interface and the view: go to today, open the palette, undo, jump, add an object of any kind from anywhere. A key never means two things. Before a key is added or reassigned, the whole registry is checked for conflicts and the owner is told ([[surface-gaps-before-deciding]]).

**Examples.** `+` or `a` adds the view's own object. `ctrl+t` returns to today or the view's equivalent anchor in every module. A global "add any object" chord opens a menu of object kinds from any view, so a stray thought (a person, a task) is captured without navigating away. `ctrl+z` and `ctrl+y` are already undo and redo, `ctrl+k` the palette.

**Open.** Decided in the keys-audit branch (2026-10-21, details in [[audit-keybindings-2026-10]]):
- `t` is Add task in every module again, Birthdays included. Go to today is `ctrl+t` (also `ctrl+up`), registry action `go-today`. ctrl+t had no use in code, two tests used it as a rebind example and moved to ctrl+e. Anchors: Birthdays the day with the past above, Tasks the first task dated today or later, Home, People and Follow-up the top. Modules without an anchor leave the key alone.
- The global add chord is `ctrl+a`, action `add-any` ("New"): it pairs with bare `a` and `+` (same letter, ctrl form means from anywhere). The owner rejected `ctrl+n` because bare `n` is Add note and `+` is add. `ctrl+n` stays list next. Terminals cannot send `ctrl++`, which is why the letter form is used. ctrl+a is tmux's and GNU screen's prefix and readline's line start, so it stays rebindable (`crm keys set add-any <key>`). In a text prompt it is still line start.
- Found and left alone: ten keys with more than one verb (`s`, `e`, `p`, `d`, `m`, `r`, `c`, `l`, `o`, `T`, plus `enter` by design), bare keys that move the view (digits, `[ ]`, `v`, `o`, `m`, `T`, `C`) and ctrl keys that act on objects (`ctrl+z`, `ctrl+y`, and the approved `ctrl+a`). The decisions list is in the audit note.
- Still open for the owner: the list of decisions A4 in [[audit-keybindings-2026-10]].

**Because.** Owner decision 2026-10-21: keys are core to the experience, "ctrl+ to manipulate the interface/view and bare keys to manipulate the objects within the view."

**Larger patterns.** [[one-verb-set]], [[objects-first]]

**Smaller patterns.** [[audit-keybindings-2026-10]], [[same-key-same-verb]], [[keys-are-rebindable]], [[show-the-key-beside-the-action]], [[bar-results-act-like-views]]

**Sources.** Owner decision 2026-10-21

**Inspired by.** [[vim-and-lazygit]], [[linear]], [[raycast]]

**Update 2026-10-27 (feat/keymap-a).** The `t`, `ctrl+t` and `ctrl+a` decisions above are superseded by [[universal-verbs]] and [[navigation-units-and-homes]]. `a` opens the add picker, `ctrl+t` and the old `ctrl+up` are retired, `ctrl+up` and `ctrl+down` jump by the view's unit, `ctrl+shift+up` and `ctrl+shift+down` go to the view's home. Sort, duplicate level, the Archived type filter and reload moved to `ctrl+o`, `ctrl+l`, `ctrl+f` and `ctrl+r`, so those bare keys no longer move the view. Still bare: the digits, `[ ]` and `v` (open-discrepancies X28). See [[keymap-layers-and-scopes]].
