---
type: pattern
scale: object
status: decided
---

# Add and edit use the same form for every object

**Problem.** People, tasks, notes, orgs and deals each take input a different way.

**Therefore.** Edit opens the same fields in the same order as Add, prefilled. Inline short syntax is available wherever it exists. A field the user empties is cleared. A save that fails reopens the form with the text kept and the error shown.

**Because.** The pipe line (see [[one-line-pipe-prompt]]) is one allowed style for small objects, the person form question (old X4) is closed by [[field-types-drive-editors]].

**Larger patterns.** [[one-verb-set]]

**Smaller patterns.** [[one-line-pipe-prompt]], [[show-parsed-chips]]

**Sources.** HubSpot and Pipedrive create and edit forms
