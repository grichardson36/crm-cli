# Working safely as an AI agent

Rules for an agent that reads or changes someone's contacts. The plain CLI with `-f json` is enough: no MCP, skill or vendor file is required.

## Orient first

```sh crm-example
crm where -f json
crm doctor -f json
crm schema -f json
crm status -f json
```

- `crm where` says which database you are about to touch. Never guess a path.
- `crm doctor` says whether the installation is healthy.
- `crm schema -f json` lists every command, flag, example, which commands change data (`mutates`), which cannot be undone (`destructive`), exit codes and the error format.
- `crm agent` prints the quick start. `crm help` lists the guides.

## Read before you write

```sh crm-example
crm person list -f json
crm search jane -f json
crm context 1 -f json
```

Search before you create, so you do not add a duplicate.

## Preview every change

Each command that changes data accepts `--dry-run`. It checks that the records exist, prints what would run and writes nothing.

```sh crm-example
crm person edit 1 --title CTO --dry-run -f json
crm person archive 3 --dry-run -f json
crm person merge 1 3 --dry-run -f json
crm tag delete vip --dry-run -f json
```

Dry-run exit code 3 means a target record does not exist. `person merge`, `import dex` and `sync run` report real counts in their dry runs. Other commands report the command, arguments, flags and targets.

## Confirm destructive actions with the owner

Commands that cannot be undone require `--yes`: `person purge` and `tag delete`. Ask the owner first. Show them the dry-run, then run with `--yes`. Prefer archive over purge. Merge is reversible only by restoring the archived people, and the combined fields stay combined, so preview it and confirm it.

## Handle errors by code

Pass `-f json` and read stderr. Branch on `error.code`, not on the message.

| error.code | Do this |
|---|---|
| not_found | Search again. Do not create a record blindly. |
| validation, usage | Fix the arguments. Check `crm schema -f json` or `crm <command> --help`. |
| conflict | Something already exists. Show it to the owner. |
| database | Run `crm doctor`. Do not retry in a loop. |

## Never touch the database file

Do not open the `.db` file with `sqlite3` or any other tool, do not copy it while crm runs, and do not delete `-wal` or `-shm` files. Use `crm` commands only. For a safe copy use `crm backup DEST.db`.

## Habits that keep data good

- Log what happened with `crm log` and a clear subject, then set the next step as a task.
- Keep `summary` on a person short and factual. Append, do not overwrite.
- Use tags for events and groups. Use `crm dupes` after bulk changes.
- Report to the owner what you changed, using record IDs.

## Optional: MCP

`crm mcp serve` exposes the same data as MCP tools, including `crm_help` and `crm_schema`, and the help topics as resources named `crm://help/TOPIC`. It is a convenience. Everything above works without it.
