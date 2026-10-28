---
type: research
---
# Sync standard: structured-to-flat mapping and round-trip fidelity

Read 2026-10-21 from first-party specs and docs. Feeds [[sync-maps-structure-to-flat-fields]], [[sync-safe-data]] and open discrepancy X23. Related: [[local-first]], [[conflicts-keep-server-store-local]], [[unresolved-names-stay-text]], [[merge-repoints-links]]. Not verified: Apple iOS/iCloud behavior (no first-party doc found), Monica and Dex conflict handling (nothing found). Treat those as open and test against a real phone ([[surface-gaps-before-deciding]]).

## Findings

- S1. **A CardDAV client can only replace the whole vCard.** RFC 6352 says a client "can only change the entire vCard data via a PUT" and "will have to cache the entire set of properties on a resource that is being changed." Preserving unknown properties is the client's job. https://www.rfc-editor.org/rfc/rfc6352
- S2. **Servers must accept X- properties and may alter stored bytes.** If the stored copy is not byte-identical to the upload, the server must not return a strong ETag, so the client refetches before the next edit. Address data can also change between syncs. https://www.rfc-editor.org/rfc/rfc6352
- S3. **Clients should use If-Match with the last ETag.** On failure, reload and merge before writing again. https://www.rfc-editor.org/rfc/rfc6352
- S4. **DAVx5 keeps unknown properties.** It keeps X- properties on import and writes them back on regenerate. It owns PRODID, UID and REV, and drops SOURCE, LOGO and SOUND. https://manual.davx5.com/technical_information.html
- S5. **DAVx5 conflict policy is server wins, whole record.** Local changes are discarded when the server copy changed. No prompt, no vCard merge. https://manual.davx5.com/technical_information.html
- S6. **Regenerating a vCard from a model loses Apple extensions.** A JSContact converter recommends patching the original vCard instead, to keep `itemN.X-ABLabel` groups and custom X- properties. Apple labels are reverse-engineered, not specified. https://mirror.wayne.edu/CPAN/authors/id/B/BR/BRONG/Text-JSContact-0.01.readme
- S7. **Server-side import that maps fields and drops the rest causes data loss.** Open-Xchange ignored unmappable properties on import, then fixed it by storing the original vCard next to the mapped fields. https://documentation.open-xchange.com/8/middleware/contacts/vcard_mapping.html
- S8. **vCard structure.** ORG is structured: name then zero or more unit names. ADR has seven components and empty ones keep their separators. TITLE and ROLE are distinct. PHOTO is a URI (may be a data URI). BDAY may be a date, a partial date or free text. https://www.rfc-editor.org/rfc/rfc6350
- S9. **REV and PRODID describe the writer, not the data.** REV is a single timestamp of the current revision. PRODID names the producing product. Neither is a reliable edit-detection signal across clients. Use ETag plus our own per-field versions. https://www.rfc-editor.org/rfc/rfc6350
- S10. **Google People API replaces only the masked fields.** `updatePersonFields` is required and "all updated fields will be replaced." Updates need `metadata.sources.etag`. A stale etag returns 400 failedPrecondition and the docs say to refetch, merge and retry. Clearing semantics are not documented on that page. https://developers.google.com/people/api/rest/v1/people/updateContact
- S11. **Google models structure richly.** Organizations carry name, title, department, type, dates and a current flag, and may overlap. Addresses have structured parts plus a derived `formattedValue`. Birthday `date` may be partial (year 0 = no year) and `text` is deprecated. Photos are output only. Each field has `primary` metadata. https://developers.google.com/people/api/rest/v1/people#Person
- S12. **CRDTs merge per property.** Ink & Switch: Automerge tracks fine-grained changes. The one case a CRDT cannot resolve itself is concurrent updates to the same property of the same object, where the app or user decides. Authors found users rarely hit conflicts. https://www.inkandswitch.com/essay/local-first/
- S13. **Field-level last-writer-wins is the common default for flat records.** Each field carries its own version, the larger version wins, and deletes need tombstones so a stale peer cannot resurrect a row. Matches our rule in [[sync-safe-data]]. (Concept from the CRDT literature linked at https://crdt.tech/, no single definition page found.)

## Decisions proposed

### 1. Per-field rule (Recommended)

| Our field | vCard | Rule |
|---|---|---|
| name | N, FN | Flat. Write N components (family, given, additional, prefix, suffix) and FN. Inbound: if only FN, keep it as display name and do not split. Field-level. |
| phones | TEL (+TYPE) | List. Match items by normalized number (digits, E.164 where possible). Keep TYPE and any `itemN.X-ABLabel` group as raw. Edit changes the matched item only. Empty TEL removes only that item on the remote. |
| emails | EMAIL (+TYPE) | List. Match by lowercase address. Same item rules as phones. |
| addresses | ADR (+TYPE) | Structured to 7 components both ways. Compare per component. Never rebuild from a single formatted string. Keep LABEL and group labels raw. |
| org | ORG | Existing rule: match record, else text plus `org-review`. Write primary org name as first ORG component. Preserve unit components (2nd+) raw on the phone. |
| title / role | TITLE (ROLE) | Our membership role maps to TITLE. Write ROLE only if it was present inbound. Inbound TITLE edits update the primary membership role. Do not mix them (S8). |
| birthday | BDAY | Structured date incl. partial (no year). Inbound free-text BDAY is kept as text, not parsed. Compare normalized dates, not strings. |
| notes | NOTE | Flat text, one field. Conflict means keep both (see section 4). Do not trim or reflow. |
| photo | PHOTO | Inbound: store only if changed (hash compare). Outbound: write only when we changed it. Otherwise leave the remote PHOTO bytes alone. Size-limit large photos. |
| everything else | X-*, itemN.*, URL, IMPP, CATEGORIES, unknown | Preserve raw. Never edit. |

### 2. Avoid rewriting the phone's text (Recommended)

1. Store the last-seen raw vCard text per remote contact, next to its ETag (S1, S6, S7).
2. Outbound = parse raw, change only properties whose field version moved since last sync, serialize, PUT with If-Match (S3). Keep unchanged lines byte-for-byte, including property order, groups, folding, unknown params.
3. Do not touch PRODID, UID or REV unless the target client does (S4, S9). Many servers bump REV themselves.
4. If nothing changed locally, send nothing.
5. After a PUT without a strong ETag in the response, refetch and store the server's bytes (S2).
6. Detect "remote changed" by ETag first, then diff parsed fields against the stored raw to find which fields changed. A byte difference alone is not an edit.

### 3. Empty-field edits (Recommended)

1. Inbound field became empty: clear only that scalar field (name part, title, birthday, note) after a field-version check. For linked structure, follow the existing rule: emptying ORG never removes a membership. Same for TITLE never deleting the membership, only blanking its role.
2. Inbound list item removed (a TEL line gone): remove that item only when the previous raw contained it (explicit delete). Absence in a vCard we never saw is not a delete.
3. Outbound clear: write the property removed or the component empty, never a placeholder. For ADR keep all seven separators (S8).
4. A blank in a vCard whose parse failed is a parse error, not a clear. Skip and flag.
5. Google clearing semantics are undocumented (S10). Test before building a Google adapter.

### 4. Conflict policy (Recommended)

1. **Field-level, not record-level.** Per field (and per list item), conflict only when both sides changed the same field since last sync. Different fields merge silently (S12, S13). Record-level server-wins (DAVx5, S5) throws away unrelated edits, which is the failure the owner reported on 2026-10-19.
2. Same-field conflict follows [[conflicts-keep-server-store-local]]: keep the server value, store the local value for review. Notes keep both texts.
3. Use ETag only as a coarse "did anything change" gate and for If-Match safety. Use our own per-field versions for the decision.
4. Deletes are tombstones per [[sync-safe-data]]. A tombstoned item is not resurrected by a stale peer.

## Open

- Q1. How iOS and iCloud treat PRODID, REV, `itemN.X-ABLabel` and unknown X- properties on write-back. Test with a real iPhone and keep the before and after raw vCards.
- Q2. How Monica and Dex resolve conflicts. No first-party text found.
- Q3. Whether Google's `updatePersonFields` clears a field sent empty. Not stated in the reference page.
