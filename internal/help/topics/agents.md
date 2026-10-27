# Working safely as an AI agent

The plain CLI with `-f json` is enough: no MCP, skill or vendor file is required. Orient, read, preview, then write.

```sh crm-example
crm where -f json
crm doctor -f json
crm status -f json
crm schema -f json
crm search jane -f json
crm context 1 -f json
```

- `crm where` names the database you are about to touch. Never guess a path.
- `crm schema -f json` lists every command, flag, example, which commands change data (`mutates`), which cannot be undone (`destructive`), exit codes and the error format.
- `crm agent` prints the quick start. Search before you create.

## Preview every change

Each command that changes data accepts `--dry-run`. It checks that the records exist, prints what would run and writes nothing. Exit code 3 means a target does not exist. `person merge`, `import dex` and `sync run` report real counts.

```sh crm-example
crm person edit 1 --title CTO --dry-run -f json
crm person delete 3 --dry-run -f json
crm person merge 1 3 --dry-run -f json
crm tag delete vip --dry-run -f json
```

`person purge` and `tag delete` need `--yes`. Ask the owner first, show the dry-run, then run. Prefer archive over purge. Merged fields stay combined, so preview and confirm a merge.

## Errors

Read stderr with `-f json` and branch on `error.code`: `not_found` (search again, do not create blindly), `validation` or `usage` (fix arguments), `conflict` (show the owner), `database` (run `crm doctor`, no retry loop).

## Habits

Never touch the database file: no `sqlite3`, no copying while crm runs, no deleting `-wal` or `-shm`. Use `crm backup DEST.db`. Log with `crm log` and a clear subject, then set a task. Keep `summary` short and append. Run `crm dupes` after bulk changes. Report changes to the owner with record IDs. `crm mcp serve` exposes the same data as MCP tools and the topics as `crm://help/TOPIC` (optional).

See also: short-syntax, errors, output, duplicates, archive, install.
