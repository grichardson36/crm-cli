---
type: feature-worklist
domains:
  - data
---
# Data work

Active work governed by [[standards/data]]. `TODO` means not built; `PART` requires fresh code inspection before work starts.

- [ ] Legacy `M23` fields and editors (PART, P1): Verify the unmerged branch before assigning a canonical ID. See [[open-discrepancies]] X49.
- [ ] `M24` (PART, P1): Verify the unmerged shared-validation branch across every write door before treating it as current behavior.
- [ ] `E164` (TODO, P1): Store a normalized phone form for matching while preserving original text for display and vCard round trips.

Blocked by [[open-discrepancies]] X43 and X45: canonical identifiers and the one migration plan need owner resolution before work begins.
