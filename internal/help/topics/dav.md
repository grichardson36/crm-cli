# Phone sync (CardDAV)

`crm dav serve` makes this database look like a CardDAV address book. A phone connects to it as an account and edits flow both ways. All active people sync. Archived and merged people are removed from the phone. One address book per database.

## Start the server

The server needs credentials and refuses to start without them. Give the password through an environment variable or a file, never on the command line.

```sh crm-example-skip
crm dav serve --password-env CRM_DAV_PASSWORD
crm dav serve --password-file ~/.crm/dav-password --user grant --base-url https://contacts.example.com
crm dav serve --password-env CRM_DAV_PASSWORD --read-only
```

- `--password-env CRM_DAV_PASSWORD` reads the password from that environment variable, which you export first.
- `--addr` defaults to `127.0.0.1:5232`. A non-loopback address is refused unless you pass `--allow-remote`.
- `--user` defaults to `$CRM_DAV_USER`, then `crm`.
- `--read-only` rejects PUT and DELETE with 403 and tells the phone the book is read-only.
- `--base-url` only changes the URLs the start-up lines and `crm dav info` print.
- Request lines go to stderr. They never include credentials.

The server speaks plain HTTP. Put a TLS-terminating reverse proxy (relayd, nginx, Caddy) in front of it and forward to the loopback address. The proxy must pass the `Authorization` header and all methods (PROPFIND, REPORT, PUT, DELETE). Run one server per database. Use a different `--addr` and `--db` for each.

## Connection info

```sh crm-example
crm dav info
crm dav info --base-url https://contacts.example.com --user grant
crm dav info -f json
crm dav status
```

`dav info` prints the server URL, user, address book URL and the steps below. `dav status` shows how many contacts are exposed, when a device last asked, last fetched and last edited, and the conflict counts.

## Android (DAVx5)

1. Install DAVx5.
2. Tap + to add an account and choose "Login with URL and user name".
3. Base URL: your public URL followed by `/dav/` (for example `https://contacts.example.com/dav/`).
4. Enter the user name and the server password, then tap Login.
5. Tick the address book (named after the database, for example "CRM personal") under CardDAV and sync.
6. Leave "Force read-only" off to edit both ways.

iOS: Settings, Contacts, Accounts, Add Account, Other, Add CardDAV Account. macOS: Contacts, Add Account, Other Contacts Account, CardDAV, manual. If discovery fails, use the address book URL that `crm dav info` prints.

## What syncs

Name, emails and phones with their labels, company, title, notes, birthday (`--MM-DD` when the year is unknown), the website, social profiles and the location. A location is one text field here, so it is stored whole in the street part of one address. If the phone splits it into street, city and so on, the CRM gets the parts joined with commas.

Stays in the CRM only: priority tier, tags, keep-in-touch cadence, last contacted, interactions and notes timeline, custom fields.

Anything else the phone sends (photo, nickname, extra addresses, IM, categories) is kept and sent back unchanged.

## Edits and conflicts

- A new contact on the phone creates a person. If an existing person has the same email, phone or social link, or the same normalized name, the contact is linked to that person instead and logged.
- Deleting a contact on the phone archives the person. `crm person restore ID` brings them back.
- Each field merges on its own against what the phone last held. A change on one side only is taken. If both sides changed the same field, the newest change wins. The CRM's change time is recorded when the person is edited. The phone's change counts as made when the server receives it.
- Emails, phones and links merge as sets. Additions from either side are kept. A removal on the phone removes only what the phone had been shown.
- Every clash is logged and can be undone, which restores the value that lost.

```sh crm-example
crm dav conflicts
crm dav conflicts list --all
crm dav conflicts list -f json
```

```sh crm-example-skip
crm dav conflicts show 1
crm dav conflicts undo 1
```

Undo fails if the field changed again since. Rows with field `_link` record a phone contact attached to an existing person. They have nothing to undo.

## Security

- Credentials are required, and the password is compared in constant time. A failed login waits a quarter of a second.
- There is no TLS in the server. Without a TLS proxy the password crosses the network in clear text. Do not expose the plain port.
- Use a long random password. The phone stores it.
- The phone gets every active person with their emails, phones, notes and birthdays. Treat the account like the database itself.
- Contacts are archived, never purged, by anything the phone does.

## Related topics

`sync`, `archive`, `duplicates`, `contact-methods`, `birthdays`.
