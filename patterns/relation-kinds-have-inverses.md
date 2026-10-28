---
type: pattern
scale: object
status: decided
---

# Relation kinds have inverses

**Problem.** "Related" with no meaning loses what the user knows: Sam is Alex's parent, not just connected. Storing the link twice, once per direction, drifts out of sync.

**Therefore.** A link between two people (or two organizations) has a kind from one shared vocabulary, relation_kind. A kind has a name and an inverse name. The link is stored once as (a, b, kind) and reads "a is parent b". Seen from b, a is labeled parent. Seen from a, b is labeled child. A symmetric kind has one name and is stored with the lower id first. Built in: related (the default, equal to Dex Related), spouse, sibling, friend, colleague, parent and child, manager and report, mentor and mentee, referred-by and referred. Users add kinds, mutual or with an inverse name, and remove the ones no link uses. Naming the inverse on input (`--kind child`) reverses the two ends. The Link form shows both labels before saving. Noun kinds show the other person's role (a parent link shows child on the parent's page). Verb kinds are flagged phrase (referred-by) and read from the page you are on: A referred-by B shows "referred-by B" on A's page and "referred A" on B's page, in the CLI, MCP and TUI.

**Because.** HubSpot has paired labels with an inverse label, Monica has an opposite for every type, Salesforce maps a role to a reverse role. Dex Related is an untyped list ([[research-relationship-models]] section 3).

**Larger patterns.** [[link-from-either-side]], [[objects-first]]

**Smaller patterns.** [[organizations-have-many-parents]]

**Sources.** [[schema-er]] (relation_kind, E1, Q23), [[research-relationship-models]]

**Inspired by.** [[hubspot]], [[monica]], [[salesforce]], [[dex]]
