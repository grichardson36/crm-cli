---
type: pattern
scale: object
status: decided
---

# Objects first, verbs second

**Problem.** Features get built per object: Restore deleted task, Restore deleted deal, a separate add flow each. Users relearn every object and code multiplies.

**Therefore.** Define objects once in an object registry: type, name, fields, form, relations, list columns, record page. Define verbs once. Every verb runs on any object that supports it through one shared implementation that takes the object type as a parameter. A new object gets every verb by registering itself. A new verb is built once.

**Because.** Palette entries, keys, help and schema derive from the registry, never hand-written per object (internal/tui/objects.go holds the registry).

**Larger patterns.** [[relationship-loop-first]]

**Smaller patterns.** [[one-verb-set]], [[link-from-either-side]], [[one-record-page]], [[name-things-once]], [[three-doors-one-core]], [[one-bar]]

**Sources.** Notion, Attio and Folk share one record model across object types

**Inspired by.** [[folk]], [[attio]], [[obsidian]], [[notion]]
