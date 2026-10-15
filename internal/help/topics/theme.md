# Themes

Color themes for the terminal UI (`crm tui`). Built in: `auto` (default, picks dark or light from the terminal), `dark`, `light`, `high-contrast`, `catppuccin` (Mocha), `nord`, `gruvbox`, `solarized`.

```sh crm-example
crm theme list
crm theme show nord
crm theme path
```

## Choose a theme

```sh crm-example-skip
crm theme set nord
```

The choice is saved per device in `~/.crm/config.toml` (`theme = "nord"`). The environment variable `CRM_THEME` overrides it for one shell or service, for example the web terminal. In the TUI open the palette (`ctrl+k`) and run `Theme: choose`. The screen previews each theme as you move through the list, `enter` keeps it and `esc` restores the old one.

## Make your own

```sh crm-example-skip
crm theme init my-theme
crm theme set my-theme
```

`crm theme init` writes `~/.crm/themes/my-theme.toml` with every role filled in. The file name is the theme name (lower case letters, digits, `-`, `_`).

```toml
description = "my custom theme"
base = "dark"          # optional: inherit missing roles from a built-in

[colors]
accent = "#ff8800"     # names and key hints
danger = "#ff5555"
```

Colors are `"#rrggbb"`, `"#rgb"` or an ANSI number `0` to `255`. Roles: `fg`, `dim`, `accent`, `danger`, `good`, `warn`, `mark`, `tab_fg`, `tab_bg`, and optionally `sel_fg` with `sel_bg` for the selected row (left out, the row is drawn in reverse video). Without `base`, every role except the two `sel_*` roles is required.

After editing, run `Theme: reload` in the palette to apply the file without restarting. A broken file never stops the TUI. `crm theme list` shows it as `invalid` with the error, naming the file and the key.

## Terminals and NO_COLOR

Terminals without truecolor get the nearest 256 or 16 colors automatically. With `NO_COLOR` set, or with `--no-color`, the TUI uses no colors. The selected row and the active tab stay visible through reverse video.

## For agents

`crm theme list -f json` and `crm theme show NAME -f json` print themes with their colors. `crm theme set NAME` saves the choice.

## See also

`tui`, `output`, `domains`.
