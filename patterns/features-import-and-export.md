---
type: feature-area
---
# Import and export

Dex CSV import, other importers, CSV export and reset.

Moved from FEATURES.md on 2026-10-29. Every row keeps its ID and text. Update a row's State and Now in the same session you change the feature ([[keep-the-tracker-accurate]]). Column meanings and the row lifecycle are in [[feature-rules]].

**Follows patterns:** [[import-is-a-snapshot-sync-is-a-relationship]], [[idempotent-by-uid]], [[never-test-on-real-data]]

**Research:** [[research-hubspot-export]]

**See also:** [[roadmap]], [[index]]

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| C07 | Import: Dex CSV (csv) | Dex | DONE | `crm import dex` idempotent, dry run | - | none | simple |  |
| C08 | Import: generic CSV with column mapping, auto-detect social URLs, create custom fields, Birthday and Tags columns (docs, REA) | Dex | TODO | Dex importer hard-codes Dex columns | Mapping UI in TUI or only `--map` flags? | `crm import csv --map col=field`; reuse Dex importer internals | extensible | P2 |
| C09 | Import: Google Contacts, iCloud, Outlook, phone contacts, vCard (docs) | Dex | TODO | none | Start with vCard files or pull Radicale CardDAV directly? | `crm import vcard` (covers Google/iCloud/phone exports); CardDAV pull from Radicale via `contacts-calendar` skill | connected | P2 |
| C10 | Import: Facebook birthdays, LinkedIn connections CSV (docs) | Dex | TODO | none | - | `crm import linkedin` from the official LinkedIn export CSV | - | P2 |
| C11 | Map view / Contacts by Location / Group Map (docs) | Dex | SKIP | none | - | Terminal. If wanted: geocode to lat/lon, `crm near <place> --km` | - | P3 |
| C12 | Export contacts, notes, network updates CSV (docs, REA) | Dex | PART | `-f csv` on lists; `crm backup` | Match Dex CSV column names exactly for round-trip? | `crm export all <dir>` writing contacts, notes, tasks CSV in Dex-compatible columns | extensible | P2 |
| C13 | Card scanner (docs) | Dex | SKIP | none | - | Agent can OCR a photo and call `person add` | - |  |
| C14 | Import is re-runnable without duplicating (docs) | Dex | DONE | Dex importer idempotent on `dex_id` | - | Apply to every importer | - |  |
| C15 | Reset all data (docs) | Dex | SKIP | n/a | - | Domain DBs are files; delete or restore a backup | - |  |
| G08 | Import and export from the TUI | Dex | TODO | `crm import dex`, CSV output exist | Path prompt or file picker? | Palette actions: Import Dex CSV (path prompt, dry-run summary, confirm), Export current list to CSV | simple | P3 |
