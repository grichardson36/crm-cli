---
type: pattern
scale: interaction
status: decided
---

# Unresolved names stay text

**Problem.** Auto-created people pollute the contact list.

**Therefore.** Never auto-create a person from a typed name. Unresolved text stays plain until the user picks or creates. Exception (owner 2026-10-27): a company text creates or reuses an organization, with an ignore list for junk and merge candidates for near names ([[convert-text-to-records-in-review-passes]]).

**Larger patterns.** [[confirm-names-in-one-batch]]

**Smaller patterns.** [[confirm-names-in-one-batch]], [[convert-text-to-records-in-review-passes]] (the company exception)

**Sources.** Nobody auto-creates people (DESIGN-PATTERNS.md F21)

**Inspired by.** [[github]], [[notion]]
