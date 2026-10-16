# Output formats

`-f` or `--format` picks `table`, `json`, `csv` or `tsv`. With no flag crm prints a table on a terminal and JSON when stdout is a pipe or file.

```sh crm-example
crm person list -f json
crm person list -f csv
crm person list -f tsv
crm person list -f table
crm person list -q
crm due -f json
```

- `json` is an array of objects for commands that return records, even for one record. `where`, `doctor`, `init`, `schema` and `person merge` return one object. Field names are stable.
- `csv` and `tsv` have a header row of field names.
- `-q` prints only IDs, one per line.
- Help commands (`crm help`, `crm agent`) print Markdown unless you pass `-f json`.

## stdout and stderr

stdout is data only, so you can pipe it. stderr carries messages: confirmations, warnings and errors.

## Exit codes and errors

Exit codes: 0 success, 1 unexpected error, 2 usage or validation, 3 not found, 4 conflict, 10 database. `crm help errors` has the table and the JSON error format.

`crm schema -f json` lists the exit codes, the error format and the output fields of each command.

See also: errors, agents, overview.
