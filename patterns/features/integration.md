---
type: feature-worklist
domains:
  - integration
---
# Integration work

Active work governed by [[standards/integration]]. `TODO` means not built; `PART` requires fresh code inspection before work starts.

## Import and export

- [ ] `C08` (TODO, P1): Add generic CSV import with explicit column mapping and validation.
- [ ] `C09` (TODO, P1): Define contact imports by source format and canonical mapping.
- [ ] `C10` (TODO, P2): Define social-data imports with consent and duplicate handling.
- [ ] `C12` (PART, P2): Complete export coverage for contacts, activities and relationship updates.
- [ ] `G08` (TODO, P3): Add TUI import/export only after the underlying adapters are stable.

## Sync and enrichment

- [ ] Legacy `A16` enrichment (TODO, P2): Define owner-approved LinkedIn enrichment before assigning a canonical ID. See [[open-discrepancies]] X49.
- [ ] `N02` (TODO, P2): Research permitted LinkedIn connection behavior, fields and consent.
- [ ] `N03` (TODO, P3): Define a local provider contract before adding enrichment providers.
- [ ] `N04` (TODO, P3): Define user-initiated organization enrichment before adding scraping.
- [ ] `N05` (TODO, P3): Define refresh, spending and dry-run policy before scheduling enrichment.
- [ ] `E06` (TODO, P3): Define email-sync mapping and approval behavior.
- [ ] `E07` (TODO, P2): Complete two-way address-book sync after the vCard mapping is decided.
- [ ] `E08` (TODO, P3): Define message-sync privacy, consent and activity mapping.
- [ ] `F13` (PART, P2): Verify local-first multi-device sync against the current model.
- [ ] `H02` (PART, P1): Complete an idempotent sync runner with observable failures.
- [ ] `H05` (TODO, P2): Prioritize adapters only after their contracts are accepted.
- [ ] `H06` (PART, P1): Complete hosted CardDAV only with a settled field map and archive behavior.

Blocked by [[open-discrepancies]] X23: the CardDAV mapping requires an owner decision.
