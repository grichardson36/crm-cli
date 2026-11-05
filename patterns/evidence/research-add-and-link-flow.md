---
type: research
question: "How do mature products let a user add and link anything fast, correct and fuzzy, and what does that mean for tags, aliases and organization links?"
domains: [interface, product-model]
informs: ["[[standards/interface]]", "[[standards/product-model]]", "[[core]]"]
status: draft
last-reviewed: 2026-11-05
---
# Research: how products add and link anything fast, correct and fuzzy

Date: 2026-11-05. Read-only research. No decision is made here.

Evidence quality words: first-party = read at the owner's official page or API reference this session. code = read in source or the program. experiment = run by us. search = search snippet or unverified page. gap = looked and found nothing. "Not documented" means gap.

Coverage gaps (read first):
- G1. Not documented in what I could fetch: Notion `[[`, Notion person or page not found behaviour, Notion select option comma and nesting rules.
- G2. Attio and Folk: no page found for inline "create new" in a record picker. Attio's API does create on no match.
- G3. Dex: nickname use is only a search snippet. No tag character rules, no Related semantics, no group nesting page.
- G4. Things: no tag name rules, no statement on whether a parent tag filter shows children.
- G5. Linear docs/labels returned 404, so the label-groups changelog is used. Raycast, Todoist label rules and Salesforce lookup "New" come from search snippets.
- G6. Obsidian search page returned 404. Tag search comes from the tags page. Whether an alias can be a tag is not documented.
- G7. Superhuman covers command search only, not record linking.
- G8. Our program facts below were supplied by the owner and not re-read in code for this note.

## Decision this informs

The owner decision on one add-and-link flow, tag character rules, and tag versus organization link (X41 in [[records/open-discrepancies]]). Related: [[research-relationship-models]], [[standards/interface]], [[standards/product-model]], [[core]]. Product notes: [[obsidian]], [[notion]], [[attio]], [[folk]], [[dex]], [[hubspot]], [[things]], [[todoist]], [[raycast]], [[linear]], [[slack]], [[superhuman]], [[salesforce]].

## Scope

Answers five questions: inline syntax and pickers, tag rules, aliases, fact versus label, and one common flow. Out of scope: sync, import, AI parsing, keybindings (see [[research-keybindings]]), visual design.

Our current facts (owner supplied, G8):
- Tags are flat names with optional "/" nesting.
- A People filter `#work` should match `work/client` but not `work/partner`.
- 219 of 261 personal tags contain spaces or special characters.
- 96 percent of company-name tags duplicate an existing Person-Organization link.
- People already have name aliases (internal/tui, table `name_aliases`).

## Findings

### Q1. Inline syntax, pickers and resolution

- F1. Obsidian: typing `[[` opens an autocomplete of notes. A link to a missing note is allowed, and following it creates the note at that path. `[[Target|Shown]]` separates target from display text. Paths resolve from the vault root. Filenames should avoid `# | ^ : %% [[ ]]`. first-party (obsidian.md/help/links).
- F2. Obsidian quick switcher: matches name or alias, subsequence fuzzy (`dn` matches "Daily notes"). Enter on no match creates the note. Shift+Enter creates with the exact typed name when similar notes exist. first-party (obsidian.md/help/plugins/quick-switcher).
- F3. Notion: `@` opens a menu for a person, page or date. Mentioning a page creates a link. `@remind` sets a reminder. Not-found and create-new behaviour: gap. Relation property: search and pick pages, limit 1 or no limit, optional two-way. Create from the picker: gap. first-party (notion.com/help/guides/reminders-and-mentions, /relations-and-rollups). Multi-select: type a name and press Enter to create an option. Comma handling: gap. first-party (notion.com/help/database-properties).
- F4. Todoist quick add: `#` project, `%` label (`@` still works until end of 2026), `/` section, `+` assignee, `!` reminder, `{date}` deadline, `p1-p3`. For project and label "you can pick an existing ... or create a new one". Multi-word names in quick add: gap. A date false match (the word "monthly") is undone by clicking it. first-party (todoist.com/help/articles/use-task-quick-add-in-todoist-va4Lhpzz). A label that does not exist offers Create after typing the full name. search.
- F5. Slack: `@` picker. If several members match a typed display name, the name gets a dotted box and the user picks one. first-party (slack.com/help/articles/205240127). Stored form is an ID: `<@U012AB3CD>` and `<#C123ABC456>`. Slack recommends IDs because names change, and deprecated automatic parsing of typed `@name`. first-party (docs.slack.dev/messaging/formatting-message-text). The non-member prompt after send is in [[slack]].
- F6. HubSpot: `@` in a note, call or task picks a team member by name or email. first-party (knowledge.hubspot.com/records/mention-a-user-on-a-record). Associations: "+Add [object]" on the sidebar card, search and tick existing records, or the "Create new" tab to create and link in one step. After selection, an optional label, and "Set as primary company". first-party (knowledge.hubspot.com/records/associate-records).
- F7. Attio API: a record-reference value may carry `target_record_reference` with `target_object`, `match` (for example company `domains`), `if_unmatched: create`, `if_multiple_matches: error`, and `create` values. The docs example links a person to a company by domain, creates "Attio" when unmatched, and errors on several matches. first-party (docs.attio.com upsert-a-record, first 100000 characters read). Inline create in the UI picker: gap (G2).
- F8. Salesforce lookup: typing filters recent items first, then shows matching records not recently used. A "New" action exists in the Lightning lookup dropdown, but is missing in `lightning-input-field` and the Spring '25 Advanced Search modal. search (help.salesforce.com snippets).
- F9. Raycast: aliases are exact, not fuzzy, and rank first on a full match. Typing alias plus space focuses the first argument. Quicklinks take up to 3 `{argument}` fields. Apps use fuzzy search. search (manual.raycast.com snippets).
- F10. Superhuman: matches title and aliases with a 0 to 1 score, best wins, a threshold filters, results show "Title (alias)", and it tracks the last used item in some lists. first-party (blog.superhuman.com/how-to-build-a-remarkable-command-palette).
- F11. Things: open a to-do, click Tags, type a name and press Return to create a tag. On Mac, typing a tag name in Quick Find selects it. first-party (culturedcode.com/things/support/articles/2803581). Linear: see [[linear]].

Pattern across products: a sigil or open bracket starts a picker, the typed text only filters, the pick stores an ID or exact target, and "create" is a visible row or keystroke, not a silent side effect (Obsidian Enter, HubSpot Create new tab, Todoist create option). Attio's API is the one explicit unmatched policy (`if_unmatched`, `if_multiple_matches`).

### Q2. Tag rules

- F12. Obsidian tags: letters, numbers, `_`, `-`, `/` and Unicode including emoji. At least one non-numeric character. "Tags can't contain blank spaces." Case-insensitive, first-created casing is displayed. Nested with `/`. Search `tag:inbox` matches `#inbox` and children such as `#inbox/to-read`, and Bases `file.hasTag("a")` matches `#a` and `#a/b`. first-party (obsidian.md/help/tags). Whether `tag:inbox/to-read` excludes siblings is implied, not stated.
- F13. Todoist: all special characters are allowed in project, section and label names, with leading and trailing whitespace trimmed. Older versions replaced them with underscores. Filter syntax needs a backslash for multi-word or special names. Label name length 1 to 128 characters (API). search.
- F14. Linear: label groups allow one level of sub-labels. Filtering a group includes its sub-labels. first-party (linear.app/changelog/2022-11-10-label-groups). Group single-select versus multi-select is inconsistent across sources. search.
- F15. Things: tags nest by dragging in the Tags window. Filtering by a tag hides to-dos without it, and Cmd-click ANDs several. Parent filter showing children: gap. Name rules: gap. first-party.
- F16. Notion select and multi-select: options are created by typing. Nesting and comma behaviour: gap. first-party.
- F17. Dex: tags are "flat labels for cross-cutting categories". Groups are named collections. CSV import separates tag values with commas or `:::`. Character rules: gap. [[dex]] lists hierarchical groups from earlier capture. first-party (getdex.com/docs/ai/dex-skill, /docs/workflows/consolidate-your-contacts).
- F18. Folk: tags are usually a multi-select custom field. Groups are separate and their custom fields belong to one group. Character rules: gap. first-party (help.folk.app contact fields) plus search.

Only Obsidian states both which characters are legal and that a parent matches children. No product documents "`work` matches `work/client` but not `work/partner`". That is just the child-of rule, and Obsidian's `tag:` semantics imply it.

### Q3. Aliases and nicknames

- F19. Obsidian: `aliases` is a default property, a YAML list. Typing an alias in `[[` autocomplete shows it with an arrow icon, and picking it writes `[[Artificial Intelligence|AI]]`. The quick switcher also finds notes by alias. first-party (obsidian.md/help/aliases, /plugins/quick-switcher). Aliases in search, and an alias acting as a tag: gap. `tags` is a separate list property, so by documentation an alias is not a tag. The links page says a reusable link name should be an alias, not a pipe.
- F20. Dex: duplicate detection reportedly uses name, email, phone and nicknames. search only. No page on a nickname field (G3).
- F21. Attio: no alias or nickname attribute on the attribute-types page. gap. Folk: contact fields list no nickname. gap.
- F22. Raycast and Superhuman use aliases for commands, shown as "Title (alias)" in Superhuman. Raycast aliases are exact match and rank first. first-party / search.

### Q4. Fact about a person versus a free label

- F23. HubSpot: company is an association with optional labels and a primary flag. Free grouping lives in lists and properties. first-party (associate-records). More in [[research-relationship-models]].
- F24. Attio: company is a record-reference attribute, a free label would be a select or multi-select attribute. Folk: Companies is a native person field, groups and multi-select "tags" are separate. Dex: company is a contact field, tags and groups are separate. first-party.
- F25. Salesforce: employer facts live on Contact and AccountContactRelation (roles, dates). See [[research-relationship-models]].
- F26. Company-name tags: no product documents guidance. Obsidian, Notion and Todoist tags are free text with no target. Where a company is a record (Attio, Folk, HubSpot), a name-tag duplicates it by construction. No product calls this an anti-pattern. gap.

Products separate a link (queryable, has a target object, can carry role) from a label (free string, no target). Our 96 percent figure says tags are being used as links.

### Q5. Common elements of an add-and-link flow

No product documents one flow for all object types. The elements seen above:
1. One entry that accepts typed text, with a sigil or a link field.
2. Typed text only filters. Rank: exact alias, fuzzy name, recent (F2, F8, F9, F10).
3. A pick stores an ID (F5, F7).
4. No match offers a create row with minimal fields (F2, F6).
5. Several matches ask, or error (F5, F7).
6. Optional link data after the pick: label or role, primary (F6).
7. Display text is separate from target (F1, F19).

### Comparison table

| Product | Trigger | Match | Create on no match | Alias | Tag chars | Tag nesting | Link carries data |
|---|---|---|---|---|---|---|---|
| Obsidian | `[[`, `#` | fuzzy name and alias | yes, Enter or follow link | `aliases` list | no spaces | `/`, parent matches children | no |
| Notion | `@`, relation cell | search | gap | gap | gap | not documented | no |
| Attio | link field | UI gap. API match rule | API `if_unmatched: create` | none found | gap | none found | not documented |
| Folk | link field | gap | gap | none found | gap | none found | favorite flag |
| Dex | n/a | keyword search | gap | nickname (search) | gap | flat tags | none found |
| HubSpot | `@`, Add association | name or email | Create new tab | none | n/a | n/a | label, primary |
| Things | tag field | Quick Find | Return creates | none | gap | drag to nest | no |
| Todoist | `#`, `%`, `/` | list | create option | none | all, trimmed | none | no |
| Raycast | alias, arguments | fuzzy apps, exact alias | n/a | exact alias | n/a | n/a | n/a |
| Linear | label picker | list | in label settings | none | gap | one level | no |
| Slack | `@`, `#` | name, pick if several | no, prompt after send | none | n/a | n/a | ID only |
| Superhuman | command box | score on title and alias | n/a | yes | n/a | n/a | n/a |
| Salesforce | lookup | recent, then match | "New" action (varies) | none | n/a | n/a | role, dates |

### Candidate shapes for our program

Options only. None is recommended.

- S1. Free-text tags, one picker. A single typeahead for every object field. `#` opens tags. The tag alphabet is widened to allow spaces and punctuation, with Enter or comma as delimiter. Company-name tags stay tags. Needs a match rule: exact, or prefix plus `/`.
- S2. Restricted tag alphabet (Obsidian style: letters, digits, `_`, `-`, `/`). Existing tags with spaces or special characters are normalized, moved to aliases or groups, or turned into links by migration. The picker offers create for objects only.
- S3. Company-name tags become Person-Organization links (X41 direction). When typed text matches an Organization, the picker shows "link to organization" first and "tag" second. Migration converts tags equal to an existing organization name. Other tags stay.
- S4. Alias-aware picker with a match policy. One typeahead for Person, Organization, Deal, Task and Activity. Rows show "Title (alias)", with `name_aliases` extended beyond people. No match shows a create row with minimal fields, then role and dates on the typed link. Several matches always ask. Typed text is stored as an ID.

The `work` / `work/client` filter rule is unchanged under S1 and S4. S2 and S3 change which tags exist before the filter runs.

## Recommendation

Smallest next decision: set the tag character rule (open, or Obsidian-style restricted). 219 of 261 personal tags fall outside the Obsidian rule, and the other choices depend on it. Smallest experiment: classify the 219 tags (space, punctuation, company name) and count how many equal an existing Organization name, to size S2 and S3 before any code.

## Gaps

- G1 to G8 above.
- Owner direction needed: may an alias stand in for a tag (Obsidian documents nothing), and what should a typed name with several matches do.
- Not tested: Attio and Folk UI pickers, Dex Related symmetry, Things parent-tag filter.

## Sources

Read 2026-11-05.
- Obsidian: obsidian.md/help/tags, /help/aliases, /help/links, /help/properties, /help/plugins/quick-switcher. /help/search returned 404.
- Notion: notion.com/help/guides/reminders-and-mentions, /help/relations-and-rollups, /help/database-properties.
- Attio: attio.com/help/reference/attio-101/attios-data-model/Understanding-attributes, docs.attio.com/rest-api/endpoint-reference/records/upsert-a-record. Search snippets from attio.com.
- Folk: help.folk.app/en/articles/4991006-contact-fields. Search snippets from developer.folk.app.
- Dex: getdex.com/docs/ai/dex-skill, /docs/workflows/consolidate-your-contacts. Search snippet for nicknames.
- HubSpot: knowledge.hubspot.com/records/associate-records, /records/mention-a-user-on-a-record.
- Things: culturedcode.com/things/support/articles/2803581.
- Todoist: todoist.com/help/articles/use-task-quick-add-in-todoist-va4Lhpzz. Search snippets for labels and API (developer.todoist.com did not resolve).
- Linear: linear.app/changelog/2022-11-10-label-groups. linear.app/docs/labels returned 404.
- Slack: slack.com/help/articles/205240127, docs.slack.dev/messaging/formatting-message-text.
- Superhuman: blog.superhuman.com/how-to-build-a-remarkable-command-palette.
- Raycast, Salesforce: search snippets from manual.raycast.com and help.salesforce.com. The Salesforce ACR article did not load.
- Vault: [[research-relationship-models]], [[records/open-discrepancies]] (X41).
