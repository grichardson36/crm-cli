---
type: index
---
# Pattern language for crm

Atomic notes. One note is one idea. Notes link to larger and smaller patterns. Read [[how-to-use]] first. Known code that breaks a pattern is listed in [[open-discrepancies]].

## Production scale
*Pertains to our internal best-practices and ethos around product development and production.*

- [[decision-cascade]] (Order to address ambiguous direction in production)

## Product scale
*Pertains to large over-arching decisions on project platform and ethos.*

- [[local-first]] (The program works with no account and no network)
- [[relationship-loop-first]] (Build for the relationship loop first)
- [[modules-are-user-arranged]] (The user arranges the modules and turning one off loses nothing)
- [[extension-ladder]] (Extend in ascending order of cost and risk)
- [[borrow-before-inventing]] (Borrow a proven pattern before inventing one)
- [[surface-gaps-before-deciding]] (Surface gaps to the user before deciding)
- [[keep-the-tracker-accurate]] (Keep FEATURES.md accurate in the same session)
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
- [[dates-are-plain-words]] (Dates are plain words)
- [[by-means-deadline]] (by means deadline)
- [[every-means-recurrence]] (every means recurrence)
- [[at-place-means-location]] (at means location) (archived with calendar)
- [[bang-means-priority]] (! means priority)
- [[duration-forms]] (Duration has three accepted forms)
- [[remind-word]] (remind sets a reminder)
- [[date-safety-rules]] (Plain-word dates need safety rules)
