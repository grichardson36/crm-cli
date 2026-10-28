# Bubble Tea v2 migration notes

Branch `feat/bubbletea-v2`. Modules: `charm.land/bubbletea/v2` v2.1.1, `charm.land/bubbles/v2` v2.2.1, `charm.land/lipgloss/v2` v2.0.6, `github.com/charmbracelet/x/ansi` v0.11.9, `github.com/charmbracelet/colorprofile` v0.4.3 (direct now, replaces termenv). `go` directive 1.24.0 to 1.26.0 (bubbletea v2 requires 1.26.0, bubbles and lipgloss need less, the research note said 1.26.7 for lipgloss, `go list -m` says 1.25).

## API changes made

1. Import paths: `github.com/charmbracelet/{bubbletea,bubbles,lipgloss}` to `charm.land/{bubbletea,bubbles,lipgloss}/v2`.
2. `View() string` is now `View() tea.View` (`toast.go`). The old string builder is `screen()`. `AltScreen = true` and `MouseMode = tea.MouseModeCellMotion` are view fields. `tea.WithAltScreen()` and `tea.WithMouseCellMotion()` are gone.
3. `tui.Run` takes `noColor bool`. `--no-color` becomes `tea.WithColorProfile(colorprofile.Ascii)`. The old `lipgloss.SetColorProfile(termenv.Ascii)` is gone.
4. Keys: `tea.KeyMsg` to `tea.KeyPressMsg` (20 non-test places). `msg.String()` to `keyName(msg)` (`internal/tui/keyname.go`, the single translation layer). `msg.Runes` and `msg.Type == KeyRunes` to `msg.Text`. Key releases fall through `Update` and do nothing.
5. Mouse: `tea.MouseMsg` split into `MouseClickMsg`, `MouseWheelMsg`, `MouseReleaseMsg`, `MouseMotionMsg`. `Update` turns clicks and wheel steps into a small `mouseEvent{X, Y, Button}`. Release and motion do nothing (same as v1, which ignored non-press). Constants `MouseButtonLeft/WheelUp/WheelDown` to `MouseLeft/MouseWheelUp/MouseWheelDown`.
6. Paste: v1 sent paste as a key message. v2 sends `tea.PasteMsg`. New `handlePaste` sends it to the focused footer prompt, row editor or overlay filter. Before this, no code looked at paste, it worked because the text inputs handled the key message.
7. Colors: Lip Gloss v2 has no `Renderer`, no `ColorProfile()`, no `SetColorProfile`. Styles always carry the exact color and Bubble Tea downsamples when it writes. `theme.NewStyles(t, colorprofile.Profile)` replaces `NewStyles(t, *Renderer)`. Profile `Ascii` or lower gives the attribute-only styles (reverse, bold, faint). The model learns the profile from `tea.ColorProfileMsg` (new field `Model.profile`, default TrueColor). `lipgloss.Color` is now a function, so `Color(string)` is used inline.
8. Dark background: `lipgloss.HasDarkBackground()` (sync) to `tea.RequestBackgroundColor` in `Init` and `tea.BackgroundColorMsg` (re-resolves the auto theme). The CLI `theme` commands call `lipgloss.HasDarkBackground(os.Stdin, os.Stdout)`.
9. `crm theme show` swatches print through `colorprofile.NewWriter`, which drops colors when stdout is not a color terminal.
10. Bubbles: `viewport.New(w, h)` to `viewport.New(WithWidth, WithHeight)`, `Width`/`Height`/`YOffset` fields to `Width()`/`SetWidth()`/`Height()`/`SetHeight()`/`YOffset()`. `LineUp/LineDown/ViewUp/ViewDown/HalfViewUp/HalfViewDown` to `ScrollUp/ScrollDown/PageUp/PageDown/HalfPageUp/HalfPageDown`. `textinput.Width` to `SetWidth`. `newTextInput()` resets the v2 default prompt, blurred text and cursor colors (v2 colors them, v1 did not, and all our colors come from the theme).
11. Cleanup A: all `runewidth` calls (35 `StringWidth`, 36 `Truncate`, 2 `FillLeft`, 1 `RuneWidth`) to `ansi.StringWidth`/`ansi.Truncate`, plus a 5-line `padLeft` in `view.go`. `go-runewidth` is no longer imported. It stays in `go.sum`/`go.mod` as an indirect dependency of `jedib0t/go-pretty`.
12. Cleanup B: `fuzzyScore` moved to `suggest.Fuzzy` (with `suggest.FuzzyWeak`), used by the palette, pickers and help search. It is the same algorithm. `suggest.Score` and `Rank` cannot replace it: they have no matching of letters in order, and the palette marks "weak" matches (rank 30 or more) that way. Behavior is identical, there is one matching package.
13. Tests: one key helper `keyPress(name)` in `keyhelpers_test.go` replaced `specialKeys` and three copies of the key-building code. `m.View()` in tests is `m.screen()`. `useProfile` is gone: styles are always colored, the no-color tests set `m.profile = colorprofile.Ascii`.

## Key names that changed

None of the registry vocabulary changed. `keyName` makes v2 names match v1:

| v2 reports | registry name |
|---|---|
| `space` (no modifier) | `" "` |
| `ctrl+alt+up` | `alt+ctrl+up` |
| `alt+shift+d` | `alt+D` |

Same in both: `enter`, `esc`, `tab`, `shift+tab`, `alt+enter`, `pgup`, `pgdown`, `home`, `end`, `ctrl+up`, `ctrl+down`, `ctrl+left`, `ctrl+right`, `ctrl+shift+up`, `ctrl+shift+down`, `shift+up`, `shift+down`, `ctrl+<letter>`, `alt+<letter>`, function keys.

New reachable name: `ctrl+shift+<letter>`. v1 folded it into `ctrl+<letter>`, v2 can tell them apart where the terminal reports modifiers. `copy-vcard` (`ctrl+shift+c`) was dead on most terminals, it can fire now.

## What the tests show about the Windows path

`keywire_test.go` feeds real byte sequences to the v2 decoder (`ultraviolet.EventDecoder`, the one Bubble Tea reads the terminal with) and checks the key names:

- xterm style sequences (Windows Terminal in VT mode, ssh, tmux): `ESC[1;5A..D` is `ctrl+up/down/right/left`, `ESC[1;6A/B` is `ctrl+shift+up/down`, `ESC[Z` is `shift+tab`, `ESC CR` and `ESC[13;3u` are `alt+enter`, `ESC[6~` is `pgdown`, `ESC[H` is `home`.
- Windows key records (win32 input mode, `ESC[vk;sc;uc;kd;cs;rc_`): ctrl+shift+up/down, ctrl+up/right/left, alt+enter, shift+tab, pgdown, home, space, letters all give the registry names.

Not tested: a live Windows Terminal session. No interactive terminal was available.

## Behavior differences observed

- B1. `ctrl+shift+<letter>` is distinct from `ctrl+<letter>` (above).
- B2. Selected-row, tab and other styles are baked into the detail pane text when it is drawn. In v1 the global profile cut colors at draw time, so a profile change took effect on cached text. In v2 the terminal output stage cuts colors, so `NO_COLOR` is still right on screen, but a profile change to Ascii mid-session leaves attribute-only styles for new draws only (the output stage strips the colors of old text anyway). One test now calls `syncDetail()` after `setTheme`.
- B3. Textinput prompt, text and cursor use plain styles (matching v1) through `newTextInput`. The placeholder color is still the library's gray 240 as in v1.
- B4. The text cursor in inputs is still the virtual reverse-video cursor (v2 default `VirtualCursor` true).
- B5. The color profile arrives after the first frame (message). The first frame uses truecolor styles, then switches. Under NO_COLOR the first frame is already stripped by the output stage, only the reverse-video fallback for the selection arrives one message later.
- B6. Auto theme: the terminal answers the background query asynchronously. The first frame uses dark, then the auto theme re-resolves. v1 asked synchronously before the program started.
- B7. `ESC` handling and bracketed paste are done by the v2 input reader. Paste now arrives as one message instead of many keys.

## Checks

Covered by tests: ctrl+shift and ctrl arrows (wire and key names), alt+enter (editor test plus wire), paste (`TestPasteGoesToTheFocusedInput`), mouse wheel and click (existing tests converted, release and motion ignored), grayscale hint (existing `TestHintIsRenderedWithTheDimStyle`), autocomplete popup (existing tests), wide characters (`TestWideCharactersFitTheirColumns`), NO_COLOR and profile (`TestColorProfileMessageSwitchesToAttributeStyles`, theme tests with a colorprofile writer), resize (existing `WindowSizeMsg` tests), alt-screen and mouse mode (`TestViewAsksForAltScreenAndCellMotionMouse`). The theme nohardcode test passes.

Not verified here: a live terminal run of the TUI, `-race` (no C compiler on this machine), pibrain's Go toolchain.

## Build and deploy

- `.github/workflows/ci.yml` and `release.yml`: `go-version` 1.24 to 1.26. `INSTALL.md`, `README.md`, `CLAUDE.md` say Go 1.26.
- `.goreleaser.yml` pins no Go version and builds with `CGO_ENABLED=0`, the arm64 builds come out of the runner toolchain. No change.
- Raspberry Pi arm64: cross-build with `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./cmd/crm` from a Go 1.26 or newer toolchain. Building on pibrain itself needs Go 1.26 there (Debian packages are older, use the go.dev tarball or `GOTOOLCHAIN=auto`, which downloads the toolchain named in `go.mod`). Not edited, outside the repo: `deploy/crm-backup.sh`, `crm-dav.service`, `crm-web.service` and `publish-snapshot.sh` pin no Go version and need no change. `deploy/dist/crm-linux-arm64` is a prebuilt binary, rebuild it after merge.

## Vault notes to update (after the vault move merges)

- `research-charm-ecosystem.md` C3: the code surface was 20 `tea.KeyMsg` places in non-test files and 21 in tests as estimated. The theme package needed a real redesign (no `Renderer`), not just a search and replace. C4: lipgloss v2.0.6 and bubbles v2.2.1 need Go 1.25, bubbletea v2.1.1 needs 1.26.0. The floor is 1.26.0, not 1.26.7.
- C22 (teatest): `keywire_test.go` now tests the decoder directly with `ultraviolet.EventDecoder`, a lighter option than teatest for key and mouse names. `ultraviolet` is a direct dependency now (tests only).
- C5/H3: the compositor was not adopted, `overlay.go` and `toast.go` are unchanged except `toast.go` holds `View()`.
- H5/H8: Bubbles list/table and Huh not adopted, as decided.
- Pattern notes on keys (`keymap-layers-and-scopes`, key audit): the vocabulary is the v1 one, `keyName` is the translation layer. Add: `ctrl+shift+<letter>` is a new distinct name.
- Pattern note on the theme (roles file): styles carry exact colors, downsampling is the output stage. `NewStyles(theme, colorprofile.Profile)`.
- `one-abstraction-many-contexts`: `suggest.Fuzzy` is now the single free-text matcher next to `suggest.Rank` for names.
