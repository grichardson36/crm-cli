---
type: pattern
scale: syntax
status: decided, owner to confirm
---

# @ means a person

**Problem.** @ was a date marker, but Slack, GitHub, Notion and HubSpot all use it for people.

**Therefore.** @Name links a person. +Name stays as a silent alias. @ followed by text that resolves to no person stays text, so jane@example.com and @home are safe. A name that matches several people yields needs_choice.

**Larger patterns.** [[sigils-for-things-words-for-time]]

**Smaller patterns.** [[confirm-names-in-one-batch]]

**Sources.** DESIGN-PATTERNS.md A recommendation

**Inspired by.** [[hubspot]], [[super-productivity]], [[slack]], [[github]]
