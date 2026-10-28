---
type: pattern
scale: interaction
status: decided
---

# Show what was parsed before saving

**Problem.** Silent parsing writes wrong data.

**Therefore.** The bar shows parsed tokens as chips with a preview line. The CLI offers --dry-run with JSON of the parsed result.

**Larger patterns.** [[bar-modes-by-first-word]], [[date-safety-rules]], [[same-input-style]]

**Sources.** Todoist and TickTick highlight and dismiss. DESIGN-PATTERNS.md B

**Inspired by.** [[attio]], [[todoist]], [[ticktick]]
