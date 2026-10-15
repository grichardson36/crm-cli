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

Colors are `"#rrggbb"`, `"#rgb"` or an ANSI number `0` to `255`. Roles: `fg`, `dim`, `accent`, `danger`, `good`, `warn`, `mark`, `tab_fg`, `tab_bg`, and optionally `sel_fg` with `sel_bg` (left out, the selected row is reverse video). Without `base`, every role except `sel_*` is required.

After editing, run `Theme: reload` in the palette. A broken file never stops the TUI: `crm theme list` shows it as `invalid` with the file and key. With `NO_COLOR` or `--no-color` no colors are used. Agents: `crm theme list -f json`.

See also: tui, tui-keys, output.
