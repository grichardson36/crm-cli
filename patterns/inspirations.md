---
type: index
---
# Inspirations

Every pattern here comes from an inspiration or from our own ideas ([[us]]). Each note below says what we take, what we leave, where the evidence came from and what is still unresearched. A pattern links to its inspirations with an **Inspired by** line, and each inspiration links to the patterns it informed.

Part of the [[index]]. Design gates and pattern guidance are in [[core]].

## CRMs

| Inspiration           | Research | Patterns informed                                                                                                                  |
| --------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| [[dex]]               | mined    | [[core]], [[core]], [[core]], [[core]] +3 |
| [[clay-me-sh]]        | partial  | [[core]], [[core]]                                                                           |
| [[hubspot]]           | partial  | [[core]], [[core]], [[link-registry]], [[core]] +9                            |
| [[monica]]            | partial  | [[link-registry]], [[core]], [[core]], [[link-map]]                |
| [[folk]]              | partial  | [[core]], [[object-registry]], [[core]]                                                                 |
| [[attio]]             | partial  | [[link-registry]], [[object-registry]], [[core]]                                                                |
| [[pipedrive]]         | partial  | [[core]], [[core]], [[core]], [[core]] +1                     |
| [[salesforce]]        | partial  | [[core]], [[core]], [[core]], [[core]], [[link-map]] |
| [[jdanielnd-crm-cli]] | mined    | [[core]]                                                                                                           |
| [[dzhng-crm-cli]]     | partial  | [[core]], [[core]]                                                                                 |

## Notes, command bars, messaging and editors

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[obsidian]] | mined | [[core]], [[core]], [[core]], [[link-registry]] +3 |
| [[pi]] | partial | [[core]] |
| [[vscode]] | partial | [[core]], [[core]] |
| [[superhuman]] | partial | [[core]], [[core]], [[core]] |
| [[raycast]] | partial | [[core]] |
| [[linear]] | partial | [[core]], [[core]] |
| [[slack]] | partial | [[core]], [[core]] |
| [[github]] | partial | [[core]], [[core]], [[core]] |
| [[notion]] | partial | [[object-registry]], [[core]], [[core]], [[core]] |
| [[microsoft-office]] | partial | [[core]], [[core]] |
| [[vim-and-lazygit]] | partial | [[core]], [[core]] |

## Task apps and quick add

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[todoist]] | partial | [[core]], [[core]], [[core]], [[core]] +1 |
| [[ticktick]] | partial | [[core]], [[core]], [[core]] |
| [[things]] | partial | [[core]] |
| [[super-productivity]] | partial | [[core]], [[core]] |

## Calendars and scheduling

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[fantastical]] | partial | [[core]], [[core]], [[core]] |
| [[motion]] | mined | [[core]], [[core]] |
| [[calendly]] | partial | [[core]] |
| [[outlook]] | bookmarked | [[idempotent-by-uid]], [[core]] |
| [[thunderbird]] | bookmarked | [[idempotent-by-uid]] |

## Standards

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[caldav-and-icalendar-standards]] | mined | [[core]], [[idempotent-by-uid]], [[core]] |

## Business and style

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[obsidian-sync-and-publish]] | bookmarked | [[core]] |
| [[simplefin-and-exe-dev]] | bookmarked | [[core]] |

## Our own

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[us]] | ongoing | [[core]], [[core]], [[core]], [[core]] +1 |

## Research status key

- mined: read in depth, rows built into the feature-area notes (see [[index]]) or a plan file.
- partial: some first-party pages read, some claims from search summaries. Each note marks which.
- bookmarked: named as an inspiration, not researched in detail.

## Research queue (gaps surfaced to the owner)

1. Clay/me.sh cadence, groups and search: third-party only. Needs first-party docs or a trial.
2. Dex REST shapes: blocked on the Professional plan.
3. TickTick and Fantastical syntax: old third-party sources. Fetch official pages.
4. Outlook and Thunderbird calendar behaviour: official pages failed. Re-check before building.
5. Monica gifts, debts and life events: marketing copy only.
6. Link verbs: how HubSpot, Attio, Folk, Notion and Monica add a link from the other side (needed for [[link-registry]] verification). Not yet researched in first-party docs.
7. Restore and trash conventions: HubSpot, Notion and Gmail restore flows (for [[core]]).
8. Object registries and generic verbs: how Attio, Notion and Folk expose one verb set across object types (for [[object-registry]]).
9. Local-first community sync standard: Ink & Switch, Automerge, cr-sqlite, Replicache, Linear, ElectricSQL, Actual Budget. Field-level merge, structured-to-flat mappings, deletes, conflict display (for [[core]]).
10. Terminal UI platform and images: which terminals draw images (Sixel, Kitty, iTerm2 protocols; Windows Terminal), whether Bubble Tea can host them, or whether another TUI stack or a native/web front end is needed for logos and photos. Owner open decision 2026-10-19 ([[features-organizations]] M16). First pass done 2026-10-20: [[research-terminal-images]]. Spikes pending.
11. Enrichment providers: Clay, Anymailfinder and similar APIs: fields, pricing, rate limits, terms (for [[core]], [[features-enrichment]]).
12. Module customization: how Notion, Obsidian, Raycast, HubSpot and Attio let users turn parts of the UI on or off and reorder them (for [[core]]).
13. Keybindings: first pass [[research-keybindings]] (herdr, Obsidian, Primer/GitLab). Still to read: Obsidian default table, Superhuman, Linear, Gmail, helix, Zellij, tmux, Things, Todoist.

## Adding an inspiration

1. Research with the `research` skill (or REA for shipped apps). Prefer first-party pages and tag each claim doc, search or general.
2. Add one note in `inspirations/` using the layout of [[dex]]: What it is, Research state, What we take (each item links to a pattern), What we leave, Where we beat it, Gaps, Sources.
3. If it changes a design decision, apply [[core]], then write or amend a supporting pattern when its criteria are met and add the **Inspired by** line.
4. Add the note to the table above.
