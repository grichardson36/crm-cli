---
type: standard
domains:
  - product
  - domain-model
  - application
---
# Product-model standard

## Scope

The CRM's user value, product language, objects, fields, links and shared behavior.

## Current model

The core objects are Person, Organization, Deal, Task, Activity and Tag. Activity is stored as `interaction`; its public name remains an open decision in [[open-discrepancies]] X42.

```text
Person <-> Organization  person_organization_link
Person <-> Person        person_relation
Organization <-> Org     org_relation
Deal <-> Person          deal_person
Deal <-> Organization    deal_org
Task or Activity <-> any activity_target
Tag <-> any object       taggings
```

## Rules

- Use an existing object, field, link or Activity before creating another representation of the same fact.
- A link represents a fact between two objects. Product language names the link; storage names use singular source and target keys ending in `_link` when that naming is repaired.
- A Person–Organization link is the source of an employment or affiliation connection. `people.company` is a duplicate tracked in [[open-discrepancies]] X41.
- A person's title and a Person–Organization link's role are separate facts. Do not copy one into the other ([[open-discrepancies]] X44).
- Derived IDs may aid reads but do not become a second source of truth. External fields map into this model at the boundary.
- User-facing behavior supports the relationship loop: find a person, understand the relationship, capture what changed, or complete a follow-up.

## Checks

- A proposed field or table names the fact it owns and cannot be expressed by an existing model element.
- A new link names both ends, its variables, lifecycle and inverse behavior.
- Every applicable door uses the canonical product term and behavior.

## Evidence

[[open-discrepancies]] records model violations. Feature work is in [[features/product-model]].
