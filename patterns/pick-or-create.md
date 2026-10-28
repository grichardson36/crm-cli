---
type: pattern
scale: object
status: decided
---

# Pick or create in one step

**Problem.** The user must leave the task to create the missing object first.

**Therefore.** One picker. Type to filter existing objects. The last row is Create typed text. Enter picks or creates and returns to where the user was. The same picker serves every relation, stage moves, and name confirmation.

**Because.** No create first, then link anywhere.

**Create-on-miss checks for a match first.** Creating an organization looks for a live one with the same normalized name (case, punctuation, and/&, a leading "the" and trailing Inc or LLC ignored) or the same domain. The CLI and MCP refuse with a conflict that names the existing organization and its id unless `--allow-duplicate` or `allow_duplicate` is given. The terminal UI asks: use the existing one, create anyway, or cancel. A company review create whose name already exists links the existing organization, and so does a second create with the same name in one batch.

**In pipe lines (2026-10-29).** A name in a link segment that is no object yet, or fits several, opens this picker when the line is entered. The answer is remembered, so each name is asked once, and `esc` returns to the line with its text. A tag name is never asked: a new name is a new tag.

**Larger patterns.** [[link-from-either-side]]

**Smaller patterns.** [[create-on-miss]], [[confirm-names-in-one-batch]], [[pick-stage-from-numbered-list]]

**Sources.** HubSpot association dialog, Notion relation picker, Obsidian link autocomplete

**Inspired by.** [[hubspot]], [[obsidian]], [[notion]]
