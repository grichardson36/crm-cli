---
type: pattern
scale: object
status: decided
---

# A current membership is open ended

**Problem.** People change jobs. Deleting the old link loses history, and a flag called "former" cannot say when.

**Therefore.** A membership has an optional start and an optional end. No end means current. An end on or after today still counts as current, an earlier end makes it past. Dates are `YYYY`, `YYYY-MM` or `YYYY-MM-DD`, because people remember a year or a month. Past memberships stay on the page, listed after the current ones and dimmed with their dates. The same person may hold the same organization twice (left and came back) and overlap with others. Setting an end date is an ordinary edit of the link (`E` in the terminal UI, `--to` on the CLI), so it undoes like any other change.

**Because.** Salesforce is the only product found with start, end and active on the person to account link ([[research-relationship-models]] section 1). HubSpot has no dates on a link. The owner accepted overlapping and repeated memberships (E3).

**Larger patterns.** [[person-belongs-to-many-organizations]]

**Smaller patterns.** [[roles-are-free-text]]

**Sources.** [[schema-er]] (membership, E3), [[research-relationship-models]]

**Inspired by.** [[salesforce]]
