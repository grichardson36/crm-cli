---
type: pattern
scale: interaction
status: decided
---

# Three doors, one core

**Problem.** A function exists in one door but not the others, or uses different names.

**Therefore.** One core (repo plus rules) owns behaviour. CLI, MCP and TUI are thin doors with the same noun and verb names, defaults and side effects. A feature is not DONE until every door that applies exists and is tested.

**Because.** See X14 and X15 in [[open-discrepancies]].

**Larger patterns.** [[objects-first]]

**Smaller patterns.** [[tui-parity-for-daily-actions]], [[cli-json-is-the-baseline]], [[agents-never-get-prompts]], [[tests-for-every-door]]

**Sources.** FEATURES.md rules 1 to 3, git porcelain over plumbing

**Inspired by.** [[jdanielnd-crm-cli]]
