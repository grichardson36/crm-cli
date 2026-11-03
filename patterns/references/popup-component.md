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

## Syntax helpers stay

A popup shows no explanatory sentences or action descriptions, but the helpers that guide typing stay:

1. The palette has one dim helper row under its filter line (`popupBox.helper`) with the scope prefixes as short words (`> actions @ people # tags = settings ? help`). It shows while the box is empty or holds only a prefix.
2. The palette command line (a verb and a space) keeps the grey shape after the cursor (`hint.go`), the token row (`#tag @Name !high ~30m by fri remind fri every 3m`) while nothing else fills the list, the preview, errors, warnings and candidates.
3. The footer prompts (task, note, organization, deal, person add lines) are not popups. They keep the pipe labels after the cursor, the error of a closed segment, the soft indicator, the autocomplete list and the chips (`pipeview.go`).
4. Record context (people, organizations) and disabled reasons stay in rows. Action descriptions are tooltips and are not drawn (they stay searchable).

## Views list

`m.newViewsList(focus)` returns the Views list as an overlay that is not shown. The Views popup shows it. A pane that embeds the list draws its rows with `m.popupListRows(o, width, rows)` and keeps the same `onKey`, `onPick` and `hintsFor`.

The list is the shown views numbered 1 to 0, a divider with no label (drawn as a line), then the hidden views as `dim` rows (gray, no number). `shift+up` and `shift+down` move the highlighted view inside its group. At the edge of its group a move toward the line crosses it: a shown view is hidden and lands first below the line, a hidden view is shown and lands last above it. A move away from the line at the edge does nothing. Settings refuses to cross because it cannot be hidden. The data is the `[modules]` table (order and hidden) and the saved views' `ShowInTabs`, a view's number follows its place among the shown ones. The filter matches names only (`nameOnly`), digits pick a view only while the filter is empty, `enter` opens the highlighted row. `shift+r` restores `modules.Default()`. Hints show only keys of the highlighted row: `up/down select`, `enter open`, `shift+up/down move` (not while a filter is typed), `shift+r reset`, `esc close`. The popup has no edit, delete or restore key: plain letters are typed into the filter. Saved views are managed in the Settings Views pane, which embeds the same list (`m.newViewsListFor(focus, true)` and `popupListRows`, `settingspane.go`) with no filter line, so `e` edit, `d` delete and `r` restore work there and the footer shows them only for the highlighted row.

## Not a popup

The list editor (`listeditor.go`) is not an overlay. It is drawn in the right pane of its Settings row ([[list-editor-standard]]).

## Check

`popup_test.go`: centered box, width limits on a small terminal, hints inside the box, filter and title variants. `helpers_visible_test.go`: palette prefix helpers, command line helpers and footer prompt helpers fail the build if they disappear. `views_screen_test.go`: line, crossing, filter, reset and hints of the Views list. `TestEveryOverlayClosesWithEsc` covers esc.
