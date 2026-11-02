---
type: reference
status: decided by owner 2026-10-30
---

# The user chooses the highlight color of each kind

**Problem.** Kind colors came only from the theme roles, so a user could not make deals stand out or tell two kinds apart on their terminal.

**Therefore.** Settings > Colors has one row per registered kind (person, organization, deal, task, note, tag, view) and a reset. A row opens a picker of the theme palette names (accent, good, warn, danger, mark, fg, dim, tab), Custom hex and Reset. Moving the cursor previews the color live, esc restores it. A hex color is checked by `validate.HexColor` (`E_COLOR_INVALID`) and previews while typing. Stored per device in `config.toml`:

```toml
[colors]
person = "accent"   # a palette name follows the theme
deal = "#e8a33d"
```

`resolveKindColor` is the one function that decides a kind color: the override, else the theme role ([[input-grammar]]). The styles are patched with it, so the related segment, popups, chips, tag page headings and record page headings all read it through `Styles.KindStyle`. Without color support nothing changes, the short kind prefixes (p:, o:, #) stay.

**Because.** Owner request 2026-10-30.

**Related.** [[features-tui-keys-and-modules]], [[core]]

**Sources.** `internal/tui/kindcolors.go`
