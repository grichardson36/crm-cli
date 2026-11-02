---
type: pattern
status: decided
core-gate: 4
domains: [domain-model, data]
---
# Link registry

**Rule.** Relationships use the existing link registry and its declared variables.

**Applies when.** Connecting CRM objects or proposing a relationship attribute under [[core]] gate 4.

**Default.** Use a relation in [[link-map]] and show it from either end.

**Exception.** Add a new relation or variable only after an owner decision shows the map cannot express the fact.

**Check.** Every relationship appears in the map and has one stored representation.
