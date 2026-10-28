---
type: feature-area
---
# People and contact details

Person fields, labels, favorites, priority tier and the person record. Archive and merge live in [[features-merge-archive-and-undo]], links in [[features-links-and-relations]].

Moved from FEATURES.md on 2026-10-29. Every row keeps its ID and text. Update a row's State and Now in the same session you change the feature ([[keep-the-tracker-accurate]]). Column meanings and the row lifecycle are in [[feature-rules]].

**Follows patterns:** [[core-objects-carry-native-fields]], [[field-types-drive-editors]], [[one-record-page]], [[person-workspace-is-the-center]], [[roles-are-free-text]], [[soft-delete-restorable]], [[sync-safe-data]]

**Research:** [[research-sync-standard]], [[research-relationship-models]]

**See also:** [[roadmap]], [[index]]

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| A01 | Name, job title, company, description (REA, csv) | Dex | DONE | TUI detail pane is an editable form: Enter edits the row inline (commit 4e19422). | - | TUI: edit row in detail pane (Enter). DEFAULT (confirm if wrong): inline one-line text box on the selected row. DECIDED: right pane is the form; inline box on the row. New contact opens the pane with empty rows to fill in quickly. | fast, tactile | P1 |
| A02 | Multiple emails with labels (REA) | Dex | DONE | TUI add/edit/delete email rows; CLI and MCP. | - | TUI: add `a`, delete `d`, edit Enter on email rows | fast | P1 |
| A03 | Multiple phones with country code (REA) | Dex | PART | TUI add/edit/delete phone rows; no E.164 normalization yet. | - | TUI rows as A02. Normalize to E.164 at write | smart | P1 |
| A04 | Social links: linkedin, facebook, instagram, twitter, telegram, whatsapp, tiktok, youtube, website (REA) | Dex | DONE | TUI add/edit/delete/open handle rows; CLI and MCP. | - | TUI rows as A02; `o` opens link | fast | P1 |
| A05 | Birthday with optional year (docs, REA) | Dex | DONE | TUI edits birthday; CLI and MCP. | - | TUI edit row; accept `--MM-DD` and `YYYY-MM-DD` | smart | P1 |
| A06 | Location text (docs, REA) | Dex | PART | `location` text on person | Geocode ever? Only if map or `near` is wanted. | TUI edit row. Geocoding is C11 | - | P2 |
| A07 | Favorite (REA) | Dex | DONE | `--favorite`, TUI `f`, MCP | - | none | - |  |
| A10 | Create contact dialog (REA) | Dex | DONE | TUI `+` creates a contact in the detail pane. | - | TUI `+` or `a` on People: prompt "Name \| email" then open for edit | fast | P1 |
| A11 | Custom fields: Text, Category, Date; typed, auto-suggest, cannot change type (docs, REA) | Dex | PART | `custom_fields` table exists; importer fills it; no CLI/MCP/TUI | Types beyond Dex (number, URL, multi-select)? Start with Text, Category, Date. | CLI `field define/list/set/unset`, MCP, TUI rows in detail pane, `#field=value` search | extensible, connected | P1 |
| A12 | Priority tier: Top, High, Medium, Low, None, renamable (docs, REA) | Dex | PART | `priority_tier` column and `--tier`; no names, no TUI | Keep Dex tier names or let Grant rename? Dex lets you rename. | Keep the 5 names in a config row; TUI `p` cycles; Tier board is C07 | tactile | P2 |
| A13 | Contact photo, initials fallback (docs) | Dex | SKIP | none | - | Terminal. Keep `image_url` only if import has it | - |  |
| A14 | Description shown above timeline (docs) | Dex | DONE | `notes` field in detail pane | - | none | - |  |
| A17 | Labels for emails/phones/socials (REA) | Dex | PART | `--label` on email/phone | - | Show label in TUI | - | P2 |
