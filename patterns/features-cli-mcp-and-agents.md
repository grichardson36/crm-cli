---
type: feature-area
---
# CLI, MCP and the agent surface

The CLI with JSON, the MCP server, embedded help, the schema manifest and install by prompt.

Moved from FEATURES.md on 2026-10-29. Every row keeps its ID and text. Update a row's State and Now in the same session you change the feature ([[keep-the-tracker-accurate]]). Column meanings and the row lifecycle are in [[feature-rules]].

**Follows patterns:** [[cli-json-is-the-baseline]], [[agents-never-get-prompts]], [[three-doors-one-core]], [[tests-for-every-door]], [[extension-ladder]], [[name-things-once]]

**See also:** [[index]]

Goal: any agent, or a person, learns the whole program from the program, the way `man` and `--help` work. No dependency on one vendor's skill format. Pi has no native MCP, so the plain CLI with JSON output must be enough on its own.

## Rows

| ID | Feature (evidence) | From | State | Now | Open questions | Action plan | Better by | P |
|---|---|---|---|---|---|---|---|---|
| F04 | REST API, Zapier (docs) | Dex | SKIP | none | - | CLI/MCP replace it | - |  |
| F05 | MCP server for AI (docs, API) | Dex | DONE | `crm mcp serve`. MERGED `objects-first` adds `crm_<object>_link|unlink`, `crm_org_create|update|delete|restore`, `crm_deal_delete|restore`, `crm_task_restore`, `crm_tag_remove`. | - | Keep parity: each new CLI feature gets a tool. Missing tools: group, view, custom fields, digest, resurface | extensible, smart | P1 |
| F06 | Agent skill (docs) | Dex | DONE | `~/.claude/skills/crm/SKILL.md` | - | Update on every new command | - |  |
| F07 | Copilot, voice mode, SMS interface, ChatGPT app (docs) | Claude agents | BEYOND | Claude agents with the skill/MCP | - | none | - |  |
| F08 | Teams: shared workspaces, Rolodex, share contacts (docs) | Dex | SKIP | One DB per domain | - |  | - |  |
| F09 | Settings: notifications, billing, dark mode (docs) | Dex | SKIP | Terminal theme | - |  | - |  |
| F11 | Local-first, offline, scriptable, JSON output (not in Dex) | Us | BEYOND | SQLite, `-f json` | - | none | - |  |
| I01 | Embedded help topics: `crm help` lists topics, `crm help <topic>` prints a short guide with examples (people, merge, keep-in-touch, archive, note-types, sync, dav, tui, domains, agents). Same text is shown in the TUI help and palette | Us | PART | MERGED: `crm help` 18 topics. INTEGRATION: more topics (calendar, caldav, book, reminders, keys, theme, short-syntax and others). | Format: Markdown rendered to the terminal? | Markdown files in `internal/help/topics/*.md` via `go:embed`; `crm help --search <text>`; `crm help <topic> -f json`. TUI: palette action "Help: <topic>" opens the same text in an overlay | fast, simple | P1 |
| I02 | Man pages and shell completion generated from the command tree | Us | DONE | `crm docs markdown|man <dir>` via cobra/doc. | - | `crm docs man <dir>` (cobra/doc), `crm docs markdown <dir>`; release script generates them | simple | P3 |
| I03 | Machine-readable manifest: `crm schema` prints every command, flag, argument, output field, exit code and example as JSON; includes MCP tool names | Us | DONE | `crm schema -f json`: commands, flags, examples, mutates/destructive annotations, exit codes, MCP tool names. Test fails if a command lacks Short, example or mutates annotation. | - | Walk the Cobra tree plus annotations on commands (`examples`, `output_fields`); golden test fails when a command lacks examples. An agent runs one command and knows the whole surface | smart, extensible | P1 |
| I04 | Harness-neutral agent entry points: root `AGENTS.md` and `README.md` (open conventions every agent reads), `crm agent` quick start, `crm help`, `crm schema`. No per-harness file formats are required to use or install the program | Us | DONE | Root `AGENTS.md`, `README.md`, `INSTALL.md`, `crm agent`, `crm agent install-prompt`. No vendor-specific files; test enforces it. | - | DECIDED 2026-10-14: only open standards. `AGENTS.md` (what the project is, where data lives, how to run `crm agent`, `crm help`, `crm schema`, safety rules) and `README.md` (human install and use) ship in the repo and are also printed by `crm agent`. A skill or MCP config is an optional generated extra (`crm agent export <name>`), never the primary path. Any agent that can read files and run a shell command reaches the same triggers | extensible, simple | P1 |
| I05 | MCP self-description: tool descriptions come from the same registry as the CLI, plus an MCP resource `crm://help/<topic>` and `crm_help` / `crm_schema` tools | Us | PART | MCP `crm_help`, `crm_schema` and `crm://help/<topic>` resources exist. Not yet one shared command registry feeding CLI, MCP and TUI. | - | Single command registry feeding CLI, MCP, TUI palette and docs so they cannot drift (same idea as the TUI action registry) | extensible, safe | P1 |
| I08 | Examples as tests: every help example is executed in CI against a fixture database so docs cannot rot | Us | DONE | Help examples tagged `crm-example` run as tests against a seeded temp database. | - | Mark examples in topics with a fence tag; test harness runs them | safe | P2 |
| I10 | Install-by-prompt: a short prompt the user pastes to any agent ("install and set up crm from <repo URL>"). The agent reads `AGENTS.md`/`INSTALL.md`, downloads the right binary, creates the data directory, runs setup, verifies, and walks the user through first use | Us | PART | `INSTALL.md` and the paste-in prompt exist; GoReleaser config fixed (version ldflags, archives). Not yet validated by a fresh agent following it against a real release. | Where is the canonical prompt hosted (repo README)? Release binaries via GoReleaser? | `INSTALL.md` written as numbered, checkable steps with expected output per step; GoReleaser releases for Windows/macOS/Linux (amd64, arm64); a prompt snippet in `README.md`. Test it by having a fresh agent follow it in a clean directory | simple, extensible | P1 |
