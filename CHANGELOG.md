# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Customizable deal roles (owner request 2026-10-29). A person's role on a deal is a sales role from a list you manage, different from the free-text role at a company. One kind-agnostic table `vocabulary` (migration 013) holds user-managed pick lists by kind, seeded with the deal roles champion, decision maker, economic buyer, influencer, blocker, end user, signer and billing contact. `crm deal roles list|add|rename|remove|reorder|backfill`, MCP `crm_deal_role_*`, help topic `deal-roles`. Rename rewrites the deal links that use the role. Removing a role that links use needs `--replace-with ROLE` or `--keep` (the text stays as plain text), else `E_VOCAB_IN_USE`. `backfill` adds the free-text roles already on deal links to the list (dry run with counts, `--apply`, one undo step in the repository), roles written before the list are never changed or lost. `crm deal link person` and `crm person link deal` take `--create` (`create` in MCP) to add an unlisted role while saving. In the terminal UI the deal role cell opens the choice editor with pick-or-create (`Create role X`, one undo step), Set field on selected offers `deal role` for people, and the palette entry Manage deal roles manages the list. Deal stages stay hardcoded, the vocabulary table is their planned home.
- Keymap part B2 (owner decisions 2026-10-28). Merge lists every candidate grouped by confidence level under divider headings, drawn like the Birthdays day dividers, through one reusable grouped list (a view declares its group field). One `ctrl+o` view options overlay serves every list view: Sort in People and Organizations, Minimum level in Merge, Type in Archived.
- Bulk actions are always available: every verb acts on the selected rows, else on the highlighted one, for every object type. `ctrl+a` selects all visible, `shift+up` and `shift+down` extend the selection. Destructive bulk verbs (delete, unlink, purge) show the count and need it typed back. New palette action "Set field on selected" sets a choice, date or number field (tier, deal stage and value, task due date and priority). `l` and `u` link and unlink the whole selection.
- Purge is a verb of every object type: `PurgeObject` and `PurgeArchived` in the repo, `crm purge --archived [--type T] [--ids ...] [--apply]` (person, organization, deal, task, note, dry run by default), MCP `crm_purge_archived`, bulk purge in the Archived view, a tombstone for every deleted synced row. Archived lists notes too.
- Link attributes (role, dates, primary, kind, notes, and a deal's stage and value) and the note type are cells of the linked row. A relation field renders its links as a small table, `left`, `right` and `tab` move across the cells, `enter` edits a cell by its field type.
- `ctrl+c` copies the focused field in the detail pane and otherwise still quits or cancels. Palette: "Copy value" and "Copy contact as vCard" (`ctrl+shift+c`, which many terminals keep for themselves, is on the swallowed-chord warning list).
- `o` opens the focused thing outside the app for any object: a URL, mail address or phone number opens the browser, mail app or dialer, anything else opens a web search. The search address is `[open] search_url` in `config.toml` (default Google).
- Tags are objects in the add picker (`a g`). Every kind declares its picker letter in the object registry, and a test keeps the letters unique.
- `ctrl+left` and `ctrl+right` are aliases of `[` and `]`.
- One validation framework (owner decision 2026-10-28): `internal/validate` checks, cleans and suggests corrections for every value, driven by the field types in `model.Fields`. Stable codes (18, listed by `crm help validation`), each with field, expected, received and fix. The CLI, MCP, terminal UI field editors, Dex import, CardDAV PUT and device sync all use it through the repositories. CLI errors exit 2 and print the code, field and fix on stderr, `-f json` errors carry `issues` and `suggestions`, MCP errors are `isError` JSON with the same fields. Writes now trim text, collapse whitespace and strip control characters. Text with U+FFFD is flagged `W_ENCODING_DAMAGED` and never rewritten. A phone number, email address or web address typed into another text field gives a hint (`crm: hint:` on stderr, never a failure, never applied).
- Name aliases are one shared table (owner decision 2026-10-28). The built-in nicknames moved from the duplicate finder into `internal/alias`, your additions live in migration 012 (`name_aliases`). `crm alias add|list|remove` and MCP `crm_alias_list|add|remove` manage them. The duplicate finder (built-in results unchanged), person search including the full-text query (`Zach` finds Zachary, `Rich` finds Richard), the command bar and the relation typeahead read the table. Typo tolerance applies only in the typeahead.
- Bulk purge of archived people (owner decision 2026-10-28): `crm purge --archived [--type person] [--ids 1,2] [--apply]` (dry run by default), MCP `crm_purge_archived` (`apply`), and the Archived palette entries Purge selected forever and Purge all remaining forever, confirmed by typing the count (`purge 37`). Same tombstones as `person purge`. Only people have a purge.
- Field types (owner decision 2026-10-28): `internal/model/fields.go` declares text, longtext, date, choice, number, list and relation per field. Organization and deal pages edit every field in place (`enter`/`e`, `enter` or `tab` saves, `esc` cancels), dates take phrases, deal stage and person priority are choice fields, relation rows open a typeahead with alias and typo matching and a Create 'X' row. `u` unlinks the focused linked row.
- Pipe-line prompts validate live and autocomplete (owner decisions 2026-10-29). Closing a segment with `|` checks it through `internal/validate` and shows the error inline in dim error style (code, expected, fix). The segment being typed shows a soft indicator only (a date preview, matching choices, what a name resolves to). Smart corrections show as hints. `enter` still validates the whole line. A reusable autocomplete component (`internal/tui/autocomplete.go`, interface `Completer`) draws a popup of at most six rows under the segment, fed by the field type: relation objects (nicknames and typo tolerance), choice options, date phrases and recent dates, existing tags. `tab` accepts, `up`/`down` move, `esc` closes, `enter` submits. The matching is `internal/suggest`, shared with the command bar completions and the relation typeahead.
- Links are pipe segments. A task, note, organization, deal, person or tag line ends with one segment per kind the relation registry joins (people, organizations, deals, tasks, notes, tags), several objects separated by commas. The highlighted object of the add picker is the first suggestion of the matching segment. After `tab` a `, ` follows, typing `|` takes it back. Backspacing over a name takes the link out before it is saved. On `enter` unresolved names offer pick-or-create, create and links are one undo step. A deal edit unlinks names taken out of its organization and person segments. `crm help tui-prompts`.
- One `related` segment replaces the per-kind link segments of every add line (owner decision 2026-10-29). Its suggestions come from your context: the highlighted object or the record page you are on, that object's linked objects of every kind (a deal's people, organizations and tags), once you chose something the objects linked to it, then recent objects. Typing narrows by name, `#` looks at tags only, chosen objects are never offered again and only kinds the relation registry allows for the new object appear. Before, the people and organizations of a highlighted deal were never suggested, because only the highlighted object itself was offered, and only in the segment of its own kind. The popup groups rows by kind under dim headings with a short prefix (`p:`, `o:`, `d:`, `t:`, `n:`, `#`), chosen objects show above the prompt as chips grouped by kind and colored by new optional theme roles (`kind_person`, `kind_org`, `kind_deal`, `kind_task`, `kind_note`, `kind_tag`). Unresolved names offer pick-or-create with a choice per kind. No key changed.

- Company text creates or reuses an organization automatically (owner decision 2026-10-27) at `person add|edit --company`, `import dex`, MCP `crm_person_create|update`, the command bar, device sync, the terminal UI person form and a phone ORG over CardDAV. Matching is the normalized name or a merge alias, near names never merge and show up in `crm org dupes` (MCP `crm_org_dupes`, "similar name" in the terminal UI). Junk strings (N/A, none, self-employed, retired ...) are ignored through an editable list: `crm org ignore add|list|remove`, MCP `crm_org_ignore_*` (migration 010, table `company_ignore`). `crm org backfill [--apply]` and MCP `crm_org_backfill` fill organizations for existing people (dry run by default, one undo step, idempotent). Emptying ORG on the phone never removes a membership.

- Merges are undoable (owner decision 2026-10-27). Person and organization merges write a persistent merge log (migration 011, table `merge_log`): `crm merge log|undo|redo`, MCP `crm_merge_log|undo|redo`, a `log_id` in the merge results, and one ctrl+z step in the terminal UI. Undo works in any later session and keeps values edited after the merge.
- Unlinking soft deletes the link, linking the same pair and kind again revives it with its old role, dates and notes. Taggings follow the same rule (they gain uuid, timestamps and deleted_at). Deleting a tag untags everything that carries it as one undo step, a tag with no live tagging leaves tag lists.
- Organization merge candidates can be dismissed as "not a duplicate": `crm org dupes dismiss|undismiss`, `crm org dupes --dismissed`, MCP `crm_org_dupes_dismiss`, terminal UI palette action (migration 011, table `org_dupe_dismissals`).

### Changed

- The role on a deal link must be on the deal role list. An unlisted role is refused with `E_VOCAB_UNKNOWN` and a fix unless `--create` is given. Roles on memberships, tasks and notes stay free text, and a deal link that already holds an unlisted role keeps it. New validation codes `E_VOCAB_UNKNOWN`, `E_VOCAB_DUPLICATE`, `E_VOCAB_IN_USE`, `E_VOCAB_INVALID` and `S_VOCAB_CREATE`.
- The grayscale "tab: link ..." offer of the add picker is gone, replaced by the link segments. A note typed in `a n` with a person in its people segment reads the short syntax of the command bar (`#tag`, `@Name`, `remind fri`), the first person is its subject. The text of a note, task or organization line can no longer contain `|`.
- Validation messages changed to the framework form (`E_CHOICE_INVALID stage: one of ...`). Emails need a domain with a dot, web addresses and domains cannot hold spaces, dates must exist (`2026-02-31` fails), text is capped at 500 characters and long text at 100000. `FieldSpec.Normalize` and `model.FieldError` moved to `validate.Field` and `validate.Error`. Removed the copies of the stage, priority, direction, cadence, link date order and org cycle checks from the CLI and repositories.
- Terminal UI keys (owner decision 2026-10-27): universal verbs `a` (add picker, `a t` task, `a n` note, `a o`, `a p`, `a d`, a highlighted object is offered as the link, `tab` accepts), `enter` and `e` (activate), `d`, `r` (restore in Archived), `l`, `u`, `x` (task done or reopened, person contacted), `m`, `ctrl+r`. Navigation is arrows, `tab`, pages, `ctrl+up`/`ctrl+down` (a unit per view), `ctrl+shift+up`/`ctrl+shift+down` (view home) and `home`/`end`. Removed: `h j k l g G`, `t`, `n`, `+`, `N`, `#`, `c`, `L`, `U`, `M`, `X`, `D`, `i`, `T`, `p`, `ctrl+t`, `ctrl+a`, bare `u` restore, bare `r` reload and the compare aliases `1 2 h l`. Moved: sort `ctrl+o`, duplicate level `ctrl+l`, Archived type filter `ctrl+f`, compare survivor `ctrl+e`, pair `ctrl+b`, pane `ctrl+w`. Palette only: sync run and dry run, inbox attach, tag, note type picker. See `crm help tui-keys` and the migration table in FEATURES.md.
- Keymap structure: framework and app layers, scopes with inherit and erase, normal and text key modes. `?` lists the keys live in the current view and mode by layer. `crm keys list` has `layer` and `scope`. `crm keys set` warns about chords terminals swallow (`ctrl+i`, `ctrl+m`, `ctrl+h`, `ctrl+[`, `ctrl+j`, `ctrl+shift` arrows) and accepts `ctrl+[`, `ctrl+]`, `ctrl+\`, `ctrl+^`, `ctrl+_`. A `[keys]` entry for a removed action or a key the framework owns is reported and skipped.
- `crm person delete` and `crm_person_delete` are the person verb. `crm person archive` and `crm_person_archive` stay as deprecated aliases. `crm_task_delete` no longer takes `restore`, use `crm_task_restore`. `crm person relate|relationships|unrelate` and `crm_person_relate` stay as deprecated aliases that point to `link`.

### Removed

- Terminal UI keys (keymap B2): `ctrl+n`, `ctrl+p`, `:`, `q` in the compare and the text pages, `J`, `K` and `r` in Modules, `F`, `ctrl+b`, `ctrl+w`, `ctrl+l`, `ctrl+f`, `*`, `E`, `y`, `shift+tab` and the `tab` toggle in the compare. Their functions are on `down`/`up`, `/`, `esc`, `shift+up`/`shift+down`, palette entries (Set keep-in-touch, Change pair, Switch pane, Modules: reset), `ctrl+o`, `ctrl+a`, cells of the linked row, `ctrl+c` and `space`. See the old-to-new table in `crm help tui-keys`.
- The palette entries Set deal stage and Set priority tier (ids `deal-stage`, `tier`): stage and priority are choice fields of the record page. Setting the tier of several space-selected people at once is gone with it.

- The Company review screen and its keys (`c`, `l`, `e`, `s`, `x`, `A`, `enter` in that screen, `C` in Organizations), the `org-review` tag and the device-local `orgconvert.*` meta keys. `crm org convert` and `crm_org_convert_*` remain as deprecated aliases of the backfill.

- Relations (schema 8): a person belongs to many organizations with a role, dates and a primary flag. Person to person and organization to organization links carry a kind with an inverse (`crm relation-kind`). Deals link to several people and organizations. Tasks and notes attach to people, organizations and deals. Merging people re-points every link and keeps an alias. `crm roles`, `--org` and `--deal` on `log add`, `task edit --clear-due`, `crm doctor` link check. MCP tools `crm_relation_kind_*`, `crm_links`, `crm_link_roles`, `crm_task_update`.
- Company review: `crm org convert` groups people by normalized company text, suggests the matching organization and applies per-group decisions in one batch with one undo step (create, link, skip, accept exact matches). MCP `crm_org_convert_list` / `crm_org_convert_apply`, terminal UI "Company review view". Nothing is created without a decision, `people.company` stays.
- Terminal UI: the Link form asks the kind and a role | from | to | primary | notes line, `E` edits a link, organizations and deals open as record pages, the Archived view lists every object kind with a type filter.

- Tags: `crm tag rename` (refuses a name that exists and offers a merge) and `crm tag merge` (preview unless `--apply`), MCP `crm_tag_rename` and `crm_tag_merge`, and "Rename tag" and "Merge tag into another" in the palette and on a tag row in the bar, each one undo step.
- Keys: `ctrl+t` goes to today in every module that has an anchor (Birthdays, Tasks, Home, People, Follow-up) and `t` is Add task everywhere again. `ctrl+a` adds any object from any view or from the bar through a kind picker and returns to where you were. Both stay rebindable (`go-today`, `add-any`). The key audit is in `patterns/audit-keybindings-2026-10.md`.
- Command bar ranking: results are objects first, then actions, then settings, as decided on 2026-10-16. A verb being typed still leads.
- Command bar: `tab` moves the focus to the results, where the same hotkeys as the object's view act on the highlighted person, organization, deal or task without leaving the bar (contacted, snooze, favorite, note, task, link, delete, done, edit), the footer lists them, `ctrl+z` undoes. A tag whose name matches the query is a result row without `#`, the exact name first, with delete, give and take off. People who matched only through a tag read "tagged #tag".
- Birthdays view: a divider per day (date at the left, relative time such as "in 1w 1d" or "3d ago" at the right), the age each person turns at the right edge, past birthdays above (90 days back, 365 ahead) and `ctrl+t` or `ctrl+up` to jump back to today. `crm birthdays` and `crm_birthdays` gain a `relative` field next to `days_until`.
- Modules: every terminal UI view is a module you turn on or off and reorder, per device (`[modules]` in config.toml). `crm modules list|on|off|move|reset`, MCP `crm_modules_list|set|reset`, palette "Modules". Off hides the view from tabs, number keys, `[` `]`, the view list and the palette, its data and links stay. The default shows every module (Home, People, Follow-up, Tasks, Organizations, Deals, Birthdays, Merge, Archived, Sync), the number keys follow the arrangement (0 is the tenth), and a narrow tab bar folds the last tabs into "v +N".
- Native organization fields (schema 9): website, size, city, region, country, LinkedIn URL, description and a logo path, in `crm org add|edit` flags, MCP, the organization page and the terminal UI edit line (`name | domain | industry | size | location | website | linkedin | description | notes`).
- Organization duplicate guard and merge: `crm org add` refuses a live organization with the same normalized name or domain unless `--allow-duplicate` (MCP `allow_duplicate`, the terminal UI asks use, create anyway or cancel). `crm org merge SURVIVOR DUPLICATE...` (preview, `--apply` writes), MCP `crm_org_merge`, and `M` on marked rows in the Organizations view merge organizations with memberships, parent links, deals, tasks, notes and tags re-pointed and one undo step. The company review links an existing organization instead of creating a duplicate.

### Changed

- The palette has one "Company review" entry (the generated "Company review view" is gone).
- Breaking: `people.org_id`, `deals.person_id`, `deals.org_id`, `tasks.person_id`, `tasks.deal_id` and the tables `relationships`, `task_people`, `interaction_people` are gone. JSON: `person_id`/`org_id` on deals and tasks become `person_ids`, `org_ids`, `deal_ids` (a deal keeps `org_id` for its primary organization). Back up before upgrading, the data moves in one migration.
- A deal gets `closed_at` re-stamped to now on every move into won or lost (including won to lost) and loses it when it reopens. A phone ORG edit moves the primary membership to the organization of that name.

## [0.1.6] - 2026-03-08

### Added

- Project website at [crmcli.sh](https://www.crmcli.sh) with install guide, FAQ, and AI use case examples
- `AGENT.md` with CRM agent instructions for Claude users
- Claude Desktop setup instructions in README

### Changed

- Improved code quality across CLI commands, repositories, formatters, and tests
- Improved README with banner, badges, and clearer structure

### Fixed

- Hardened input validation, error sanitization, and integer overflow checks in MCP server
- Restricted `~/.crm/` directory permissions to user-only (0700)
- CSV headers now use title-case to match table output
- Mobile responsiveness fixes for website (FAQ cards, MCP section, footer)

### Security

- MCP server no longer leaks internal error messages to clients
- Added interaction type and direction validation
- Added entity existence checks before applying tags

## [0.1.5] - 2026-03-07

### Added

- Duplicate person detection on create (by email)
- `crm_person_delete` MCP tool

## [0.1.4] - 2026-03-07

### Fixed

- Code review findings across codebase

## [0.1.3] - 2026-03-07

### Added

- `crm_person_relate` MCP tool for creating relationships between people via AI agents

## [0.1.2] - 2026-03-07

### Changed

- Switched Homebrew distribution from cask to formula
- Rewrote README to document actual features

## [0.1.1] - 2026-03-07

### Added

- Person CRUD commands (`add`, `list`, `show`, `edit`, `delete`)
- Organization CRUD commands
- Interaction logging (`call`, `email`, `meeting`, `note`, `message`)
- Polymorphic tagging system
- Person-to-person relationships (`colleague`, `friend`, `manager`, `mentor`, `referred-by`)
- Deals and pipeline management with stages
- Tasks and follow-ups with priorities and due dates
- Cross-entity full-text search (FTS5)
- Context briefing command for pre-meeting prep
- Dashboard summary (`crm status`)
- MCP server with 17 tools for AI agent integration
- Output formats: table, JSON, CSV, TSV
- Quiet mode (`-q`) for scripting
- GitHub Actions CI/CD
- GoReleaser for cross-platform builds
- Homebrew tap

[0.1.6]: https://github.com/jdanielnd/crm-cli/releases/tag/v0.1.6
[0.1.5]: https://github.com/jdanielnd/crm-cli/releases/tag/v0.1.5
[0.1.4]: https://github.com/jdanielnd/crm-cli/releases/tag/v0.1.4
[0.1.3]: https://github.com/jdanielnd/crm-cli/releases/tag/v0.1.3
[0.1.2]: https://github.com/jdanielnd/crm-cli/releases/tag/v0.1.2
[0.1.1]: https://github.com/jdanielnd/crm-cli/releases/tag/v0.1.1
