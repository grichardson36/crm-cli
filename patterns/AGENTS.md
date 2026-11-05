---
type: guide
---
# Pattern language for crm

This vault is a small decision system. Load [[core]], which supplies the gates. This file routes a change to the standards that govern it. [[open-discrepancies]] records owner-blocked questions and code that breaks a governing rule.

## Domains and standards

Domains label the parts of the program a change affects. They select area standards; they do not create another decision layer.

| Domain                             | Read first                  |
| ---------------------------------- | --------------------------- |
| product, domain-model, application | [[standards/product-model]] |
| interface                          | [[standards/interface]]     |
| data                               | [[standards/data]]          |
| integration                        | [[standards/integration]]   |
| architecture, runtime              | [[standards/system]]        |
| operations                         | [[standards/operations]]    |

Most changes apply to more than one domain. Start with [[core]], identify every applicable domain, then read its standards, relevant roadmap task, discrepancy and evidence.

## Evidence and records

- Research notes in `research/`
- [[inspirations]]
- [[open-discrepancies]]
- [[completed-roadmap]]

Never test on a real database. Use a temporary database or a copy.
