---
type: reference
---
# Popup component

Every overlay is one popup: a centered box with a border over the dimmed screen. The top row is a filter line or a plain title, the list is below, and the key hints of the popup are the bottom row inside the box. Code: `internal/tui/popup.go` (box) and `internal/tui/overlay.go` (list, keys, config). Navigation keys follow [[navigation-standard]]. Hints follow [[key-hints-in-one-place]], except that a popup shows its own hints in the box and not in the footer.

```
┌──────────────────────────────┐
│ > Select a command…          │  filter line, or the title
├──────────────────────────────┤
│ Add person                   │
│ Add task                     │
├──────────────────────────────┤
│ enter run | esc close | up/down select │
└──────────────────────────────┘
```

## Add a popup by config

Call `m.openPicker(title, items, onPick)`, or build an `overlay` with `m.newOverlay` and show it with `m.showOverlay`. The config is the `overlay` struct:

1. `title`: the title row. With a filter line it is the placeholder.
2. `items` (`overlayItem`): `label`, `hint` (a key, drawn at the left), `desc` (drawn dim at the right), `disabled` (reason, drawn instead of desc, enter does nothing), `divider` (heading row), `selector` with `current` (a row whose value left and right change), `isCur` (accent label).
3. Filter: the line shows when the popup is the palette, has `create`, has more than 7 items, or something is typed. A short picker shows its title until the first letter. Typing filters in every popup except one with `noFilter`.
4. Hints: default is the keys of the highlighted row (`enter pick`, `left/right change` on a selector, `esc close`, `up/down select`). `hints` sets fixed hints. `hintsFor` builds them from the highlighted row. One word per action.
5. Shortcuts and callbacks: `digits`, `letters`, `onPick`, `onCursor` (preview), `onCancel`, `onKey` (extra keys).
6. `noDesc` hides the desc column. `minW` keeps a minimum text width when the hints need room.

## Size rules

The box is centered. Text width is the widest row within 44 to 76 cells (the palette 96), then cut to the terminal width minus 4. The list is at most 14 rows and at most the terminal height minus 8, and scrolls. A popup with a filter keeps its height while the filter changes.

## Views list

`m.newViewsList(focus)` returns the Views list as an overlay that is not shown. The Views popup shows it. A pane that embeds the list draws its rows with `m.popupListRows(o, width, rows)` and keeps the same `onKey`, `onPick` and `hintsFor`.

## Not a popup

The list editor (`listeditor.go`) still draws a full screen through `overlay.draw`.

## Check

`popup_test.go`: centered box, width limits on a small terminal, hints inside the box, filter and title variants. `TestEveryOverlayClosesWithEsc` covers esc.
