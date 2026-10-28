---
type: research
---
# Research: a renderer-agnostic standard for nestable, query-bound view modules

Date 2026-10-26. Trigger: owner vision of a stack- and language-agnostic standard of view modules (list, table, board, calendar, markdown, map and more) that nest, each bound to a query, with arrow and tab navigation, inline edit, and a command bar that is itself a view. TUI is renderer one. Companions: [[research-tui-building-blocks]] (B1 to B31, layers L1 to L9), [[research-tui-inspirations]], [[modules-are-user-arranged]], [[extension-ladder]], [[one-verb-set]], [[objects-first]], [[keys-are-rebindable]], [[three-doors-one-core]], [[bar-results-act-like-views]], [[local-first]]. Evidence tags: doc (first-party page read through a summarizing fetch on 2026-10-26), search (search summary only), gap (not verified). All pages passed through a summarizing model, so field names should be re-read before they go into a spec. Findings already in [[research-tui-building-blocks]] (Bases, Airtable, Notion views, Datasette metadata, JSON Forms, Adaptive Cards, A2UI, AG-UI, gh-dash, k9s, Terminal.Gui, ARIA grid) are referenced by their B-number and not repeated.

## Findings

### Part 1. Ink & Switch and local-first tooling

- **V1. "Malleable software" essay (doc).** Ink & Switch, June 2025 (Litt, Horowitz, van Hardenberg, Matthews). Three patterns it calls essential: gentle slope from user to creator ("each incremental increase in tailoring power should only require a small incremental investment of learning and skill"), tools not apps (shared data plus composable UI, compound documents, embedded editors), communal creation. Two infrastructure threads: local-first data with live code stored as data and version control, and dynamic documents. Unsolved by its own account: schema compatibility between tools (Cambria "promising but not production-ready"), composition and embedding ("cramped and limited" embedding models, richer UI-to-UI communication still being explored), version control for code, security of shared modifications, business model, quality signalling, and that AI code generation alone does not give malleability. https://www.inkandswitch.com/essay/malleable-software/
- **V2. Embark (doc).** Nov 2023, LIVE 2023. Three blocks: mentions (inline links to structured items), formulas (spreadsheet-like, inputs from the outline, outputs new nodes), views (maps, calendars that render data from a node or its subtree). "Roughly the data, computations, and views that traditional apps bundle together." Views read data they do not own, one view can combine sources, and properties set on an outline branch (a color) inherit down the tree and change the view. Stated limits: functions and views hard-coded so not extensible at runtime, data schemas implicit and unenforced, formulas take concrete arguments and do not update when related data changes, collaboration poor. https://www.inkandswitch.com/embark/
- **V3. Potluck (doc, thin).** Summer 2022, Project 024. Blocks: live searches that extract structure from freeform text, formulas over the extracted data, dynamic annotations shown in the original document. The page lists no limitations beyond saying findings were shared elsewhere (gap). https://www.inkandswitch.com/project/potluck/
- **V4. Cambria (doc, thin).** Fall 2020, Project 016. TypeScript library of bidirectional lenses that convert data between schema versions so software can read many versions. The page names no limitations. The essay (V1) calls it not production-ready. https://www.inkandswitch.com/project/cambria/
- **V5. Inkbase (doc).** Nov 2022, first shown at LIVE 2021. Hand-drawn marks become live objects with properties that hold values or formulas in a homegrown Lisp, plus spatial queries (`query/inside`, `query/overlaps`). Limits stated: no sound reuse beyond duplication, compound objects and collections need work, no coherent design for reactive creation of objects, large systems hard to build on the device. Relevant here only as a precedent for properties-as-formulas plus queries over objects. https://www.inkandswitch.com/inkbase/
- **V6. Patchwork and automerge-repo (doc, thin).** The Patchwork page says only "a research project investigating dynamic environments for creative work" and lists research areas (local-first, malleable, programmable ink, universal version control). Its building blocks and difficulties are not on the page (gap). https://www.inkandswitch.com/patchwork/notebook/ . automerge-repo is a wrapper over the Automerge CRDT with pluggable storage adapters (IndexedDB, Node filesystem) and network adapters (WebSocket, MessageChannel, BroadcastChannel), many documents at once. Document handles and URLs not covered by the fetched page (gap). https://github.com/automerge/automerge-repo
- **V7. What Ink & Switch tells us.** Their working split is data, computation, view, with data outside the view. The open problems they name are the ones this standard would hit: implicit schemas (V2), composition and embedding (V1), runtime extensibility of views (V2). They publish no view-module spec.

### Part 2. Systems that bind views to queries

- **V8. Moldable development, Glamorous Toolkit (search plus doc).** A method annotated with a `<gtView>` pragma is detected at run time and adds a view to the inspector for that class of object. The 2015 Moldable Inspector paper (Pharo) reports 131 custom views for 84 objects in 15 applications, 9.2 lines of code per view on average. Inspector also supports actions and search interfaces per object. The model is object in, many contextual views out, so a view is chosen by what the object is, not by a stored query. How inspector panes chain on selection was not retrievable (the fetched index page had no content on it, gap). Sources: https://book.gtoolkit.com/moldable-tool-98p0537dgjllrko3jqmdk8243, https://scg.unibe.ch/archive/papers/Chis15a-MoldableInspector.pdf (search summaries).
- **V9. Smalltalk Morphic (search).** Maloney and Smith, UIST 1995, "Directness and Liveness in the Morphic User Interface Construction Environment". Directness: inspect or change a component by pointing at it. Liveness: the interface is always running. Morphic replaced the original MVC toolkit in Squeak and is built on display trees of morphs. One Squeak list post notes direct manipulation edits instances, not classes (secondary). https://apps.hci.rwth-aachen.de/borchers-old/cs377a/handouts/p21-maloney.pdf , https://wiki.squeak.org/squeak/morphic
- **V10. Notion blocks (doc).** Block object: `id`, `type`, a same-named object holding type data, `parent`, `has_children`; children are fetched by a separate endpoint and nest only in some types (column, toggle, table, synced block, child database and others). Types include `column_list`, `column`, `child_database`, `synced_block`, `table_of_contents`. Unsupported blocks return `type: "unsupported"`. Database view types are in B6. Notion help for relations did not cover a "this page" filter on embedded views (gap). https://developers.notion.com/reference/block
- **V11. Obsidian Bases (doc).** Beyond B4: in an embedded base, `this` points at properties of the embedding file, so a base inside a note can filter by that note (the page does not show a worked example, `file.hasLink(this.file)` is the fetch tool's inference). https://obsidian.md/help/bases/syntax . This is the closest file-format precedent for "child query parameterized by the host".
- **V12. Airtable Interfaces (gap).** The Interface Designer help URL returned 404. Only the record-view vocabulary in B5 is verified. Linking a list element to a record-detail element is unverified.
- **V13. Retool (doc).** `{{ }}` blocks evaluate JavaScript. A component property reads `query.data` (`{{ listUsers.data }}`) and another component's state (`table1.selectedRow`). Read-only queries run automatically when a referenced input changes, write queries only run manually. Queries run through event handlers or `.trigger()` with `onSuccess` and `onFailure`. https://docs.retool.com/queries/quickstart
- **V14. Appsmith (doc).** Same model. `{{Input1.text}}` in a query, run modes Automatic, Manual, On page load. Events call `{{getUsers.run()}}`. Docs warn that Automatic plus an event trigger can run the query twice. https://docs.appsmith.com/connect-data/reference/query-settings
- **V15. Budibase (doc, thin).** Bindings (Handlebars or JavaScript) on nearly every component setting, references like `Query 1.Customer.name`, data source queries use `{{ value }}`. Repeater row context and form block context not covered by the page (gap). https://docs.budibase.com/docs/bindings
- **V16. Streamlit (doc).** Widget interaction reruns the script top to bottom. `@st.fragment` (1.37.0) reruns one function only. Return values from fragments are ignored during fragment reruns, so sharing between regions goes through Session State. Fragments cannot detect input changes. https://docs.streamlit.io/develop/concepts/architecture/fragments
- **V17. Gradio (doc).** Explicit wiring: `button.click(fn, inputs, outputs)`, triggers per component (`click`, `change`, `submit`), chaining with `.then()`, `.success()`, `.failure()`, `gr.on()` for several triggers. Returning a new component with properties changes the existing one. https://www.gradio.app/guides/blocks-and-event-listeners
- **V18. NiceGUI (doc).** Property-level binding: `bind_value` (two-way), `_from` and `_to` (one-way) with `forward` and `backward` transforms. Bindable properties propagate on write. Other bindings are polled in a refresh loop (0.1 s default) and the page warns that binding to large lists or dicts is costly. https://nicegui.io/documentation/section_binding_properties
- **V19. Observable (gap).** The cells page returned HTTP 429. Reactive dataflow between cells is not verified in this pass.
- **V20. Home Assistant Lovelace (doc).** Cards are YAML objects with `type`. Grouping cards (`vertical-stack`, `horizontal-stack`, `grid`) hold `cards:` lists, so cards nest. Logic cards (`conditional`, `entity-filter`) and per-card visibility conditions (user, screen, time, entity state, template). Actions are a closed set on `tap_action`, `hold_action`, `double_tap_action`: `toggle`, `more-info`, `perform-action`, `navigate`, `url`, `assist`, `none`, with optional `confirmation`. Templates are not allowed in actions. https://www.home-assistant.io/dashboards/vertical-stack/ , https://www.home-assistant.io/dashboards/actions/ . Field names of the generic card beyond these were not on the fetched pages (gap).
- **V21. Grafana (doc).** Dashboard JSON has `panels[]` (each with `type`, `title`, `id`, `gridPos {x,y,w,h}`, grid of 24 columns, `h` unit 30 px), `templating.list[]` variables (`name`, `type` custom/query/interval, `query`, `current`, `options`, `multi`, `includeAll`, `datasource`, `refresh`) and `links[]`. A panel gets data through one or more queries (up to 26), each in the language of its data source, with a "Mixed" source and a transformations pipeline after the queries. Queries starting with `$` are templates filled from variables. Variable values sync to the URL as `var-<name>=value`, so a link can carry context to another dashboard. Repeating options render one panel per variable value. Panel-level `datasource` and `targets` fields were not on the fetched page (gap). https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/view-dashboard-json-model/ , https://grafana.com/docs/grafana/latest/panels-visualizations/query-transform-data/ , https://grafana.com/docs/grafana/latest/dashboards/variables/
- **V22. Datasette plugins (doc).** Hook list: `register_output_renderer`, `register_routes`, `render_cell`, `extra_template_vars`, `extra_js_urls`, `extra_body_script`, `table_actions`, `database_actions`, `menu_links`. Extension is by hooks around a fixed table/query page, not by composing views. https://docs.datasette.io/en/stable/plugin_hooks.html

### Part 3. Renderer-agnostic UI description formats

- **V23. A2UI v0.9 (doc).** Server to client messages, each with `"version"` and exactly one key: `createSurface` (`surfaceId`, `catalogId`, optional `theme`), `updateComponents` (flat `components` array), `updateDataModel` (`surfaceId`, JSON Pointer `path`, `value`), `deleteSurface`. Components are an adjacency list, not a tree: each has `id` and `component` type name, containers hold child ids or a template `{path, componentId}` that repeats over an array, and a component `root` must exist. Dynamic properties are a literal, `{"path": ...}` or a function call. Relative paths resolve inside a template (`name` becomes `/employees/0/name`). Inputs write to the local data model at once and reach the server only when an action fires. Actions are `event` (`name`, `context` with path references) or `functionCall`. `checks` give validation and disable buttons. Client to server `action` message carries `name`, `surfaceId`, `sourceComponentId`, `timestamp`, `context`. Basic catalog components: Text, Image, Row, Column, List, Card, Tabs, Modal, Button, TextField, CheckBox, ChoicePicker, Slider and more. Custom catalogs identified by `catalogId`. This is the closest published match to "data model plus component tree plus bindings plus actions". It has no table, board, calendar or map in the basic list (the page says "among others", so the full list is not verified). https://a2ui.org/specification/v0.9-a2ui/
- **V24. AG-UI (doc).** Event types: lifecycle (`RunStarted`, `RunFinished`, `RunError`, `StepStarted`, `StepFinished`), text message, tool call, `StateSnapshot`, `StateDelta`, `MessagesSnapshot`, activity, subagent, reasoning, `Raw`, `Custom`. `StateDelta` is an array of JSON Patch operations (RFC 6902) and `StateSnapshot` replaces state. https://docs.ag-ui.com/concepts/events . The state snapshot plus patch model is reusable for pushing data model updates to a renderer. It defines no components.
- **V25. Adaptive Cards (doc).** The card schema page only points to the schema explorer at https://adaptivecards.io/explorer/ (not read, gap). Principles are in B9.
- **V26. JSON Forms (doc).** Rules attach to a UI schema element: `rule {effect, condition}`, `effect` in `HIDE`, `SHOW`, `DISABLE`, `ENABLE`, `condition {scope, schema}` where scope is a JSON pointer such as `#/properties/counter` and `schema` is a JSON Schema checked against that data. https://jsonforms.io/docs/uischema/rules/ . Declarative conditional UI with no code.
- **V27. Vega-Lite (doc).** Grammar parts per the docs sections: data, transform, mark, encoding, projection, view composition (facet, layer, concat, repeat, resolve), parameters. A selection is a `param` with a `select` property of type `point` or `interval`. A selection can be used in a filter transform, a conditional encoding or a scale domain. In facet and repeat, `resolve` is `global`, `union` or `intersect`. Limits on the page: selections over aggregated fields work only in their own view. The syntax of referencing a selection from another view was not on the fetched page (gap). https://vega.github.io/vega-lite/docs/ , https://vega.github.io/vega-lite/docs/selection.html . Precedent for "small closed vocabulary, composition operators, selection as a named value".
- **V28. Server-driven UI practice (search).** Airbnb's Ghost Platform, post by Ryan Brooks, June 2021, "A Deep Dive into Airbnb's Server-Driven UI System". Per InfoQ's summary: backend response sets screen layout, section arrangement, section data and actions, with three elements (sections, screens, actions) over a GraphQL data layer (Viaduct). The primary post was not read, only search summaries (gap). https://www.infoq.com/news/2021/07/airbnb-server-driven-ui/ . Confirms the pattern "section = unit, screen = layout, action = intent" in production.

### Part 4. Selection context, mutations, focus

- **V29. How child views get parent context.** Five mechanisms appear:
  1. Named variable substituted into the child query: Grafana `$var` and URL `var-x` (V21), Appsmith and Retool `{{widget.value}}` (V13, V14), Budibase bindings (V15).
  2. Direct reference to another component's state: Retool `table1.selectedRow` (V13).
  3. Explicit wiring of inputs to outputs per event: Gradio (V17), NiceGUI property bindings (V18).
  4. Host-note scope: Bases `this` (V11).
  5. Path into one shared data model: A2UI relative and absolute paths, template over an array (V23). Shared global state as fallback: Streamlit Session State (V16). Vega-Lite selection param with `resolve` (V27).
  Grafana, Retool and Appsmith run the child query again when the value changes, which is parameterized child queries in practice. A2UI and Vega-Lite keep the selection in a data model or param that views read.
- **V30. How writes are modeled.** Not uniform. A2UI: inputs write locally, action message sends them (V23). Retool and Appsmith: write queries are manual and triggered by events (V13). Gradio: a function with outputs (V17). NiceGUI: two-way binding writes the model directly (V18). Home Assistant: a closed action set (V20). Notion and Airtable inline edit is a product feature, mutation format not read (gap). Airbnb: actions in the response (V28). No system found names a generic "edit cell" mutation with optimistic update and undo (absence from this pass only).
- **V31. Focus and keyboard.** Of the systems in Part 2 and 3, none was found to specify focus order across nested panels in its docs pass (gap, not exhaustively searched). Models that do exist: ARIA grid and roving focus (B25), Terminal.Gui command names with layered key maps (B19), Textual priority then focused widget then app (B16), yazi and aerc layered keymaps ([[research-tui-inspirations]] T3, T5), lazygit panel focus by number and `[` `]` (T1).

### Part 5. Failure modes

- **V32. Lowest common denominator (search).** Java AWT: Eclipse SWT docs say AWT's use of native widgets forces developers to the least common denominator, lacking trees and rich text. Swing drew its own widgets, so apps looked foreign and (per a 2003 forum post) ran slowly. SWT used native widgets and emulated missing ones, at the cost of being a subset of Swing. Sources are old (2003 and earlier) and forum-level. https://archive.eclipse.org/eclipse/downloads/documentation/2.0/html/plugins/org.eclipse.platform.doc.isv/guide/swt.htm . Adaptive Cards (B9) accepted this on purpose with "when in doubt, keep it out".
- **V33. Platform shutdown, no migration path (search, multiple news sources).** Google App Maker: shutdown announced 2020-01-27, no new apps from 2020-04-15, closed 2021-01-19. Google said low usage, and "Due to the specific source code used for App Maker, you can't directly migrate your apps". https://9to5google.com/2020/01/27/google-app-maker-shutdown/ , https://betanews.com/2020/01/28/google-app-maker-closing/ . Lesson for a spec: definitions in an open, readable file format survive a vendor.
- **V34. Low-code criticism (search, general).** An arXiv study of practitioner posts and several vendor blogs list lack of customization (rigid layout and design control), vendor lock-in, slow loading and publishing, and trouble at scale. Nothing specific to Retool or Appsmith abandonment was found. https://arxiv.org/pdf/2107.07482 . Treat as directional.
- **V35. Escape hatch sprawl (search plus doc, indirect).** Retool and Appsmith put JavaScript in `{{ }}` on almost any property (V13, V14), Budibase allows JavaScript bindings (V15), Datasette exposes about a dozen hooks (V22). That shows where the escape hatch lives in each, not that it caused failures. No named case of a project abandoned because of sprawl was found (gap).
- **V36. Performance hints in the docs.** NiceGUI warns binding to large lists is costly and logs propagation over 0.01 s (V18). Streamlit full-script reruns led to fragments (V16). Appsmith warns of double query runs (V14). Grafana warns that resizing a panel changes calculated values (V21). These are documented costs of reactive binding, not abandonment evidence.
- **V37. Ink & Switch's own account (doc).** Embark views and functions are hard-coded, schemas implicit (V2). Inkbase has no reuse beyond duplication (V5). The essay lists embedding as cramped (V1).

## Concept comparison

Cells say what the system calls the concept, or "none found". Verified only where a finding is cited.

| Concept | Grafana (V21) | Retool / Appsmith (V13, V14) | Lovelace (V20) | Bases / Notion (B4, B6, V10, V11) | A2UI (V23) | Vega-Lite (V27) | Embark (V2) | GT (V8) |
|---|---|---|---|---|---|---|---|---|
| Panel / view unit | panel | component / widget | card | view / block | component | view / layer | view | view on an object |
| Datasource | data source per query | resource | entity registry (not read) | files / database | data model | data | outline nodes | the inspected object |
| Query | `targets`, one panel up to 26 | query object | none (entity id) | filters, sort, group | none (paths) | transform | node subtree | method on class |
| Binding | `$var` in query | `{{ }}` JS | entity field | `this`, formulas | `{path}` | encoding | formula refs | n/a |
| Layout | `gridPos`, rows | canvas grid | stacks, grid | view type, columns | Row, Column, List, Tabs | layer, facet, concat | outline tree | inspector tabs |
| Nesting | rows, repeat | containers | stack in stack | blocks, embedded base | child ids, templates | compose | subtree | pane chain (gap) |
| Context passing | variables, URL | `selectedRow`, widget values | navigate action | `this` | relative path | selection param + `resolve` | inherited props | gap |
| Mutations | read mostly | write queries by event | `perform-action` | inline edit, not read | `event` / `functionCall` | none | formulas | actions on objects |
| Keyboard / focus | none found | none found | none found | none found | none found | none found | none found | gap |

## Common concepts (extracted)

1. **Unit.** A named, typed, configurable region (panel, card, component, block). Every system has it.
2. **Data source plus query.** A query is parameterized and owned by the unit (Grafana targets, Retool queries) or by a shared data model (A2UI paths).
3. **Binding.** A reference from a unit property or query to a value. Three forms: template variable (`$x`), expression (`{{ }}`), path (JSON Pointer).
4. **Layout.** Either a coordinate grid (Grafana 24 columns), or nested containers (Lovelace, A2UI Row and Column).
5. **Context passing.** Always "a named value changes, dependents re-query or re-render". The value is a selection, a variable, or a path.
6. **Actions.** A closed list (Lovelace) or named events with context (A2UI, Gradio). Mutation form varies.

## Taxonomy of view modules

Proposed list. "Shape" is the data a module expects from its query, stated field-neutrally. All modules take an optional `selection` (out) and `context` (in). Proposal, not a verified standard. Calendar, map and board shapes follow B4 to B6 and Embark (V2) vocabulary.

| Module | Shape expected | Selection out | Notes |
|---|---|---|---|
| list | rows with a title field, optional subtitle, badge, mark | row id | one line per row, the default for a master view |
| table | rows, ordered columns (field, width, type) | row id, cell | columns typed, so inline edit knows the editor |
| detail (record) | one row, ordered fields with types, optional link rows | field | the child of a list. Editable per field |
| board | rows, a group-by field with ordered values | row id, group | move row between groups writes the group field |
| calendar (day, week, month, year, rolling n) | rows with a start, optional end and title | row id, date range | span is a config value, not five modules. Rolling is a window relative to now |
| timeline / gantt | rows with start, end, optional parent | row id | later |
| text (markdown) | one text value, optional links | link target | read or edit. Links can set context |
| map | rows with a point or polygon, label | row id | needs a tile or geometry source. TUI renderer is hard (gap) |
| chart (bar, line, spark) | rows or aggregates with x, y, series | point | a Vega-Lite subset is the precedent (V27) |
| tree / outline | rows with a parent id | node id | k9s xray and Embark outline are precedents |
| form | one row, fields with types and rules | none | JSON Forms precedent (V26) |
| stat | one aggregate value, label, optional delta | none | dashboard use |
| container: split / stack / tabs | child module list, arrangement | child focus | holds modules. The nesting mechanism |
| command bar | rows of commands, objects and settings, typed text | chosen row | same list shape as any other view (see Recommended) |

## Applies to us

- Nothing found defines the whole stack. The nearest fragments: A2UI (component tree, data model, actions), Grafana JSON (panel, query, layout, variables), Bases (stored query plus view type plus `this`), Terminal.Gui (command names and layered keys, B19).
- The context-passing options in V29 reduce to one choice for us: a child query takes parameters, and a parent exposes named values. Grafana variables and Bases `this` are the file-level precedents.
- Open, readable definition files (V33) match [[local-first]] and [[modules-are-user-arranged]].
- Escape hatches (V35) should not be inside the definition format. Put them at the verb and renderer level, per [[extension-ladder]].

## Recommended

Not a decision. A layered model and the smallest real spec.

### Layers

1. **Data.** Typed collections with fields and relations. Standard needs only: collection name, field name, type, relation. Our `model.Relations` is the seed ([[research-tui-building-blocks]] L1).
2. **Query.** A named, parameterized filter over one collection: filter tree, sort, group, limit, and parameters (`$selected`). Shape from Bases and gh-dash (B4, B14), parameters from Grafana (V21).
3. **View module.** A type from the taxonomy plus a query reference plus type-specific options (columns, group field, span). Exposes `selection` and takes `context`.
4. **Layout and nesting.** Containers (split, stack, tabs) hold module ids. Arrangement is data, as [[modules-are-user-arranged]] has it. Use nested containers, not a coordinate grid, because a TUI is cell-based and the A2UI adjacency list (V23) stays flat for easy edits.
5. **Interaction and focus.** One focus path through the container tree. Named commands for movement (`focus-next`, `focus-child`, `focus-parent`, `select`), keys bound by command name with layered scopes (B19). ARIA grid rules for table, list and board (B25).
6. **Mutations and actions.** One edit operation, `set(collection, id, field, value)`, plus named actions from a closed registry with args (Lovelace V20, [[one-verb-set]]). Inline edit calls `set`. Undo belongs to the action layer.
7. **Command bar.** A module of type list over a query on a system collection (commands, objects, settings). Selecting a row runs an action or sets context ([[bar-results-act-like-views]]).
8. **Theme.** Roles, not colors (B9, our theme package). Host owns look.

### Smallest spec that would be real

1. A JSON or TOML document with four parts: `queries` (named, parameterized), `modules` (id, type, query, options), `layout` (nested containers of module ids), `bindings` (module output name to query parameter).
2. Closed vocabularies: module types (list, detail, table, text, tabs, split to start), commands (movement and edit), actions (name plus args), color roles.
3. A conformance test: a fixed data file and a fixed arrangement, and expected rendered text for the TUI renderer. Without one, the spec is words.
4. A reference implementation reading the spec in the existing Go TUI, then a second renderer or second language to prove it. Proof of agnosticism needs the second.
5. Order of work: list plus detail with a `selected` parameter first. This tests parameterized child queries, the hardest part (V29, V1 "composition and embedding").

### What not to specify first

Map, calendar, board, chart layouts. User-written expressions (V35). A coordinate grid. A keymap interchange format beyond command name, keys, scope.

## Could not verify

- Observable cells (429). Airtable Interface Designer (404). Retool and Hasura app export formats. Notion "this page" filtering and Coda blocks (not fetched). Smalltalk Morphic details beyond one search. GT inspector pane chaining, the Moldable Inspector paper content beyond a search summary. Patchwork building blocks. Potluck and Cambria lessons beyond the essay. Adaptive Cards element and action lists. A2UI full catalog and the v1.0 candidate changes. Ghost Platform primary post. Lovelace generic card fields. Grafana panel `datasource` and `targets` fields. Vega-Lite syntax for referencing a selection across views. Focus-order specs across nested panels in all systems (V31).
- Failure modes: no named case of abandonment from escape-hatch sprawl or performance was found. V33 is a platform shutdown for low usage, V32 is dated criticism, V34 is general.
- Absence claims ("none found") rest on a single pass through the pages fetched.
- All page content came through a summarizing fetch model. Re-read before quoting field names.

## Open questions for the owner

- Q1. Is the standard the file format (JSON or TOML) plus conformance tests, or a runtime protocol like A2UI and AG-UI (messages over a stream)? The first suits local-first and agents editing files. The second suits live agent-driven UI.
- Q2. Should it reuse A2UI's component model and add data modules to a catalog, or define its own? A2UI has no terminal renderer and no list/table/board/calendar in the basic catalog as read. Reuse buys agent tooling and costs fit.
- Q3. Where do parameters come from: a single selection per container, or named values any module may publish (Grafana-style variables)?
- Q4. Is one `set(collection, id, field, value)` enough for inline edit, or do you need typed editors, optimistic update and conflict handling now?
- Q5. Which data layer does the spec assume: SQLite and SQL, a document model (Automerge), or neither (an adapter interface with `query` and `set`)?
- Q6. Are user-written expressions allowed in queries and columns (Bases formulas, Retool `{{ }}`)? This is where the escape hatch usually forms.
- Q7. Is the first conformance renderer the TUI only, and what is the second renderer that proves agnosticism?
- Q8. Does the command bar read a `commands` collection from the same registry as the keymap ([[keys-are-rebindable]]), so key, bar and CLI share names ([[three-doors-one-core]])?

## Sources
- Ink & Switch: https://www.inkandswitch.com/essay/malleable-software/, https://www.inkandswitch.com/embark/, https://www.inkandswitch.com/project/potluck/, https://www.inkandswitch.com/project/cambria/, https://www.inkandswitch.com/inkbase/, https://www.inkandswitch.com/patchwork/notebook/
- automerge-repo: https://github.com/automerge/automerge-repo
- Glamorous Toolkit: https://book.gtoolkit.com/moldable-tool-98p0537dgjllrko3jqmdk8243, https://scg.unibe.ch/archive/papers/Chis15a-MoldableInspector.pdf
- Morphic: https://apps.hci.rwth-aachen.de/borchers-old/cs377a/handouts/p21-maloney.pdf, https://wiki.squeak.org/squeak/morphic
- Notion blocks: https://developers.notion.com/reference/block
- Obsidian Bases: https://obsidian.md/help/bases/syntax
- Retool: https://docs.retool.com/queries/quickstart
- Appsmith: https://docs.appsmith.com/connect-data/reference/query-settings
- Budibase: https://docs.budibase.com/docs/bindings
- Streamlit: https://docs.streamlit.io/develop/concepts/architecture/fragments
- Gradio: https://www.gradio.app/guides/blocks-and-event-listeners
- NiceGUI: https://nicegui.io/documentation/section_binding_properties
- Home Assistant: https://www.home-assistant.io/dashboards/vertical-stack/, https://www.home-assistant.io/dashboards/actions/, https://www.home-assistant.io/dashboards/cards/
- Grafana: https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/view-dashboard-json-model/, https://grafana.com/docs/grafana/latest/panels-visualizations/query-transform-data/, https://grafana.com/docs/grafana/latest/dashboards/variables/
- Datasette hooks: https://docs.datasette.io/en/stable/plugin_hooks.html
- A2UI: https://a2ui.org/specification/v0.9-a2ui/
- AG-UI: https://docs.ag-ui.com/concepts/events
- Adaptive Cards schema pointer: https://learn.microsoft.com/en-us/adaptive-cards/authoring-cards/card-schema
- JSON Forms rules: https://jsonforms.io/docs/uischema/rules/
- Vega-Lite: https://vega.github.io/vega-lite/docs/, https://vega.github.io/vega-lite/docs/selection.html
- Airbnb SDUI (secondary): https://www.infoq.com/news/2021/07/airbnb-server-driven-ui/
- Google App Maker: https://9to5google.com/2020/01/27/google-app-maker-shutdown/, https://betanews.com/2020/01/28/google-app-maker-closing/
- Low-code study: https://arxiv.org/pdf/2107.07482
- AWT/SWT: https://archive.eclipse.org/eclipse/downloads/documentation/2.0/html/plugins/org.eclipse.platform.doc.isv/guide/swt.htm
