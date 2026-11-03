---
type: pattern
status: decided
core-gate: 6
domains: [interface]
---
# Navigation standard

**Rule.** Every overlay, popup, pane and screen moves, changes values and closes the same way.

**Applies when.** Adding or changing a popup, picker, overlay, two-pane screen or editor under [[core]] gate 6.

**Default.**

1. `esc` always leaves the current overlay, popup, pane or screen. It never saves a change that was not confirmed.
2. A list or popup: `up` and `down` move, `enter` picks or acts, `esc` closes.
3. A two-pane screen (Settings, Tags, Views, Keys, the record pages): `up` and `down` move in the pane that has focus. `right` or `tab` enters the right pane. `left` at the left edge of the right pane, `shift+tab` and `esc` go back to the list.
4. `left` and `right` change a value only on a row that is a selector (a value in view options). On a selector, `left` of the first value goes back to the list and `right` of the last value stays.
5. A popup that chooses how far to go back (undo history, `ctrl+shift+z`) lists the newest first. Picking an entry applies it and everything between it and the top.
6. Key hints show in the footer ([[key-hints-in-one-place]]), never in a title or body. A popup shows its own hints in the bottom row inside its box ([[popup-component]]) and not in the footer.
7. The Settings panes (list editor lists, Views, Colors) have no filter line, so bare keys act on the highlighted row and the footer shows only the keys that work on it.

**Exception.** The help view (`?`) closes on any key, and a text entry line (the footer prompt) keeps the keys of its field. Inside the Theme pane of Settings `tab` changes the mode and does not leave the pane, `esc` and `shift+tab` still do ([[custom-themes]]). Inside a list pane of Settings (deal stages, deal roles, Notes, Links, ignored company texts) `tab` cycles the value of the row where it has one and does not leave the pane, `esc`, `left` and `shift+tab` do ([[list-editor-standard]]).

**Check.** `TestEveryOverlayClosesWithEsc` and `TestTwoPaneScreensFollowTheStandard` in `internal/tui/navstandard_test.go`. A new overlay or two-pane screen is added to their tables in the same change.

See [[keyboard-and-feedback]], [[interface-conventions]] and [[UI-modules]].
