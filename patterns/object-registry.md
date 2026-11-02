---
type: pattern
status: decided
core-gate: 4
domains: [domain-model, application, architecture]
---
# Object registry

**Rule.** Each CRM object is declared once and shared behavior derives from that declaration.

**Applies when.** Adding an object, field, relation, editor, list or record page under [[core]] gate 4.

**Default.** Add the fact to the existing object, field or link registry before adding a separate implementation.

**Exception.** A fact that cannot be expressed by the existing model needs an owner decision first.

**Check.** The same object definition drives its names, validation and available behavior in every applicable door.
