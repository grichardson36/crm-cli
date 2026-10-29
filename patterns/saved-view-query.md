---
type: pattern
scale: object
status: built, owner request 2026-10-29 (branch feat/saved-views)
---

# The saved view query is neutral data

**Problem.** A saved view must run in the CLI, MCP and the terminal UI, travel in a TOML file and sync between devices. Raw SQL, or a text grammar only one door parses, would tie it to one of them and let hostile input reach the database.

**Therefore.** A query is declarative data: `filter` (a list of conditions, AND across them), `sort` (one or two keys), `group_by` with a date `bucket`, `columns` and `search`. A condition is `{field, op, values}` and holds when the field matches any of the values (none of them for the negative operators). The fields of a kind come from the field-type registry (`model.Fields`) plus a few computed ones (last contacted, status, outcome, created, updated). The field type, called the class here, decides the operators ([[field-types-drive-editors]]):

| Class | Operators | Notes |
|---|---|---|
| text | `is`, `is_not`, `contains`, `empty`, `not_empty` | ignores case, `%` and `_` are literal. A list field (email, phone) matches any entry |
| date | `before`, `after`, `within_last N`, `in_next N`, `empty`, `not_empty` | values: `2026-12-01`, `today`, `90 days ago`, `in 30 days`, `-90d`, `next friday`. Resolved when the view runs |
| choice | `is`, `is_not`, `in` | an option of the field, or a stage of the stage list |
| number | `=`, `<`, `>`, `between` | `$` and thousands separators ignored |
| relation | `linked_to`, `not_linked_to`, `has_any` | stored as uuids, typed as a name, id or uuid |
| tag | `has`, `does_not_have` | a parent tag also matches its children ([[nested-tags]]) |

A birthday is a date without a year: `within_last` and `in_next` compare month and day, so they cross the new year. `before` and `after` on it are refused. Sort keys are single-valued fields, empty values sort last. A group is a text, choice, date, number, relation or tag field. A date group is bucketed by day, week (Monday) or month. Groups are ordered sensibly: dates by time, choices in option order (deal stages in stage order), numbers by value, text alphabetically, empty last. A descending sort on the group field reverses the groups.

**One package.** `internal/query` validates (codes live in `internal/validate`, one place), compiles to one parameterized SELECT and runs it. Every identifier in the SQL comes from the field catalog of the package. Every value from the query is an argument, never part of the SQL text. Limits: 20 conditions, 50 values, 12 columns, 2 sort keys, 200 characters per value. An unknown kind, field or operator, an operator the field type does not take, a malformed value, a bad sort or group fails with an `E_QUERY_*` code. The text form on the command line is `FIELD OPERATOR VALUE|VALUE` (`crm views save --where`).

**Because.** One neutral form works in every door, exports to a small file and cannot carry SQL. The class table is the only place that says which operator fits which type, and the editor in the terminal UI reads the same table.

**Larger patterns.** [[saved-views]], [[research-view-module-standard]], [[one-validation-framework]]

**Smaller patterns.** [[grouped-lists-with-dividers]], [[field-types-drive-editors]]

**Sources.** Owner request 2026-10-29. `internal/query`.
