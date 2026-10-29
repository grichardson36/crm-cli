---
type: guide
---
# crm peers (device sync commands)

The commands for [[sync-contract]]. The code is described in [[replica-package]]. In the program, `crm help peers` is the short version.

## Setup in four steps

Pick one always-on device as the relay, for example pibrain. Every device keeps a full copy and works offline. Reach the relay over Tailscale or the LAN only. The traffic is plain HTTP with a token and no encryption.

1. On every device, on the database that holds its data: `crm peers init --name laptop`. It records the current values as a baseline without changing a row and turns on capture. Do it once. Do not copy a database after this step, because the copy would share the device id.
2. On the relay: `crm peers serve --addr 100.64.0.1:7478` (a Tailscale or LAN address). It stays running. Any enrolled device can be a relay.
3. On the relay, once per new device: `crm peers enroll --label phone`. It prints a one-time token that expires in 24 hours.
4. On the new device: `crm peers add home http://100.64.0.1:7478 --token crmenr_... --as phone`, then `crm peers sync`.

Run `crm peers sync` whenever you want, from cron or by hand. It pulls first, then pushes what the relay lacks. Several relays can be added. A device can relay for another because the cursor is a version vector.

## Commands

| Command | What it does |
|---|---|
| `peers init [--name N]` | Turns sync on. Safe to run again. |
| `peers status` | Device id, cursor, stored changes, open reviews, blocked records, known devices. |
| `peers serve --addr A [--read-only] [--allow-public]` | Runs the relay. Refuses a public or wildcard address (`0.0.0.0`, `::`, a host name) unless `--allow-public`. Loopback, LAN, link-local, Tailscale (100.64.0.0/10) and IPv6 unique-local addresses are accepted. `--read-only` serves pulls and refuses pushes and enrollments. |
| `peers enroll [--label L] [--ttl 24h]` | One-time token for a new device. Only its hash is stored. |
| `peers add NAME URL --token T [--as DEVICE_NAME]` | Joins a relay with a one-time token and remembers it with a long-lived device token. |
| `peers list [--all]` | Relays this device connects to and devices enrolled here. |
| `peers remove NAME` | Forgets a relay connection. |
| `peers pull [NAME] [--full]` | Applies new changes. `--full` ignores the local cursor and takes the relay's whole per-field state (the snapshot, for a new or reset device). Nothing applies twice. |
| `peers push [NAME]` | Sends the changes the relay lacks. |
| `peers sync [NAME] [--full]` | Pull, then push, with one relay or all. |
| `peers retire DEVICE` | Drops a lost device: revokes its tokens and stops waiting for it before tombstones are purged. It can join again with a new token. |
| `peers purge` | Removes deletes that every enrolled device has passed. Run on the relay. |
| `peers review [--all]` | Lists records one device deleted and another edited. |
| `peers resolve ID` with `--keep` or `--delete` | Decides a review item. |

Output follows `-f json`. Exit codes: 2 for sync not enabled or a refused address, 3 for an unknown device or review item, 4 for a relay that refused (bad or used token, duplicate device id).

## Review queue

When one device deletes a record and another edits it without having seen the delete, the record is parked. It stays visible with the edits on every device. `peers sync` prints the count. `peers review` lists the items. `peers resolve ID --keep` restores the record everywhere, `--delete` removes it everywhere. After either, run `peers sync` so the other devices get the decision. The MCP tools `crm_peers_status` and `crm_peers_review` are read only.

## Safety rules

- The database is opened the normal way, so the automatic backup before a migration (migration 016 adds the sync tables) applies. Take `crm backup` before the first `peers init`.
- A read-only relay never changes its data. There is no other read-only mode for pull and push: they write.
- Never run the commands against `~/.crm` in tests. Use `--db` with a temporary file.
- A blocked record (see `peers status`) is not lost: it waits for what it needs.

Related: [[replica-package]], [[local-first]], [[soft-delete-restorable]].
