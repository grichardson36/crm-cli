---
type: pattern
scale: interaction
status: decided by owner 2026-10-27, built in feat/phone-dav (migration 015)
---
# Phone numbers are stored as typed and matched as E.164

**Problem.** The same number is typed many ways: `(415) 555-2671`, `415.555.2671`, `+1 415 555 2671`. Matching on the last 10 digits mixed up countries, and rewriting the typed text breaks the phone's own copy of a contact.

**Therefore.** Every phone contact method keeps two values. `phone_number` is the text exactly as typed. `e164` is the normalized form (`+14155552671`), empty when the text cannot be read as a possible number. Matching, duplicate detection and sync use E.164. Display uses the region: national format for numbers of the default region, international format for the rest. A correction is only ever a suggestion, the typed text is never silently rewritten.

- **Library.** `github.com/nyaruka/phonenumbers` (MIT, a pure Go port of Google libphonenumber, no CGO, maintained). It adds about 10.8 MB to the binary (28.3 to 39.1 MB) for the embedded numbering plan metadata. Only `internal/phone` imports it.
- **Default region.** `[phone] region = "US"` in `~/.crm/config.toml`, per device, US when unset. `crm phone region [CODE]` shows or sets it. A number without a country code is read in that region. Changing the region does not touch stored text: run `crm phone normalize --apply` to refill the E.164 forms.
- **Key.** `dedupe.PhoneKey` is the E.164 form plus the extension (`+14155552671x23`), so two people at one switchboard stay apart. Text that cannot be parsed keeps the old key (last 10 digits, nothing under 7 digits). A number that parses but is not in the region numbering plan still gets an E.164 form.
- **Validation** (one framework, [[one-validation-framework]]). `E_PHONE_INVALID` keeps its meaning (wrong characters or 3 to 20 digits). New warning `W_PHONE_UNLIKELY`: the text parses but does not exist in the region numbering plan (for example area code 555). The write succeeds and the text is stored as typed. The suggestion `S_PHONE_IN_TEXT_FIELD` for phone-like text in other fields is unchanged.
- **Same number, other format.** Adding a number the person already has in another format updates that row (label) and keeps its typed text. Removing works by any format. Editing a number in the terminal UI removes the old row first for the same reason.
- **Backfill.** `crm phone normalize` is a dry run with counts (total, fill, current, unparseable, unlikely, duplicates on one person). `--apply` fills only the `e164` column in one transaction and is idempotent. `--undo` clears the column, the one step back. MCP: `crm_phone_normalize`, `crm_phone_parse`.
- **Sync.** The vCard TEL line carries the typed text, so the phone's copy is not rewritten ([[sync-maps-structure-to-flat-fields]], [[research-sync-standard]]).

**Because.** Owner decision 2026-10-27: use the industry standard. E.164 is what dialers, SMS gateways and libphonenumber exchange.

**Larger patterns.** [[validate-and-correct-at-every-boundary]], [[field-sources-are-ranked]]

**Sources.** ITU-T E.164, libphonenumber, `internal/phone`, `internal/dedupe/normalize.go`
