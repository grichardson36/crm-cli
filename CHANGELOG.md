# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Company text creates or reuses an organization automatically (owner decision 2026-10-27) at `person add|edit --company`, `import dex`, MCP `crm_person_create|update`, the command bar, device sync, the terminal UI person form and a phone ORG over CardDAV. Matching is the normalized name or a merge alias, near names never merge and show up in `crm org dupes` (MCP `crm_org_dupes`, "similar name" in the terminal UI). Junk strings (N/A, none, self-employed, retired ...) are ignored through an editable list: `crm org ignore add|list|remove`, MCP `crm_org_ignore_*` (migration 010, table `company_ignore`). `crm org backfill [--apply]` and MCP `crm_org_backfill` fill organizations for existing people (dry run by default, one undo step, idempotent). Emptying ORG on the phone never removes a membership.

- Merges are undoable (owner decision 2026-10-27). Person and organization merges write a persistent merge log (migration 011, table `merge_log`): `crm merge log|undo|redo`, MCP `crm_merge_log|undo|redo`, a `log_id` in the merge results, and one ctrl+z step in the terminal UI. Undo works in any later session and keeps values edited after the merge.
- Unlinking soft deletes the link, linking the same pair and kind again revives it with its old role, dates and notes. Taggings follow the same rule (they gain uuid, timestamps and deleted_at). Deleting a tag untags everything that carries it as one undo step, a tag with no live tagging leaves tag lists.
- Organization merge candidates can be dismissed as "not a duplicate": `crm org dupes dismiss|undismiss`, `crm org dupes --dismissed`, MCP `crm_org_dupes_dismiss`, terminal UI palette action (migration 011, table `org_dupe_dismissals`).

### Changed

- `crm person delete` and `crm_person_delete` are the person verb. `crm person archive` and `crm_person_archive` stay as deprecated aliases. `crm_task_delete` no longer takes `restore`, use `crm_task_restore`. `crm person relate|relationships|unrelate` and `crm_person_relate` stay as deprecated aliases that point to `link`.

### Removed

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
