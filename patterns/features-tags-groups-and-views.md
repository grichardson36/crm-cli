---
type: feature-area
---
# Tags, groups, saved views and search

Tags, groups, saved views, the filter language, sheet and board views, bulk edit and search. The command bar (B14) is in [[features-tui-keys-and-modules]].

Moved from FEATURES.md on 2026-10-29. Every row keeps its ID and text. Update a row's State and Now in the same session you change the feature ([[keep-the-tracker-accurate]]). Column meanings and the row lifecycle are in [[feature-rules]].

**Follows patterns:** [[hash-means-tag]], [[bulk-actions-always]], [[one-bar]], [[bar-scope-prefixes]], [[modules-are-user-arranged]], [[one-abstraction-many-contexts]]

**Research:** [[research-view-module-standard]]

**See also:** [[index]]

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| B01 | Tags: add/remove, global edit, filter, click to list (docs) | Dex | DONE | TUI `T` adds/removes tags (single or marked); CLI and MCP. | - | TUI `T` add/remove on selected, tag picker with search | fast | P1 |
| B02 | Tags settings page (rename, delete) (docs) | Dex | PART | `tag` CLI | - | TUI Tags view with counts, `r` rename, `d` delete | simple | P2 |
| B03 | Groups, hierarchical, many per contact, emoji, counts (docs, REA) | Dex | TODO | Dex groups were imported as tags (G6) | Dex groups were imported as tags. Add an import marker so they can be converted to real groups? | Migration: `groups(id,parent_id,name,emoji)` + `group_people`; CLI `group ...`; TUI `G` Browse Groups tree | extensible | P2 |
| B04 | Browse groups, shortcut G then D (docs) | Dex | TODO | none | - | TUI Groups view | - | P2 |
| B05 | Organize with AI / Autotag (docs) | Claude agents | BEYOND | Agent can tag via MCP | - | Document a skill recipe. No built-in AI | smart | P3 |
| B06 | Saved views (filter tabs, favorite, rename, reorder) (docs, REA) | Dex | TODO | none | Query language syntax for saved views (see B07). | `views(name, query, sort, columns)`; CLI `view ...`; TUI tab per view, `V` save current filter | fast, extensible | P1 |
| B07 | Filter set: name, title, company, description, education, location, has phone, has email, groups, tags, favorited, archived, cadence, last interaction, creation date, priority, custom fields (docs) | Dex | PART | Search is text + `#tag`; `person list` has some flags | - | One query language shared by CLI `--where`, TUI `/`, saved views. Accent-insensitive. DEFAULT: grammar `field:value`, `#tag`, `field:>date`, quoted phrases, bare words = name/notes search. One parser in `internal/query` used by CLI `--where`, TUI `/`, saved views. | extensible, fast | P1 |
| B08 | Map-location filter (distance from point) (docs) | Dex | SKIP | none | - | Needs geocoding. See C11 | - |  |
| B09 | Sheet view with selectable columns, hide columns, sort by last interaction (docs) | Dex | PART | One list with `o` sort | Which columns by default? | TUI column picker `v`; sort keys: name, last contact, created | tactile | P2 |
| B10 | Board view by category custom field (docs) | Dex | TODO | none | Which Category field defaults the board? Cadence? | TUI Board view (columns = values of a Category field); also covers D04 and A12 board | tactile | P2 |
| B11 | Bulk edit: groups, tags, cadence, archive, priority (docs) | Dex | DONE | TUI space/`*` marks; T, F, p, f apply to marked rows. | - | TUI `space` marks rows, `*` all; then T, F, #, p apply to marked. CLI accepts many ids. DEFAULT: marks clear when the view changes or after an action is applied. | fast, tactile | P1 |
| B12 | Pin contacts/groups/views to sidebar (docs) | Dex | TODO | `f` favorite only | - | TUI Favorites tab (favorites + pinned views) | - | P3 |
| B13 | Search accent-insensitive; "k" shortcut (docs) | Dex | PART | FTS5 search; TUI `/` | - | Fold diacritics on search and on `#tag` | smart | P2 |
| G07 | Tag management in the TUI | Dex | PART | BRANCH bar-rank. Rename and merge built once: `TagRepo.Rename` (refuses a name that exists in any case and offers a merge), `Merge` / `MergeDryRun` / `UndoMerge` / `RedoMerge` (moves taggings, counts duplicates once, removes the source with a tombstone). CLI `crm tag rename`, `crm tag merge` (preview unless `--apply`), MCP `crm_tag_rename`, `crm_tag_merge` (`dry_run`). TUI registry actions "Rename tag" and "Merge tag into another" in the palette (they ask which tag) and on a tag row in the bar with keys `r` and `m`, one undo step each. Delete already existed | - | A Tags view with counts | Tags view with counts | simple | P2 |
