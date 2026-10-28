---
type: feature-area
---
# Briefs, insights and outreach

Pre-meeting brief, email from a contact, mail merge, insights and the network graph.

Moved from FEATURES.md on 2026-10-29. Every row keeps its ID and text. Update a row's State and Now in the same session you change the feature ([[keep-the-tracker-accurate]]). Column meanings and the row lifecycle are in [[feature-rules]].

**Follows patterns:** [[relationship-loop-first]], [[enrichment-is-suggested-then-accepted]]

**See also:** [[index]]

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| E11 | Pre-meeting brief (docs) | Dex | PART | `crm context <id>` and MCP | Triggered from calendar automatically or on demand? | Skill recipe: calendar event -> attendees -> context. Optional `crm brief --event` later | smart, connected | P2 |
| E12 | Send email from a contact, mailto, bulk (docs) | Dex | TODO | none | - | TUI `e` opens `mailto:`; `crm emails --tag x` prints addresses | - | P2 |
| E13 | Mail merge (docs) | Dex | SKIP | none | - | Agent plus `imap-smtp` skill | - |  |
| E14 | Network Insights: where they work, studied, live; strongest connections (docs, REA) | Dex | TODO | `status` only | Which dimensions first (company, city, tag)? | `crm insights` (counts by company, city, tag; top by interactions); TUI Insights view | smart | P3 |
| E15 | Network graph / relationships view (REA) | Dex | SKIP | none | - |  | - |  |
