# AGENTS.md

Quick start for AI agents that use `crm`. Any agent that can run a shell command can use it. No MCP, skill or vendor file is needed.

## What this is

`crm` is a local-first personal CRM. It is one binary and one SQLite file per database. It stores people, organizations, interactions (calls, emails, meetings, notes), tasks, deals, tags and relationships. No account, no network.

## Where the data lives

One database file per domain (for example personal, work). The active file is chosen in this order:

1. `--db PATH`
2. the `CRM_DB` environment variable
3. `~/.crm/crm.db`

Run `crm where` to see the active file. Never guess a path.

## The commands to run first

1. `crm where -f json` shows the active database, how it was chosen, schema version and counts.
2. `crm help` lists the built-in guides. `crm help <topic>` prints one. `crm help --search <text>` finds a word.
3. `crm schema -f json` lists every command, flag, example, exit code and MCP tool, and marks which commands change data.
4. `crm agent` prints this file. `crm agent install-prompt` prints the prompt that installs and sets up crm.
5. `crm doctor -f json` checks the installation and prints a one-line fix for each problem.
6. `crm init` creates the data directory and a database. Use `--yes` when not on a terminal. It never overwrites.
7. `crm person list -f json` reads the people. Add `--tag`, `--limit` or `--archived` to narrow it.
8. `crm search <text> -f json` searches people, organizations, interactions and deals.

If `crm` is not installed, read `INSTALL.md`.

## Safety rules

1. Use `-f json` for every command whose output you parse. Data is on stdout, messages and errors are on stderr.
2. Read before you write. Search before you create, so you do not add duplicates.
3. Run `--dry-run` first on any command that changes data. It prints what would change and writes nothing.
4. Commands that cannot be undone need `--yes` (`person purge`, `tag delete`). Ask the owner before using them. Prefer `person delete`, which can be restored.
5. Preview a merge with `crm person merge KEEP DROP --dry-run` and confirm it with the owner.
6. With `-f json`, errors are JSON on stderr: `{"error":{"code":"not_found","message":"...","exit_code":3}}`. Branch on `code`.
7. Exit codes: 0 ok, 1 unexpected error, 2 usage or validation, 3 not found, 4 conflict, 10 database.
8. Never touch the database file directly. Do not open it with sqlite3, copy it while crm runs, or delete its `-wal` and `-shm` files. Use `crm` commands. Use `crm backup DEST.db` for a safe copy.
9. Never put the database in a OneDrive, Dropbox or iCloud folder. `crm doctor` warns about it.
10. Report what you changed to the owner, with record IDs.

## Optional: MCP

`crm mcp serve` exposes the same data as MCP tools (including `crm_help` and `crm_schema`) and the help topics as resources. Everything above works without it.

## More

`README.md` is the human guide. `INSTALL.md` is the install checklist. `crm help agents` is the full guide for agents.

## Working on this repository

The `patterns/` folder is an Obsidian vault and the source of truth for design decisions and features. Open `patterns/index.md` first.

1. Read the patterns that apply before any design or feature change. Feature rows (IDs such as M27) are in the `patterns/features-*.md` notes.
2. If code breaks a pattern, or no pattern covers the question, surface it to the owner with a recommendation. Do not decide alone. The live list is `patterns/open-discrepancies.md`.
3. Log each new rule as one atomic note in `patterns/`, linked from `patterns/index.md`. Update the feature rows you change in the same session.
4. Never test on the real databases.
