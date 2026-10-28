---
type: research
---
# Research: TUI-first apps and what they teach our keys, modules and bar

Date 2026-10-26. Trigger: owner asked for study of TUI-first apps beyond Gmail, Linear, herdr and Obsidian. Companions: [[audit-keybindings-2026-10]] (decisions D1 to D12 below are its A4 items 1 to 12), [[research-keybindings]], [[ctrl-moves-the-view-bare-moves-the-object]], [[modules-are-user-arranged]], [[show-syntax-inline-as-you-type]], [[same-key-same-verb]], [[open-discrepancies]]. Evidence tags: doc (first-party page read, quoted by the fetch tool's summary), search (search summary only), gap (could not verify). Every page was read through a summarizing fetch, so key lists are as reported by the summary, not copied from the page.

## Findings

### T1. lazygit (doc)
- Layout: persistent side panels (Files, Branches, Commits, Stash and more) plus a main pane. Context footer shows the focused panel's actions.
- Navigation: panel focus by number and `[` `]` tabs (from [[audit-keybindings-2026-10]], doc). `enter` drills in, `esc` backs out.
- Keys: all bare. `space` is the primary action, `d` remove or discard, `e` edit, `n` new, `/` filter, `?` menu, `q` quit, `z`/`Z` undo/redo through the reflog, `:` shell command. Global keys are few (`P` push, `p` pull, `R` refresh, `@` log menu). Modifiers are for secondary actions: `ctrl+o` copy, `ctrl+t` external diff, `ctrl+j/k` move a commit, `ctrl+s`/`ctrl+b` filter options, `ctrl+r` switch repo. Uppercase is the bigger twin (`n` branch, `N` move commits to new branch, `c` commit, `C` commit with editor).
- Verb consistency: partial. The page itself says `r` is rebase, reword, rename or refresh depending on panel. `d` and `e` are steadier.
- Discoverability: `?` keybinding menu, footer hints, per-panel menus for a letter with many options (`d` opens "discard options").
- Filter: `/` in almost every list panel, called Search in some and Filter in others.
- Relations: not drawn. `enter` on a branch opens its commits, which is parent-to-children drill-down.
- Lesson: one panel-local verb that is overloaded (`r`) is exactly the C1 to C6 problem in our audit. lazygit solves heavy letters with a small menu (`d` then choose), not with more letters.

### T2. k9s (doc)
- Layout: one full-width table of a resource kind, a header showing context, cluster and active hotkeys, a breadcrumb of views.
- Navigation: `:` command mode jumps to a resource kind by name or alias (`:pod`, `:ns`, `:ctx`). Arguments after the kind scope it (`:pod ns-x`, `:pod /fred`, `:pod app=fred`, `:pod @ctx1`). `esc` leaves a view, command or filter. `ctrl-a` lists every alias.
- Keys: bare keys act on the selected row (`d` describe, `v` view, `e` edit, `l` logs). The dangerous verbs take a modifier: `ctrl-d` delete with confirmation, `ctrl-k` kill with none. This is the reverse of our rule for delete (ours is bare `d`, soft and undoable).
- Discoverability: `?` shows active mnemonics. User hotkeys (`hotkeys.yaml`) appear in `?` too. The header showing hotkeys was not confirmed (gap).
- Filter: `/regex`, `/!` inverse, `/-l` label selector, `/-f` fuzzy. The filter is part of the same input that takes commands.
- Relations: `:xray` shows a resource with its dependents as a tree. How to jump between related resources was not described (gap).
- Lesson: a command line whose first word is an object kind, followed by scoping words, is our bar with objects first ([[objects-first]]). Modifier keys guard irreversible verbs. Ours are reversible, so bare `d` stays.

### T3. yazi (doc)
- Layout: file manager with parent, current and preview columns (stated from general knowledge, the quick-start page did not describe it, gap). Tabs.
- Navigation: `h` up to the parent, `l` into the directory, `j` `k`, `gg` `G`. `J` `K` scroll the preview (the same as our detail scroll). Tabs by `1` to `9`, `[` `]` to rotate, `{` `}` to reorder, `t t` to create. `g` is a goto prefix (`g g`, `g t`).
- Keys: bare keys are verbs: `a` create, `r` rename, `d` trash, `D` delete for good, `y` `x` `p` copy cut paste, `space` toggle selection, `v` visual mode, `q` quit. `ctrl+a` select all, `ctrl+r` invert selection. Uppercase is the heavier or variant twin (`d` trash, `D` delete).
- Discoverability: `F1` or `~` opens help. The keymap is split into layers (mgr, tasks, spot, pick, input, confirm, cmp, help), each with its own rules. Each rule has a `desc`. `noop` disables a default key and removes it from the `which` popup. A which-key popup exists (the page names the `which` component) but its behavior was not described (gap).
- Filter and search: `f` filters the current view, `/` and `?` find next and previous, `n` `N` step, `s` name search (fd), `S` content search (ripgrep).
- Relations: none between records. The preview pane shows the child content of the highlighted entry.
- Lesson: keymaps scoped per layer with a description on every rule is what [[keys-are-rebindable]] and a generated `?` need. Using the same tab keys as ours (`1` to `9`, `[` `]`) is evidence for D6.

### T4. VisiData (doc, thin)
- Layout: sheets. Every derived view (frequency table, join, pivot) opens as a new sheet on a stack.
- Navigation: `hjkl` and arrows, `<` `>` jump to the next differing value in the column, `{` `}` to the next selected row. `q` closes the current sheet and exits when the stack is empty. `ctrl+^` jumps to the previous sheet. `S` lists every sheet, `z S` the stack.
- Keys: bare letters and symbols for actions. Prefixes carry scope: `g` means all (all rows, all columns, `gs` select all), `z` means the single item or a one-time variant (`zy` copy cell). `U` undo, `R` redo. Column types by one symbol (`#` int, `@` date, `~` text).
- Discoverability: `ctrl+h` opens the command reference, `z ctrl+h` the Commands sheet. `space` opens a prompt for any command by long name (reported by the cheat sheet, not detailed on the page). The command-menu page was not read (gap).
- Filter and search: `/` `?` regex on the current column, `g/` `g?` all columns, `z/` expression search, `n` `N`.
- Relations: joins, frequency tables and pivots are new sheets (`F` frequency of a column, `gF` of key columns). The join page was not read (gap).
- Lesson: scope by prefix (`g` = all, `z` = one) is a compact way to say "marked set" against "this one". It matches our `*` mark-all and `space` mark. Derived views as a stack with `q` to pop is the model for bar results acting like views ([[bar-results-act-like-views]]).

### T5. aerc (doc)
- Layout: account tabs, a folder sidebar (`sidebar-width`), a message list that can be split, a viewer in its own tab. Tabs can be pinned, titles are templates.
- Navigation: `j` `k`, `g` `G`, `J` `K` next and previous folder, `H` `L` collapse and expand folder. `ctrl+d/u/f/b` paging. `ctrl+up/down` change folder.
- Keys: nearly everything is bare. `d` delete, `a` archive, `v` mark, `c` change folder, `C` compose, `T` toggle threads. Multi-key prefixes group a family: `r` replies (`rr`, `rq`, `Rr`, `Rq`), `z` folds, `p` patches. Key syntax `<C-A-S-key>`. `:` opens a command line. Bindings are written as typed commands, so the key table and the command line are the same thing (`rq = :reply -q<enter>`).
- Contexts: `[messages]`, `[view]`, `[compose]`, `[terminal]`, with account and folder overrides. Global keys at the top apply everywhere unless `$noinherit`. An empty binding (`d =`) erases an inherited key.
- Discoverability: `keyhint` popup lists continuations after a prefix, off by default, 200 ms delay. Descriptions come from comments in `binds.conf`. Completion popovers on the command line (`completion-popovers`, fuzzy option).
- Filter and search: `/` search, `\` filter, `n` `N`, `esc` clears.
- Relations: threads. Not read for details (gap).
- Lesson: bind keys as commands so one verb has one name in the key map, the bar and the CLI ([[three-doors-one-core]]). A delayed key-hint popup after a prefix is the cheap discoverability aid if we ever add a `g` prefix (D6).

### T6. gh-dash (doc, thin)
- Layout: sections you define per view (PRs, issues, notifications), each a filtered list, with a preview sidebar. Details of layout pages were not read (gap).
- Navigation: `nextSection` and `prevSection`, up, down, `firstLine`, `lastLine`, page up and down. `viewIssues` and `viewPrs` jump from a PR to its related issues and back.
- Keys: vim style. Built-in command names exist for every action (`approve`, `merge`, `close`, `reopen`, `assign`, `comment`, `checkout`, `diff`, `search`, `help`, `refresh`, `togglePreview`). Defaults per key were not read (gap).
- Customization: `keybindings:` in YAML grouped by `universal`, `prs`, `issues`, `notifications`. Each entry is `key` plus `builtin` (override) or `command` (a shell template with fields like `{{.PrNumber}}`) and a `name` that shows in help.
- Discoverability: `?` toggles a help menu for the current context. Custom entries appear with their `name`.
- Filter: each section has a search string in GitHub query syntax, `search` key edits it. Syntax not read (gap).
- Relations: a PR to its issues by command, no inline link rows.
- Lesson: sections as saved filters are a second way to build modules ([[modules-are-user-arranged]]). User-defined key plus shell command plus template fields is an extension door that fits [[extension-ladder]].

### T7. taskwarrior-tui (doc)
- Layout: task report table with a detail pane under or beside it. `z` toggles the info view, `ctrl+e` `ctrl+y` scroll the details.
- Navigation: `j` `k`, `J` `K` page, `g` `G`, `[` `]` next and previous view, `c`/`R` switch context or report, `:` jump to a task ID, `1`-`9` run user shortcuts.
- Keys: bare. `a` add, `l` log, `m` modify, `A` annotate, `e` edit in editor, `y` duplicate, `d` done, `x` delete, `s` start or stop, `t` toggle quick tag, `v` mark, `V` mark all in filter, `u` undo, `!` shell. Note: `d` is done and `x` is delete. Ours is the opposite (`x` done, `d` delete). `a`, `l`, `m`, `A`, `/`, `!` open a one-line prompt that takes typed text.
- Discoverability: `?` help. Tab completion and history (Up/Down) in every prompt. Prompts use readline keys (`ctrl+a/e/k/u/w`, `alt+b/f/d`).
- Filter: `/` opens a live filter prompt in Taskwarrior filter syntax. The task list narrows while typing.
- Relations: `depends` and `project` are report columns. No navigation along them.
- Lesson: the one-line prompt per verb with completion is the pipe-line input ([[one-line-pipe-prompt]]). Live narrowing while typing is the behavior our bar should have. Their `d`/`x` split shows no convention exists for done against delete (our `x` done follows task apps, kabmat and Gmail select, see T8).

### T8. kabmat (doc)
- Layout: main menu of boards, then a board of columns and cards, then a card window (title, description, checklist window), plus an input field and a yes/no confirm.
- Navigation: `h` `l` change column, `j` `k` change card, `enter` opens, `esc` back.
- Keys: strong verb pairs by case. Lowercase acts on the card (`c` create, `e` edit, `d` delete). Uppercase acts on the column (`C` `E` `D`). The same three letters work in the main menu (boards) and the checklist. `H` `L` move a card between columns, `J` `K` reorder, `ctrl+h/l` move a column. `space` toggles done in the checklist.
- Discoverability: `?` help window, `q` closes. Footer hints not described (gap). No command mode (the page lists CLI flags only).
- Filter: none described (gap).
- Relations: containment only (board, column, card, checklist).
- Lesson: one verb set (create, edit, delete) applied unchanged at every level, with case choosing the level, is the cleanest verb consistency in this set. It is the closest working model of [[one-verb-set]]. It uses `c` for create where we use `a` and `c` is contacted.

### T9. sup (doc, thin)
- Layout: buffers (tabs) of thread lists. Sup shows threads, not messages, and a thread holds a tree of messages.
- Navigation: `b`/`B` switch buffer, `x` kill buffer, `;` list buffers. Go-to views by capital letter: `I` inbox, `U` unread, `C` contacts, `L` labels. This is the model for a view letter without digits.
- Keys: bare. `l` edit labels, `*` star, `d` delete or undelete, `S` spam, `a` archive in inbox, `u` undo, `r` reply, `G` reply all, `m`/`c` compose, `q` quit with confirmation, `Q` without. `t` tags the thread under the cursor and `T` tags all, then `+` or `=` applies the next command to the tagged set. Tag first, act on the set is their bulk model.
- Discoverability: `?` help, keyboard reference page, customizable keys page (not read).
- Search: `\` or `F` search all messages, `|` refine in inbox, `^G` cancel. Query language not read (gap).
- Relations: labels are free tags on threads and a label is a searchable view (`L` lists them, enter opens the label as a search). The people list (`C`) is a view of its own.
- Lesson: a tag is a saved search you can open from a list, and one "tagged set" followed by a command is a bulk verb. Fits our `space` mark and `*` mark-all. Note `x` is kill buffer here, not done.

### T10. sqlit (search, README via mirror)
- Layout: Explorer (objects), Query editor, Results. `space f` maximizes the focused pane, `space e` toggles the explorer.
- Navigation: pane focus by single letters `e` `q` `r`. `i` and `esc` switch insert and normal mode in the editor, which uses vim motions.
- Keys: leader key `space` opens a Commands Menu (`c` connect, `x` disconnect, `e`, `f`, `t` theme, `h` help, `q` quit). Menu commands can also be bound directly in `keymap.json`. Bare letters act on the focused pane (`s` select top 100, `y` copy, `v` view cell, `n` new query, `d` clear).
- Discoverability: bindings shown at the bottom of the screen and change with context. `?` help. Lazygit-inspired.
- Filter: results can be filtered by content with fuzzy match (key not listed, gap).
- Relations: the explorer lists tables, views and indexes. Foreign keys are not described (gap).
- Lesson: leader menu on a key that is free in lists gives view and interface commands a place without ctrl chords. It mirrors herdr's prefix ([[research-keybindings]] K1) and would be a possible home for the bare view keys in D6 and D7. It conflicts with `space` as mark in our lists, so it would need another leader.

### T11. abook (doc, not usable)
Home page covers only features and formats (mutt alias, LDIF, vCard export). It gives no layout, keys or search. Nothing could be verified. The manual was not fetched. (gap) Known from general knowledge only, not verified here: abook is a two-pane-less list with a detail screen, `/` search, `a` add, `d` delete, `?` help.

### T12. WTF / wtfutil (doc, thin)
- Layout: a configurable grid of modules, each a widget (calendar, GitHub, clocks, Jira and dozens more). The grid sizes come from `config.yml` (page titled Grid Layout, contents not read, gap).
- Navigation, focus keys and help: not retrievable. The README and home page list section titles only (gap). The project announces a rename to Tessera.
- Key model and filter: not verified.
- Relations: none, each module is independent.
- Lesson: modules are independent widgets chosen and placed by the user in a file. That supports [[modules-are-user-arranged]], and its absence of cross-module links is the contrast to our rule that off modules keep their data and links.

## Comparison

| App | Layout | Go-to | Verb keys | Modifier use | Overloaded letters | Help and hints | Filter | Relations |
|---|---|---|---|---|---|---|---|---|
| lazygit | side panels + main | number, `[` `]` | bare, `space` primary | secondary actions | yes (`r`, `d`, `s`) | `?` menu, footer | `/` | drill down |
| k9s | one table | `:kind` | bare on row | guards destructive (`ctrl-d`) | low | `?`, `ctrl-a` aliases | `/` regex, fuzzy | xray tree |
| yazi | 3 columns + tabs | `1`-`9`, `[` `]`, `g` prefix | bare | select-all, invert | low | `F1`, layered keymap | `f`, `/`, `s` | none |
| VisiData | sheet stack | `S`, `ctrl+^`, `q` | bare + `g`/`z` prefix | rare | low | `ctrl+h`, `space` longname | `/` `g/` `z/` | derived sheets |
| aerc | tabs, sidebar, list | `J` `K` folders | bare + prefix families | paging | low | keyhint popup | `/` and `\` | threads |
| gh-dash | sections + preview | `nextSection` | vim + named builtins | custom | n/a | `?` per context | per-section query | PR to issues |
| taskwarrior-tui | list + detail | `[` `]`, `:` id | bare | scroll only | `d`/`x` inverse of ours | `?`, completion | `/` live | columns |
| kabmat | menu, board, card | `enter`, `esc` | bare, case = level | column move | none found | `?` window | none | containment |
| sup | buffers | capital letters | bare, tag then act | few | `x` kill buffer | `?` | `\` query language | label = search |
| sqlit | 3 panes | letters, `space` menu | bare per pane | few | n/a | on-screen, `?` | in results | none confirmed |
| abook | not verified | - | - | - | - | - | - | - |
| WTF | grid of widgets | not verified | - | - | - | - | - | none |
| Ours | tabs of user-arranged modules + detail | digits, `[` `]`, `v`, bar | bare, one verb set | ctrl moves view | yes (C1 to C6) | `?`, footer, palette | bar | link rows |

## Applies to us

Tag legend: module name, D1 to D12 (decisions in [[audit-keybindings-2026-10]] A4: D1 `e`, D2 `s`, D3 `p`, D4 `d` in Sync, D5 `m`/`r`, D6 bare view navigation, D7 view-level bare keys, D8 bare `z` undo, D9 audit test, D10 `?` by context, D11 terminal chords, D12 `ctrl+a`), X-numbers from [[open-discrepancies]].

1. **Keep digits and `[` `]` bare** (yazi, taskwarrior-tui, lazygit all do). Tag: D6, X28, modules (tab bar and numbering).
2. **Keep `g` then letter off the table for now.** Only sup (capital letters) and yazi (`g` prefix) use go-to prefixes here, and the cost is a second key. If added, aerc's delayed keyhint popup is the aid. Tag: D6, X28.
3. **Delete stays bare `d`.** k9s guards delete with a modifier, but our delete is soft and undoable. taskwarrior-tui uses `d` done and `x` delete, the inverse of ours, so no cross-app convention exists to import. Tag: D4, D8, X14.
4. **Heavy letters get a small menu, not another letter.** lazygit does this for discard options. Candidate for deal stage (`s`) and sync run, in place of `S` and `R`. Tag: D2, Deals module, Sync module, X27.
5. **Case selects the level (kabmat) and the heavier twin (yazi `d`/`D`, lazygit).** We already use `D` dismiss, `X` purge, `C` Company review. Rule to adopt: shift is the heavier or wider twin of the same verb. `C` and `T` in lists break it today. Tag: D7, Organizations, Archived, X27.
6. **Take the `g`/`z` prefix idea only as a scope word.** VisiData `g` = all marked, `z` = one. We have `space` and `*`. No change needed. Tag: People, Follow-up, Birthdays (mark verbs), X12.
7. **Put view-level settings on a leader or the palette, not bare letters.** sqlit's leader menu is the model. `space` is taken for mark in our lists, so a leader would need another key. Candidates `o` sort, `m` level, `T` filter. Tag: D7, People, Merge, Archived, X28.
8. **Bar grammar: first word an object kind, then scope words** (k9s `:pod /fred`, aerc typed commands). Matches objects first. Add inline grayscale hints for the scope words (k9s and taskwarrior-tui show none inline, so we go further). Tag: bar, [[show-syntax-inline-as-you-type]], X17.
9. **Live narrowing as you type in the bar and in `/` filters** (taskwarrior-tui, k9s). Tag: bar, all list modules.
10. **Filter syntax with negation and fuzzy flags** (k9s `/!`, `/-f`). Optional and only if the bar gets filter words. Tag: bar, X4.
11. **Help generated from the registry with a description on every rule** (yazi `desc`, `noop` hides from which-key, gh-dash `name`, aerc annotations). Tag: D10, D9, X29.
12. **Per-context key maps with inherit and erase** (aerc `[messages]`, `$noinherit`, empty binding erases). Our `ConflictFor` should compare view and mode, not action group. Tag: D9, X29.
13. **Key hint popup after a prefix, with a delay** (aerc 200 ms). Needed only if a prefix key is introduced (item 2 or item 7). Tag: D6, D7.
14. **Same verbs at every level of nesting** (kabmat main menu, board and checklist). Supports one verb set and shows the gap where org and deal pages cannot add a task or note. Tag: Organizations, Deals, X16, X13, X8.
15. **Tag as a saved search and a tagged set followed by a command** (sup). Matches bar tag rows (`bar-tag-link`, `bar-tag-unlink`, `bar-tag-delete`). Tag: bar, X26.
16. **Sections as saved filters, user-defined key plus command template** (gh-dash). Possible module kind: a module defined by a stored filter. Possible extension door for custom keys. Tag: modules, D12, [[extension-ladder]].
17. **Derived views as a stack with a pop key** (VisiData `q`, `ctrl+^`, `S` sheet list). Maps to bar results opening as views and back with `esc`. Tag: bar, [[bar-results-act-like-views]].
18. **Relations shown by drill-down or a jump command** (lazygit `enter`, gh-dash `viewIssues`/`viewPrs`, k9s xray). None draws inline link rows the way our record page does, so there is nothing to import beyond a "jump to the linked kind" key. Tag: Organizations, Deals, X8, X20.
19. **Prompt text editing uses readline keys** (taskwarrior-tui, matches our prompts). `ctrl+a` is line start there, same as our D12 note. Tag: D12, D11.
20. **Bare `z` undo is common** (lazygit, sup uses `u`, VisiData `U`, taskwarrior-tui `u`). Tag: D8, X9, X20.

## Conflicts with current decisions (flag for owner)

- F1. **Modifier-guarded destructive keys (k9s `ctrl-d`) conflict with bare `d` delete.** We keep bare `d` because delete is soft. Owner confirm. Tag: D4.
- F2. **taskwarrior-tui `d` done and `x` delete, kabmat `c` create, yazi `d` trash and `D` delete.** Direct opposites of our `x` done, `d` delete, `a` add, `c` contacted. No change recommended. Tag: Tasks.
- F3. **A leader key (sqlit) needs a free key.** `space` is mark in lists and `ctrl+t`, `ctrl+a` are taken. Do not adopt without owner choice. Tag: D6, D7.
- F4. **aerc binds each key as a typed command, with the key map and the command line the same list.** Our registry holds actions, with the bar verbs separate. Merging them would change [[keys-are-rebindable]]. Owner decision whether bar verbs and key actions should be one list. Tag: D9, X17.
- F5. **sup uses capital letters as go-to (`I`, `U`, `C`, `L`).** Conflicts with our rule that shifted letters are heavier twins (`C` is Company review today). Tag: D6, D7.
- F6. **gh-dash modules as stored filters** would make "module" mean a view definition, wider than today's [[modules-are-user-arranged]] (fixed set of kinds, user chooses and orders). Tag: modules.

## Could not verify
- abook (T11): home page only. WTF (T12): home page and README only. sup (T9): keyboard page only partly, labels and search syntax not read. aerc default `[messages]` file read, but `aerc-binds` defaults and key hint behavior beyond config options not read. gh-dash: default keys and search syntax not read. VisiData: `space` palette, joins and command menus not read. k9s: header hint bar and relation navigation not described. yazi: the three-pane layout and which-key popup not confirmed by the pages. lazygit panel focus keys taken from the audit's earlier read.
- Every key list passed through a summarizing fetch model, so single entries may be misreported. Re-read before quoting a key in a decision.

## Sources
- lazygit keybindings: https://github.com/jesseduffield/lazygit/blob/master/docs/keybindings/Keybindings_en.md
- k9s commands and key table: https://k9scli.io/topics/commands/
- k9s hotkeys: https://k9scli.io/topics/hotkeys/
- yazi quick start: https://yazi-rs.github.io/docs/quick-start
- yazi keymap: https://yazi-rs.github.io/docs/configuration/keymap
- VisiData docs index: https://www.visidata.org/docs/
- VisiData navigation: https://www.visidata.org/docs/navigate/
- VisiData cheat sheet (third party): https://jsvine.github.io/visidata-cheat-sheet/en/
- aerc wiki index: https://man.sr.ht/~rjarry/aerc/
- aerc-config(5): https://man.archlinux.org/man/aerc-config.5
- aerc-binds(5): https://man.archlinux.org/man/aerc-binds.5
- aerc default binds: https://github.com/rjarry/aerc/blob/master/config/binds.conf
- gh-dash repo: https://github.com/dlvhdr/gh-dash
- gh-dash keybinding config: https://www.gh-dash.dev/configuration/keybindings/
- gh-dash getting started: https://www.gh-dash.dev/getting-started/usage/
- taskwarrior-tui repo: https://github.com/kdheepak/taskwarrior-tui
- taskwarrior-tui keybindings: https://kdheepak.com/taskwarrior-tui/keybindings/
- kabmat: https://github.com/PlankCipher/kabmat
- sup site: https://supmua.dev/
- sup keyboard reference: https://github.com/sup-heliotrope/sup/wiki/Keyboard-reference
- sqlit README mirror (project github.com/Maxteabag/sqlit, repo URL seen in search, search): https://cdn.jsdelivr.net/gh/maxteabag/sqlit@main/README.md
- abook: https://abook.sourceforge.io/
- WTF: https://wtfutil.com/ and https://github.com/wtfutil/wtf/blob/master/README.md
