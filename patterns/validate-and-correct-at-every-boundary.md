---
type: pattern
status: proposed by owner 2026-10-27, to be refined after the verbs and keys phase
---
# Validate and correct at every boundary

Every program built on the framework validates data from day one, at every boundary where data enters: forms, the pipe line prompt, the command bar, the CLI, MCP tools, imports and sync. The checks are the same everywhere because they come from the schema, not from each door.

## Checks built in

- Input validation, field and form validation, type checking and constraint enforcement, driven by the field types in the schema ([[objects-first]], [[one-verb-set]]).
- Sanitization of text, such as control characters and damaged encodings.
- Schema validation of view files, keymaps and sync records.
- Business rule validation, such as one primary membership per person and no cycles in org parents.

## Errors that help

Every failure carries a stable error code, the field it concerns, what was expected, what was received and a suggested fix. The same error shape reaches the CLI (exit code and stderr), MCP (`isError` content) and the TUI (inline under the field).

## Smart correction from context

Where the data's shape points to a better place, the program suggests the correction and does not silently change it. Examples: a value in the company field that looks like a phone number, an email address typed into the name field, a title typed into the company field, a date typed as text. The suggestion is accepted with one key, following [[enrichment-is-suggested-then-accepted]] and [[show-syntax-inline-as-you-type]]. Ranked sources apply ([[field-sources-are-ranked]]).

## Open

- Which corrections may apply automatically (for example trimming whitespace and normalizing phone numbers to E.164, see `ROADMAP.md`) and which always ask.
- The shared error code vocabulary and where it lives in the framework.
- Whether corrections are logged so they can be undone as one step.
