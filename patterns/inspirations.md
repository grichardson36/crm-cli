---
type: index
---
# Inspirations

Every pattern here comes from an inspiration or from our own ideas ([[us]]). Each note below says what we take, what we leave, where the evidence came from and what is still unresearched. A pattern links to its inspirations with an **Inspired by** line, and each inspiration links to the patterns it informed.

Part of the [[index]]. Process rules are in [[how-to-use]] and [[surface-gaps-before-deciding]].

## CRMs

| Inspiration           | Research | Patterns informed                                                                                                                  |
| --------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| [[dex]]               | mined    | [[cli-json-is-the-baseline]], [[home-is-an-action-queue]], [[import-is-a-snapshot-sync-is-a-relationship]], [[one-record-page]] +3 |
| [[clay-me-sh]]        | partial  | [[home-is-an-action-queue]], [[relationship-loop-first]]                                                                           |
| [[hubspot]]           | partial  | [[advanced-objects-off-main-path]], [[at-sign-means-person]], [[link-from-either-side]], [[one-bar]] +9                            |
| [[monica]]            | partial  | [[link-from-either-side]], [[one-record-page]], [[person-workspace-is-the-center]], [[relation-kinds-have-inverses]]                |
| [[folk]]              | partial  | [[home-is-an-action-queue]], [[objects-first]], [[sync-safe-data]]                                                                 |
| [[attio]]             | partial  | [[link-from-either-side]], [[objects-first]], [[show-parsed-chips]]                                                                |
| [[pipedrive]]         | partial  | [[home-is-an-action-queue]], [[one-record-page]], [[pick-stage-from-numbered-list]], [[same-key-same-verb]] +1                     |
| [[salesforce]]        | partial  | [[advanced-objects-off-main-path]], [[extension-ladder]], [[one-record-page]], [[current-membership-is-open-ended]], [[person-belongs-to-many-organizations]] |
| [[jdanielnd-crm-cli]] | mined    | [[three-doors-one-core]]                                                                                                           |
| [[dzhng-crm-cli]]     | partial  | [[cli-json-is-the-baseline]], [[extension-ladder]]                                                                                 |

## Notes, command bars, messaging and editors

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[obsidian]] | mined | [[create-on-miss]], [[empty-bar-shows-recents]], [[extension-ladder]], [[link-from-either-side]] +3 |
| [[pi]] | partial | [[extension-ladder]] |
| [[vscode]] | partial | [[bar-scope-prefixes]], [[one-bar]] |
| [[superhuman]] | partial | [[one-bar]], [[show-the-key-beside-the-action]], [[toast-names-action-and-target]] |
| [[raycast]] | partial | [[bar-modes-by-first-word]] |
| [[linear]] | partial | [[empty-bar-shows-recents]], [[one-bar]] |
| [[slack]] | partial | [[at-sign-means-person]], [[confirm-names-in-one-batch]] |
| [[github]] | partial | [[at-sign-means-person]], [[hash-means-tag]], [[unresolved-names-stay-text]] |
| [[notion]] | partial | [[objects-first]], [[pick-or-create]], [[remind-word]], [[unresolved-names-stay-text]] |
| [[microsoft-office]] | partial | [[undo-appears-in-the-bar]], [[undo-redo-two-stacks]] |
| [[vim-and-lazygit]] | partial | [[keys-are-rebindable]], [[undo-redo-two-stacks]] |

## Task apps and quick add

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[todoist]] | partial | [[date-safety-rules]], [[dates-are-plain-words]], [[every-means-recurrence]], [[hash-means-tag]] +1 |
| [[ticktick]] | partial | [[bang-means-priority]], [[date-safety-rules]], [[show-parsed-chips]] |
| [[things]] | partial | [[dates-are-plain-words]] |
| [[super-productivity]] | partial | [[at-sign-means-person]], [[duration-forms]] |

## Calendars and scheduling

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[fantastical]] | partial | [[at-place-means-location]], [[bang-means-priority]], [[date-safety-rules]] |
| [[motion]] | mined | [[by-means-deadline]], [[undo-redo-two-stacks]] |
| [[calendly]] | partial | [[local-first]] |
| [[outlook]] | bookmarked | [[idempotent-by-uid]], [[import-is-a-snapshot-sync-is-a-relationship]] |
| [[thunderbird]] | bookmarked | [[idempotent-by-uid]] |

## Standards

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[caldav-and-icalendar-standards]] | mined | [[conflicts-keep-server-store-local]], [[idempotent-by-uid]], [[import-is-a-snapshot-sync-is-a-relationship]] |

## Business and style

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[obsidian-sync-and-publish]] | bookmarked | [[local-first]] |
| [[simplefin-and-exe-dev]] | bookmarked | [[local-first]] |

## Our own

| Inspiration | Research | Patterns informed |
|---|---|---|
| [[us]] | ongoing | [[agents-never-get-prompts]], [[cli-json-is-the-baseline]], [[local-first]], [[soft-delete-restorable]] +1 |

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
6. Link verbs: how HubSpot, Attio, Folk, Notion and Monica add a link from the other side (needed for [[link-from-either-side]] verification). Not yet researched in first-party docs.
7. Restore and trash conventions: HubSpot, Notion and Gmail restore flows (for [[soft-delete-restorable]]).
8. Object registries and generic verbs: how Attio, Notion and Folk expose one verb set across object types (for [[objects-first]]).
9. Local-first community sync standard: Ink & Switch, Automerge, cr-sqlite, Replicache, Linear, ElectricSQL, Actual Budget. Field-level merge, structured-to-flat mappings, deletes, conflict display (for [[sync-maps-structure-to-flat-fields]]).
10. Terminal UI platform and images: which terminals draw images (Sixel, Kitty, iTerm2 protocols; Windows Terminal), whether Bubble Tea can host them, or whether another TUI stack or a native/web front end is needed for logos and photos. Owner open decision 2026-10-19 ([[features-organizations]] M16). First pass done 2026-10-20: [[research-terminal-images]]. Spikes pending.
11. Enrichment providers: Clay, Anymailfinder and similar APIs: fields, pricing, rate limits, terms (for [[enrichment-is-suggested-then-accepted]], [[features-enrichment]]).
12. Module customization: how Notion, Obsidian, Raycast, HubSpot and Attio let users turn parts of the UI on or off and reorder them (for [[modules-are-user-arranged]]).
13. Keybindings: first pass [[research-keybindings]] (herdr, Obsidian, Primer/GitLab). Still to read: Obsidian default table, Superhuman, Linear, Gmail, helix, Zellij, tmux, Things, Todoist.

## Adding an inspiration

1. Research with the `research` skill (or REA for shipped apps). Prefer first-party pages and tag each claim doc, search or general.
2. Add one note in `inspirations/` using the layout of [[dex]]: What it is, Research state, What we take (each item links to a pattern), What we leave, Where we beat it, Gaps, Sources.
3. If it changes a design decision, follow [[surface-gaps-before-deciding]], then write or amend a pattern and add the **Inspired by** line.
4. Add the note to the table above.
