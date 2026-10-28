---
type: guide
---
# Feature rules, columns and row lifecycle

Moved from FEATURES.md on 2026-10-29. FEATURES.md is retired. Its rows live in the `features-*` notes listed in [[index]]. This note keeps the rules, column meanings, lifecycle and inspiration status that headed the old tracker.

Purpose: every feature we want comes from an inspiration (Dex, Clay, HubSpot, Monica, Folk, Attio and others, or our own ideas). Each row says where it came from, what our program does today, what is still undecided, and the plan. When the open questions are answered, an agent builds it so it matches the inspiration or beats it. Beating it means the "Better by" axes are met: **fast, simple, intuitive, tactile (visually and physically rewarding), connected (linked to other features and data), extensible, smart, safe**.

## Design direction (owner, 2026-10-14)

"Better" means reaching the thing faster and more intuitively than a click-and-wait web GUI, while the interface stays clean and simple. Prefer fewer keystrokes, instant response, and an uncluttered screen over more visible controls. Agent extensibility is a day-one requirement: the program is local files plus one binary, so any agent (Claude, Codex, Pi, OpenCode and others) can pick it up and learn it from the program itself, and the human experience does not change when an agent is involved. Fast input comes from typing meaning inline (short syntax), plus tooltips, pickers and inline settings wherever a value is needed, so the program feels lightning fast in the fingers and in the CLI. Documentation is the minimum that answers a basic question quickly and points to the next page that answers the next one; nothing longer. Everything is reachable from the palette; the screen shows only what the current task needs.

## Rules for every feature

1. **TUI parity.** Every function added to the CLI or MCP must also exist in the TUI as a palette action (`ctrl+k`) and, where a human does it often, a key. A row is not DONE until the TUI path exists, or the row says why not (for example, `backup`, which is a scripting command). The palette registry in `internal/tui/actions.go` is the checklist.
2. **Tests.** Repo unit test, CLI integration test, MCP test, TUI key-driven test.
3. **Docs and agent surface.** Every feature ships its help topic in the embedded docs (`crm help <topic>`), appears in the machine-readable command manifest (`crm schema`), and in the TUI palette description. No agent-specific setup may be required to use it: the CLI with `-f json` is the baseline, MCP and skills are conveniences. Update the generated skill (`crm agent skill`) rather than hand-editing per-agent files.
4. **Sync-safe.** UUID, `updated_at` bumped on every change, soft deletes (purge writes a tombstone).
5. **Local data.** Never test against the real databases. Use a copy.
6. **Premium pattern first.** For every design question ask: what premium design pattern already works in the best existing software? Borrow an agreed industry convention before inventing one. It must make semantic sense to the user without being cumbersome to type. Log the pattern chosen and its source in `DESIGN-PATTERNS.md`.
7. **Keep the feature-area notes accurate.** Other agents rely on them. Whoever changes a feature updates its State and Now in the same session, in the `features-*` note that holds the row. Use `BRANCH` in Now when the work exists only on an unmerged branch (name the branch), and `MERGED` once it is on main.
8. **Pattern language first.** Before any design decision read [[index]] (start with [[how-to-use]]). Build objects first and verbs once ([[objects-first]]), never a copy of an action per object. If a question has no pattern, or code breaks one, stop and surface it to the owner after researching the convention in our inspirations and in software practice. Record violations in [[open-discrepancies]]. Rule 6 feeds the pattern language: when a borrowed pattern is adopted, write it as an atomic note in the vault.

## Columns

| Column | Meaning |
|---|---|
| ID | Stable code. Letter = section, number = order added. Never reuse an ID. |
| Feature (evidence) | What the inspiration does. Evidence tags: `docs` product documentation, `REA` reverse engineering of the shipped app, `API` tool/API reference, `csv` our own data export, `web` other public page. |
| From | Inspiration: `Dex`, `Clay`, `HubSpot`, `Monica`, `Folk`, `Attio`, `Us` (our idea), `Claude agents`. A feature seen in several gets the first and the rest in the Feature text. |
| State | `DONE` works and is tested. `PART` partly there. `TODO` not built. `SKIP` decided not to build. `BEYOND` we have it and the inspiration does not. |
| Now | What exists in our code today. Must be verified, not assumed. |
| Open questions | Decisions that block building. `-` means none, so the row is ready for an agent. |
| Action plan | How it becomes CLI + MCP + TUI. Every feature ends up in CLI and MCP. The TUI gets the ones a human does at a keyboard. |
| Better by | Which axes we aim to beat the inspiration on (list above). `-` means match it first. |
| P | Priority: `P1` needed to use the TUI instead of Dex daily, `P2` nice to have, `P3` later. Proposals until Grant confirms. |

## Row lifecycle

1. **Intake.** Add the row with From, evidence, Now, Open questions, Action plan, Better by. State TODO or PART.
2. **Decide.** Grant answers the Open questions. Replace the cell with `-` and fold the answer into Action plan.
3. **Build.** An agent takes a row with `-` open questions, builds CLI + repo + MCP (+ TUI where listed), adds tests, updates the skill and `Now`, and sets State to DONE.
4. **Beat check.** Before DONE, the agent states how each "Better by" axis is met, or notes why not. Matching the inspiration is the floor.

## Inspirations

| Source | Status | Notes |
|---|---|---|
| Dex | Mined (docs, REA, API, csv). 94 rows below. | Local-first personal CRM. Our migration target. |
| Clay | Not mined | Enrichment, relationship scoring, nudges, network search. |
| HubSpot | Not mined | Deals, pipelines, sequences, email logging, reports. Business domain candidate. |
| Monica | Not mined | Personal CRM: life events, gifts, debts, journal, relationship types. |
| Folk, Attio | Not mined | Custom objects, views, lists, enrichment. |
| Motion, Outlook, Thunderbird, Calendly | Bookmarked (section J), ARCHIVED 2026-10-16 | Calendar view, task scheduling, smart scheduling, booking links. Not yet researched in detail. |
| Us | Ongoing | Multi-domain data, agent-first, offline, reversible actions. |

Each inspiration also has an atomic note in `inspirations/` (hub: [[inspirations]]) linked to the patterns it informed. Add or update that note when you mine an inspiration.

To add one: research with the `research` skill (or REA for shipped apps), then append rows in a new section of the table below with the next free IDs.

## Suggested build order

1. TUI editing core: A01-A05, A10, B01, B11, E01, F05 tools for what is added. Focus pane, selectable rows, Enter to edit, `y` copy, `d` delete, `a` add, `space` mark.
2. Duplicates: C01-C05 backend is DONE. Next is G01 (TUI Merge view), G02-G04 (archive, note types, note edit in the TUI).
3. Home, Review and KIT views: D04, D07, D09, D11, D17.
4. Archive and create: A08, A10. Saved views and query language: B06, B07.
5. Groups, custom fields in TUI, timeline: B03, A11, E03.
6. Import/export, digest, resurface: C08-C10, C12, D14, D15.
7. Integrations and the editable web question: E05-E08, F01.

## Notes for future agents

- Dex's merge keeps the oldest record per the API doc and prefers newest data per the help doc. We chose: survivor = the one the user keeps, archive the other with `merged_into`.
- Dex Shift+Y, `#`, E, H, F, P, T, N, G+D shortcuts are listed above. Mirror them where they do not collide with ours.
- Priority tier names in Dex: Top, High, Medium, Low, None.
- Dex exports the three CSVs listed in NOTES.md. `feed_items` (LinkedIn title changes) are not imported (D19).
