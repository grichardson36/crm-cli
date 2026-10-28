---
type: research
---
# Research: Charm and Bubble Tea ecosystem, what the framework can lean on

Date 2026-10-28. Trigger: owner is building a framework for modular local-first TUI programs (view modules, field editors by field type, autocomplete popups, scoped keymap, themes, grouped lists) and asked what to borrow instead of hand-building. Companions: [[research-tui-building-blocks]] (layers L1 to L9), [[research-view-module-standard]] (module taxonomy), [[research-tui-inspirations]], [[research-terminal-images]], [[autocomplete-component]], [[field-types-drive-editors]], [[related-segment]], [[keymap-layers-and-scopes]], [[grouped-lists-with-dividers]], [[one-abstraction-many-contexts]], [[borrow-before-inventing]], [[modules-are-user-arranged]], [[three-doors-one-core]].

Evidence tags: api (GitHub API through `gh`: repo metadata, releases, `go.mod`, directory listings, README text, read 2026-10-28), code (our repo, read directly), search (GitHub repository search, first results only), gap (not verified). Star counts, license ids and dates are as the API reported them on that day. Nothing was built, installed or run, so every behavior claim is from a README, a source file name or a source skim, not a test.

## What we hand-built (code)

Read-only pass over `crm-cli/internal` on 2026-10-28.

| Area | Size | Notes |
|---|---|---|
| `internal/tui` | 62 non-test Go files, 19,162 lines. 41 test files, 12,631 lines | Biggest: `form.go` 1,149, `model.go` 1,100, `merge_compare.go` 995, `history.go` 651, `orgdeal.go` 631, `overlay.go` 602, `objects.go` 574, `actions.go` 563, `cmdline.go` 543 |
| Keymap and actions | `keymap.go` 354, `keybind.go` 332, `actions.go` 563, `internal/keys` 255 | Scopes, layers, overrides by action id |
| Autocomplete | `autocomplete.go` 228, `pipecomplete.go` 310, `typeahead.go` 188, `internal/suggest` 174 | Popup, ranker, typo, aliases |
| Field editors and cells | `fieldedit.go` 420, `cells.go` 380, `setfield.go` 289 | Inline in the row |
| Theme | `internal/theme` 690 (3 files) | Eleven roles, TOML themes |
| Modules | `modules.go` 245, `internal/modules` 381 | Arrangement as data, views are Go |
| Rendering helpers | `view.go` 506, `grouped.go` 65, `hint.go` 159, `toast.go` 64, `mouse.go` 142 | `fit`, `cleanCell`, tab layout with x ranges |

Charm imports today (go.mod): bubbletea v1.3.4, bubbles v0.20.0 (only `textinput`, `viewport`, `spinner` are used), lipgloss v1.1.0, x/ansi v0.8.0 (one use), go-runewidth v0.0.16 (17 files, about 60 calls), termenv v0.16.0 (profile in theme and tests). `go 1.24.0`. No glamour, huh, bubblezone, harmonica, teatest.

## Findings

### Part 1. Core framework status

- **C1. Bubble Tea v2 is stable and v1 is frozen (api).** v2.0.0 released 2026-02-24, now v2.1.1 (2026-10-28), 45.5k stars, MIT, pushed daily. Last v1 release is v1.3.10 on 2025-09-17, so v1 has had no release in 13 months. Module path moved to `charm.land/bubbletea/v2`. Needs Go 1.26.0 (`go.mod`). https://github.com/charmbracelet/bubbletea, upgrade guide https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md (read in full).
- **C2. What v2 changes (api, upgrade guide).** `View()` returns `tea.View` (a struct with `Content`, `AltScreen`, `MouseMode`, `Cursor`, `WindowTitle`, `ProgressBar`, `KeyboardEnhancements`, `OnMouse`). Alt screen, mouse and cursor options and commands are gone and become View fields. `tea.KeyMsg` becomes an interface, `tea.KeyPressMsg` for presses, space is `"space"`, new key release events and shift+enter style keys on terminals with the kitty keyboard protocol. Mouse messages split by type (`MouseClickMsg`, wheel, release, motion). New "Cursed Renderer" (ncurses-style diff, built on ultraviolet). Built-in color downsampling. `tea.WithColorProfile` and `tea.WithWindowSize` for tests.
- **C3. Migration cost for us (code plus api).** Surface found: one `View() string`, one `tea.NewProgram(... WithAltScreen, WithMouseCellMotion, WithContext)` line, `tea.KeyMsg` at 41 places in 22 files (20 places in 14 non-test files, 21 in 8 test files that build key messages by hand), mouse handling in `mouse.go` and `overlay.go` (about 20 lines on `MouseButton*` and `MouseAction*`). The theme package uses lipgloss color profiles and termenv (35 hits in 9 files, mostly tests). Bubbles v2 only works with Bubble Tea v2 (its `go.mod` requires `charm.land/bubbletea/v2`), and the textinput, viewport and spinner APIs changed (width and height become getters and setters, styles restructured, `DefaultKeyMap` becomes a function; `charm.land/bubbles/v2` upgrade guide https://github.com/charmbracelet/bubbles/blob/main/UPGRADE_GUIDE_V2.md, read in part). So it is one bundle: bubbletea, bubbles, lipgloss together. The code surface is small. The risk is behavior, not compile errors: key strings (`"space"`), grapheme width, renderer output, Windows Terminal. Those are gap until a spike runs.
- **C4. Go toolchain floor rises (api).** bubbletea v2 `go 1.26.0`, bubbles v2 `go 1.26.0`, lipgloss v2 `go 1.26.7`, glamour v2 `go 1.26.7`, ultraviolet and x/cellbuf `go 1.25`. Our `go.mod` says 1.24.0. The local machine has Go 1.27.0. Whether pibrain, the OpenBSD VPS and any CI have a new enough Go was not checked (gap).
- **C5. Lip Gloss v2 adds a compositor (api, source skim).** v2.0.6 (2026-08-11), MIT. Root package now has `Layer` (content, `X`, `Y`, `Z`, nested child layers, `ID`), `Compositor` (`Render`, `Draw`, and `Hit(x, y)` returning the top layer under a cell with its ID and bounds) and `Canvas` (a cell buffer on ultraviolet). Subpackages `table`, `tree`, `list`. Styles no longer query the terminal, Bubble Tea owns I/O, `compat` package keeps `AdaptiveColor`-style helpers. `lipgloss.Width` and wrapping are grapheme-aware (depends on `clipperhouse/displaywidth` and `rivo/uniseg`). https://github.com/charmbracelet/lipgloss. This is the first-party answer to popups over content and mouse hit testing. Whether `Compositor.Hit` handles inline spans (not just rectangles) is gap, it works on layers.
- **C6. Ultraviolet (api).** `charmbracelet/ultraviolet`, MIT, pushed 2026-10-23, no tagged release (pseudo-versions), 398 stars. "Primitives for building TUIs in Go": cell buffer, input decoding, diffing renderer. Powers Bubble Tea v2 and Lip Gloss v2, can be used alone. Its `examples/advanced/image` imports `x/ansi/kitty`, `x/ansi/sixel` and `x/ansi/iterm2` and offers a blocks encoding. https://github.com/charmbracelet/ultraviolet. Use only through Bubble Tea and Lip Gloss. Standalone use would replace Bubble Tea's model loop, which we do not want.

### Part 2. Bubbles components

- **C7. Bubbles v2 (api).** v2.2.1 (2026-08-24), MIT, 9k stars, pushed 2026-10-27. Packages: `cursor`, `filepicker`, `help`, `key`, `list`, `paginator`, `progress`, `spinner`, `stopwatch`, `table`, `textarea`, `textinput`, `timer`, `tree` (new, "feat: tree bubble" 2026-08-21), `viewport`. https://github.com/charmbracelet/bubbles.
- **C8. `list` (api, source skim).** Batteries-included browser: pagination, fuzzy filter (uses `sahilm/fuzzy`, MIT, last release 2026-06-11), help, spinner, status messages. Item interface needs `FilterValue()`. I found no multi-select, no non-selectable divider rows and no per-row inline editing in the README or the source names I read (gap: not exhaustively searched). It fits "a list of titled items". Our list has marks and bulk verbs ([[bulk-actions-always]]), group dividers ([[grouped-lists-with-dividers]]), meta columns and link cells.
- **C9. `table` (api, source skim).** Columns, rows, a cursor, a focus state, `WithStyles`, `SelectedRow()`, a viewport inside. Read-only cells, no sort, filter or edit built in. Third-party `Evertras/bubble-table` (MIT, 578 stars, v0.23.0 2026-09-09, deps already on bubbles v2 and bubbletea v2) adds filter, sort and pagination (from its description, features not fully read).
- **C10. `textinput` and `textarea` (api, source skim).** textinput has inline suggestions (`ShowSuggestions`, `AcceptSuggestion` tab, `NextSuggestion`, `PrevSuggestion`): one ghost-text completion, no popup. textarea is multi-line with its own cursor and key map. We use textinput already. Our longtext editor is a one-line input where a new line is `alt+enter` ([[field-types-drive-editors]]), which textarea would replace.
- **C11. `viewport` v2 (api, upgrade guide).** Adds soft wrap, a left gutter function (line numbers), regex highlight with next and previous, `StyleLineFunc`, horizontal scroll, `SetContentLines`. Useful for the markdown and detail modules. We use viewport in `model.go` for help and detail.
- **C12. `help` and `key` (api).** `key.Binding` with `WithKeys` and `WithHelp`, `help` renders short and full help from any `KeyMap`. No scopes, layers, rebinding file or conflict check. Every bubble takes a `KeyMap` of `key.Binding`, so a small adapter from our action registry would let third-party bubbles follow our keys.
- **C13. `filepicker`, `paginator`, `progress`, `spinner`, `timer`, `stopwatch`, `cursor` (api).** All present in v2. `filepicker` could back a file attach or import-path editor ([[field-types-drive-editors]] has no file type yet). The others have no CRM use now, except `progress` for sync or import (not needed yet).
- **C14. `tree` bubble (api).** New in August 2026 (one feature commit visible, 2 months old), built on `lipgloss/v2/tree`, with expand and collapse, page and half-page keys, `help` integration. Maturity unknown (gap). A candidate for the tree module in [[research-view-module-standard]].

### Part 3. Huh, Glamour, small libraries

- **C15. Huh v2 (api).** v2.0.3 (2026-03-10), MIT, 7.2k stars, pushed 2026-10-01. Forms made of groups of fields: `Input` (with `Suggestions`, `SuggestionsFunc`, `Validate`, `CharLimit`, `EchoMode`, `Placeholder`), `Text`, `Select`, `MultiSelect`, `Confirm`, `Note`, `FilePicker`. Validation per field, dynamic forms (`...Func` variants re-evaluated when a bound value changes), `WithAccessible(true)` drops the TUI for plain prompts for screen readers, column and grid layouts, embeddable in a Bubble Tea program as a model. https://github.com/charmbracelet/huh. It is a sequential form model. Our editors are inline in a record row, driven by `model.Fields` and shared with CLI and MCP checks ([[one-validation-framework]], [[three-doors-one-core]]). Huh would be a second validation and field vocabulary. It fits one-shot wizards (first run, import mapping), not row editors.
- **C16. Glamour v2 (api).** v2.0.1 (2026-06-12), MIT, 3.7k stars. Renders markdown to styled ANSI with stylesheets (dark, light, custom JSON), word wrap, built on goldmark and chroma (syntax highlighting). Read-only rendering. Used by Glow (27.8k stars). https://github.com/charmbracelet/glamour. We have no markdown renderer today (not in go.mod), so this is greenfield for the markdown view module. Editing markdown is not provided, textarea or our own editor does that.
- **C17. Harmonica (api).** Physics-based spring animation, v0.2.0 (2022-04-15), repo pushed 2026-08-13 (typo fix), MIT, 1.6k stars. Tiny and finished. No animation need in a CRM list UI. Skip.
- **C18. Log (api).** `charmbracelet/log` v2.0.1 (2026-09-03), MIT. Leveled colored logger. Bubble Tea owns stdout, so TUI debug logging needs a file anyway. Optional.

### Part 4. charmbracelet/x

`charmbracelet/x` (MIT, 324 stars, pushed 2026-10-28) is a multi-module repo of experimental packages. Most modules have pseudo-versions or few tags, so pin by commit through the module cache. https://github.com/charmbracelet/x.

- **C19. `x/ansi` (api).** Last change 2026-10-08. ANSI parser and helpers: `StringWidth`, `Truncate`, `TruncateLeft`, `Cut`, `Wrap`, `Wordwrap`, `Hardwrap`, `Strip`, style builders, hyperlinks, clipboard (OSC 52), `kitty`, `sixel`, `iterm2` image encoders, progress bar, OSC 7501 program status. Already a dependency (v0.8.0, one use). Everything we do with `runewidth.Truncate` plus manual padding exists here and is ANSI-safe and grapheme-aware.
- **C20. `x/cellbuf` (api).** Cell buffer package. Last change 2026-08-11 (lint). Ultraviolet's README says it replaces the ad-hoc terminal primitives of earlier versions, so treat cellbuf as legacy. It stays an indirect v1 dependency only.
- **C21. `x/vt` (api).** Virtual terminal emulator (parses escape output into a screen), pushed 2026-10-12, used by Crush for interactive shells (commit message). Not published as a stable tag (only `vt/v0.1.0`). A way to assert on the final screen cells of our program in tests, instead of string compare of ANSI output. Not tried (gap).
- **C22. `x/exp/teatest` (api).** A `v2` module exists (last commit 2026-03-11, "update teatest v2 to use charm.land"). Runs a Bubble Tea program in a test with simulated input and output capture, plus `x/exp/golden` golden files. Its `go.mod` requires `bubbletea/v2 v2.0.0-rc.1`, a pre-release, so version selection would lift it to v2.1.1, compatibility untested (gap). Our 12.6k lines of tests call `Update` directly and are fast. Teatest adds whole-program tests and golden screens.
- **C23. `x/term`, `x/input`, `x/termios`, `x/conpty`, `x/windows`, `x/wcwidth`, `x/mosaic`, `x/editor` (api).** `x/term` is an indirect dependency now. Input, termios and conpty are what Bubble Tea v2 and ultraviolet use, no direct use for us. `x/editor` opens `$EDITOR`. `x/mosaic` renders an image as colored half-block cells, README says a fuller Bubble Tea image implementation is intended, "this package is one step in that direction". No kitty or sixel widget exists in Bubble Tea or Bubbles (code search of the bubbletea repo found only examples).

### Part 5. Other Charm products

- **C24. Wish v2 (api).** v2.0.5 (2026-10-01), MIT, 5.6k stars. SSH server middleware that serves Bubble Tea apps. With v2 renderer the README says bandwidth falls "by orders of magnitude". Relevant if the phone door moves from ttyd to SSH ([[research-terminal-images]] F4). Image protocols over SSH depend on the client terminal (gap).
- **C25. Gum (api).** v2.0.2 (2026-09-24), MIT, 24.5k stars. A CLI of shell-script TUI widgets (input, choose, filter, confirm, table, pager, spin, format). Not a Go library. Useful in shell scripts around `crm`, nothing for the framework.
- **C26. VHS (api).** v0.12.1 (2026-09-24), MIT, 21k stars. Records a terminal from a `.tape` script to GIF. README: "terminal GIFs as code for integration testing and demoing". Needs ttyd and ffmpeg (from install notes, not fully read). Windows support not checked (gap). Good for pattern vault demos and visual checks.
- **C27. Soft Serve (api).** Archived. README: "we've sunset Soft Serve... No further security updates." A release v0.12.4 is dated 2026-10-23 despite that. Skip.
- **C28. Others seen (api).** Fang v2.0.1 (cobra starter kit, styled help and errors, MIT, we use cobra, so this is a CLI-door option, not a TUI one). Crush (27k+ stars, license reported as NOASSERTION, the LICENSE file is FSL-1.1-MIT, source-available): the biggest app on this stack, worth reading for structure, not for copying. Colorprofile v0.4.3 (MIT, color depth detection, replaces termenv in the v2 stack). Mods and Charm (cloud) are archived.

### Part 6. Third-party libraries (search plus api)

- **C29. bubblezone (api).** `lrstanley/bubblezone` v2.0.0, MIT, 920 stars, pushed 2026-10-15, built on bubbletea v2 and lipgloss v2 (`go.mod`). Wraps strings in invisible zone markers, then maps a mouse event to the zone it hit. Covers inline spans that a rectangle layer cannot. Our `mouse.go` and `tabLayout` compute x ranges by hand and it works.
- **C30. Overlay and popup libraries (search plus api).** `rmhubbert/bubbletea-overlay` (MIT, 126 stars, v0.6.10 2026-10-12) targets Bubble Tea v1 (`go.mod` pins bubbletea v1.3.10; its description says "v1"). `floatpane/bubble-overlay` (3 stars), `sraaaaaaay/bubbletea-modal` (0), `blackwell-systems/bubbletea-commandpalette` (2) are tiny. On v2 the Lip Gloss compositor makes them unnecessary.
- **C31. No popup autocomplete library (search).** Searches for autocomplete, dropdown and command palette on bubbletea returned only tiny or unrelated repos. Inline ghost text exists in textinput and huh. Our popup (grouped rows, kind prefixes, context-ranked candidates) has no library equivalent found.
- **C32. Layout libraries (search plus api).** `winder/bubblelayout` (25 stars, 2026-07, MIT, `go 1.20`, declarative layout), `76creates/stickers` (402 stars, MIT, flexbox and table, v1.5.0 2025-09-28, mixed bubbletea v1.1.1 with lipgloss v2 in `go.mod`), `mieubrisse/teact` (52 stars, last push 2023), `termkit/skeleton` (65 stars, GPL-3.0, multi-tab). Study stickers' flexbox for the split and stack containers. Skip the rest.
- **C33. Tree, kanban, calendar, datepicker (search).** Tree: first-party bubble (C14), `savannahostrowski/tree-bubble` (32 stars, 2023). Kanban: no reusable component found, only apps (`gitsocial`, `lazymark` 99 stars MIT with a Kanban board, `cli_kanban`). Calendar: apps only (`samuelstranges/chronos` 39 stars MIT, a vim-like calendar TUI, `tui-calendar` GPL-3.0). Datepicker: `EthanEFung/bubble-datepicker` (45 stars, MIT, 2026-01, built on bubbles v0.16.1, so old). Board and calendar modules would be ours. The apps are code to read, not libraries. One search pass, so a missed library is possible.
- **C34. Charts (api).** `NimbleMarkets/ntcharts` v2 module (MIT, 812 stars, pushed 2026-10-28, deps bubbles v2.2.1, bubbletea v2.1.0, lipgloss v2.0.6): sparkline, bar, line, time series, heatmap charts for Bubble Tea. Fits the chart and stat modules in [[research-view-module-standard]] when they are needed.
- **C35. Images (search plus api).** `blacktop/go-termimg` (MIT, 68 stars, pushed 2026-05-11, `go 1.26`, depends on `x/cellbuf` and `x/mosaic`): kitty, sixel and halfblock rendering. [[research-terminal-images]] F5 cites `danielgatis/go-termimg`, which the API reports as not found (404), so that note names the wrong repo and should be corrected by its owner. `BakedSnake/goberzurg` (MIT, 0 stars). `thebanri/limoni` (Apache-2.0, 183 stars, pushed 2026-10-28, `go 1.25`) is a separate TUI engine, not Bubble Tea, that advertises tests which click and agents that drive the UI, with a WebAssembly build. I read only its README header (gap).
- **C36. Component kits and design systems (search).** `nitrictech/pearls`, `miniaturebase/pearls`, `harrisoncramer/boba`, `RemiG26/bubbles` are small personal libraries, mostly 2021 to 2024. `grafviktor/termview` (1 star) targets v2. No maintained kit or design system for Bubble Tea found. Nearest "design system" is the written one in [[research-tui-building-blocks]] B28 (monospace-design-tui, a spec, not Go code).

## Comparison table

Verdict: adopt = use as a dependency in the framework. study = read it and borrow ideas, or use later if a need appears. skip = do not use. Dates are last release or, where there is no release, last push (api, 2026-10-28).

| Piece | What | License | Last release or push | Maturity | Verdict |
|---|---|---|---|---|---|
| Bubble Tea v2 | Elm-style TUI loop, cursed renderer | MIT | v2.1.1, 2026-10-28 | Stable since 2026-02, 45k stars | adopt (C1 to C3) |
| Bubble Tea v1 | Same, old API | MIT | v1.3.10, 2025-09-17 | Frozen | skip after migration |
| Lip Gloss v2 | Styles, layout, Layer and Compositor, table, tree, list | MIT | v2.0.6, 2026-08-11 | Stable, 12k stars | adopt (C5) |
| Bubbles `textinput`, `viewport`, `spinner` | Inputs and scroll, in use | MIT | v2.2.1, 2026-08-24 | Stable | adopt (already used) |
| Bubbles `textarea` | Multi-line input | MIT | same | Stable | adopt for longtext |
| Bubbles `tree` | Tree view | MIT | pushed 2026-10-27, first commit 2026-08-21 | New | study, then adopt for tree module |
| Bubbles `list` | Item list, fuzzy filter | MIT | same | Stable | skip (C8) |
| Bubbles `table` | Read-only table | MIT | same | Stable | skip (C9) |
| Bubbles `help`, `key` | Binding type and help | MIT | same | Stable | study, adapter only (C12) |
| Bubbles `filepicker` | File picker | MIT | same | Stable | study, adopt if a file field appears |
| Bubbles `paginator`, `progress`, `timer`, `stopwatch`, `cursor` | Small parts | MIT | same | Stable | skip until needed |
| Huh v2 | Forms, validation, dynamic, accessible | MIT | v2.0.3, 2026-03-10 | Stable, 7k stars | study (wizards only, C15) |
| Glamour v2 | Markdown to ANSI | MIT | v2.0.1, 2026-06-12 | Stable, 3.7k stars | adopt for markdown module |
| Harmonica | Spring animation | MIT | v0.2.0, 2022 | Finished | skip |
| Log v2 | Logger | MIT | v2.0.1, 2026-09-03 | Stable | skip, file log if needed |
| x/ansi | ANSI width, cut, wrap, images | MIT | commit 2026-10-08 | Used by all of Charm, no stable tag | adopt |
| x/cellbuf | Old cell buffer | MIT | commit 2026-08-11 | Superseded by ultraviolet | skip |
| ultraviolet | Cell buffer, input, renderer | MIT | commit 2026-10-23, untagged | Powers v2 | skip direct use |
| x/vt | Virtual terminal emulator | MIT | commit 2026-10-12 | Experimental | study for screen tests |
| x/exp/teatest v2, x/exp/golden | Program test harness, golden files | MIT | commit 2026-03-11 | Experimental | study, adopt for end-to-end tests |
| x/mosaic | Half-block image render | MIT | commit 2026-07-13 | Experimental | study for the image fallback |
| Wish v2 | Serve a TUI over SSH | MIT | v2.0.5, 2026-10-01 | Stable, 5.6k stars | study (phone door) |
| Gum | Shell script widgets | MIT | v2.0.2, 2026-09-24 | Stable | skip for the framework |
| VHS | Terminal recorder, tape scripts | MIT | v0.12.1, 2026-09-24 | Stable, 21k stars | study (demos, visual checks) |
| Soft Serve | Git server | MIT | v0.12.4, 2026-10-23, repo archived | Sunset | skip |
| Fang | Cobra starter kit | MIT | v2.0.1, 2026-03-11 | Stable | skip (CLI door, out of scope) |
| bubblezone v2 | Mouse zones | MIT | v2.0.0, pushed 2026-10-15 | Active, 920 stars | study, adopt if inline click targets multiply |
| bubbletea-overlay | Overlay on v1 | MIT | v0.6.10, 2026-10-12 | v1 only | skip |
| bubble-table (Evertras) | Table with sort and filter | MIT | v0.23.0, 2026-09-09 | Active, 578 stars | study |
| stickers (76creates) | Flexbox and table | MIT | v1.5.0, 2025-09-28 | Slow | study |
| bubblelayout | Declarative layout | MIT | pushed 2026-07-28 | 25 stars, old Go base | skip |
| ntcharts v2 | Charts | MIT | pushed 2026-10-28 | Active, 812 stars | adopt later (chart module) |
| bubble-datepicker | Date picker | MIT | pushed 2026-01-11 | Built on old bubbles | skip |
| go-termimg (blacktop) | Kitty, sixel, half-block images | MIT | pushed 2026-05-11 | 68 stars | study (spike) |
| limoni | Separate TUI engine, test-first | Apache-2.0 | pushed 2026-10-28 | 183 stars, young | study ideas only |
| Crush | Large Bubble Tea v2 app | FSL-1.1-MIT | pushed 2026-10-28 | 28k stars | study structure only |
| Component kits, popup autocomplete libraries | none maintained found | | | | none |

## Pretext

Source: https://github.com/chenglou/pretext, read 2026-10-28 (README in full, `CHANGELOG.md` head, `package.json`, `LICENSE`, `RESEARCH.md` head, `src/layout.ts` head, grep of `src/measurement.ts`, file list with sizes).

- **What it is.** A TypeScript library for multi-line text measurement and layout in browsers. Two steps: `prepare(text, font)` segments the text at break opportunities and measures segments with Canvas `measureText`, then `layout(prepared, maxWidth, lineHeight)` is pure arithmetic returning `{ height, lineCount }`. Other calls return the lines (`layoutWithLines`, `layoutNextLine` for per-line widths such as text flowing around an image), shrink-wrap widths (`walkLineRanges`, `measureLineStats`) and mixed-font inline runs (`rich-inline`). Its purpose: get text height without reading the DOM, which forces a browser reflow. It predicts what Chrome, Safari and Firefox do, using per-engine break rules ported into the library (`line-breaks.ts`, `gecko-line-breaks.ts`, engine profiles in `measurement.ts`).
- **Language and runtime.** TypeScript, published to npm as `@chenglou/pretext` (v1.0.0, 2026-10-14, ESM, `sideEffects: false`), built with `bun` and `tsc`. Runtime needs Canvas 2D text measurement, Unicode property escapes and `Intl.Segmenter`. The README documents running outside a browser only by supplying a fake `OffscreenCanvas` with `measureText`, and says lines are then "exact where you paint those lines yourself (a terminal, a PDF)".
- **License.** MIT (`LICENSE` and `package.json`). The GitHub API reports NOASSERTION because the copyright line reads "Pretext contributors", not a license detection problem in the text.
- **Maturity.** Created 2026-03-07, 50.9k stars, 66 open issues, 2,762 forks, 30 commits visible on the first API page, one author with 2,340 contributions out of about 2,350 (the next has 2). 1.0.0 shipped a breaking API change (argument order of `layoutNextLine`) and the changelog already lists an unreleased fix. `RESEARCH.md` and `PLATFORM_BUGS.md` document browser differences at length. Fast moving, one maintainer, popular. The README states limits: no bidi ordering, default `word-spacing` and kerning only, `system-ui` unsafe on macOS, not a full font engine.
- **Does it apply to a Go TUI framework? No.** A terminal has no proportional fonts and no canvas. Width is a count of cells per grapheme, which Go already does (`x/ansi.StringWidth`, Lip Gloss `Width`, `rivo/uniseg`, `displaywidth`), and wrapping is `x/ansi.Wrap` and `Wordwrap`. Pretext's work (matching browser font metrics and engine break quirks) has no counterpart in a terminal. Running it from Go would mean embedding a JavaScript runtime and writing a fake canvas that returns cell widths, to reach a result `ansi.Wrap` gives directly. That is pure cost. Unicode line breaking rules (UAX 14, CJK, no-space scripts) are the only overlap, and Go has `uniseg`.
- **What it could apply to.** (a) A web renderer for the view-module standard ([[research-view-module-standard]], Q7 asks for a second renderer): height prediction for virtualized lists with wrapped rows, shrink-wrapped chat bubbles, no-reflow layout. That is a browser job, so only if a web door ([[research-terminal-images]] item 4) is built in TypeScript. (b) An idea to copy, not code: split work into a one-time `prepare` (segment and measure, cached) and a cheap `layout` for each width, and a cursor-based line iterator for variable-width lines. In a TUI this would matter only for a virtualized list of wrapped rows with thousands of items, which a CRM does not have.
- **Verdict.** Skip for the Go TUI framework. Study the prepare and layout split only if a wrapped-row virtual list shows a measured cost. Reconsider for a TypeScript web renderer. Not verified: its accuracy claims (the harness records browser output, I did not run it), and whether any terminal user of the fake-canvas route exists.

## Map to framework layers

Layers L1 to L9 are from [[research-tui-building-blocks]]. Modules are from the taxonomy in [[research-view-module-standard]].

| Layer or module | What the framework needs | Library to lean on | Keep ours |
|---|---|---|---|
| Loop, input, renderer | Event loop, keys, mouse, resize | Bubble Tea v2 (C1, C2) | |
| L1 Schema, L2 Query | Fields, relations, filters | none | all ours (`model.Fields`, `model.Relations`) |
| L3 View: list | Rows, marks, groups | none (bubbles list loses features, C8) | `listView`, `grouped.go` |
| L3 View: table | Typed columns, cell edit | Lip Gloss `table` for static rendering (study) | `cells.go`, inline edit |
| L3 View: detail | Record page rows | `viewport` v2 for scroll | rows, pages |
| L3 View: markdown | Render and scroll | Glamour v2 plus `viewport` v2 | link context |
| L3 View: tree | Expand and collapse | Bubbles `tree` (C14) | selection and verbs |
| L3 View: board, calendar | Group columns, date grid | none found (C33) | ours, read `lazymark` and `chronos` |
| L3 View: chart, stat | Spark and bar | ntcharts v2 (C34) | |
| Containers (split, stack, tabs) | Nesting, layout | Lip Gloss `JoinHorizontal`, `Layer`, stickers flexbox ideas | tab bar, focus path |
| Command bar | Palette and bar as a view | `textinput` | `bar.go`, `overlay.go` rows |
| L4 Verbs | Action registry | none | ours |
| L5 Keymap | Scopes, layers, overrides | `key.Binding` as an output adapter only | `keymap.go`, `keys` |
| L6 Theme | Roles | Lip Gloss v2 `LightDark`, colorprofile | roles file |
| L8 Widgets: field editors | Typed inline editors | `textinput`, `textarea`, `filepicker`, huh for wizards | `fieldedit.go`, registry |
| L8 Widgets: autocomplete | Popup, ranker | `textinput` ghost text is a subset | `autocomplete.go`, `suggest` |
| L8 Widgets: overlay and popup | Float over content, hit test | Lip Gloss v2 `Layer` and `Compositor` (C5) | content of the popups |
| L9 Help | Keys shown from registry | `help` exists, ours is registry-driven | `footerView`, `helpView` |
| Text width and clipping | Cell widths | `x/ansi`, Lip Gloss | |
| Images | Logos and photos | `x/ansi` encoders, `x/mosaic`, go-termimg (C35) | detection, cache, fallback |
| Testing | Unit, whole program, screen | `teatest` v2, `x/exp/golden`, `x/vt`, VHS | existing `Update` tests |
| Remote | Phone or browser | ttyd today, Wish (C24) | |

## Hand-rolled vs library

Cost scale: S under a day, M one to three days, L over a week. Risk is the chance of a visible regression or lost feature. "Duplicated" means a library or another package of ours already provides it ([[one-abstraction-many-contexts]]).

| # | Hand-rolled | Where | Library or package that provides it | Cost | Risk | Notes |
|---|---|---|---|---|---|---|
| H1 | Width, truncate and pad with `go-runewidth` plus `fit`, `cutTo`, `cleanCell` | 17 files, about 60 calls, `view.go` | `x/ansi` `StringWidth`, `Truncate`, `Cut` (already a dependency), Lip Gloss `Width` | S | Low to medium | Not ANSI-aware today. `cleanCell` strips emoji only to keep widths right, grapheme-aware width may make that unnecessary. Check on Windows Terminal (gap). Works on v1 already. |
| H2 | `fuzzyScore` in `overlay.go` | `overlay.go:80`, 22 lines | `internal/suggest.Score` and `Rank` (ours), `sahilm/fuzzy` (third party) | S | Low | This duplicates our own ranker, the one [[autocomplete-component]] says is the only matcher. Palette and picker ranking differ from the typeahead (no nicknames, no typo). Consolidate inside the repo, no new dependency. |
| H3 | Popup, palette and toast drawn by string concatenation. Palette and picker replace the whole screen, the toast overwrites the last line | `overlay.go` 602, `toast.go` 64, `autocomplete.go` | Lip Gloss v2 `Layer`, `Compositor`, `Canvas` | M | Low to medium | Gives real floating popups and z-order. Needs the v2 migration first. |
| H4 | Mouse hit mapping by x ranges (`tabSeg`, row index math) | `mouse.go` 142, `view.go` tab layout | `Compositor.Hit` for rectangles, bubblezone v2 for inline spans | S | Low | Low value now, grows with click targets in new modules. |
| H5 | List, row and column rendering | `view.go` `listView`, `cells.go`, `grouped.go` | Bubbles `list`, `table`, Evertras `bubble-table` | L | High | Do not migrate. Bubbles list and table lack marks, bulk selection, group dividers and cell edit (C8, C9). |
| H6 | Keymap with scopes, layers and overrides | `keymap.go`, `keybind.go`, `keys` (about 940 lines) | `key.Binding` and `help` (no scopes, no rebinding) | S to add an adapter, L to replace | High to replace | Keep. Add an adapter so Bubbles components get our keys. |
| H7 | Help and footer from the action registry | `view.go` footer and help | Bubbles `help` | M | Medium | Ours draws click targets and the registry, keep. |
| H8 | Inline field editors and link cells | `fieldedit.go`, `cells.go`, `form.go` | Huh v2 fields | L | High | Different model (C15), would duplicate [[one-validation-framework]]. Keep. |
| H9 | Longtext editing as a one-line input with `alt+enter` | `fieldedit.go` | Bubbles `textarea` | M | Low to medium | Gains real multi-line cursor and wrap. |
| H10 | Autocomplete popup and ranker | `autocomplete.go`, `pipecomplete.go`, `typeahead.go`, `suggest` (about 900 lines) | textinput suggestions (one ghost completion), huh `Suggestions` | n/a | n/a | No library equivalent (C31). Keep. |
| H11 | Detail and help scrolling | `viewport` in `model.go` | already `viewport` | n/a | n/a | v2 adds soft wrap, gutter, highlight, no hand work needed. |
| H12 | Theme roles and profile handling | `internal/theme` 690, `termenv` in 3 places | Lip Gloss v2 `LightDark` and `compat`, `colorprofile` | M (only as part of v2) | Medium | Roles stay. `AdaptiveColor` and renderer color profile calls change in v2 (upgrade guide, `lipgloss.SetColorProfile` is used in tests). Drop `termenv`. |
| H13 | Screen-level test assertions | `Update`-level tests, 12.6k lines | `teatest` v2, `x/vt`, `x/exp/golden`, VHS | S to M | Low | Adds, does not replace. |
| H14 | Images | not built (M16) | `x/ansi` kitty, sixel, iterm2 encoders, `x/mosaic`, go-termimg | M | Medium | Library covers encoding and fallback. Coexistence of image escapes with the v2 diff renderer is gap, needs the spike in [[research-terminal-images]] A2. |
| H15 | Markdown rendering | not built | Glamour v2 | S | Low | Greenfield. |

## Recommended

Ranked by value over cost. Not a decision.

1. **A1. Consolidate width and truncation on `x/ansi` (H1).** Cost S, works on v1, removes a second width implementation and an ANSI-unsafe one. Do first because v2 and Lip Gloss v2 measure with the same grapheme rules, so the code will already agree with them.
2. **A2. Point the palette ranker at `internal/suggest` (H2).** Cost S, no dependency, closes an in-repo duplicate.
3. **A3. Migrate to Bubble Tea, Bubbles and Lip Gloss v2 as one change, before building more view modules (C1 to C5).** Cost M, risk medium. Every new module written on v1 adds to the migration, v1 has had no release since 2025-09, and the next items need v2. Start with a branch spike on Windows Terminal: keys (`space`), mouse, alt screen, resize, color profile, emoji widths, and a ttyd run. Raise the Go floor to 1.26.7 only after checking the deploy hosts (Q1).
4. **A4. Add Glamour v2 for the markdown module (H15).** Cost S after A3, greenfield, MIT, same stack.
5. **A5. Add a Lip Gloss v2 compositor layer for popups and hit testing (H3, H4).** Cost M after A3. It lets the autocomplete popup, palette, toast and later nested modules float over content in one mechanism ([[one-abstraction-many-contexts]]).
6. **A6. Add screen-level tests: teatest v2 plus golden files for a few whole-program flows, `x/vt` if cell assertions are needed.** Cost S to M. Keep the existing `Update` tests.
7. **A7. Use Bubbles `tree` for the tree module, `textarea` for longtext, `viewport` v2 for markdown and detail.** Cost S each after A3. Wrap them behind the module interface so a bubble stays replaceable ([[extension-ladder]]).
8. **A8. Image spike (H14).** Cost M. Use `x/ansi` encoders (already a dependency) and `x/mosaic` for the half-block fallback, compare go-termimg. Fix the repo name in [[research-terminal-images]] F5.
9. **A9. Add bubblezone only when inline click targets multiply, ntcharts when a chart or stat module is built, Wish if the phone door leaves ttyd.** Each is a separate small decision.
10. **Do not adopt:** Bubbles `list` and `table` (feature loss), Huh as the editor model (second vocabulary), bubbletea-overlay (v1 only), component kits (none maintained), Pretext (browser-only), Soft Serve (sunset), Gum and Log (no framework role).

Framework design rule that follows: keep the module and editor interfaces free of Charm types (`tea.Msg`, `lipgloss.Style`) at the boundary, so the renderer (Bubble Tea v2) is one adapter. [[research-view-module-standard]] already assumes a second renderer, and it keeps a library swap or a web renderer possible.

## Could not verify

- Everything behavioral. No library was built or run. Claims about Bubbles list, table and tree features come from README text and file or function names, not from a feature test. "No multi-select, no dividers in Bubbles list" is from a skim, not a source audit.
- Bubble Tea v2 on Windows Terminal, ttyd and tmux. Key string changes in our key normalization (`keys.go`). Rendering differences of the cursed renderer. Emoji and ZWJ widths against `cleanCell`.
- Go toolchain versions on pibrain, the OpenBSD VPS, grant-linux-pc and any CI, against the 1.26.7 floor.
- Whether `lipgloss.Compositor.Hit` handles non-rectangular or inline hit targets. Whether image escape sequences survive the ultraviolet diff renderer.
- Compatibility of `teatest/v2` (pinned to `bubbletea v2.0.0-rc.1`) with v2.1.1. Whether `x/vt` can host a full Bubble Tea program in a test.
- VHS on Windows, and its exact install requirements.
- Maturity of the Bubbles `tree` (2 months old). Quality and stability of limoni, bubble-table, stickers, go-termimg (only READMEs, `go.mod` and metadata read).
- Licenses were read from the API license field, plus the LICENSE text for Pretext and Crush. NOASSERTION cases were resolved by reading the file.
- Absence claims (no popup autocomplete library, no kanban or calendar component, no maintained kit) rest on one GitHub search pass with the first six results per query.
- Pretext: accuracy claims, its test harness, and any real non-browser use. I read the README and a skim of the source (layout header, measurement grep), not the 68 KB line-break code.
- Gum, Wish, Log, Harmonica, Fang, Crush: descriptions and metadata only, no source read.
- Star counts and dates are one-day snapshots.

## Open questions for the owner

- Q1. Which machines must build the framework and the apps? The v2 stack needs Go 1.26.7. Check pibrain, the VPS and CI before A3.
- Q2. Is the framework Bubble Tea only, or must the module interfaces stay free of Charm types for a second renderer (web, or another language)? The answer decides how much of A5 and A7 sits behind an adapter.
- Q3. Should the framework live in the `crm-cli` module or its own Go module? A separate module needs its own go.mod and makes the v2 migration a framework-only change first.
- Q4. Is a web door planned? If yes, Pretext becomes relevant for that renderer. If no, close it.
- Q5. Is the target terminal set limited to Windows Terminal plus ttyd in a browser? That decides the image protocol choice and how much v2 keyboard enhancement (shift+enter, release events) can be relied on.
- Q6. Do you want huh for one-shot wizards (first run, import mapping) even though it is a second field vocabulary, or should wizards also be built from `model.Fields`?
- Q7. Do you accept pre-release or untagged dependencies (ultraviolet, `x/vt`, `x/exp/teatest`) in test and tooling code only?

## Sources
- Bubble Tea: https://github.com/charmbracelet/bubbletea, https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md, release v2.0.0 notes (via API)
- Bubbles: https://github.com/charmbracelet/bubbles, https://github.com/charmbracelet/bubbles/blob/main/UPGRADE_GUIDE_V2.md
- Lip Gloss: https://github.com/charmbracelet/lipgloss (`layer.go`, `canvas.go`, `go.mod`)
- Ultraviolet: https://github.com/charmbracelet/ultraviolet
- Huh: https://github.com/charmbracelet/huh
- Glamour: https://github.com/charmbracelet/glamour
- Harmonica: https://github.com/charmbracelet/harmonica
- Log: https://github.com/charmbracelet/log
- charmbracelet/x: https://github.com/charmbracelet/x (ansi, cellbuf, vt, term, exp/teatest, exp/golden, mosaic)
- Wish: https://github.com/charmbracelet/wish
- Gum: https://github.com/charmbracelet/gum
- VHS: https://github.com/charmbracelet/vhs
- Soft Serve: https://github.com/charmbracelet/soft-serve
- Fang: https://github.com/charmbracelet/fang
- Crush: https://github.com/charmbracelet/crush
- Colorprofile: https://github.com/charmbracelet/colorprofile
- bubblezone: https://github.com/lrstanley/bubblezone
- bubbletea-overlay: https://github.com/rmhubbert/bubbletea-overlay
- bubble-table: https://github.com/Evertras/bubble-table
- stickers: https://github.com/76creates/stickers
- bubblelayout: https://github.com/winder/bubblelayout
- ntcharts: https://github.com/NimbleMarkets/ntcharts
- bubble-datepicker: https://github.com/EthanEFung/bubble-datepicker
- go-termimg: https://github.com/blacktop/go-termimg
- goberzurg: https://github.com/BakedSnake/goberzurg
- limoni: https://github.com/thebanri/limoni
- sahilm/fuzzy: https://github.com/sahilm/fuzzy
- lazymark: https://github.com/MathiasDrizzy/lazymark, chronos: https://github.com/samuelstranges/chronos
- Pretext: https://github.com/chenglou/pretext (README, CHANGELOG.md, RESEARCH.md, src/layout.ts, src/measurement.ts, LICENSE, package.json)
- Our code: `crm-cli/go.mod`, `crm-cli/internal/tui`, `internal/theme`, `internal/keys`, `internal/suggest`, `internal/modules`
