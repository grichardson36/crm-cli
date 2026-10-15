# crm — your contacts, in your terminal.

![crm banner](.github/banner-crm-cli.png)

[![CI](https://github.com/jdanielnd/crm-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/jdanielnd/crm-cli/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/jdanielnd/crm-cli)](https://goreportcard.com/report/github.com/jdanielnd/crm-cli)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A local-first personal CRM that lives in your terminal. Manage contacts, organizations, interactions, deals, and tasks — all from the command line. Ships with a built-in [MCP server](https://modelcontextprotocol.io/) so AI agents like Claude can read and update your CRM directly.

Single static binary. SQLite database. No cloud. No accounts. Your data stays on your machine.

> This is a fork of [jdanielnd/crm-cli](https://github.com/jdanielnd/crm-cli) (MIT). The original copyright and license notice are kept. See [License](#license).

## Install with your AI agent

Paste this into any agent that can run shell commands. It installs `crm`, creates your database, checks the setup and walks you through the first steps.

```text
Install and set up the crm program for me. Fetch https://raw.githubusercontent.com/grichardson36/crm-cli/main/INSTALL.md and follow its numbered steps in order. Run each check, show me the result, and ask before you change anything outside the crm data directory. When setup passes `crm doctor`, walk me through the first three things to try.
```

The steps are in [INSTALL.md](INSTALL.md). Agents learn the program from the program itself: [AGENTS.md](AGENTS.md), `crm help`, `crm schema -f json` and `crm agent`. No vendor-specific file is needed.

## Features

- **People & Organizations** — contacts with full profiles, linked to companies
- **Interaction Log** — track calls, emails, meetings, notes, and messages
- **Deals & Pipeline** — sales opportunities with stages, values, and a pipeline view
- **Tasks** — follow-ups with due dates, priorities, and completion tracking
- **Tags** — labels on any entity for flexible organization
- **Relationships** — person-to-person links (colleague, mentor, referred-by, …)
- **Full-Text Search** — find anything across all entities instantly
- **Context Briefing** — one command to get everything about a person before a meeting
- **Keep in Touch** — cadences, a due list, birthdays, duplicate detection and merge, archive and restore
- **Agent-Ready** — built-in help topics, a JSON command manifest, `--dry-run` on every change, JSON errors, and an optional MCP server
- **Multiple Output Formats** — table, JSON, CSV, TSV — pipe into anything
- **Zero Dependencies** — single binary, pure-Go SQLite, runs anywhere

## Install

The easiest way is the prompt above. By hand:

1. Download the archive for your system from [GitHub Releases](https://github.com/grichardson36/crm-cli/releases): `crm_<os>_<arch>.tar.gz` (`.zip` on Windows), with os `linux`, `darwin` or `windows` and arch `amd64` or `arm64`. Check it against `checksums.txt`.
2. Put `crm` on your PATH.
3. Run `crm --version`, then `crm init` and `crm doctor`.

Or build from source with Go 1.24+:

```bash
go build -o crm ./cmd/crm
```

## Quick Start

```bash
# Create your database and check the setup
crm init
crm doctor

# Learn the program from the program
crm help
crm help overview

# Add a company
crm org add "Acme Corp" --domain acme.com

# Add a contact and link them to the org
crm person add "Jane Smith" --email jane@example.com --org 1

# Log a call
crm log call 1 --subject "Intro call"

# Get a full briefing before your next meeting
crm context 1

# See your dashboard
crm status
```

## Commands

`crm help` has a guide for each area below, with examples that are tested on every build. `crm <command> --help` shows the flags of one command, and `crm schema -f json` lists them all.

### Setup and discovery

```bash
crm init                                 # create the data directory and database
crm init --domain work                   # a separate database: ~/.crm/work.db
crm where                                # which database is active and how it was chosen
crm domains                              # every database in ~/.crm
crm doctor                               # health check with a fix for each problem
crm help [topic]                         # built-in guides; --search <text> to find a word
crm schema -f json                       # every command, flag, example and exit code
crm agent                                # the quick start for AI agents (AGENTS.md)
crm backup ~/.crm/backups/crm.db         # consistent copy of the database
```

### People

```bash
crm person add "Jane Smith" --email jane@example.com --phone 555-1234 --org 1
crm person list                          # all contacts
crm person list --tag vip --limit 10     # filtered
crm person show 1                        # full details
crm person edit 1 --title "CTO" --company "Acme Corp"
crm person archive 1                     # reversible; crm person restore 1 brings them back
crm person edit 1 --title CTO --dry-run  # preview any change without writing
```

### Organizations

```bash
crm org add "Acme Corp" --domain acme.com --industry SaaS
crm org list
crm org show 1 --with-people             # show org + its members
crm org edit 1 --domain acme.io
crm org delete 1
```

### Interactions

```bash
crm log call 1 --subject "Discussed roadmap" --content "Agreed on Q2 timeline"
crm log email 1 --subject "Follow-up" --direction outbound
crm log meeting 1 2 --subject "Product demo" --at "2026-03-05 14:00"
crm log note 1 --subject "Quick check-in" --content "Seemed interested"
crm log list --person 1 --limit 5        # recent interactions with person
```

### Deals

```bash
crm deal add "Website Redesign" --value 15000 --person 1 --stage proposal
crm deal list --open                     # exclude won/lost
crm deal list --stage proposal
crm deal show 1
crm deal edit 1 --stage won --closed-at 2026-03-15
crm deal delete 1
crm deal pipeline                        # summary by stage
```

### Tasks

```bash
crm task add "Follow up on proposal" --person 1 --due 2026-03-14 --priority high
crm task list                            # open tasks
crm task list --overdue                  # past due
crm task list --all                      # include completed
crm task show 1
crm task edit 1 --priority medium
crm task done 1                          # mark completed
crm task delete 1
```

### Tags

```bash
crm tag apply person 1 "vip"
crm tag apply deal 1 "q2"
crm tag show person 1                    # tags on a person
crm tag remove person 1 "vip"
crm tag list                             # all tags
crm tag delete "old-tag" --yes           # remove tag entirely (cannot be undone)
```

### Relationships

```bash
crm person relate 1 2 --type colleague --notes "Met at ReactConf"
crm person relationships 1               # list all relationships
crm person unrelate 1                    # remove by relationship ID
```

Relationship types: `colleague`, `friend`, `manager`, `mentor`, `referred-by`

### Search

```bash
crm search "jane"                        # searches people, orgs, interactions, deals
crm search "roadmap" --type interaction  # filter by entity type
```

### Context Briefing

```bash
crm context 1                            # full briefing by person ID
crm context "Jane Smith"                 # or by name
```

Returns person profile, organization, recent interactions, open deals, pending tasks, relationships, and tags — everything you need before a meeting.

### Dashboard

```bash
crm status
```

Shows contact/org counts, open deals with total value, task counts, overdue items, pipeline breakdown, and recent activity.

### Quick input (short syntax)

Type meaning inline. `#tag`, `@fri 3pm`, `!deadline`, `~30m`, `*3m` (keep in touch), `+Name`, `^high`. The same text works in the CLI, the TUI command bar (`ctrl+k`) and for agents.

```bash
crm do "n Sam called about the deal #deal @fri"   # note, tag and a reminder in one line
crm do "set Sam: company=Acme title=CTO" --dry-run
crm task add "Send proposal @mon 3pm ^high +Sam"
crm person add "Sam Lee #friend *3m"
```

See `crm help short-syntax`.

## Output Formats

Every command supports `--format` / `-f`:

```bash
crm person list                          # table (default in TTY)
crm person list -f json                  # JSON (default when piped)
crm person list -f csv                   # CSV
crm person list -f tsv                   # TSV
```

Use `-q` / `--quiet` for minimal output (just IDs), useful for scripting:

```bash
crm person list --tag vip -q | xargs -I{} crm tag apply person {} "priority"
```

## Piping & Composition

`crm` follows Unix conventions — data to stdout, messages to stderr, structured exit codes — so it plays well with other tools:

```bash
# Bulk tag everyone at an org
crm person list -f json | jq '.[] | select(.org_id == 1) | .id' | xargs -I{} crm tag apply person {} "acme-team"

# Interactive selection with fzf
crm person list -f tsv | fzf | cut -f1 | xargs crm person show

# Export contacts to CSV
crm person list -f csv > contacts.csv

# Overdue task notifications
crm task list --overdue -f json | jq -r '.[] | "OVERDUE: \(.title)"'
```

Exit codes: `0` success, `1` error, `2` usage, `3` not found, `4` conflict, `10` database error.

With `-f json`, errors are printed to stderr as `{"error":{"code":"not_found","message":"...","exit_code":3}}`.

## AI agents

Any agent that can run a shell command can use `crm` through the CLI with `-f json`. It needs no MCP server, skill or vendor-specific file.

- [AGENTS.md](AGENTS.md) is the quick start: where data lives, the first commands to run and the safety rules. `crm agent` prints it.
- `crm help agents` is the full guide. `crm help --search <text>` finds a word in every guide.
- `crm schema -f json` lists every command, flag, example, which commands change data, exit codes and MCP tool names.
- Every command that changes data has `--dry-run`. Commands that cannot be undone need `--yes`.

## Optional: MCP server

`crm` also includes a built-in [MCP](https://modelcontextprotocol.io/) server for agents that prefer a structured protocol. It adds `crm_help` and `crm_schema` tools, and the help topics as resources named `crm://help/<topic>`.

### Claude Code

```bash
claude mcp add crm -- crm mcp serve
```

Then point the agent at [`AGENTS.md`](AGENTS.md) so it knows how to use the CRM effectively.

### Claude Desktop (macOS)

1. Open the config file at `~/Library/Application Support/Claude/claude_desktop_config.json`

2. Add the CRM server (if the file already has other servers, add the `"crm"` entry inside the existing `mcpServers` object):

```json
{
  "mcpServers": {
    "crm": {
      "command": "/opt/homebrew/bin/crm",
      "args": ["mcp", "serve"]
    }
  }
}
```

> **Note:** Use the full path to the binary. Homebrew on Apple Silicon installs to `/opt/homebrew/bin/crm`, on Intel Macs to `/usr/local/bin/crm`. Run `which crm` to find yours.

3. Restart Claude Desktop

4. Create a new **Project**, click the project settings, and paste the contents of [`AGENTS.md`](AGENTS.md) (or the longer workflow notes in [`AGENT.md`](AGENT.md)) into the project instructions. This teaches Claude when to log interactions, how to maintain contact summaries, tagging conventions, and more.

### Available Tools

| Tool | Description |
|------|-------------|
| `crm_person_search` | Search people by name, email, tag, org |
| `crm_person_get` | Get full details for a person |
| `crm_person_create` | Create a new person |
| `crm_person_update` | Update person fields |
| `crm_person_delete` | Delete (archive) a person |
| `crm_person_relate` | Create relationship between people |
| `crm_org_search` | Search organizations |
| `crm_org_get` | Get org details with members |
| `crm_interaction_log` | Log a call, email, meeting, or note |
| `crm_interaction_list` | List interactions for a person |
| `crm_search` | Cross-entity full-text search |
| `crm_context` | Full person briefing |
| `crm_deal_create` | Create a deal |
| `crm_deal_update` | Update deal stage/fields |
| `crm_task_create` | Create a task |
| `crm_task_list` | List open tasks |
| `crm_tag_apply` | Apply a tag |
| `crm_stats` | CRM summary stats |
| `crm_help` | Built-in help topics: list, read one, or search |
| `crm_schema` | Machine-readable manifest of the CLI |
| `crm_do` | Run one line of text: note, task, log, set, tag, snooze |

This table lists the main tools. `crm schema -f json` lists every tool name. `AGENT.md` has workflow notes for Claude users.

### Example AI Workflow

After a meeting, tell Claude:

> "Just had a great meeting with Jane. We agreed on the timeline, she'll sign next week."

Claude can:
1. Log the interaction with `crm_interaction_log`
2. Update the deal stage with `crm_deal_update`
3. Create a follow-up task with `crm_task_create`
4. Update Jane's summary with `crm_person_update`

The `summary` field on contacts acts as a living dossier that AI agents maintain — reading it before meetings for context, updating it after interactions with new facts and preferences.

## Configuration

The database lives at `~/.crm/crm.db` by default. Override with:

```bash
# Flag (highest priority)
crm --db /path/to/crm.db person list

# Environment variable
export CRM_DB=/path/to/crm.db
```

`crm init` creates the directory and database. They are also created on first use. `crm where` shows the active database and `crm domains` lists every database in `~/.crm`. Keep several databases (domains) to separate, for example, personal and work contacts: `crm init --domain work`, then `crm --db ~/.crm/work.db ...`.

### Do not sync the database folder

Do not put the database in OneDrive, Dropbox or iCloud. Their sync clients copy the file and its write-ahead log at different moments, which can corrupt SQLite. Keep the database local and write backups into the synced folder:

```bash
crm backup "$HOME/Library/Mobile Documents/com~apple~CloudDocs/crm-backups/crm.db"
```

`crm doctor` warns when the database is inside one of these folders.

## Building from Source

```bash
git clone https://github.com/grichardson36/crm-cli.git
cd crm-cli
go build -o crm ./cmd/crm
go test ./...
```

Requires Go 1.24+. No CGO — builds anywhere Go runs.

## Tech Stack

| Component | Choice |
|-----------|--------|
| Language | Go |
| Database | SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, no CGO) |
| CLI | [Cobra](https://github.com/spf13/cobra) |
| MCP | [mcp-go](https://github.com/mark3labs/mcp-go) |
| Search | SQLite FTS5 |

## License

MIT. This project is a fork of [jdanielnd/crm-cli](https://github.com/jdanielnd/crm-cli), Copyright (c) the original authors, released under the MIT License. The original copyright and license notice stay with all copies of this software.
