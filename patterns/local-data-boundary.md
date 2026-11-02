---
type: pattern
status: decided
core-gate: 3
domains: [product, data, architecture, runtime, operations]
---
# Local data boundary

**Rule.** CRM data remains usable and owned locally without an account or network.

**Applies when.** Adding storage, sync, authentication, hosted services or integrations under [[core]] gate 3.

**Default.** Local files and the binary provide the complete primary workflow. Network services are optional adapters.

**Exception.** A feature may require a remote source only for the value that source uniquely provides.

**Check.** The user can read, edit, export and back up CRM data while offline.
