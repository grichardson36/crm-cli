# Back up and upgrade

`crm backup DEST.db` writes a consistent copy of the active database. It is safe while other crm commands run. The destination must not exist.

```sh crm-example-skip
crm backup ~/.crm/backups/crm-2026-10-14.db
```

Never copy the `.db` file by hand while crm runs, and never delete its `-wal` or `-shm` files. Write backups into a synced folder such as OneDrive, but keep the live database outside it (see `crm help domains`).

## Upgrade

1. Back up with `crm backup`.
2. Replace the binary.
3. Run any command. The first one that opens the database migrates it to the new schema automatically.
4. Run `crm doctor` to confirm the schema version and integrity.

A database from a newer crm than the binary fails the schema check, so do not downgrade. To go back, restore the backup you made before upgrading.

## Check backups

`crm doctor` reports whether recent backups exist. `crm where` shows the active database path and size.

```sh crm-example
crm doctor
crm where
```

See also: install, domains, agents.
