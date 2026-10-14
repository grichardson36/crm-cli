# Install and first-time setup

crm is one static binary. The full, step-by-step guide for an agent is `INSTALL.md` in the repository. This topic is the short version.

## Install

1. Download the archive for your OS and CPU from the GitHub releases page: `crm_<os>_<arch>.tar.gz` (`.zip` on Windows). The os is `linux`, `darwin` or `windows`. The arch is `amd64` or `arm64`. Verify it against `checksums.txt`.
2. Put `crm` (`crm.exe` on Windows) in a directory on your PATH.
3. Check it:

```sh crm-example
crm --version
```

Without a release for your system, build from source with Go 1.24 or newer: `go build -o crm ./cmd/crm`.

## Create the database

```sh crm-example-skip
crm init
crm init --domain work
crm init --import-dex-contacts dex_contacts.csv --import-dex-notes dex_notes.csv
```

`crm init` creates the data directory and a new database, asks for confirmation on a terminal, and needs `--yes` when it is not on a terminal. It never overwrites an existing database. With no flags it creates the default `~/.crm/crm.db`. `--domain NAME` creates `~/.crm/NAME.db` (see `crm help domains`).

## Check the setup

```sh crm-example
crm doctor
crm where
```

`crm doctor` checks the binary, data directory, database, integrity, schema version, write-ahead log, backups, disk space, synced folders, sync sources and MCP. Each check is `ok`, `warn` or `fail` with a one-line fix. The exit code is 1 only when a check fails.

## Back up

```sh crm-example-skip
crm backup ~/.crm/backups/crm-2026-10-14.db
```

`crm backup` writes a consistent copy and is safe while other crm commands run. The destination must not exist. Back up before upgrading crm.

## Upgrade

Replace the binary. The next command that opens the database migrates it to the new schema automatically. Run `crm doctor` afterwards. A database from a newer crm than the binary fails the schema check, so do not downgrade.

## Try it

```sh crm-example
crm person add "Sam Rivera" --email sam@example.com --keep-in-touch "1 mon"
crm log note 1 --content "Met at a conference"
crm due
```

## Related topics

`overview`, `domains`, `agents`.
