# Phone sync (CardDAV)

`crm dav serve` makes this database look like a CardDAV address book. A phone connects to it as an account and edits flow both ways. All active people sync. Archived and merged people are removed from the phone. One address book per database.

```sh crm-example
crm dav info
crm dav info --base-url https://contacts.example.com --user grant
crm dav status
```

`dav info` prints the server URL, user, address book URL and the phone setup steps. `dav status` shows how many contacts are exposed, when a device last asked, fetched and edited, and the conflict counts. To start the server and set up Android or iOS, see `crm help dav-server`.

## What syncs

Name, emails and phones with labels, company, title, notes, birthday (`--MM-DD` when the year is unknown), website, social profiles and location. A location is one text field here, stored whole in the street part of one address. If the phone splits it, the CRM gets the parts joined with commas.

Stays in the CRM only: priority tier, tags, keep-in-touch cadence, last contacted, interactions, notes timeline, custom fields. Anything else the phone sends (photo, nickname, extra addresses, IM, categories) is kept and sent back unchanged.

## Edits and conflicts

- A new phone contact creates a person. If an existing person has the same email, phone, social link or normalized name, the contact is linked to that person instead and logged.
- Deleting a contact on the phone archives the person. `crm person restore ID` brings them back.
- Each field merges on its own against what the phone last held. A change on one side only is taken. If both changed the same field, the newest change wins. The phone's change counts as made when the server receives it.
- Emails, phones and links merge as sets. A removal on the phone removes only what the phone had been shown.
- Every clash is logged and can be undone, which restores the value that lost.

```sh crm-example
crm dav conflicts
crm dav conflicts list --all
```

```sh crm-example-skip
crm dav conflicts show 1
crm dav conflicts undo 1
```

Undo fails if the field changed again since. Rows with field `_link` record a phone contact attached to an existing person and have nothing to undo.

See also: dav-server, sync, archive, duplicates, contact-methods.
