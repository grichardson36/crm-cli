# Overview

crm is a local-first personal CRM for the terminal: one binary, one SQLite file per database, no account and no network.

## What it stores

- People, with emails, phone numbers, social links, a keep-in-touch cadence and a birthday.
- Organizations, deals, tasks and tags.
- Interactions: calls, emails, meetings, messages and notes, each linked to one or more people.
- Relationships between people.

Every record has an integer ID for typing and a UUID for syncing. Deleting a person archives them, so it can be undone.

## First look

```sh crm-example
crm where
crm status
crm person list
crm search jane
crm due
```

## Conventions

- Data goes to stdout and human messages go to stderr, so `crm ... | jq` works.
- `-f json` makes any command print JSON.
- Commands that change data accept `--dry-run`. Commands that cannot be undone need `--yes`.
- `crm <command> --help` shows the flags and examples of one command. `crm help` lists every topic and `crm schema -f json` lists every command.

See also: people (add, find, edit), keep-in-touch (who is due), notes (log what happened), output (formats, exit codes), agents (safe use by an AI agent), install.
