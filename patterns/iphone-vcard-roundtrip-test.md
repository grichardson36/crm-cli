---
type: pattern
status: manual checklist, to run once on a real iPhone (open question Q1 of [[research-sync-standard]])
---
# iPhone vCard round trip test

Goal: prove that the CRM does not rewrite what iOS wrote. Compare the raw vCard of one contact before and after the CRM changes a different field. Nothing here needs a code change. Use a throwaway contact, not a real one, and a test database (`--db`), never the real one.

## Setup

1. Make a test database and start the server: `crm --db /tmp/iphone-test.db dav serve --password-env CRM_DAV_PASSWORD` (see `crm help dav-server`).
2. Add it on the iPhone as a CardDAV account (Settings, Contacts, Accounts).
3. On the iPhone create a contact "Test Roundtrip" with: two phone numbers with different labels (one custom label such as "Mum's flat"), an email, a company, a job title, a birthday without a year, a note, a nickname, a related name and a URL.

## Capture before

The raw text the phone sent is stored per contact. Find the contact and print the card:

```sh crm-example-skip
crm --db /tmp/iphone-test.db search "Test Roundtrip" -f json
curl -s -u crm:$CRM_DAV_PASSWORD http://127.0.0.1:5232/dav/crm/contacts/ -X PROPFIND -H "Depth: 1" -d '<propfind xmlns="DAV:"><prop><getetag/></prop></propfind>'
curl -s -u crm:$CRM_DAV_PASSWORD http://127.0.0.1:5232/dav/crm/contacts/DAVID.vcf > before.vcf
```

`DAVID` is the file name in the PROPFIND answer (a UUID). The GET answer is the stored card text, byte for byte.

## Change something else in the CRM

Edit a field the phone did not touch, for example the job title:

```sh crm-example-skip
crm --db /tmp/iphone-test.db person edit ID --title "Head of Sales"
```

Wait for the phone to sync (or pull to refresh), then capture again:

```sh crm-example-skip
curl -s -u crm:$CRM_DAV_PASSWORD http://127.0.0.1:5232/dav/crm/contacts/DAVID.vcf > after.vcf
diff before.vcf after.vcf
```

## What must hold

1. The diff shows only the `TITLE` line.
2. `PRODID` is the iPhone's (`-//Apple Inc.//iPhone OS ...`), not `-//crm-cli//CardDAV//EN`.
3. `REV` is unchanged.
4. Every `itemN.X-ABLabel` line is still next to its `itemN.TEL` or `itemN.EMAIL`, with the same group number.
5. Every unknown `X-` property (`X-ABUID`, `X-ABADR`, related names, anything custom) is present and unchanged.
6. Folded lines (long notes) are still folded the same way.
7. On the iPhone, open the contact: nothing else changed, the custom phone label is intact.

## Then the other direction

Edit the job title on the iPhone to something else and sync. Run `crm person show ID`: the headline title is the new text and the contact still has its organization memberships. Blank the title on the iPhone: the headline clears and the next card carries the role of the primary membership again.

## Record the result

Save `before.vcf` and `after.vcf` and note the iOS version. Answer Q1 in [[research-sync-standard]]: does iOS keep PRODID, REV, `itemN.X-ABLabel` and unknown `X-` properties when it writes back? If a line differs that the CRM did not touch, iOS rewrote it: keep both files and report the diff.

Related: [[research-sync-standard]], [[sync-maps-structure-to-flat-fields]], [[phone-numbers-are-e164]].
