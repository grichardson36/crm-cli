---
type: pattern
scale: object
status: decided
---

# A person belongs to many organizations

**Problem.** One organization per person cannot hold a consultant with three clients, a founder who also advises, or someone who changed jobs. A single `org_id` forces the wrong answer or a text field nobody can query.

**Therefore.** Person and organization are many to many through one link, the Person–Organization link. The Person–Organization link carries a role ([[roles-are-free-text]]), a start and end date ([[current-Person–Organization link-is-open-ended]]) and a primary flag. At most one live Person–Organization link per person is primary. The first current Person–Organization link becomes primary by itself and moving the primary demotes the old one. The person page lists Organizations with the primary first and past stays dimmed. The organization page lists its Members. A person with no Person–Organization link is valid and `people.company` stays as plain text until it is converted. The primary Person–Organization link fills the vCard ORG when the phone syncs.

**Because.** 28 percent of contacts in the owner's HubSpot export have two or more companies ([[research-hubspot-export]] H1). HubSpot, Folk and Salesforce all treat it as many to many with a primary flag ([[research-relationship-models]]). Dates and roles on the link go beyond HubSpot and match Salesforce AccountContactRelation.

**Larger patterns.** [[link-from-either-side]], [[objects-first]]

**Smaller patterns.** [[roles-are-free-text]], [[current-Person–Organization link-is-open-ended]], [[merge-repoints-links]]

**Sources.** [[schema-er]] (table Person–Organization link, E3 and E4), [[research-hubspot-export]], [[research-relationship-models]]

**Inspired by.** [[hubspot]], [[salesforce]], [[folk]]
