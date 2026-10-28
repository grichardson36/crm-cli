---
type: research
---
# Research: images in the terminal interface (FEATURES M16)

Date 2026-10-20. Question from the owner: can the TUI show logos and profile photos, or must we change the TUI framework or leave the terminal? Evidence tags: search (web search summaries, first-party pages not yet read) and unverified.

## Findings

- F1. **Three image protocols exist:** Sixel, the Kitty graphics protocol and iTerm2 inline images. Terminals support different ones. (search)
- F2. **Windows Terminal supports Sixel since 1.22** (Preview in August 2024, stable later; sources disagree on the first stable version, check the release page). Sources: the Windows Terminal release notes and Microsoft's devblog. It does not document the Kitty protocol in what was found. (search)
- F3. **VS Code's integrated terminal shows Sixel and iTerm2 images** through the xterm.js image addon, behind a setting (`terminal.integrated.enableImages`, since v1.80). (search)
- F4. **ttyd (our phone and browser terminal on pibrain) is built on xterm.js.** One third-party list marks it Sixel capable as "reported". No ttyd documentation or release note confirmed that the image addon is loaded. (unverified, needs a test)
- F5. **Bubble Tea has no image widget.** Go libraries write image escape sequences directly: go-termimg (Kitty and Sixel), goberzurg (pure Go, advertises Bubble Tea compatibility, requires a recent Go version), streakboard's tui package (Kitty Unicode placeholders as Bubble Tea component). Maintenance and version fit not checked. (search)
- F6. **Other stacks have more built in.** ratatui-image (Rust) supports Sixel, Kitty, iTerm2 and a halfblocks fallback with protocol detection and font-size query. textual-image (Python, Textual) supports Kitty and Sixel with a Unicode fallback. Neither is a reason alone to rewrite a Go program. (search)
- F7. **The fallback is universal:** half-block characters in color, or initials on a colored block, work in any terminal that has color.

## What this means for us

1. **Staying on Bubble Tea is viable.** Images are an optional layer: detect the protocol, draw through a small Go library, and fall back to initials or half-blocks. The logo path column already exists (M15 `logo_path`).
2. **The risk is mixing graphics with a redrawing TUI** (flicker, clearing, scrolling over an image, tmux passthrough). That needs a spike on Windows Terminal before committing.
3. **Phone and browser via ttyd** may or may not show images. Test before promising.
4. **The cleaner long-term door for rich media is a web door** over the same core (three doors, one core: CLI, MCP, TUI, plus HTML). That avoids replacing the TUI and gives phones real images, but it is a large new door.
5. **No framework move is recommended.** Rewriting in Rust or Python would discard tested Go code for a feature that has a fallback.

## Proposed next steps (owner to confirm)

- A1. Check the owner's terminals and versions (Windows Terminal version, phone terminal).
- A2. Spike: a 100-line Bubble Tea program drawing a logo with go-termimg and goberzurg in Windows Terminal, scrolling and resizing. Measure flicker.
- A3. Spike: the same through ttyd on pibrain from a phone.
- A4. If the spike is clean, add `internal/imageview` with detection, cache of resized logos, initials fallback, and show it in the org and person page headers.
- A5. Decide on a web door only if A2 or A3 fail.

## Sources
- Windows Terminal Preview 1.22 release post: https://devblogs.microsoft.com/commandline/windows-terminal-preview-1-22-release
- Microsoft Terminal releases: https://github.com/Microsoft/console/releases/
- go-termimg: https://pkg.go.dev/github.com/blacktop/go-termimg
- goberzurg: https://pkg.go.dev/github.com/bakedsnake/goberzurg
- ratatui-image: https://docs.rs/ratatui-image
- textual-image: https://pypi.org/project/textual-image
