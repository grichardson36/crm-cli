---
type: feature-area
---
# Platforms, domains and themes

Web, phone and desktop access, multi-domain data, themes and learn mode.


**Follows patterns:** [[core]], [[core]], [[core]]

**Research:** [[research-charm-ecosystem]], [[research-terminal-images]]

**See also:** [[index]]

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| F01 | Web app | Dex | PART | Read-only TUI at crm.andersonvdc.com (snapshot) | Superseded by F13 (local-first sync). | Editable web needs pibrain master or sync (Q1.1 pending) | simple | P2 |
| F02 | Mobile app | Dex | PART | Same web terminal on phone | Phone layout acceptable in web terminal? | Phone layout check; no native app | - | P3 |
| F03 | Desktop app | Dex | PART | `crm.exe tui` | - | none | - |  |
| F10 | Multi-domain data separation (not in Dex) | Us | BEYOND | `--db`/`CRM_DB`, personal + business DBs | Startup picker or key to switch domains? | TUI domain switcher (`ctrl+o`) and a startup picker | simple | P2 |
| F14 | Phone-friendly TUI over the browser terminal (touch targets, no modifier-key reliance) | Us | PART | Footer buttons and tappable status line exist; untested on a real phone. | Which keys work on a phone keyboard? | Make every palette action tappable; footer buttons for the top 6 actions; avoid ctrl chords as the only path (palette via `:` or a tap on the status bar) | tactile, simple | P2 |
| F15 | Themes and custom theming for the TUI (colors, selection style, density), switchable from the palette; user theme files | Us | PART | MERGED to main 2026-10-16 (merge b376a1a). 8 built-in themes, custom TOML in ~/.crm/themes, `crm theme`, palette choose with live preview, NO_COLOR. 2026-10-30 (feat/themes-more): anthropocene, anthropocene-dark and anthropocene-light built-ins, `bg` role and variants, the mode (auto, light, dark) paints every cell and sets the terminal default colors (OSC 10/11). 2026-11-03 (feat/theme-slice, X46): one entry per theme with a dark and a light palette (`anthropocene`, `catppuccin`, `default`, `gruvbox`, `high-contrast`, `nord`, `solarized`, plus custom), the mode picks the palette (auto never paints, light and dark paint), no follower or `-dark`/`-light` names, custom files are TOML with `[dark.colors]` and `[light.colors]`. The Theme row of Settings holds the mode line, the theme list (preview, enter loads, c copies, and on custom themes e edits, a adds, d deletes, shift+r resets) and a hex field with a swatch per color. `crm theme list|show|set|init|mode|path`. Doors: CLI and TUI, no MCP tool. Design in [[custom-themes]]. Open: interactive color picker, snippets, generator. | Ship how many built-ins (dark, light, high-contrast, solarized, gruvbox)? Theme file format (TOML/JSON in `~/.crm/themes/`)? | Central `Theme` struct (named color roles: fg, dim, accent, sel, warn, ok, mark), loaded from built-ins or `~/.crm/themes/*.toml`; palette action "Theme: ..." with live preview; remembered per device in config; web terminal honors it | tactile | P2 |
| I09 | In-TUI learn mode: first-run tour and "What can I do here?" palette entry that lists actions for the current view | Us | TODO | Palette and `?` exist; no first-run tour. | - | Palette empty query shows context-relevant actions first | intuitive | P2 |
