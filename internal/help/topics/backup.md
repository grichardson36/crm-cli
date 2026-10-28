# Back up and upgrade

`crm backup DEST.db` writes a consistent copy of the active database. It is safe while other crm commands run. The destination must not exist.

```sh crm-example-skip
crm backup ~/.crm/backups/crm-2026-10-14.db
```

Never copy the `.db` file by hand while crm runs, and never delete its `-wal` or `-shm` files. Write backups into a synced folder such as OneDrive, but keep the live database outside it (see `crm help domains`).

## Upgrade

1. Replace the binary.
2. Reopen the program (any command, the terminal UI or the MCP server).

That is all. The first open that finds pending migrations copies the database to `backups/` next to it, named like `personal-pre-v13-20261029-101500.db` (old schema version, then date and time), prints one line on stderr with the path, and then migrates. The newest 5 such copies per database are kept and older ones are removed. Other files in `backups/` are never touched. If the copy fails, the migration does not run and the database is unchanged. A new or in-memory database gets no copy. When two crm processes open the same database at once (for example the terminal UI and a command), one migrates and the other waits.

A database from a newer crm than the binary is refused with `database is schema N, this build supports M, update the program`, so do not downgrade. To go back, replace the binary with the old one only after restoring a `pre-v` backup over the database file.

Run `crm doctor` afterwards to confirm the schema version and integrity.

## Check backups

`crm doctor` reports whether recent backups exist. `crm where` shows the active database path and size.

```sh crm-example
crm doctor
crm where
```

See also: install, domains, agents.
