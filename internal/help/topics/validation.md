# Validation and error codes

One validation framework checks every value from the CLI, MCP, the terminal UI, imports, CardDAV and device sync, driven by the field types in the schema. A failure exits 2 (HTTP 400 over CardDAV) and names the stable code, field, expected, received and a fix. With `-f json` the error has `issues` and `suggestions`. MCP returns the same fields as `isError` content. Branch on the code.

`crm: error: E_EMAIL_INVALID email: an email address like name@example.com (got "nope"), check for a missing @ or domain`

| Code | Meaning |
|---|---|
| E_FIELD_REQUIRED | a required field is empty |
| E_FIELD_TOO_LONG | longer than the field allows |
| E_FIELD_UNKNOWN | the object has no such field |
| E_EMAIL_INVALID | not shaped like an email address |
| E_URL_INVALID | not shaped like a web address |
| E_PHONE_INVALID | not shaped like a phone number |
| E_DOMAIN_INVALID | not a domain such as example.com |
| E_DATE_INVALID | not a date the date parser understands |
| E_DATE_ORDER | an end date is before its start date |
| E_CHOICE_INVALID | not one of the field's options |
| E_NUMBER_INVALID | not a number |
| E_NUMBER_TOO_LOW | below the field's minimum |
| E_PRIMARY_MEMBERSHIP_MULTIPLE | more than one primary membership |
| E_ORG_PARENT_CYCLE | an organization would be its own ancestor |
| W_ENCODING_DAMAGED | warning: text holds U+FFFD (damaged encoding) |
| S_PHONE_IN_TEXT_FIELD | suggestion: phone-like text in another field |
| S_EMAIL_IN_TEXT_FIELD | suggestion: email-like text in another field |
| S_URL_IN_TEXT_FIELD | suggestion: web-address-like text in another field |

Only `E_` codes fail a command. Every write trims text, collapses whitespace and strips control characters. Text with U+FFFD is kept as typed and flagged. A suggestion is never applied: the CLI prints `crm: hint: ...` on stderr and the command succeeds, so move the value yourself.

```sh crm-example
crm person add "Jane Smith" --company "+1 555 010 0100" --dry-run
```

See also: errors, output, people.
