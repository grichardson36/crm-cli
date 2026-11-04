---
type: reference
---
# Link map

This is the current link vocabulary, not a pattern. Use it to decide whether a feature can use an existing link before proposing a new object, field, table or attribute. It is the only product-facing source for link variables. Do not add a link variable or relation unless an owner decision shows that this map cannot express the fact.

| Objects | Meaning | Variables | Constraint | Internal storage key |
|---|---|---|---|---|
| Person → Organization | A person's organization connection | role, from, to, primary, notes | Links can repeat for separate roles or time periods. One current primary organization per person. | `person_organization_link` |
| Person → Person | A directed or mutual personal relationship | kind, anniversary (spouse kind only), notes | One live link for the same pair and kind. | `person_relation` |
| Organization → Organization | A directed or mutual organization relationship | kind, notes | One live link for the same pair and kind. | `org_relation` |
| Deal → Person | A person's role in a deal | role | One live link per pair. | `deal_person` |
| Deal → Organization | An organization's role in a deal | role, primary | One live link per pair. One primary organization per deal. | `deal_org` |
| Task or Note → Person, Organization or Deal | An object affected by the task or note | role | One live link per pair. | `task_*`, `interaction_*` |

Tags are attachments to any object, not links. Deleting an object hides its links; restoring it restores them. A merge re-points links and removes only exact duplicates. 
