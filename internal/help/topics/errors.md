# Exit codes and errors

Every command exits with a code that says what went wrong. With `-f json`, errors go to stderr as one JSON object and the exit code is the same. Branch on `error.code`, not on the message.

| Code | Meaning | JSON error code |
|---|---|---|
| 0 | success | |
| 1 | unexpected error (`crm doctor`: a check failed) | unknown |
| 2 | bad arguments, flags or values | usage, validation |
| 3 | the record does not exist | not_found |
| 4 | conflict with existing data | conflict |
| 10 | database could not be opened or written | database |

```json
{"error":{"code":"not_found","message":"person 99: not found","exit_code":3}}
```

Without `-f json`, the error is `crm: error: MESSAGE`.

## What to do

- `not_found`: search again. Do not create a record blindly.
- `validation`, `usage`: fix the arguments. See `crm <command> --help` or `crm schema -f json`.
- `conflict`: something already exists. Show it to the owner.
- `database`: run `crm doctor`. Do not retry in a loop.

A dry run (`--dry-run`) exits 3 when a target record does not exist.

```sh crm-example
crm person delete 3 --dry-run -f json
```

See also: output, agents, install.
