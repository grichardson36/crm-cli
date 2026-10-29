# Device sync (peers)

`crm peers` keeps this database in step with your other devices, one field at a time. Each device holds a full copy and works offline. One device runs a relay, the others sync with it. Two edits to different fields of one record both stay. Two edits to the same field resolve to the latest (notes keep both).

```sh crm-example
crm peers status
crm peers init --name laptop
```

`peers init` gives the database a sync identity and records the current values as a baseline. It changes no row. Run it once per device, before you copy or share the database, never after.

## Set up a relay and a device

```sh crm-example-skip
crm peers serve --addr 100.64.0.1:7478
crm peers enroll --label phone
crm peers add home http://100.64.0.1:7478 --token crmenr_0123 --as phone
crm peers sync
```

1. `serve` runs the relay. Use a Tailscale or LAN address. The traffic is plain HTTP with a token and no encryption. A public or wildcard address (`0.0.0.0`, `::`, a host name) is refused unless `--allow-public`.
2. `enroll` prints a one-time token for a new device. It works once and expires in 24 hours.
3. `add` on the new device joins with that token. `sync` pulls, then pushes.

## Day to day

`crm peers sync` pulls, then pushes. `pull --full` takes the relay's whole state, for a new or reset device, and nothing applies twice. `list` and `remove` manage relays. `retire DEVICE` drops a lost device so it no longer holds back `purge`, which removes deletes every enrolled device has passed.

## Delete versus edit

If one device deletes a record that another edited, the record stays visible with the edits and waits in the review queue. Nothing is lost.

`crm peers review` lists the items. `crm peers resolve 3 --keep` restores the record on every device, `--delete` removes it everywhere. Then run `crm peers sync`.

## What syncs

People, organizations, notes and calls, deals, tasks, tags and what they are on, contact methods, custom fields, links between people, organizations and deals, relation kinds, deal roles and stages, note types, name aliases, ignored company texts and dismissed duplicates. It does not sync source and CardDAV state, the merge undo log or device settings. A field a newer build added is kept, not applied, until this build learns it.

See also: sync, dav, backup, archive.
