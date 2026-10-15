# Install and first-time setup

crm is one static binary. This topic is the short version. The step-by-step guide for an agent is `INSTALL.md` in the repository.

## Install

1. Download `crm_<os>_<arch>.tar.gz` (`.zip` on Windows) from the GitHub releases page. The os is `linux`, `darwin` or `windows`. The arch is `amd64` or `arm64`. Verify it against `checksums.txt`.
2. Put `crm` (`crm.exe` on Windows) in a directory on your PATH.
3. Check it with `crm --version`.

Without a release for your system, build from source with Go 1.24 or newer: `go build -o crm ./cmd/crm`.

## Create the database

```sh crm-example-skip
crm init
crm init --domain work
crm init --import-dex-contacts dex_contacts.csv --import-dex-notes dex_notes.csv
```

`crm init` creates the data directory and a new database. It asks on a terminal and needs `--yes` otherwise. It never overwrites. With no flags it creates `~/.crm/crm.db`. `--domain NAME` creates `~/.crm/NAME.db`.

## Check the setup

```sh crm-example
crm doctor
crm where
crm person add "Sam Rivera" --email sam@example.com --keep-in-touch "1 mon"
crm log note 1 --content "Met at a conference"
crm due
```

`crm doctor` checks the binary, data directory, database, integrity, schema version, write-ahead log, backups, disk space, synced folders, sync sources and MCP. Each check is `ok`, `warn` or `fail` with a one-line fix. Exit code 1 only when a check fails. The last three commands add a first person, log a note and show who is due.

Back up before upgrading. To upgrade, replace the binary. The next command migrates the database. See `crm help backup`.

See also: backup, overview, domains, agents.
