---
type: pattern
scale: product
status: decided
---

# Extend in ascending order of cost and risk

**Problem.** Plugins arrive first and run code with user permissions.

**Therefore.** Order: saved views and columns, templates and playbooks, CLI and MCP, read-first change feed, explicit opt-in plugins and hooks. Use data and instructions before code.

**Because.** Extensions never show as default navigation.

**Larger patterns.** [[local-first]], [[advanced-objects-off-main-path]]

**Smaller patterns.** [[advanced-objects-off-main-path]], [[modules-are-user-arranged]]

**Sources.** CRM-UX-RESEARCH.md R5, Pi skills versus extensions

**Inspired by.** [[salesforce]], [[dzhng-crm-cli]], [[obsidian]], [[pi]]
