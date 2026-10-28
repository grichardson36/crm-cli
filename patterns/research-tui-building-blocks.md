---
type: research
---
# Research: a standard for fast, repeatable TUI building blocks

Date 2026-10-26. Trigger: owner asked whether a standard exists (or could be made) for terminal UI/UX abstract enough to build TUIs fast, like web components: building blocks, views driven by queries over a database, one keybind set across programs. Companions: [[research-tui-inspirations]], [[research-keybindings]], [[objects-first]], [[one-verb-set]], [[modules-are-user-arranged]], [[show-syntax-inline-as-you-type]], [[keys-are-rebindable]], [[extension-ladder]], [[borrow-before-inventing]]. Evidence tags: doc (first-party page read through a summarizing fetch), search (search summary only), code (our repo, read directly), gap (not verified). Key and field lists passed through a summarizing fetch model, so re-read before quoting in a decision.

## Findings

### Part 1. Data-driven, declarative UI specs

- **B1. Django admin (doc).** Admin is generated from model metadata. A `ModelAdmin` overrides defaults with declarative attributes: `list_display`, `list_filter`, `search_fields`, `ordering`, `actions`, `date_hierarchy` for the list page, `fieldsets` and `inlines` for the form. Default list shows `__str__()`. This is the closest web precedent for "object schema in, list and form out, small overrides on top". Source: https://docs.djangoproject.com/en/stable/ref/contrib/admin/ (read 2026-10-26, first 100k of 182k characters only).
- **B2. Rails scaffold (doc).** One-time code generator: migration, model, controller, ERB and JSON views, routes, tests. Not runtime. After generation the code is hand-owned. Source: https://guides.rubyonrails.org/command_line.html. Lesson: generate-once and interpret-at-runtime are different models. Ours is runtime (registry read every launch).
- **B3. Hasura (doc, thin).** Generates a GraphQL API from the database schema. The page says nothing about generating a UI. Source: https://hasura.io/docs/latest/index/. Retool: the queries docs URL returned 404, so Retool's binding model and any exportable app format are unverified (gap).
- **B4. Obsidian Bases (doc).** A `.base` file is YAML: global `filters` (`and`/`or`/`not`), `formulas`, `properties` (display names), and `views` list. Each view has `type`, `name`, own `filters`, `order` (columns), `groupBy {property, direction}`. View types: table, cards (v1.9), list, map (v1.10, needs plugin), kanban (v1.14); community plugins may add layouts. Sorting is by `order` and `groupBy` direction in the example; a separate `sort` key was not shown (gap). Sources: https://obsidian.md/help/bases/views, https://obsidian.md/help/bases/syntax. This is the nearest existing file-format spec of "a view is a stored query plus a layout type".
- **B5. Airtable (doc).** View types: grid, form, calendar, gallery, kanban, timeline, list, gantt. Per-view: show/hide and order fields, row height, filter, group, sort, record coloring. Which options each type supports is not fully stated. Source: https://support.airtable.com/docs/getting-started-with-airtable-views.
- **B6. Notion (doc).** View types: table, board, timeline, calendar, list, gallery, chart. Per-view layout, property visibility, filters (AND/OR groups nested up to three levels), sorts, group and sub-group. Filters and sorts can be saved for everyone or personal only. Source: https://www.notion.com/help/views-filters-and-sorts. Both products converge on the same view vocabulary: filter, sort, group, visible fields, layout type. Neither publishes it as an open spec (not found).
- **B7. Datasette (doc).** Table-level metadata: `sort`/`sort_desc`, `columns` descriptions, `units`, `size`, `sortable_columns`, `label_column`, `hidden`, and canned `queries` (`sql`, `title`). Source: https://docs.datasette.io/en/stable/metadata.html (page names `metadata.json/yml`, not `datasette.yaml`). Facets and plugin view/renderer hooks were not on the page (gap). Closest "SQL query as a named view" precedent.
- **B8. JSON Forms (doc, thin).** A UI schema is a JSON object separate from the JSON Schema. Elements are Controls and Layouts, plus Rules. `options` are renderer-specific. The overview page did not explain Control-to-schema binding or renderer sets (gap). Source: https://jsonforms.io/docs/uischema/. Pattern: data schema and UI schema kept apart, renderer picked per toolkit.
- **B9. Adaptive Cards (doc).** Open card exchange format, JSON, "purely declarative - no code is needed or allowed". Host owns look and feel, author owns content. Principles: semantic not pixel-perfect, "when in doubt, keep it out", "expressive enough in the same way Markdown is". Templating separates data from layout. Renderers listed: Android, iOS, JavaScript, ASP.NET, .NET WPF, Windows, React Native. No terminal renderer listed. Source: https://learn.microsoft.com/en-us/adaptive-cards/ (page dated 2017, updated 2025-12-10). The newer hub https://adaptivecards.microsoft.com/ returned only a title. Best model for the "host owns style" rule.
- **B10. A2UI (doc).** Google-originated (Apache 2.0, Copyright 2025 footer), agents send declarative component descriptions that clients render with native widgets from an approved catalog. Versions on the page: v1.0 candidate, v0.9.1 current, v0.9, v0.8 legacy. "Early stage public preview". Renderers listed: Lit, Angular, React, Flutter (unpublished), SwiftUI (unpublished). No terminal renderer listed. Message type names and data-binding rules were not retrievable (gap). Sources: https://a2ui.org/, https://github.com/a2ui-project/a2ui.
- **B11. AG-UI (doc).** Event-based protocol between agent runtime and frontend: state, UI intents, user interactions. Transport only, does not define components, pairs with A2UI. Born from CopilotKit with LangChain and CrewAI. Event type list not read (gap). Source: https://docs.ag-ui.com/introduction. Relevant to us only if an agent drives the TUI, which [[three-doors-one-core]] already covers through MCP.

### Part 2. Config- or data-driven TUIs

- **B12. VisiData (doc).** Sheets with typed columns, commands bound by `BaseSheet.addCommand(key, longname, code)`. Configuration shown on the page is imperative Python in `~/.visidatarc`; no declarative format was shown. Sheet and Column API pages not read (gap). Source: https://www.visidata.org/docs/api/. Every command has a long name, so key, palette and scripts share one name.
- **B13. k9s (doc).** Plugins in YAML: `shortCut`, `description`, `scopes` (resource names or `all`), `command`, `background`, `args` with `$NAME`-style variables (https://k9scli.io/topics/plugins/). Column views in `views.yaml`, keyed by resource (`v1/pods`), with `sortColumn: AGE:asc` and `columns` entries `NAME[:json_expr][|attrs]` where attrs include W, S, H, R, L (https://k9scli.io/topics/columns/). Closest TUI precedent for "per-object-type column list and sort in a user file". Aliases and hotkeys.yaml were covered in [[research-tui-inspirations]] T2.
- **B14. gh-dash (doc).** `prSections` entries: `title`, `filters` (GitHub search string), `limit` (default 20), `layout` (column width, grow, visibility). Overall config has a JSON schema page for IDE autocomplete (https://www.gh-dash.dev/configuration/schema/, listed, not read). Source: https://www.gh-dash.dev/configuration/pr-section/. A section is a saved query plus a column layout, the same shape as a Base view (B4).
- **B15. lazygit customCommands (doc).** Fields: `key`, `context` (files, commits, global, comma list), `command` (Go template), `prompts` (types input, confirm, menu, menuFromCommand, each with optional `condition`), `output` (none, terminal, log, logWithPty, popup), `description`, `after`. Template variables are the selected object (`SelectedCommit`, `SelectedFile`, and others) and `Form.<key>`. Source: https://github.com/jesseduffield/lazygit/blob/master/docs/Custom_Command_Keybindings.md. Commands with no `key` run from the `?` menu. This is a verb defined as data.
- **B16. Textual (doc, thin).** `BINDINGS` is a list of (key, action, description) or `Binding(key, action, description, show, priority)`. Lookup: priority bindings, then focused widget, then up to the App. Footer can show bindings; `show=False` hides. User override by config is not addressed, and the page says bindings cannot be modified at runtime. Source: https://textual.textualize.io/guide/input/. Styling via Textual CSS exists as a guide section, contents not read (gap).
- **B17. Bubble Tea bubbles (doc).** Components: spinner, textinput, textarea, table, progress, paginator, viewport, list (pagination, fuzzy filter, auto help), filepicker, timer, stopwatch, help. The `key` package defines bindings with `WithKeys` and `WithHelp`, supports remap, and `help` renders from them. Source: https://github.com/charmbracelet/bubbles. We already use it (`internal/tui` imports bubbles). It is a code library, not a spec.
- **B18. Ratatui (doc).** Built-in widgets: Block, BarChart, Calendar, Canvas, Chart, Clear, Gauge, LineGauge, List, Paragraph, Scrollbar, Sparkline, Table, Tabs. Traits `Widget` and `StatefulWidget`. A third-party showcase exists. Source: https://ratatui.rs/concepts/widgets/. Rendering primitives only. No key model, no data binding.
- **B19. Terminal.Gui v2 (doc).** `Command` enum of generic operations, views declare supported commands with `AddCommand`, keys map via `KeyBindings`. Three layers, lowest to highest: `Application.DefaultKeyBindings`, `View.DefaultKeyBindings`, per-view-type defaults; a higher layer does not overwrite a key a lower one bound. `PlatformKeyBinding` has All/Windows/Linux/Macos. JSON overrides keyed by command name, per view type via `View.ViewKeyBindings`. Sources: https://github.com/gui-cs/Terminal.Gui, https://tui-cs.github.io/Terminal.Gui/docs/keyboard.html. This is the most complete "command name, layered key map, JSON override" design found in a TUI toolkit. It is single-framework, not cross-app.
- **B20. Turbo Vision (search).** Borland character-mode UI framework, circa 1990, in Borland Pascal and C++. C++ source released to public domain around 1997. Status bar, menu and F10 behavior not confirmed. A 1992 Programming Guide PDF exists (https://jexer.sourceforge.io/tvision/Turbo_Vision_Version_2.0_Programming_Guide_1992.pdf), not read. Sources: https://en.wikipedia.org/wiki/Turbo_Vision, https://tvision.sourceforge.net/. This is the historical case of a TUI with one look and key set across many programs because the toolkit enforced it.
- **B21. Config-built TUIs from YAML (search).** tui-builder (Go) builds terminal UIs from YAML with tables and multi-screen flows. BoxMux (Rust) is a YAML shell-command dashboard framework. Neither was opened, adoption unknown. Sources: https://pkg.go.dev/github.com/jsdrews/tui-builder, https://docs.rs/crate/boxmux/latest. Treat as unverified.

### Part 3. Keyboard standards

- **B22. IBM CUA (search).** A 1987 CUA demo page lists F1 help, F3 exit current level, F10 moves between panel body and action bar. Wikipedia says F1 help was common before CUA, and WordPerfect used F3. CUA influenced Microsoft Windows (per one overview, not primary). Alt+F4 is not tied to CUA by any result. SAA specifics not found. Sources: https://en.wikipedia.org/wiki/IBM_Common_User_Access, https://toastytech.com/guis/cua1987.html. IBM i and z/OS ISPF still show F1=Help, F3=Exit. Primary IBM CUA publications not read (gap).
- **B23. Norton Commander (doc, thin).** Wikipedia page confirms F3 viewer and F4 editor only. The page lists clones that follow its style: Midnight Commander, Total Commander, FAR Manager, Double Commander, Volkov Commander, DOS Navigator. F5 copy, F6 move, F7 mkdir, F8 delete, F10 quit were not on the page (gap). Source: https://en.wikipedia.org/wiki/Norton_Commander. Evidence of a de facto cross-app key bar, as a visible F-key footer.
- **B24. Readline (doc).** `~/.inputrc` (or `$INPUTRC`, fallback `/etc/inputrc`) is read by every program built on readline: Bash, FTP, GDB named on the page (psql not confirmed). Syntax `Control-u: universal-argument` or `"\C-x\C-r": re-read-init-file`. Default mode Emacs, `set editing-mode vi` switches, vi mode follows POSIX sh. Source: https://tiswww.case.edu/php/chet/readline/readline.html (first 100k of 266k characters). The one working case of a single user key file shared across unrelated programs. It covers line editing only.
- **B25. WAI-ARIA Authoring Practices (doc).** Data grid: arrows move one cell, Page Up/Down move a set number of rows, Home/End first/last cell in row, Ctrl+Home/End first/last cell of grid. Keyboard-interface page: focus must persist, focus differs from selection, roving tabindex. Standard keys listed: Shift+F10 context menu, Ctrl+C/V/X/Z, Ctrl+Y redo (Cmd+Shift+Z on macOS). The page says nothing on exchange of keymaps and has "to be written" sections. Sources: https://www.w3.org/WAI/ARIA/apg/patterns/grid/, https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/. ARIA is a good source for the grid and tab models, which map onto a list view.
- **B26. Vim and emacs (not fetched).** Used by TUIs as de facto vocabularies (`hjkl`, `gg`, `G`, `/`, `n`, `ctrl+a/e/k`). No primary page read this session (gap). [[research-tui-inspirations]] documents per-app use.
- **B27. Keymap interchange format (search).** None found. Results were editor-specific: VS Code keymap extensions, a JetBrains-to-VS Code map where each record has mac/linux/win keys plus a command name, a `.vimrc` to VS Code converter. Absence is from one search, not proof. Nearest shared shape across B13, B15, B16, B17, B19 is `{command name, keys, description, scope}` and it is not standardized.

### Part 4. Cross-app TUI conventions and design systems

- **B28. monospace-design-tui (doc).** Repo by coreyt, v0.3.0, CC BY-SA 4.0, 9 stars, 1 fork, 43 commits (as shown, no last-updated date). Contents: a design standard (grid, keyboard interaction, navigation topology, components, color, borders, typography, state, accessibility, motion, five archetypes), a pattern library (footer command bars, focused surfaces, master-detail, expand-to-focus, object-local actions, command jumps, selection grammar, live drill-down), rendering reference, appendices for Textual, Rich, Bubble Tea, Ratatui, Ink. A Designer toolset with canonical YAML (`dsl.yaml`), ASCII wireframes, `mono-designer lint`, and an MCP server with 18 tools including keybindings. It says it draws on the IBM CUA keyboard model. It lists no actual key assignments on the page, and the YAML schema was not shown. Source: https://github.com/coreyt/monospace-design-tui. Closest thing found to an explicit TUI design system and spec. Adoption outside the repo is not evidenced.
- **B29. Guidance packs and notes (search).** A tui-design skill proposes four keybinding layers (universal Enter/Esc up to action keys) and picks direct keys, leader or palette by app complexity (https://lobehub.com/skills/hyperb1iss-hyperskills-tui-design). A blog lists cross-app conventions `q ? / Esc hjkl Tab Ctrl+P` (https://griffen.codes/post/tui-design-skill-claude/). Both are summaries of practice, not standards.
- **B30. Per-project consistency rules (search).** Examples: "one operation, one key" (https://github.com/lezli01/vincent/pull/354), leader `ctrl+x` plus keybindings.json and fixed approval keys (https://github.com/jwfing/anchi/pull/31). Single projects, not cross-app.
- **B31. Result of the search for a cross-app TUI convention.** No ratified standard, no body, no widely adopted spec found. Practice converges informally on `?` help, `/` filter, `q`/`Esc` back, `hjkl`, `:` command line ([[research-tui-inspirations]] comparison table). One search pass, so a missed standard is possible.

## Layers table

Every system above splits into some of these layers. "Spec exists" means a published, reusable format, not just a feature.

| Layer | Question it answers | Best precedent found | Spec exists? |
|---|---|---|---|
| L1 Object schema | What things exist, their fields, relations | Django models (B1), JSON Schema (B8) | Yes for fields. Relations with labels, no |
| L2 Query | Which records | Bases filters (B4), gh-dash filters (B14), Datasette canned SQL (B7) | Per product only |
| L3 View type | How a result set is laid out (table, board, list, calendar, detail) | Bases, Airtable, Notion vocabularies (B4 to B6) | Same words everywhere, no open spec |
| L4 Verbs | What you can do to an object | Terminal.Gui `Command` enum (B19), lazygit customCommands (B15), our verb set | Per toolkit |
| L5 Keymap | Which key triggers which verb, per scope | Terminal.Gui layered JSON (B19), aerc (T5), readline inputrc (B24) | Readline only, for line editing |
| L6 Theme | Colors and styles by role | Adaptive Cards host config (B9), our role file | Per product |
| L7 Modules / layout | Which views exist, in what order | WTF grid, gh-dash sections, Bases views | Per product |
| L8 Widgets | Draw a table, list, input | bubbles (B17), Ratatui (B18), Terminal.Gui (B19) | Code libraries per language |
| L9 Help and discoverability | Show keys and verbs from L4 and L5 | bubbles `help` from `key` (B17), yazi desc (T3) | Pattern, no spec |

Reading the table: layers L1 to L3 and L7 are where the web ecosystem (and Notion, Airtable, Bases) is already close to data, L8 is solved per language, and L4 to L5 have working designs only inside single toolkits.

## Seed in our code

Read-only pass over `crm-cli/internal` on 2026-10-26. Evidence tag: code.

| Layer | What exists | Where | Data or hard-coded |
|---|---|---|---|
| L1 Object schema | `model.ObjectKinds` (person, organization, deal, task) and `model.Relations`, 11 entries with A, B, attrs, kinded, primary-per-A, unique, and both-side labels. `AttrSpecs` has key, label, type (text, date, bool, kind), suggest. Read by CLI, MCP, TUI, doctor and schema. Field lists and column definitions per object are in `tui/objects.go` (`fields`, `cols`). DB columns are in SQL migrations | `internal/model/relations.go`, `internal/db/migrations` | Relations are data. Per-field types of the objects themselves are not in a registry (only link attributes are) |
| L2 Query | Repo methods with typed filters (`model.DealFilters`, `model.TaskFilters`, `FindAll`). Views call them in Go | `internal/db/repo`, `tui/model.go` `reload`/`rebuild` | Hard-coded per view. No stored or user-writable query |
| L3 View type | `viewKind` enum: Home, People, Due, Tasks, Birthdays, Merge, Archived, Sync, plus Orgs, Deals, Convert. Each view is a Go renderer | `tui/model.go`, `tui/view.go` | Hard-coded |
| L4 Verbs | Fixed `verbSet` (add, open, edit, link, unlink, delete, restore, mark, undo, redo). `objDef` registry with `can` map and hooks. `action` struct with id, title, group, keys, when, keyIf, module, run. About 84 `run:` actions | `tui/objects.go`, `tui/actions.go` | Verb set is data-like. Hooks and bodies are Go. Four objects registered in code |
| L5 Keymap | Defaults in `verbKeys` and per action `keys`. User overrides by action id in `[keys]` of `config.toml`, loaded and normalized, saved while keeping the rest of the file. Context predicates (`when`, `keyIf`, `keyFor`, `keyHere`) are Go functions | `internal/keys/keys.go`, `tui/keybind.go`, `tui/actions.go` | Bindings are data. Contexts are code, not a named scope list |
| L6 Theme | Eleven color roles, built-in themes, custom TOML themes in `~/.crm/themes`, a test that bans color literals outside the package | `internal/theme` | Data |
| L7 Modules | `modules.All` (id, name, blurb), `Arrangement{Order, Hidden}` in `[modules]` of `config.toml`, aliases, number key by position | `internal/modules/modules.go` | Arrangement is data. The module list and what each module is are fixed in Go. A module is a Go view, not a stored definition |
| L8 Widgets | bubbles `textinput`, own list, form, overlay, tabs, footer, toast | `tui/*.go` | Code |
| L9 Help | Help screen, footer, palette and bar verbs generated from the action registry. Bar verbs filter registry groups (`barVerbGroups`, `barNoVerb`) | `tui/barverbs.go`, `tui/view.go` | Generated, with some hand lists |

Seed summary: relations, verbs, keys, themes and module order are already data with one reader. What is missing is the middle: no stored query (L2), no view definition (L3), and no way to add a module that is not a Go file (L7). `objDef.cols` is a list of names, but the notes in `objects.go` show values like "stage and organization", so columns are labels for hand-written rendering and not column specs.

## Applies to us

- Bases (B4) and gh-dash sections (B14) already combine a filter, a column list and a layout type in a small file. That is the missing L2 plus L3 for us, in a format a user can read.
- k9s `views.yaml` (B13) shows per-object column lists with a sort and per-column flags, small enough to copy without a spec.
- Terminal.Gui (B19) layering order (app, view, view type, user JSON keyed by command name) matches our `[keys]` by action id and would add scopes. aerc's inherit and erase (T5) is the other model.
- lazygit prompts (B15) are a data form for "ask these questions, then run". Our `fields` and link-attribute forms are the same idea.
- No cross-app standard exists to adopt (B27, B31). Anything cross-app means writing a small spec and shipping a reference implementation.

## Recommended

Not a decision. Options for the owner, from smallest.

### Layered model

1. **Schema.** Object kinds, fields (with type), relations. Already half there (`model.Relations`, `AttrSpecs`). Add field specs for the four objects.
2. **Query.** A named filter over one object kind: filter tree (and/or/not), sort, group, limit. Shape taken from Bases (B4) and gh-dash (B14).
3. **View.** A query plus a layout type from a short fixed list (table, list, board, detail) plus a column list and sort. Shape taken from Bases and k9s (B13).
4. **Verb.** Name, object kinds it supports, optional shell or template body for user verbs (lazygit B15). Our fixed verb set stays the core.
5. **Keymap.** `{command name, keys, scope, description}` per entry, scopes layered app, view type, view (Terminal.Gui B19). One file format, already how `[keys]` works.
6. **Theme.** As built.
7. **Module.** A module is a stored view (3) plus an optional name and position, so the arrangement in [[modules-are-user-arranged]] covers user-defined views as well as built-in screens.

Spec to publish, if any, would be layers 2, 3 and 5 only. Layers 1, 6 and 8 have enough prior art.

### First slice (smallest that tests the idea)

1. Define one view file format (TOML to match `config.toml`, or YAML to match Bases and gh-dash) with `object`, `filter`, `sort`, `group`, `columns`, `layout = "table"`.
2. Re-express one existing hard-coded list view (for example Tasks or Deals) as a built-in view file read by the same loader a user file would use. Ship it as the default.
3. Let `[modules]` accept a user view file as a module id.
4. Leave all other views, verbs and keys untouched.
5. Success check: a second user-written view over deals or tasks appears as a tab, takes the registry verbs and keys with no new Go code.

Out of scope for the slice: a published cross-app spec, a keymap interchange format, user-defined verbs, board and calendar layouts.

## Could not verify

- Retool app model (URL 404). Hasura UI features (page covered API only). Rails scaffold is code generation per the guide, not compared with admin generators beyond that.
- A2UI message names and data binding. AG-UI event list. Adaptive Cards hub page content. JSON Forms Control binding and renderers.
- Textual CSS and any user-level binding override. Ratatui and Terminal.Gui widget lists beyond what pages stated. VisiData Sheet and Column API.
- Turbo Vision menu, status bar and F10 behavior. Norton Commander F5 to F10. SAA specifics and Alt+F4 origin. Primary IBM CUA documents. vim and emacs primary docs.
- tui-builder and BoxMux, quality and adoption. monospace-design-tui adoption, last-update date and YAML schema.
- Absence claims (no keymap interchange format, no cross-app TUI standard) rest on one search each.
- Internal-code claims come from reading `relations.go`, `objects.go`, `actions.go`, `modules.go`, `theme.go`, the start of `barverbs.go`, and file listings. Query building inside `model.go` and rendering in `view.go` were not read line by line.

## Open questions for the owner

- Q1. Is the goal a spec others can implement (cross-app), or a shared internal model for your own programs first? The first needs a written spec and a second implementation. The second needs only the first slice.
- Q2. View file format: TOML (matches `config.toml`, [[keys-are-rebindable]]) or YAML (matches Bases and gh-dash)? Affects whether a Bases file could be read as-is.
- Q3. Should a user-written view be allowed to define columns by expression (k9s `NAME:json_expr`, Bases `formulas`), or only pick from fields?
- Q4. Should verbs be user-definable data (lazygit style shell templates), or stay the fixed set of [[one-verb-set]] with extensions only through [[extension-ladder]]?
- Q5. Which layout types after table: board (deals by stage) or calendar (birthdays, follow-ups)? Both exist in Notion, Airtable and Bases.
- Q6. Keymap scopes: adopt named scopes (app, view type, view) with inherit and erase, which changes how `ConflictFor` works?
- Q7. If cross-app, what is the first second program, and does it use Go (Bubble Tea) so code can be shared, or another language?

## Sources
- Django admin: https://docs.djangoproject.com/en/stable/ref/contrib/admin/
- Rails command line: https://guides.rubyonrails.org/command_line.html
- Hasura: https://hasura.io/docs/latest/index/
- Obsidian Bases views and syntax: https://obsidian.md/help/bases/views, https://obsidian.md/help/bases/syntax
- Airtable views: https://support.airtable.com/docs/getting-started-with-airtable-views
- Notion views: https://www.notion.com/help/views-filters-and-sorts
- Datasette metadata: https://docs.datasette.io/en/stable/metadata.html
- JSON Forms UI schema: https://jsonforms.io/docs/uischema/
- Adaptive Cards: https://learn.microsoft.com/en-us/adaptive-cards/
- A2UI: https://a2ui.org/, https://github.com/a2ui-project/a2ui
- AG-UI: https://docs.ag-ui.com/introduction
- VisiData API: https://www.visidata.org/docs/api/
- k9s plugins and columns: https://k9scli.io/topics/plugins/, https://k9scli.io/topics/columns/
- gh-dash PR section: https://www.gh-dash.dev/configuration/pr-section/
- lazygit custom commands: https://github.com/jesseduffield/lazygit/blob/master/docs/Custom_Command_Keybindings.md
- Textual input guide: https://textual.textualize.io/guide/input/
- Bubbles: https://github.com/charmbracelet/bubbles
- Ratatui widgets: https://ratatui.rs/concepts/widgets/
- Terminal.Gui: https://github.com/gui-cs/Terminal.Gui, https://tui-cs.github.io/Terminal.Gui/docs/keyboard.html
- Turbo Vision: https://en.wikipedia.org/wiki/Turbo_Vision, https://tvision.sourceforge.net/
- IBM CUA: https://en.wikipedia.org/wiki/IBM_Common_User_Access, https://toastytech.com/guis/cua1987.html
- Norton Commander: https://en.wikipedia.org/wiki/Norton_Commander
- Readline: https://tiswww.case.edu/php/chet/readline/readline.html
- WAI-ARIA APG: https://www.w3.org/WAI/ARIA/apg/patterns/grid/, https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/
- monospace-design-tui: https://github.com/coreyt/monospace-design-tui
- Guidance and projects: https://lobehub.com/skills/hyperb1iss-hyperskills-tui-design, https://griffen.codes/post/tui-design-skill-claude/, https://github.com/lezli01/vincent/pull/354, https://github.com/jwfing/anchi/pull/31
- Config-built TUIs: https://pkg.go.dev/github.com/jsdrews/tui-builder, https://docs.rs/crate/boxmux/latest
