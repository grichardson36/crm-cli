---
type: pattern
scale: syntax
status: decided
---

# Plain-word dates need safety rules

**Problem.** monthly in Create monthly report becomes a date.

**Therefore.** 1) Parse bare words only in task, event and log commands, never notes. 2) Show every parsed token as a highlighted span with a preview and let a key turn a span back to text. 3) Backslash escape and quoted text are no-parse zones. 4) Ambiguous words (may, sat, sun, mon) count only with a day number or when not a capitalised name. 5) --no-syntax turns parsing off.

**Larger patterns.** [[dates-are-plain-words]], [[sigils-for-things-words-for-time]]

**Smaller patterns.** [[show-parsed-chips]], [[remind-word]]

**Sources.** Todoist documents the failure. Fantastical quotes. DESIGN-PATTERNS.md A

**Inspired by.** [[todoist]], [[ticktick]], [[fantastical]]

**Note (2026-10-16).** Archived with calendar: there are no event commands on main. The rule applies to task and log commands.
