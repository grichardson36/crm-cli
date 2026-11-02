---
type: guide
---
# Feature records

Feature records state the checked-out behavior, its governing decision and one bounded next step. They do not restate core gates, supporting patterns, research or implementation history.

## Record fields

| Field | Meaning |
|---|---|
| ID | Stable code. Never reuse it. |
| State | `DONE`, `PART`, `TODO`, `SKIP` or `BEYOND`. |
| Current evidence | One factual sentence about checked-out behavior, with useful links to code, tests, research or an inspiration. |
| Decision | Governing supporting pattern or reference, or `open` when the owner must decide. |
| Next work | One bounded implementation step. |
| P | Owner-approved priority: `P1`, `P2` or `P3`. |

Existing wide rows remain readable. Convert a row to this format only when changing it.

## Lifecycle

1. **Intake.** Add a row with current evidence and one bounded next step. Set Decision to `open` when needed.
2. **Decide.** Apply [[core]]. Link the decision or record the owner question in [[open-discrepancies]].
3. **Build.** Implement the agreed step. Do not add a supporting pattern or core gate unless [[core]] says it qualifies.
4. **Verify.** Test every applicable door. Update help, schema and TUI palette details when the behavior is exposed there.
5. **Update.** Revise State, Current evidence and Next work in the same session. Do not add branch history or an implementation narrative.

## DONE

A row is DONE only when every applicable door has the agreed behavior or an omitted door is stated, appropriate tests pass, its user and agent surfaces are current, and its Current evidence describes the checked-out result.
