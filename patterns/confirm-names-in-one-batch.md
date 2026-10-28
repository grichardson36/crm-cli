---
type: pattern
scale: screen
status: decided
---

# Confirm every unresolved name in one panel

**Problem.** A note names people the program cannot resolve.

**Therefore.** Resolve at type time where possible (tab on @Sa). On save, if any name is unresolved or ambiguous, show one panel listing all of them. Each row offers 1-9 candidates, c create (minimal person form prefilled), x keep as text. A applies a choice to all rows with the same name. enter saves. A single strong match is preselected. Zero or several matches default to keep as text. The default is never create.

**Because.** The note stores the original text and links as separate rows, so keep as text loses nothing. A choice is remembered for the session.

**Larger patterns.** [[pick-or-create]], [[unresolved-names-stay-text]], [[at-sign-means-person]]

**Smaller patterns.** [[unresolved-names-stay-text]]

**Sources.** Slack batch prompt, Notion and HubSpot pick at typing time (DESIGN-PATTERNS.md F)

**Inspired by.** [[slack]]
