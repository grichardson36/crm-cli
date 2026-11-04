---
type: feature-worklist
domains:
  - product
  - domain-model
---
# Product-model work

Active work governed by [[standards/product-model]]. IDs from retired sheets are retained where unique. `TODO` means not built; `PART` requires fresh code inspection before work starts. Read [[core]] before starting an item.

## Model repair

- [ ] `R01` (TODO, P1): Remove `people.company`; migrate it to primary Person–Organization links. See [[open-discrepancies]] X41.
- [ ] `R02` (TODO, P1): Rename the public Activity object consistently across storage boundaries and doors. See [[open-discrepancies]] X42.
- [ ] `R03` (TODO, P1): Define the smallest shared write path for Person and Person–Organization updates.
- [ ] `R04` (TODO, P1): Identify the smallest shared metadata seam for object knowledge used across doors.
- [ ] `R05` (TODO, P1): Define one reusable person-briefing query.
- [ ] `R06` (TODO, P2): Map CardDAV organization data to the primary Person–Organization link after R01.
- [ ] `M11` (PART, P1): Verify and integrate the unmerged link-model branch only after X41, X42 and X45 are resolved.
- [ ] `A15` (PART, P1): Verify the unmerged related-contacts implementation against the canonical link model.

## People, relationships and follow-up

- [ ] `A03` (PART, P1): Support multiple phone numbers with a canonical stored and matching form.
- [ ] `A06` (PART, P2): Complete location behavior across applicable doors.
- [ ] `A11` (PART, P2): Decide the smallest typed custom-field model before extending field types.
- [ ] `A12` (PART, P2): Complete priority-tier behavior without creating a second state system.
- [ ] `A17` (PART, P2): Add labels for email, phone and social contact methods.
- [ ] `D03` (PART, P1): Make incoming and outgoing interactions reset keep-in-touch cadence consistently.
- [ ] `D04` (PART, P1): Complete the keep-in-touch view and its grouping behavior.
- [ ] `D08` (TODO, P2): Decide how ignored reminders recur after a full period.
- [ ] `D09` (TODO, P2): Design quick follow-up actions from a relationship-centered workflow.
- [ ] `D10` (TODO, P2): Set a bounded daily keep-in-touch notification policy.
- [ ] `D14` (TODO, P3): Research digest and resurface behavior before adding scheduled delivery.
- [ ] `D15` (TODO, P3): Define resurfacing for contacts without cadence.
- [ ] `D16` (TODO, P3): Define relationship-safe random contact suggestions.
- [ ] `D19` (PART, P2): Turn verified external title changes into a user-approved activity or follow-up.
- [ ] Legacy `E11` pre-meeting brief (PART, P2): Complete the briefing before assigning a canonical ID. See [[open-discrepancies]] X49.
- [ ] `E12` (TODO, P3): Define outbound email behavior before adding mail actions.
- [ ] `E14` (TODO, P3): Define relationship insights before deriving recommendations from the graph.

## Tasks, activities, tags and views

- [ ] `D11` (PART, P1): Complete recurring reminders and task grouping.
- [ ] `D12` (PART, P2): Create a reminder from a person or Activity through the same task behavior.
- [ ] `E02` (TODO, P3): Decide attachment ownership, storage and recovery before implementation.
- [ ] `E03` (PART, P1): Complete a searchable person activity timeline.
- [ ] `E04` (TODO, P2): Define the global activity timeline after person timeline behavior is settled.
- [ ] `B02` (PART, P1): Complete tag rename and deletion behavior.
- [ ] `B03` (TODO, P2): Decide whether groups are distinct from tags before modeling them.
- [ ] `B04` (TODO, P2): Add group browsing only if B03 establishes a distinct model.
- [ ] `B06` (TODO, P2): Complete saved-view lifecycle and ownership.
- [ ] `B07` (PART, P1): Define the canonical filter vocabulary.
- [ ] `B09` (PART, P2): Complete configurable list columns without per-view query copies.
- [ ] `B10` (TODO, P3): Add board behavior only after category fields are settled.
- [ ] `B12` (TODO, P3): Decide whether pinned items are modules, saved views or a separate concept.
- [ ] `B13` (PART, P1): Complete accent-insensitive search behavior.
- [ ] `G07` (PART, P1): Complete tag management across applicable doors.

## Merge and deals

- [ ] `M05` (PART, P1): Complete the merge review surface with explicit field choices.
- [ ] `F12` (PART, P1): Define reversible merge behavior and an auditable undo record.
- [ ] `C06` (TODO, P3): Research conservative merge suggestions before automating them.
- [ ] `M27` (PART, P2): Verify the unmerged customizable deal-role branch against the current model.
- [ ] `M28` (PART, P2): Verify the unmerged customizable deal-stage branch against the current model.
