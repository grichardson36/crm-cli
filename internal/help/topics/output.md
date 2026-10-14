# Output formats and exit codes

How to read what crm prints, and what its exit codes mean.

## Formats

`-f` or `--format` picks the format: `table`, `json`, `csv`, `tsv`. With no flag, crm prints a table on a terminal and JSON when stdout is a pipe or file.

```sh crm-example
crm person list -f json
crm person list -f csv
crm person list -f tsv
crm person list -f table
crm person list -q
```

- `json` is an array of objects for commands that return records, even for one record. `where`, `doctor`, `init`, `schema` and `person merge` return one object. Field names are stable.
- `csv` and `tsv` have a header row of field names.
- `-q` prints only IDs, one per line.
- Help text commands (`crm help`, `crm agent`) print Markdown text unless you pass `-f json`.

## stdout and stderr

- stdout is data only. Pipe it.
- stderr is messages: confirmations, warnings, errors.

```sh crm-example
crm person list -f json
crm due -f json
```

## Exit codes

| Code | Meaning | JSON error code |
|---|---|---|
| 0 | success | |
| 1 | unexpected error (`crm doctor`: a check failed) | unknown |
| 2 | bad arguments, flags or values | usage, validation |
| 3 | the record does not exist | not_found |
| 4 | conflict with existing data | conflict |
| 10 | database could not be opened or written | database |

## Errors as JSON

With `-f json`, errors go to stderr as one JSON object. The exit code is the same.

```json
{"error":{"code":"not_found","message":"person 99: not found","exit_code":3}}
```

Codes: `not_found`, `validation`, `conflict`, `database`, `usage`, `unknown`. Without `-f json`, the error is `crm: error: MESSAGE`.

`crm schema -f json` lists the exit codes, the error format and the output fields of each command.
