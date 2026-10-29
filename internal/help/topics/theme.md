# Themes

Color themes for the terminal UI (`crm tui`). Built in: `auto` (default, picks dark or light from the terminal), `dark`, `light`, `high-contrast`, `catppuccin` (Mocha), `nord`, `gruvbox`, `solarized`, and the Anthropic-inspired `anthropocene` (follows the terminal), `anthropocene-dark` (warm near-black, terracotta accent) and `anthropocene-light` (warm off-white).

```sh crm-example
crm theme list
crm theme show nord
crm theme path
```

## Choose a theme

```sh crm-example-skip
crm theme set nord
```

Saved per device in `~/.crm/config.toml` (`theme = "nord"`). The environment variable `CRM_THEME` overrides it, for example in the web terminal. In the TUI run `Theme: choose` from the palette (`ctrl+k`): it previews each theme as you move, `enter` keeps it, `esc` restores the old one.

## Make your own

```sh crm-example-skip
crm theme init my-theme
crm theme set my-theme
```

`crm theme init` writes `~/.crm/themes/my-theme.toml` (the file name is the theme name).
```toml
base = "dark"          # optional: inherit missing roles from a built-in

[colors]
accent = "#ff8800"     # names and key hints
danger = "#ff5555"
```

Colors are `"#rrggbb"`, `"#rgb"` or an ANSI number `0` to `255`. Roles: `fg`, `dim`, `accent`, `danger`, `good`, `warn`, `mark`, `tab_fg`, `tab_bg`, and optionally `sel_fg` with `sel_bg` (left out, the selected row is reverse video). Optional `bg` is the screen background (used by the fill setting below) and `variant = "dark"` or `"light"` (top level key) says which kind it is, otherwise it is read from `bg`. Optional `kind_person`, `kind_org`, `kind_deal`, `kind_task`, `kind_note` and `kind_tag` color the object kinds in the related segment of pipe lines (left out, a kind takes the color of accent, good, warn, mark, fg or dim). Without `base`, every role except `sel_*` and `kind_*` is required.

## Light or dark whatever the terminal is

A theme with a `bg` color can paint the whole screen, so a light theme stays light on a dark terminal. Settings > Appearance > "Fill the whole screen with the theme background" (stored per device as `fill_background` in `~/.crm/config.toml`):

- `auto` (default): on for a light theme, for a dark theme only when the terminal reports a light background. `auto` and `anthropocene` follow the terminal and never need it.
- `on`: always paint. `off`: never paint.

Every cell gets the theme background and foreground, and where the terminal supports it the program also sets the terminal default colors (OSC 10 and 11), so margins and cleared areas match. The terminal's own colors come back when the program exits. Without color support (`NO_COLOR`, `--no-color`) nothing is painted. A terminal that ignores OSC 10 and 11 may show its own color in the margin outside the screen grid.

After editing, run `Theme: reload` in the palette. A broken file never stops the TUI: `crm theme list` shows it as `invalid` with the file and key. With `NO_COLOR` or `--no-color` no colors are used. Agents: `crm theme list -f json`.

See also: tui, tui-keys, output.
