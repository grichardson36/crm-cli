---
type: pattern
scale: interaction
status: decided by owner 2026-10-30, built in feat/settings-polish
---

# Key hints live in one place

**Problem.** Hints about which keys work were scattered: dim lines inside detail panes ("r restores, ctrl+o filters by type"), popup titles ("enter run esc close"), descriptions that restated keys, empty-state texts, status lines. They disagreed with each other and drifted when a key was rebound.

**Therefore.** Every screen and mode shows its key hints on its last line, the footer, and nowhere else. One order: verbs, then navigation, then what is specific to the screen, then the keys that control the view and the app (find, help, views). One format: the key in the name style, the label dim, two spaces between hints.

- The main screen builds its footer from the action registry (`footerSegs`). The order comes from the same layer table as the grouped key list ([[settings-view]]): framework verbs, navigation, app-specific, view control, then the `foot` number inside a class.
- A mode that takes over the screen (field edit, prompt, question, key capture, picker, palette, command bar, names panel, view editor, text page, keys page) builds its footer from `internal/tui/hints.go`. A hint names a registry action, so the key shown is the key bound today, or a fixed key that no action owns (enter, esc, the arrows, y, n).
- Not allowed outside the footer: rows or panes that say "enter opens it", titles with keys, descriptions or empty states that restate a key, confirm prompts ending in "y/n".
- Allowed: the dim syntax hint after the cursor of an input ([[show-syntax-inline-as-you-type]]), the keys page and the rebind list (they list keys by design), the key column of the palette ([[show-the-key-beside-the-action]]).
- A status line that reports a result may still say that ctrl+z undoes it. It is feedback, not a screen hint.

**Check.** `TestKeyHintsAreOnlyInTheFooter` renders every screen and mode and fails when a line above the footer reads like a hint (a key name followed by a verb). `TestFooterHintsFollowTheKeymap` rebinds a key and expects the footer to follow. `TestFooterOrderIsVerbsThenNavigationThenSpecifics` checks the order.

**Because.** Owner request 2026-10-30.

**Larger patterns.** [[show-the-key-beside-the-action]], [[keys-are-rebindable]], [[keymap-layers-and-scopes]]

**Smaller patterns.** [[show-syntax-inline-as-you-type]]

**Sources.** `internal/tui/hints.go`, `internal/tui/hints_test.go`, `internal/tui/view.go`
