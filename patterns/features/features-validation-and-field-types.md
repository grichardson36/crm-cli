---
type: feature-area
---
# Validation and field types

The field vocabulary, the one validation framework, aliases, add-with-link and bulk purge (part B1).


**Follows patterns:** [[core]], [[core]], [[core]], [[core]], [[core]]

**See also:** [[roadmap]], [[index]]

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| M23 | Part B1, branch feat/fields-editors (owner decisions 2026-10-28; closes X4) | Notion, Attio, HubSpot, Us | DONE | BRANCH feat/fields-editors (not merged, schema head 12). **Add link everywhere**: `a` offers the highlighted object (person, organization, deal, task, note) as a link chip for any new kind the relation registry joins (`model.RelationBetween`), a highlighted tag as a tag, links are pipe segments (owner decision 2026-10-29, see M25), create and link are one undo step (`afterCreate` in `internal/tui/pipelink.go`), a new person links from the inline name row.  A new tag is not in the add picker (no free letter), tags are made by linking. **Bulk purge**: Archived palette entries Purge selected forever and Purge all remaining forever, typed `purge N`, no bare key, `d` unchanged (single purge, typed name). CLI `crm purge --archived [--type person] [--ids 1,2] [--apply]` (dry run by default, `--yes` same as `--apply`), MCP `crm_purge_archived` (`apply`). Only people have a purge, other archived kinds are refused. Same tombstones as `person purge` (`PersonRepo.PurgeArchived`). **Field types**: `internal/model/fields.go` is the one vocabulary (text, longtext, date, choice, number, list, relation) with `model.Fields` per kind, `FieldSpec.Normalize` and `FieldError`. TUI `internal/tui/fieldedit.go`: org and deal pages list every field as a row, `enter`/`e` edits in place, `enter` or `tab` saves, `esc` cancels, dates take phrases ("next friday") and preview the result in grayscale, choice rows pick with up/down or a letter, `d` clears an optional field, deal stage and person priority are choice fields (palette entries removed, bulk tier gone). `internal/tui/typeahead.go`: relation rows open a typeahead (alias and typo matching, last row Create 'X', `tab` completes, `u` unlinks the focused chip). The person page keeps its rows, now with hints and the date type on birthday. `FieldSpec.Normalize` and `FieldError` moved into `internal/validate` (M24). **Shared aliases**: `internal/alias` (built-in table moved out of dedupe), migration 012 `name_aliases`, `crm alias add\|list\|remove`, MCP `crm_alias_*`; read by the duplicate finder (builtin results unchanged), `PersonRepo.Search` and `PersonFTSQuery` (each word expands one hop, so Rich finds Richard, not Dick), the bar and list filter, and the relation typeahead (which alone forgives typos through `dedupe.Ratio`). | - | Bulk tier on marked people (was palette `tier`) and the `[keys]` ids `tier` and `deal-stage` are gone. | Done in M24 | Us |
| M24 | One validation framework (owner decision 2026-10-28, patterns/one-validation-framework) | Notion, Attio, Us | DONE | BRANCH feat/validate-core (not merged, schema head 12). **Package** `internal/validate`, driven by `model.Fields` (new `FieldSpec.Format` and `Max`). `validate.Field` checks one value by field type (text, longtext, date, choice, number, list entry, relation name) and format (email, url, phone, domain), `validate.Object` checks several fields, create or partial, and the adapters `PersonCreate|Update`, `OrgCreate|Update`, `DealCreate|Update`, `TaskCreate|Update`, `InteractionCreate|Update` describe each write path. Rules: `OneOf`, `DateOrder` (link end before start), `OrgParentCycle`, `OnePrimaryPerson–Organization link` (the link repo demotes the old primary, so this rule is only a guard). Result: stable code (`validate.Codes`, 18 codes, E_ errors fail, W_ warns, S_ suggests), field, expected, received, fix, plus suggestions. **Sanitization** (trim, collapse whitespace, strip control characters and the BOM) runs on every write. U+FFFD is flagged W_ENCODING_DAMAGED with a suggestion and never rewritten. **Smart correction** (never applied): phone-like, email-like and URL-like text in a text field gives a suggestion naming the target field and the value. **Wiring**: the repositories call the adapters, so CLI, MCP, importers, DAV PUT and device sync all share the checks. CLI exit code 2, stderr `crm: error: CODE field: expected (got ...), fix`, suggestions as `crm: hint:` lines, `-f json` errors carry `issues` and `suggestions`. MCP errors are isError JSON with code, field, expected, received, fix, issues, suggestions. The Dex importer keeps every row and lists findings in the report (`validation_issues`, `validation_suggestions`). TUI field editors (`fieldedit.go`, `form.go`) call `validate.Field`. `crm help validation` lists the codes. | - | Phone E.164 normalization (extension point: `validate.IsPhone` and `Field`). The TUI shows a suggestion in the status line after a save, accepting it with `tab` is not wired because `tab` already saves in the field editors (key decision for Grant). The deal stage check in `internal/tui/orgdeal.go` is a duplicate left for the TUI branch. Importer rows with errors are reported, not rejected. | Decide the tab conflict, add E.164 | Us |

## Typed date forms

One parser, `shortsyntax.ParseDate`, reads a typed date for every door. `validate.Field` uses it by default (`Options.Now` injects the clock), so the CLI, MCP, quick bar and TUI give the same answer for a date field such as task `due`. Accepted, with today 2026-10-15:

| Form | Example | Stored |
|---|---|---|
| ISO | `2026-12-25` | `2026-12-25` |
| month/day/year or month-day-year, four-digit year | `12/25/2026`, `12-25-2026` | `2026-12-25` |
| same, two-digit year (read as 20yy) | `12/25/26` | `2026-12-25` |
| month/day, next occurrence on or after today | `12/25`, `10/14` | `2026-12-25`, `2027-10-14` |
| month name | `Dec 25`, `December 25 2026`, `25 Dec 2026`, `25 December` | as above |
| phrases | `next friday`, `in 2 weeks` | a date |

The first number is the month. A first number above 12 with a second up to 12 is day-first (`25/12/2026`). Anything else is E_DATE_INVALID: both numbers above 12, a day that does not exist, mixed separators (`12/25-2026`), month-dash-day without a year (`12-25`), a year of three or five digits. The error message example stays "for example 2026-12-01 or next friday".

A birthday keeps its own year rules (`shortsyntax.ParseBirthday`): a year is four digits and may be left out (`3/14`, `Mar 14` store `--03-14`), a two-digit year is refused. `crm person edit --birthday` and `crm_person_update` run through the same check. Not covered: interaction timestamps (`log --at`, `note --at`), which have their own parser and error.

Tests: `internal/shortsyntax/datefmt_test.go`, `internal/validate/datefmt_test.go`, `TestParseBirthday`.
