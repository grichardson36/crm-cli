---
type: pattern
scale: object
status: decided
---

# Every object opens the same three regions

**Problem.** Each object has an ad hoc detail layout.

**Therefore.** Identity (name and key fields), Now (next action, state, primary verbs), Context (links, then a chronological timeline with upcoming items above history). The timeline is the [[notes-feed]]: wrapped, at most 5 lines per entry, expanded in place. Rare fields live under a Details tab. Search and filters in the timeline appear only on request.

**Because.** Person is the model. A read-only text pane is a temporary state.

**Larger patterns.** [[objects-first]], [[link-from-either-side]]

**Smaller patterns.** [[notes-feed]], [[person-workspace-is-the-center]], [[link-from-either-side]], [[timeline-upcoming-first]]

**Sources.** HubSpot record layout, Salesforce Activity Timeline, Pipedrive Contacts Timeline (CRM-UX-RESEARCH.md O1, R1)

**Inspired by.** [[dex]], [[hubspot]], [[monica]], [[pipedrive]], [[salesforce]]
