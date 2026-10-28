---
type: pattern
status: decided by owner 2026-10-28, built in feat/validate-core (not merged), refines [[validate-and-correct-at-every-boundary]]
---
# One validation framework

Validation, normalization and smart correction live in a single package driven by the field types in the schema ([[field-types-drive-editors]]). The CLI, MCP, TUI, importers, DAV and device sync all call it. No interface carries its own copy of a rule, and a rule is added once.

Each check returns a structured result: error code, field, what was expected, what was received and a suggested fix. Interfaces only decide how to show it (stderr and an exit code, MCP `isError` content, an inline message under the field).

Smart correction (a phone number typed in the company field) is also one implementation, producing suggestions that interfaces present. See [[validate-and-correct-at-every-boundary]] for the full list of checks and the open questions.

## Status (feat/validate-core)

Package `internal/validate`. `Field` checks one value by field type and format, `Object` several, the adapters (`PersonCreate`, `OrgUpdate`, `DealCreate`, `TaskUpdate`, `InteractionCreate` and so on) describe each write path, and `Sanitize` cleans text. The repositories call the adapters, so every door that writes through a repository is covered: CLI, MCP, Dex import (reports findings, keeps every row), CardDAV PUT (HTTP 400) and device sync (a record error). The TUI field editors call `validate.Field`.

Sanitization trims, collapses whitespace and strips control characters. U+FFFD is a warning with a suggestion and is never rewritten.

Smart correction, first three: phone-like, email-like and URL-like text in a text field. A suggestion names the target field and the value and is never applied.

Phone E.164 normalization is a separate roadmap item. The extension point is `validate.IsPhone` and the phone format check in `Field`: return the normalized form as `Result.Value`.

Live in pipe lines (2026-10-29): a segment closed with `|` is checked at once and the error shows inline, the segment being typed shows a soft indicator only, smart corrections show as hints ([[one-line-pipe-prompt]], [[autocomplete-component]]).

Open: accepting a suggestion with `tab` in the TUI. `tab` already saves in the field editors, so the suggestion shows in the status line and acceptance is not bound until Grant picks a key.

## Error codes

Defined once in `internal/validate/codes.go` (`validate.Codes`). Only `E_` codes fail a write. `crm help validation` lists them.

| Code | Kind | Meaning |
|---|---|---|
| E_FIELD_REQUIRED | error | a required field is empty |
| E_FIELD_TOO_LONG | error | longer than the field allows (text 500, longtext 100000) |
| E_FIELD_UNKNOWN | error | the object has no such field |
| E_EMAIL_INVALID | error | not shaped like an email address |
| E_URL_INVALID | error | not shaped like a web address |
| E_PHONE_INVALID | error | not shaped like a phone number (3 to 20 digits, phone characters only) |
| E_DOMAIN_INVALID | error | not a domain such as example.com |
| E_DATE_INVALID | error | not a date the date parser understands |
| E_DATE_ORDER | error | an end date is before its start date |
| E_CHOICE_INVALID | error | not one of the field's options |
| E_NUMBER_INVALID | error | not a number |
| E_NUMBER_TOO_LOW | error | below the field's minimum |
| E_PRIMARY_MEMBERSHIP_MULTIPLE | error | more than one primary membership for a person |
| E_ORG_PARENT_CYCLE | error | an organization would be its own ancestor |
| W_ENCODING_DAMAGED | warning | text holds U+FFFD |
| S_PHONE_IN_TEXT_FIELD | suggestion | phone-like text in a text field |
| S_EMAIL_IN_TEXT_FIELD | suggestion | email-like text in a text field |
| S_URL_IN_TEXT_FIELD | suggestion | web-address-like text in a text field |

Related: [[one-abstraction-many-contexts]].
