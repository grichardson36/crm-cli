---
type: pattern
scale: interaction
status: decided (owner 2026-10-27), built in feat/keymap-a
---

# Navigation units and homes

**Problem.** Every list scrolled the same way, one row or one page, though a birthday list wants a day, a name list wants a letter and a calendar wants today. Per-view keys (`ctrl+t`, `g`, `G`) grew to fill the gap.

**Therefore.** Navigation is arrow keys, `tab` and `shift+tab` only. On top of that, each view declares in one place (`viewNavs` in `internal/tui/navunits.go`) how far `ctrl+up` and `ctrl+down` jump and, optionally, a home destination. A new view that declares nothing inherits a page and no home, and one entry gives it more.

| Keys | Does |
|---|---|
| `up`, `down`, `tab`, `shift+tab` | Move a row, or the pane focus. |
| `ctrl+u`, `ctrl+d` | Half a page. |
| `pgup`, `pgdn` | A full page. The difference from the ctrl pair is documented in `?`. |
| `ctrl+up`, `ctrl+down` | Jump by the view's unit: a day in Birthdays, the first-letter group in People and Organizations, a page elsewhere. From the middle of a group, `ctrl+up` goes to the start of that group first. |
| `ctrl+shift+up`, `ctrl+shift+down` | The view's home destination when it declares one (today in Birthdays and future calendar-like views), else the first and last row. |
| `home`, `end` | The first and last row in every view. They are the fallback. |

**Retired.** `h j k l g G`, the `1` and `2` aliases in compare, `ctrl+t` and the old `ctrl+up` go-to-today. "Go to today" stays as a palette entry for the Tasks anchor.

**Terminal support.** Bubble Tea reports `ctrl+shift+up` and `ctrl+shift+down` (`CSI 1;6A` and `1;6B`) only where the terminal sends the xterm modifier sequences. Many terminals and multiplexers do not. The chords work where they are reported, `home` and `end` work everywhere, and `crm keys set` warns when a binding uses a chord terminals commonly swallow ([[keymap-layers-and-scopes]]).

**Because.** Owner decision 2026-10-27. Ctrl keys move the view ([[ctrl-moves-the-view-bare-moves-the-object]]), so jumping and going home are ctrl chords. Declaring the unit in one place is [[objects-first]] applied to views.

**Larger patterns.** [[ctrl-moves-the-view-bare-moves-the-object]], [[universal-verbs]]

**Smaller patterns.** [[keymap-layers-and-scopes]], [[keys-are-rebindable]]

**Sources.** Owner decision 2026-10-27. `internal/tui/navunits.go`

**Inspired by.** [[vim-and-lazygit]]
