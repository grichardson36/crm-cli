# Terminal UI: old keys and where they went

Keymap part B2 (owner decisions 2026-10-28). Every function a removed key had is on another key or in the palette.

| Old key | Now |
|---|---|
| `ctrl+n`, `ctrl+p` | `down`, `up` |
| `:` | `/` or `ctrl+k` |
| `q` in the compare and in text pages | `esc` |
| `J`, `K` in Modules | `shift+down`, `shift+up` |
| `r` in Modules | palette: Modules: reset |
| `F` | palette: Set keep-in-touch |
| `ctrl+b`, `ctrl+w` in the compare | palette: Change pair, Switch pane |
| `ctrl+o` sort, `ctrl+l` level, `ctrl+f` type | `ctrl+o` view options |
| `*` | `ctrl+a` |
| `E` | cells of the linked row or note |
| `y` | `ctrl+c` (palette: Copy value) |
| `tab`, `shift+tab` toggle in the compare | `space` toggles, `tab` moves to the next field |
| bulk tier, bulk stage | palette: Set field on selected |
| purge of people only | `d` or the palette, any archived type |

Pipe-line prompts (owner decision 2026-10-29): the grayscale "tab: link ..." offer of the add picker (and the dropped link chip with `l`/`u`) is replaced by link segments. `tab` accepts the highlighted suggestion of the popup while it is open, `up`/`down` move through it, `esc` closes it. No other key changed.

| Old | Now |
|---|---|
| `a`, kind, `tab` (link the highlighted object) | `a`, kind, type the line, `tab` in the segment of that kind (the highlighted object is its first suggestion) |
| unlink the offered object | backspace over its name before `enter` |

Kept on purpose: `ctrl+e`, `m`, `d`, the `y` and `n` answers in prompts, `alt+d`, the digit hints in pickers, the digits `0` to `9`, `[`, `]` and `v` for views, `ctrl+d`, `ctrl+u`. `ctrl+left` and `ctrl+right` are new aliases of `[` and `]`.

See also: tui-keys, tui-selection, keys.
