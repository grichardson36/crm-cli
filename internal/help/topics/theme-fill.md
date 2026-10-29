# Theme fill: light or dark whatever the terminal is

A theme with a `bg` color can paint the whole screen, so a light theme stays light on a dark terminal. Settings > Appearance > "Fill the whole screen with the theme background" (stored per device as `fill_background` in `~/.crm/config.toml`):

```sh crm-example
crm theme show anthropocene-light
```

- `auto` (default): on for a light theme, for a dark theme only when the terminal reports a light background. `auto` and `anthropocene` follow the terminal and never need it.
- `on`: always paint. `off`: never paint.

Every cell gets the theme background and foreground, and where the terminal supports it the program also sets the terminal default colors (OSC 10 and 11), so margins and cleared areas match. The terminal's own colors come back when the program exits. Without color support (`NO_COLOR`, `--no-color`) nothing is painted. A terminal that ignores OSC 10 and 11 may show its own color in the margin outside the screen grid.

See also: theme, tui.
