# Contact methods

A person can have several email addresses, phone numbers and social links. Each change prints the updated person.

## Emails and phones

```sh crm-example
crm person email add 1 jane@work.example --label Work
crm person email add 1 jane@home.example --label Home --primary
crm person phone add 1 "+1 555 0100" --label Mobile
crm person email rm 1 jane@example.com
```

Removing is by value. `--primary` makes that address or number the primary one. The primary email is also the `email` field in list output.

## Social links and website

One link per kind. Setting a kind again replaces it.

```sh crm-example
crm person handle set 1 linkedin https://linkedin.com/in/janesmith
crm person handle set 1 website https://janesmith.example
crm person handle rm 1 linkedin
```

Kinds: linkedin, twitter, facebook, instagram, telegram, whatsapp, tiktok, youtube, website, other.

## Read them back

```sh crm-example
crm person show 1 -f json
```

In JSON the person has `emails`, `phones` and `handles` arrays. In the table view they appear as comma separated columns. Duplicate detection (`crm help duplicates`) and sync matching (`crm help sync`) use emails, phones and links. Keep them accurate.

See also: people, duplicates, sync.
