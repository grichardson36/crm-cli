# Run the phone sync server

`crm dav serve` starts the CardDAV server. It needs credentials and refuses to start without them. Give the password through an environment variable or a file, never on the command line.

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

The server speaks plain HTTP. Put a TLS-terminating reverse proxy (relayd, nginx, Caddy) in front of it and forward to the loopback address. The proxy must pass the `Authorization` header and all methods (PROPFIND, REPORT, PUT, DELETE). Run one server per database, each with its own `--addr` and `--db`.

## Android (DAVx5)

1. Install DAVx5. Tap + and choose "Login with URL and user name".
2. Base URL: your public URL followed by `/dav/` (for example `https://contacts.example.com/dav/`).
3. Enter the user name and the server password, then tap Login.
4. Tick the address book (named after the database, for example "CRM personal") under CardDAV and sync.
5. Leave "Force read-only" off to edit both ways.

iOS: Settings, Contacts, Accounts, Add Account, Other, Add CardDAV Account. macOS: Contacts, Add Account, Other Contacts Account, CardDAV, manual. If discovery fails, use the address book URL that `crm dav info` prints.

## Security

- The password is compared in constant time. A failed login waits a quarter of a second.
- There is no TLS in the server. Without a TLS proxy the password crosses the network in clear text. Do not expose the plain port.
- Use a long random password. The phone stores it.
- The phone gets every active person with emails, phones, notes and birthdays. Treat the account like the database itself.
- Nothing the phone does purges a contact. Deletes archive.

See also: dav, sync, domains.
