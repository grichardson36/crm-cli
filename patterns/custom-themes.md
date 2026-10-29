---
type: pattern
scale: product
status: proposed 2026-10-30 (owner request), first parts built in feat/themes-more
---

# Custom themes adapt Obsidian's theme model to TOML

**Problem.** Built-in themes and one TOML file per custom theme exist ([[features-platform-and-themes]] F15). Missing: light and dark variants in one theme, inheritance between custom themes, sharing, a generator, and a picker that shows more than a name. The owner also wants a light theme to stay light on a dark terminal.

## What Obsidian does (first party docs)

- A theme is a folder in `<vault>/.obsidian/themes/<name>/` with `manifest.json` (name, description) and `theme.css`. The folder name must equal the manifest name. Community themes install from Settings > Appearance > Themes > Manage ("Install and use", "Stop using this theme") and are updated by hand ("Check for updates").
- `theme.css` only overrides CSS variables. Foundations are a neutral scale (`--color-base-00` to `--color-base-100`, separate light and dark defaults), accent as three numbers (`--accent-h`, `--accent-s`, `--accent-l`) from which `--color-accent`, `-1` and `-2` derive, extended named colors (`--color-red`), and semantic colors derived from those (`--background-primary`, `--text-normal`, `--interactive-accent`). The docs say base values should typically only be defined by themes.
- Variants: variables go under `.theme-dark` or `.theme-light`, shared ones under `body`. The Base color scheme setting (light, dark, follow system) chooses which block applies.
- Snippets are plain CSS files in `<vault>/.obsidian/snippets/`, each with an on/off toggle, applied on top of the theme. Saved changes are detected automatically, and "Reload snippets" refreshes the list.
- Per theme settings are not part of core. Themes expose them through the Style Settings plugin.
- Variable groups: foundations, components, editor, plugins, window.

Sources: docs.obsidian.md (CSS variables, Foundations > Colors, Build a theme) and obsidian.md/help (Themes, CSS snippets). The docs pages did not state how community themes are listed or versioned, so that part is not copied.

## Adaptation

**O1. Roles are the variables.** Our roles are the foundations (fg, dim, accent, danger, good, warn, mark, tab, selection, bg) and the per-kind colors (`kind_person` and the rest). There is no component layer, views only take styles from the theme ([[one-abstraction-many-contexts]]). A role left out falls back to another role (kinds) or to the base theme.

**O2. File layout.** Keep `~/.crm/themes/<name>.toml` (the file name is the theme name). Add a folder form for themes with several parts, `~/.crm/themes/<name>/theme.toml` plus optional `README.md` and `LICENSE`. Snippets are small files in `~/.crm/themes/snippets/*.toml` that hold only `[colors]`, applied over the active theme in file name order and switched on in `config.toml` (`snippets = ["warm-selection"]`). As in Obsidian, the theme replaces the look and a snippet adjusts it.

**O3. Variants.** One file declares both variants, like `.theme-dark` and `.theme-light`:

```toml
description = "Anthropic colors"
[colors]            # shared by both variants
accent = "#D97757"
[dark]
bg = "#141413"
fg = "#FAF9F5"
[light]
bg = "#FAF9F5"
fg = "#141413"
accent = "#B4532F"
```

A bare theme name follows the terminal like `auto` does, and `name-dark` / `name-light` pick one explicitly. Built: the three `anthropocene` entries work this way (generated in code). Not built: `[dark]` and `[light]` tables in a file. Today a file is one variant, declared with `variant = "dark"` or `"light"`, or derived from `bg`.

**O4. Inheritance.** `base = "<built-in>"` exists. Extend it to `base = "<custom name>"` (cycles are an error that names the chain). Missing roles come from the base, so a theme can be two lines.

**O5. Validation.** Parsing already collects every problem with file and key. Move the checks into `internal/validate` so `crm validate`, `crm doctor` and the picker report the same codes ([[one-validation-framework]]): unknown role (with suggestion), bad color, half selection pair, bad variant, unknown base, cycle. New checks: a text role below 3:1 against `bg` (error), below 4.5:1 (warning), selected row not distinct from `bg`, two kinds with one color. `crm theme check <name>` prints the report and the 16-color mapping.

**O6. Live reload.** `Theme: reload` re-reads on demand today. Add a watcher on `~/.crm/themes` while the TUI runs (poll the directory mtime every second, no new dependency) so saving a file in an editor repaints, as Obsidian does for snippets. A broken save keeps the current look and shows the error in the status line.

**O7. Sharing and import.** A theme is one TOML file, so sharing is copying the file. `crm theme export <name>` writes the resolved theme (inheritance flattened) to stdout. `crm theme import <path-or-url>` validates, shows the report, refuses built-in names, and copies into `~/.crm/themes` after a yes. The format is data and import never runs anything.

**O8. Picker with preview.** The picker already previews live while the cursor moves and restores on `esc`. Add a sample in the detail pane that renders each role (heading, selected row, tab, kind chips, error, ok, dim), the variant, contrast warnings and whether the fill applies. Group the list as built-in, custom, invalid.

**O9. No per theme settings.** Obsidian needs a plugin for this. We do not copy it. The device settings are global: the theme name and the fill setting.

## Forcing light or dark regardless of the terminal (built)

Problem: a frame only colors cells that have a style. Unstyled cells and the area outside the grid keep the terminal background, so a light theme on a dark terminal is half dark.

Therefore:

1. The theme has an optional `bg` role and a variant. All built-ins have both.
2. The per device setting `fill_background` (`auto`, `on`, `off`, in Settings > Appearance and as `fill_background` in `config.toml`) decides. `auto` paints for a light theme, and for a dark theme only when the terminal reported a light background. `auto` and `anthropocene` follow the terminal and never paint by themselves. A theme without `bg`, or a terminal without color (NO_COLOR), never paints.
3. When painting, `View()` redraws the frame into a cell buffer (ultraviolet, the library under Bubble Tea v2). Every cell with no background gets `bg` and every cell with no foreground gets `fg`. Styled cells keep their colors, so selection, tabs and reverse video still work.
4. `View.BackgroundColor` and `View.ForegroundColor` set the terminal default colors (OSC 11 and 10), so margins and cleared areas match. Bubble Tea resets them (OSC 111 and 110) on exit and updates them when they change, so the picker preview repaints live.

Limits that tests cannot show: a terminal that ignores OSC 10 and 11 (some multiplexers, older Windows consoles) keeps its own color outside the cell grid. At 16 colors the brand hues show as the nearest palette colors. Tests cover the bytes the real renderer writes (OSC 10 and 11 set and reset, background on cells) and every cell of every view in a cell buffer, not the pixels a terminal draws.

## Anthropocene palette (theme-factory method)

Mood: warm, paper-like, quiet, one terracotta accent. Sources: the Anthropic brand colors (near-black `#141413`, off-white `#FAF9F5`, light gray `#E8E6DC`, mid gray `#B0AEA5`, orange `#D97757`, blue `#6A9BCC`, green `#788C5D`) and the look of Claude Code in a terminal (orange accent, gray secondary text, no blocks of color).

| Role | Dark | Light | Why |
|---|---|---|---|
| bg | `#141413` | `#FAF9F5` | brand dark and light |
| fg | `#FAF9F5` | `#141413` | 17.5:1 |
| dim | `#9C9A91` | `#6B695F` | mid gray adjusted to keep 4.5:1 |
| accent, tab_bg | `#D97757` | `#B4532F` | the brand orange fails 4.5:1 on off-white, so the light variant uses a darker terracotta |
| danger, good, warn | `#E5736F`, `#8DAA6B`, `#E2B259` | `#B3392F`, `#5A7240`, `#8A5A00` | red, brand green, amber |
| mark, person | `#6A9BCC` | `#3F73A6` | brand blue |
| sel | `#FAF9F5` on `#2E2D2A` | `#141413` on `#E8E6DC` | a warm gray bar |

Kinds: person blue, organization kraft tan, deal green, task violet, note pale gray, tag teal. Tests check 4.5:1 for every text role and kind against `bg`, the tab and selection pairs, and that no role lands on the background color at 16 colors.

## What is built

Eleven built-ins including `anthropocene`, `anthropocene-dark` and `anthropocene-light`, custom TOML themes with `base`, `crm theme list|show|set|path|init`, live preview in the picker, `Theme: reload`, NO_COLOR handling, the nohardcode test, the `bg` role and `variant`, the fill setting.

## Not built

Variant tables in one file, custom `base`, snippets, validate codes, watcher, export and import, the preview pane, the generator ([[roadmap]]).

**Because.** Owner request 2026-10-30: more themes, custom themes designed like Obsidian's, and a light theme that stays light on a dark terminal.
