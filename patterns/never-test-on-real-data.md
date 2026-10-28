---
type: pattern
scale: product
status: decided
---

# Test on copies, never on the real databases

**Problem.** A test or preview run damages the owner's contacts.

**Therefore.** Use a copy (for example preview.db). Back up before any schema upgrade.

**Larger patterns.** [[local-first]]

**Sources.** [[feature-rules]] rule 5
