---
type: reference
---
# App operations

The primary deployment is one local binary with one local SQLite database. Migrations run through the application. Backups use `crm backup`; users should not manipulate a live database file directly.

Optional services are CardDAV for device contact sync and the replica HTTP relay for device-to-device sync. The relay has no transport encryption of its own and is intended for a trusted LAN or Tailscale unless explicitly exposed.
