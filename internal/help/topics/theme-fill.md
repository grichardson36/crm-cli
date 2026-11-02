# Theme mode: light or dark whatever the terminal is

The mode decides whether the screen is light or dark. Settings > Appearance, `left` and `right` on the mode line (stored per device as `mode` in `~/.crm/config.toml`):

```sh crm-example
crm theme show anthropocene-light
```

- `auto` (default): `auto` and `anthropocene` take the terminal's dark or light. A fixed theme paints its own background only when it disagrees with the terminal.
- `light` or `dark`: `auto` and `anthropocene` take that variant, and the whole screen is painted with the theme background.

Every cell gets the theme background and foreground, and where the terminal supports it the program also sets the terminal default colors (OSC 10 and 11), so margins and cleared areas match. The terminal's own colors come back when the program exits. Without color support (`NO_COLOR`, `--no-color`) nothing is painted. A terminal that ignores OSC 10 and 11 may show its own color in the margin outside the screen grid.

See also: theme, tui.
