---
type: pattern
scale: object
status: decided
---

# Link from either side

**Problem.** A relation can be created from one object only, or in one interface only.

**Therefore.** Every relation is made, viewed and removed from both ends in CLI, MCP and TUI. On a person: Link organization. On an organization: Link person. The same holds for person and deal, deal and organization, task and person, note and person, event and person, tag and anything. One stored link, two views. Linked objects show as rows on the record page that open the other object.

**Because.** Each object registers its relations once (model.Relations: storage, attributes, labels). A link may carry attributes (role, dates, primary, kind). See X8 in [[open-discrepancies]] for the relations still outside it.

**Larger patterns.** [[objects-first]], [[one-record-page]]

**Smaller patterns.** [[pick-or-create]], [[one-record-page]], [[person-belongs-to-many-organizations]], [[organizations-have-many-parents]], [[relation-kinds-have-inverses]], [[activities-attach-to-any-object]], [[hidden-not-removed-links]], [[merge-repoints-links]]

**Sources.** HubSpot associations, Attio and Folk record links, Monica relationships

**Inspired by.** [[hubspot]], [[monica]], [[attio]], [[obsidian]]
