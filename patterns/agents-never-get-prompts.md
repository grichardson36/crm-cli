---
type: pattern
scale: interaction
status: decided
---

# Agents get structured output, never prompts

**Problem.** Interactive confirmations block scripted callers.

**Therefore.** Non-interactive calls exit 2 with candidates in JSON when input is ambiguous. Flags such as --link-new and --keep-unresolved answer the question up front. Support --dry-run on every write.

**Larger patterns.** [[three-doors-one-core]], [[cli-json-is-the-baseline]]

**Sources.** DESIGN-PATTERNS.md recommendation F

**Inspired by.** [[us]]
