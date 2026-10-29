# Phone numbers

Every phone number is stored twice: the text exactly as you typed it, and its E.164 form (`+14155552671`), the standard that dialers and SMS use. Matching, duplicate detection and phone sync use E.164, so `(415) 555-2671` and `+1 415 555 2671` are the same number. Your typed text is never rewritten.

## Region

A number without a country code is read in your default region, US unless you change it. The setting is per device, in `~/.crm/config.toml` as `[phone] region`. Numbers of that region show in national format, all others in international format.

```sh crm-example
crm phone region
crm phone parse "(415) 555-2671"
crm phone parse "020 7946 0958" --region GB
```

```sh crm-example-skip
crm phone region GB
```

## Existing numbers

Numbers saved before this existed have no E.164 form yet, and matching reads their text. `normalize` fills the forms. It is a dry run with counts unless you add `--apply`, changes only the E.164 column in one transaction and is safe to run again. `--undo` clears the column.

```sh crm-example
crm phone normalize
crm phone normalize --apply
```

Run it again after changing the region. The counts show numbers that cannot be parsed (they keep matching on their last 10 digits) and numbers that parse but do not exist in the region.

## Checks

`E_PHONE_INVALID` fails a write: wrong characters or fewer than 3 or more than 20 digits. `W_PHONE_UNLIKELY` is a warning: the number is shaped right but is not in the region numbering plan, for example area code 555. It is stored as typed. Phone-like text in another field gives a hint to move it, never an automatic change.

The same number added again in another format updates the existing row (its label) and keeps the text it was first typed with.

See also: contact-methods, validation, duplicates, dav.
