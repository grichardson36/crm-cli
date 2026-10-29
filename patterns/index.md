---
type: index
---
# Pattern language for crm

Atomic notes. One note is one idea. Notes link to larger and smaller patterns. Read [[how-to-use]] first. Known code that breaks a pattern is listed in [[open-discrepancies]].

This is the single entry point for the repository's decisions and features. Start here before any design decision or feature change.

## How to use this vault

1. Read [[how-to-use]], then find the patterns that apply below. Follow links to larger and smaller patterns and use [[decision-cascade]] when the direction is unclear.
2. To learn what a feature does today, open the feature-area note for it (list below). Its table has one row per feature with a stable ID (A01, M27 and so on), State, Now and open questions. [[feature-rules]] explains the columns and the row lifecycle.
3. If no pattern covers the question, or code breaks a pattern, follow [[surface-gaps-before-deciding]] and tell the owner. Do not decide alone.
4. After the owner decides, write one atomic note, link it from [[index]], and update the affected feature-area rows and [[open-discrepancies]] in the same session ([[keep-the-tracker-accurate]]).
5. Never test on the real databases ([[never-test-on-real-data]]).

Note types (frontmatter `type`): `pattern` (one decided rule), `feature-area` (feature rows grouped by area), `research` (read-only findings), `audit`, `schema`, `guide`, `discrepancies` (code that breaks a pattern), `roadmap`, `index`.

## Production scale
*Pertains to our internal best-practices and ethos around product development and production.*

- [[decision-cascade]] (Order to address ambiguous direction in production)

## Product scale
*Pertains to large over-arching decisions on project platform and ethos.*

- [[local-first]] (The program works with no account and no network)
- [[relationship-loop-first]] (Build for the relationship loop first)
- [[modules-are-user-arranged]] (The user arranges the modules and turning one off loses nothing)
- [[saved-views]] (Planned, not built: user-created saved views that appear as modules)
- [[extension-ladder]] (Extend in ascending order of cost and risk)
- [[borrow-before-inventing]] (Borrow a proven pattern before inventing one)
- [[surface-gaps-before-deciding]] (Surface gaps to the user before deciding)
- [[keep-the-tracker-accurate]] (Keep the feature-area notes accurate in the same session)
- [[never-test-on-real-data]] (Test on copies, never on the real databases)
- [[inspirations]] (Lists the apps/programs this program borrows the best implementation from to influence new design direction)
## Object scale

- [[objects-first]] (Objects first, verbs second)
- [[core-objects-carry-native-fields]] (Core objects carry native fields, not custom ones)
- [[one-verb-set]] (One verb set for every object)
- [[same-input-style]] (Add and edit use the same form for every object)
- [[one-record-page]] (Every object opens the same three regions)
- [[person-workspace-is-the-center]] (The person workspace is the center)
- [[timeline-upcoming-first]] (Timeline shows upcoming work above completed history)
- [[link-from-either-side]] (Link from either side)
- [[person-belongs-to-many-organizations]] (A person belongs to many organizations)
- [[organizations-have-many-parents]] (Organizations have many parents)
- [[relation-kinds-have-inverses]] (Relation kinds have inverses)
- [[activities-attach-to-any-object]] (Activities attach to any object)
- [[hidden-not-removed-links]] (Deleting an object hides its links, it does not remove them)
- [[convert-text-to-records-in-review-passes]] (Convert text to records in review passes)
- [[merge-repoints-links]] (A merge re-points every link and keeps an alias)
- [[roles-are-free-text]] (Roles are free text with suggestions)
- [[deal-roles]] (Deal roles are a list the user manages, apart from the role at a company)
- [[deal-stages]] (Deal stages are a list the user manages, each with an outcome)
- [[current-membership-is-open-ended]] (A current membership is open ended)
- [[pick-or-create]] (Pick or create in one step)
- [[field-types-drive-editors]] (The field type decides the editor, the check and the hint)
- [[soft-delete-restorable]] (Delete is soft and restorable everywhere)
- [[delete-asks-yes-no]] (Delete asks y or n)
- [[sync-safe-data]] (Data is sync-safe from day one)
- [[sync-maps-structure-to-flat-fields]] (Two-way sync maps structure to flat fields, open standard)
- [[enrichment-is-suggested-then-accepted]] (Fetched data is a suggestion until the user accepts it)
- [[advanced-objects-off-main-path]] (The default navigation is loose and the user arranges it)

## Screen scale
*UI decisions.*

- [[home-is-an-action-queue]] (Home is a short queue of relationship actions)
- [[tree-view-module]] (One reusable tree renders any collection with a hierarchy path field)
- [[views-screen]] (One Views screen lists, opens, shows, hides and orders every view)
- [[settings-view]] (Settings is one module over one registry, with a scope on each row)
- [[one-bar]] (One bar finds objects and runs verbs)
- [[bar-results-act-like-views]] (Tab into the results and the view hotkeys act on the highlighted object)
- [[confirm-names-in-one-batch]] (Confirm every unresolved name in one panel)

## Interaction scale
*UX decisions.*

- [[three-doors-one-core]] (Three doors, one core)
- [[tui-parity-for-daily-actions]] (Daily human actions have a TUI path)
- [[cli-json-is-the-baseline]] (CLI with JSON is the agent baseline)
- [[agents-never-get-prompts]] (Agents get structured output, never prompts)
- [[tests-for-every-door]] (Every feature is tested in every door)
- [[name-things-once]] (Name things once)
- [[ctrl-moves-the-view-bare-moves-the-object]] (Ctrl keys move the view, bare keys move the object)
- [[same-key-same-verb]] (The same key means the same verb)
- [[keys-are-rebindable]] (Keys are rebindable with conflict detection)
- [[show-the-key-beside-the-action]] (Show the key beside every action)
- [[bar-modes-by-first-word]] (The first word picks verb mode or search mode)
- [[bar-scope-prefixes]] (Scope prefixes narrow the search)
- [[empty-bar-shows-recents]] (An empty bar shows recents and suggestions)
- [[create-on-miss]] (Enter with no match offers to create)
- [[undo-appears-in-the-bar]] (Undo is an action in the bar)
- [[show-parsed-chips]] (Show what was parsed before saving)
- [[unresolved-names-stay-text]] (Unresolved names stay text)
- [[undo-redo-two-stacks]] (Undo and redo are two stacks)
- [[toast-names-action-and-target]] (A toast names the action and its target)
- [[undo-refuses-when-stale]] (Undo refuses when the target changed)
- [[one-line-pipe-prompt]] (Small objects add and edit on one pipe-separated line)
- [[autocomplete-component]] (One matcher and one small popup for every input)
- [[related-segment]] (One related segment on every add line, suggested from context)
- [[pick-stage-from-numbered-list]] (Stages are chosen from a numbered list)
- [[import-is-a-snapshot-sync-is-a-relationship]] (Import is a snapshot, sync is a relationship)
- [[idempotent-by-uid]] (Imports are idempotent by UID) (archived with calendar)
- [[conflicts-keep-server-store-local]] (On conflict keep the server value and store the local one)

## Syntax scale

- [[sigils-for-things-words-for-time]] (Sigils name things, plain words name time)
- [[at-sign-means-person]] (@ means a person)
- [[hash-means-tag]] (# means a tag)
- [[nested-tags]] (A tag named parent/child is a child of parent, the hierarchy is the name)
- [[dates-are-plain-words]] (Dates are plain words)
- [[by-means-deadline]] (by means deadline)
- [[every-means-recurrence]] (every means recurrence)
- [[at-place-means-location]] (at means location) (archived with calendar)
- [[bang-means-priority]] (! means priority)
- [[duration-forms]] (Duration has three accepted forms)
- [[remind-word]] (remind sets a reminder)
- [[date-safety-rules]] (Plain-word dates need safety rules)

## Feature areas

Feature rows (moved from FEATURES.md, which is retired). Each note lists the patterns the area follows and the research behind it. The IDs are stable and cited by agents, never reuse one.

- [[feature-rules]] (Rules every feature follows, column meanings, row lifecycle, inspiration status, build order)
- [[features-people-and-contact-details]] (Person fields, labels, tier, favorites)
- [[features-organizations]] (Organizations, company text, duplicate guard, native fields, images)
- [[features-deals]] (Deal stages and roles)
- [[features-tasks-and-notes]] (Tasks, reminders, notes, note types, timeline)
- [[features-keep-in-touch-and-birthdays]] (Cadences, due, snooze, birthdays, digests, Home)
- [[features-tags-groups-and-views]] (Tags, groups, saved views, filters, bulk edit, search)
- [[features-tags-module]] (The Tags view: list, tree, tag page, rename, merge, nest, clean up)
- [[features-links-and-relations]] (Typed links between all objects)
- [[features-merge-archive-and-undo]] (Duplicates, merge and merge log, archive, purge, undo)
- [[features-import-and-export]] (Dex CSV, other importers, export)
- [[features-sync-and-dav]] (Device sync, sync runner and inbox, CardDAV server, mail and message sync)
- [[features-insights-and-outreach]] (Briefs, email from a contact, mail merge, insights)
- [[features-enrichment]] (Suggested data, providers, refresh policy)
- [[features-tui-keys-and-modules]] (Keys, the one bar, rebinding, modules)
- [[current-tui-keys]] (Current key tables, old key to new key, registry structure)
- [[features-tui-input-and-syntax]] (Short syntax, pipe lines, autocomplete, related segment, hints)
- [[features-platform-and-themes]] (Web, phone, desktop, domains, themes, learn mode)
- [[features-cli-mcp-and-agents]] (CLI with JSON, MCP, help topics, schema manifest, install)
- [[features-validation-and-field-types]] (Field types, the one validation framework, aliases)
- [[features-data-safety]] (Backup, doctor, dry runs, migrations)
- [[features-calendar-archived]] (Calendar, scheduling and booking links, archived 2026-10-16)

## Research notes

Read-only findings that feed the patterns. Verify before relying on them.

- [[research-charm-ecosystem]] (What Bubble Tea and Charm libraries provide)
- [[research-hubspot-export]] (How real HubSpot data uses the relation schema)
- [[research-keybindings]] (Keybindings that stay intuitive across modules)
- [[research-local-first-stacks]] (Stay in Go, rewrite in Rust, or hybrid)
- [[research-mesh-architectures]] (Reticulum, Meshtastic, MeshCore and the sync contract)
- [[research-relationship-models]] (How mature CRMs model relationships)
- [[research-sync-building-blocks]] (Libraries to adopt or study for sync)
- [[research-sync-engines]] (Sync engines that fit Go and SQLite)
- [[research-sync-standard]] (Structured-to-flat mapping and round-trip fidelity)
- [[research-sync-standardization]] (The smallest database-agnostic sync contract)
- [[research-terminal-images]] (Images in the terminal interface)
- [[research-tui-building-blocks]] (Standard TUI building blocks)
- [[research-tui-inspirations]] (TUI-first apps and what they teach)
- [[research-view-module-standard]] (Renderer-agnostic view modules)

## Design records

- [[schema-er]] (Schema proposal: relationships that match the real world)
- [[sync-contract]] (Sync contract v1)
- [[audit-keybindings-2026-10]] (Key audit, 2026-10-21)
- [[audit-keybindings-2026-10-simplify]] (Keys to simplify, 2026-10-27)

## Open discrepancies and roadmap

- [[open-discrepancies]] (Live list of code that breaks a pattern, with the owner question for each)
- [[roadmap]] (Items not built yet)
