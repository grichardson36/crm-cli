---
type: pattern
scale: interaction
status: decided
---

# CLI with JSON is the agent baseline

**Problem.** Agent support requires vendor-specific setup.

**Therefore.** crm ... -f json works for any agent. MCP and skills are conveniences. Every feature ships a help topic (crm help topic), a crm schema manifest entry and a palette description. Generate the skill with crm agent skill rather than hand-editing per-agent files.

**Larger patterns.** [[three-doors-one-core]]

**Smaller patterns.** [[agents-never-get-prompts]]

**Sources.** FEATURES.md rule 3

**Inspired by.** [[dex]], [[dzhng-crm-cli]], [[us]]
