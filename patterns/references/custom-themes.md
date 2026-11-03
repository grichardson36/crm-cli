---
type: pattern
scale: product
status: built 2026-11-03 (feat/theme-slice), later parts proposed
---

# Themes: one entry per theme, a dark and a light palette

**Problem.** Built-in themes and custom TOML files existed as separate dark and light entries, with follower themes and a fill setting on top. The owner wants one entry per theme, a mode that picks the palette, and a pane that edits it ([[features-platform-and-themes]] F15).

## Model

- A theme is one name with two complete palettes (`Theme.Dark`, `Theme.Light` in `internal/theme/theme.go`). Each palette sets every role: `fg`, `dim`, `accent`, `danger`, `good`, `warn`, `mark`, `tab_fg`, `tab_bg`, `sel_fg`, `sel_bg`, `kind_person`, `kind_org`, `kind_deal`, `kind_task`, `kind_note`, `kind_tag`, `bg`. There is no optional role, no base, no inheritance, no follower theme.
- Built-ins, sorted by name: `anthropocene`, `catppuccin`, `default`, `gruvbox`, `high-contrast`, `nord`, `solarized`. `default` is the old `dark` and `light`. The name stays until the owner names the app.
- The list, the CLI and the pane sort all themes (built-in and custom) alphanumerically, ignoring case. Names match ignoring case.
- The mode (`auto`, `light`, `dark`, `internal/theme/mode.go`) picks the palette. `auto` takes the dark or light the terminal reports and never paints the background. `light` and `dark` force that palette and paint the whole screen.
- An unknown theme name in `config.toml` or `CRM_THEME` falls back to `default` and the TUI status line shows the error. The CLI reports `source: fallback`.
- Kind colors per object kind in Settings > Colors stay separate overrides of the palette, so people keep them when they change theme ([[kind-colors-are-chosen]]).

## Custom theme file

`~/.crm/themes/<file>.toml`. The file name comes from the theme name (`Warm Sand` is `warm-sand.toml`, a taken file name gets `-2`). A rename rewrites the same file.

```toml
name = "Warm Sand"          # optional, the file name is used without it
description = "..."         # optional
copied_from = "nord"        # optional, what shift+r resets to
[dark.colors]
fg = "#D8DEE9"              # all 18 roles
[light.colors]
fg = "#2E3440"              # all 18 roles
```

Parsing collects every problem with the file and key. A name taken by a built-in or by another file, an unknown role, a bad color and a missing role are errors, and the file is listed as invalid. The old single `[colors]` table is not read.

## Theme pane (Settings > Theme)

Entered with `enter` on the Theme row. Layout: the mode line (`auto | light | dark`), the theme list (`>` marks the chosen one), then the name and the colors of the highlighted theme for the palette of the mode shown. Color rows use plain names: text, secondary text, accent, error, success, warning, marked, tab text, tab background, selection text, selection background, background. Each is a hex field with a swatch.

| Key | Where | Does |
|---|---|---|
| `up`, `down` | pane | move, the screen previews the theme under the cursor |
| `enter` | theme | load it (the chosen theme and `>` move, saved on this device) |
| `tab` | pane | next mode, saved. While editing it changes which palette is edited |
| `c` | any theme | copy as `{name} - Copy{n}` |
| `e` | custom theme | cursor into the name and color rows, `enter` edits a row |
| `a` | custom theme | add `New theme` (`New theme 2`, ...) from the default colors |
| `d` | custom theme | delete after a question, deleting the chosen theme chooses `default` |
| `shift+r` | custom theme with `copied_from` | reset both palettes to that theme after a question |
| `esc` | pane | from the color rows back to the list, from the list close the pane and restore the chosen theme |

The footer shows only what the highlighted item can do: a built-in shows `enter`, `c`, `esc` (and `tab`, `up/down`). Built-in colors are listed dim and cannot be edited.

## CLI

`crm theme list|show|set|init|mode|path`. A theme is one name. `list -f json` shows each theme once with `dark` and `light` maps. `show` prints both palettes and the mode. `init <name>` writes the two-table template (the default colors, `copied_from = "default"`). `mode auto|light|dark` saves the mode. No MCP tool.

## Light palettes authored for the built-ins

Each takes the official light variant where there is one and keeps every role. Text roles and kind colors are at least 3:1 against `bg` (a test checks every built-in in both palettes), so a few official hues are darkened until they pass.

| Theme | Light source | Adjusted for 3:1 |
|---|---|---|
| catppuccin | Catppuccin Latte | green `#3F9E2A`, yellow `#C27B19`, pink `#CD67B2`, peach `#EC5D0A` |
| gruvbox | Gruvbox light (`#FBF1C7`, faded colors) | none |
| solarized | Solarized light (`#FDF6E3`, base01 text) | green `#839700`, yellow `#B38700`, cyan `#299D94` |
| nord | Snow Storm (`#ECEFF4`, nord0 text, nord10 accent) | aurora colors darkened: green `#5A8445`, yellow `#9A7418`, purple `#8A5F96`, orange `#B5603F` |
| high-contrast | white, black text, strong hues, 5.9:1 or more | none |
| default | the old `light` | none |
| anthropocene | the old `anthropocene-light` | none |

## Anthropocene palette

Mood: warm, paper-like, quiet, one terracotta accent. Sources: the near-black `#141413`, off-white `#FAF9F5`, light gray `#E8E6DC`, mid gray `#B0AEA5`, orange `#D97757`, blue `#6A9BCC`, green `#788C5D`. The brand orange fails 4.5:1 on off-white, so the light palette uses terracotta `#B4532F`. Tests check 4.5:1 for every text role and kind, the tab and selection pairs, and that no role lands on the background at 16 colors.

## Forcing light or dark regardless of the terminal

A frame only colors cells that have a style, so a light palette on a dark terminal is half dark. When the mode is `light` or `dark`, `View()` redraws the frame into a cell buffer (ultraviolet): every cell with no background gets `bg`, every cell with no foreground gets `fg`, styled cells keep their colors. `View.BackgroundColor` and `View.ForegroundColor` set the terminal default colors (OSC 11 and 10), so margins and cleared areas match, and Bubble Tea resets them on exit. Auto never sets a terminal color: leaving a painted mode sends the reset (OSC 110 and 111) and nothing else, so a transparent or custom terminal background is kept. NO_COLOR never paints.

Limits tests cannot show: a terminal that ignores OSC 10 and 11 keeps its own color outside the cell grid, and at 16 colors the hues show as the nearest palette colors. Tests cover the bytes the real renderer writes and every cell of every view.

## Not built

An interactive color picker (the pane has hex fields, [[core]]), snippets, theme checks in `internal/validate` with contrast warnings, a file watcher, export and import, a rendered sample in the pane, the generator ([[core]]).

**Because.** Owner decisions 2026-11-02 and 2026-11-03: one entry per theme with both palettes, mode picks the palette, a pane that edits custom themes, and no legacy names in the code.
