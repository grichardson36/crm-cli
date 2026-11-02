---
type: pattern
status: decided
core-gate: 7
domains: [data, integration, operations]
---
# Reversible records

**Rule.** Record changes preserve identity and remain recoverable.

**Applies when.** Creating, updating, deleting, merging, importing or syncing data under [[core]] gate 7.

**Default.** Validate at entry, preserve stable identity, soft-delete before purge and retain enough state for undo or sync.

**Exception.** A permanent purge writes the required tombstone.

**Check.** A changed or deleted record can be identified, validated and recovered or reconciled safely.
