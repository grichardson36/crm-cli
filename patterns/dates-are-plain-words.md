---
type: pattern
scale: syntax
status: decided
---

# Dates are plain words

**Problem.** A date sigil collides with @ for people.

**Therefore.** fri, tomorrow 3pm, next week, in 2w, mar 4 are parsed in task, event and log commands. Notes do not parse bare words.

**Because.** Todoist, TickTick, Fantastical and Things do this. The cost is false matches, handled by [[date-safety-rules]].

**Larger patterns.** [[sigils-for-things-words-for-time]]

**Smaller patterns.** [[date-safety-rules]]

**Sources.** DESIGN-PATTERNS.md A, B

**Inspired by.** [[todoist]], [[things]]

**Note (2026-10-16).** Archived with calendar: there are no event commands on main. Plain-word dates apply to task and log commands.
