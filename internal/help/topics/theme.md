# Themes

A theme is one name with a dark palette and a light palette. Built in: `anthropocene`, `catppuccin` (Mocha and Latte), `default`, `gruvbox`, `high-contrast`, `nord`, `solarized`. The list is sorted by name.

```sh crm-example
crm theme list
crm theme show nord
crm theme path
```

```sh crm-example-skip
crm theme set nord
crm theme mode dark
```

Saved per device in `~/.crm/config.toml` (`theme = "nord"`, `mode = "dark"`). `CRM_THEME` overrides the theme, for example in the web terminal. An unknown theme name falls back to `default` and the TUI shows an error.

The mode picks the palette. `auto` uses the dark or light the terminal reports and never paints the background. `light` and `dark` force that palette and paint the whole screen, and where the terminal supports it set its default colors (OSC 10 and 11). Without color support (`NO_COLOR`, `--no-color`) nothing is painted. A terminal that ignores OSC 10 and 11 may show its own color in the margin.

In the TUI open Settings > Theme (or `Theme: choose` in the palette) and press `enter`. `up` and `down` preview a theme, `enter` loads it, `tab` changes the mode, `c` copies it as `{name} - Copy1`, `esc` closes. On a custom theme `e` edits the name and colors, `a` adds one from the default colors, `d` deletes, `shift+r` resets the colors to the theme it was copied from.

## Make your own

```sh crm-example-skip
crm theme init my-theme
```

`crm theme init` writes `~/.crm/themes/my-theme.toml`: the default colors in two full tables.
```toml
name = "my-theme"           # optional, the file name is used without it
copied_from = "default"     # optional, what shift+r resets to
[dark.colors]
accent = "#ff8800"          # every role is set, in both tables
[light.colors]
accent = "#b4532f"
```
Roles: `fg`, `dim`, `accent`, `danger`, `good`, `warn`, `mark`, `tab_fg`, `tab_bg`, `sel_fg`, `sel_bg`, `kind_person`, `kind_org`, `kind_deal`, `kind_task`, `kind_note`, `kind_tag`, `bg`. A color is `"#rrggbb"`, `"#rgb"` or an ANSI number `0` to `255`. Settings > Colors overrides the kind colors without changing the theme. After editing a file run `Theme: reload`. A broken file never stops the TUI, `crm theme list` shows it as `invalid` with the file and key. Agents: `crm theme list -f json`.

See also: tui, tui-keys, output.
