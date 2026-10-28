# INSTALL.md

Install and set up `crm` from a pasted prompt. This file is written for an AI agent that can run shell commands. A person can follow it too.

## Prompt to paste into your agent

```text
Install and set up the crm program for me. Fetch https://raw.githubusercontent.com/grichardson36/crm-cli/main/INSTALL.md and follow its numbered steps in order. Run each check, show me the result, and ask before you change anything outside the crm data directory. When setup passes `crm doctor`, walk me through the first three things to try.
```

`crm agent install-prompt` prints the same prompt.

## Rules for the agent

- Do each step in order. After each step, run its check and compare with the expected output. Stop and tell the user if a check fails.
- Ask the user before you create directories outside `~/.crm`, edit PATH, or download anything.
- Use only `crm` commands on the data. Never open or copy the database file yourself.
- Everything here works with a plain shell. No MCP server, skill or vendor-specific file is needed.

## Steps

### 1. Detect the operating system and CPU

Linux and macOS:

```sh
uname -s
uname -m
```

Windows (PowerShell):

```powershell
$env:OS
$env:PROCESSOR_ARCHITECTURE
```

Map the output to the release name parts:

| Output | os | arch |
|---|---|---|
| `Linux` | `linux` | |
| `Darwin` | `darwin` | |
| `Windows_NT` | `windows` | |
| `x86_64`, `AMD64` | | `amd64` |
| `arm64`, `aarch64`, `ARM64` | | `arm64` |

Check: you have one os and one arch. Example: `darwin` and `arm64`.

### 2. Check whether crm is already installed

```sh
crm --version
```

Expected when installed: `crm version 1.2.3` (a built-from-source binary prints `crm version dev`). If it is installed, run `crm doctor`. If every check is `ok` or `warn`, skip to step 7.

Expected when not installed: `command not found` (Windows: `The term 'crm' is not recognized`). Continue.

### 3. Download the release

Release assets are named `crm_<os>_<arch>.tar.gz`, or `crm_windows_<arch>.zip` on Windows, and `checksums.txt` lists their SHA-256. The URLs do not contain a version:

```
https://github.com/grichardson36/crm-cli/releases/latest/download/crm_<os>_<arch>.tar.gz
https://github.com/grichardson36/crm-cli/releases/latest/download/checksums.txt
```

Linux and macOS, with `os` and `arch` from step 1 (example: `linux` and `amd64`):

```sh
mkdir -p /tmp/crm-install && cd /tmp/crm-install
curl -fsSLO https://github.com/grichardson36/crm-cli/releases/latest/download/crm_linux_amd64.tar.gz
curl -fsSLO https://github.com/grichardson36/crm-cli/releases/latest/download/checksums.txt
grep crm_linux_amd64.tar.gz checksums.txt | sha256sum -c -        # macOS: shasum -a 256 -c -
tar -xzf crm_linux_amd64.tar.gz
```

Windows (PowerShell), example `amd64`:

```powershell
$dir = Join-Path $env:TEMP "crm-install"; New-Item -ItemType Directory -Force $dir | Out-Null; Set-Location $dir
Invoke-WebRequest https://github.com/grichardson36/crm-cli/releases/latest/download/crm_windows_amd64.zip -OutFile crm_windows_amd64.zip
Invoke-WebRequest https://github.com/grichardson36/crm-cli/releases/latest/download/checksums.txt -OutFile checksums.txt
$want = ((Select-String crm_windows_amd64.zip checksums.txt).Line -split '\s+')[0]
$got = (Get-FileHash crm_windows_amd64.zip -Algorithm SHA256).Hash
if ($want -ieq $got) { "checksum OK" } else { "checksum MISMATCH" }
Expand-Archive crm_windows_amd64.zip -DestinationPath . -Force
```

Check: `crm_linux_amd64.tar.gz: OK` (Windows: `checksum OK`), and a `crm` file (`crm.exe` on Windows) in the directory. Stop on a mismatch.

#### 3b. No release yet: build from source

If the download fails with HTTP 404, no release is published yet. Build from source. This needs Go 1.26 or newer (`go version`) and git. No C compiler is needed.

```sh
git clone https://github.com/grichardson36/crm-cli.git
cd crm-cli
go build -o crm ./cmd/crm
```

On Windows build `crm.exe`: `go build -o crm.exe ./cmd/crm`. Check: a `crm` file exists in the current directory. A binary built this way reports version `dev`.

### 4. Put crm on the PATH

Linux and macOS (ask the user first):

```sh
mkdir -p ~/.local/bin
install -m 0755 crm ~/.local/bin/crm
echo "$PATH" | tr ':' '\n' | grep -x "$HOME/.local/bin"
```

Check: the last command prints `/home/you/.local/bin` (or the macOS equivalent). If it prints nothing, add `export PATH="$HOME/.local/bin:$PATH"` to the shell profile (`~/.zshrc` or `~/.bashrc`) and open a new shell.

Windows (PowerShell, ask the user first):

```powershell
$bin = Join-Path $env:LOCALAPPDATA "Programs\crm"
New-Item -ItemType Directory -Force $bin | Out-Null
Copy-Item .\crm.exe $bin -Force
$user = [Environment]::GetEnvironmentVariable("Path", "User")
if (($user -split ';') -notcontains $bin) { [Environment]::SetEnvironmentVariable("Path", "$user;$bin", "User") }
$env:Path += ";$bin"
```

Check: `Get-Command crm` shows `crm.exe` in that directory.

### 5. Verify the binary

```sh
crm --version
```

Expected: `crm version 1.2.3` or `crm version dev`. If the shell says command not found, step 4 did not take effect. Open a new shell or fix PATH.

### 6. Create the database

Do not place the database in a OneDrive, Dropbox or iCloud folder: SQLite can corrupt there. Use the default location.

Tell the user where the data goes (`~/.crm/crm.db`, on Windows `%USERPROFILE%\.crm\crm.db`), then run:

```sh
crm init --yes
```

Expected: `Created domain "crm" at /home/you/.crm/crm.db (schema version 5)` followed by `Next steps`. Add `-f json` to get `{"domain":"crm","path":"...","created":true,...}`. It refuses to overwrite an existing database with exit code 4: in that case keep the existing data and continue.

Optional, ask the user: do they have a Dex export (Settings, Import/Export in Dex)? If so, preview the import and then run it:

```sh
crm import dex --contacts dex_contacts.csv --notes dex_notes.csv --dry-run
crm import dex --contacts dex_contacts.csv --notes dex_notes.csv
```

Expected: a JSON report with `contacts_imported` above 0. Re-running is safe.

A separate database for another part of life (work, a client) is `crm init --domain work --yes`. It creates `~/.crm/work.db`, used with `--db ~/.crm/work.db` or `CRM_DB`. See `crm help domains`.

### 7. Run the health check

```sh
crm doctor
```

Expected: every line starts with `ok` except possibly `backups`, which is `warn` on a new install, and a final line like `10 ok, 1 warn, 0 fail`. The exit code is 0 unless a check is `fail`. Fix each `fail` with the `fix:` line shown under it, then run `crm doctor` again.

Make the first backup to clear the warning:

```sh
mkdir -p ~/.crm/backups
crm backup ~/.crm/backups/crm-first.db
```

Expected: `Wrote /home/you/.crm/backups/crm-first.db`. On Windows use `New-Item -ItemType Directory -Force $HOME\.crm\backups` and `crm backup $HOME\.crm\backups\crm-first.db`.

### 8. Walk the user through the first three things to try

Use `-f table` so the user sees tables. Without it, a non-terminal prints JSON.

**Thing 1: add a person and note what happened.**

```sh
crm person add "Jane Smith" --email jane@example.com --company "Acme Corp" --keep-in-touch "1 mon" -f table
crm log note 1 --subject "Met at a conference" --content "Interested in a demo" -f table
```

Expected: a table with one row, `ID 1`, `Jane Smith`, `jane@example.com`, `Acme Corp`. Then a table with one `note` row, `Met at a conference`, `PEOPLE [1]`. Ask the user to replace Jane with a real person if they like.

**Thing 2: find them and get a briefing.**

```sh
crm search jane -f table
crm context 1 -f table
```

Expected: a one-row table, then a briefing that starts with `# Jane Smith (ID: 1)` and lists the email, company and the recent interaction.

**Thing 3: add a follow-up and see the dashboard.**

```sh
crm task add "Send demo invite" --person 1 --due 2026-11-01 --priority high -f table
crm status
```

Expected: a one-row task table with `PRIORITY high`, then two lines such as `1 contacts | 0 organizations | 0 open deals ($0)` and `1 open tasks | 0 overdue | 1 interactions this week`.

Then tell the user:

- `crm tui` opens the full-screen interface. `ctrl+k` opens the command palette and `?` lists every key.
- `crm due` lists who they are due to contact, based on the `--keep-in-touch` cadence.
- `crm help` lists the built-in guides. `crm help overview` is the best next read.
- `crm agent` prints the guide for AI agents. Use `--dry-run` before any change and ask before deleting.

### 9. Report back

Tell the user: the installed version, the database path, the result of `crm doctor`, where the first backup is, and that nothing else on the machine was changed except the PATH entry from step 4.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `crm: command not found` | PATH was not updated in this shell. Open a new shell, or run the binary by full path. |
| macOS says the binary cannot be opened because the developer cannot be verified | `xattr -d com.apple.quarantine ~/.local/bin/crm`, then run it again. |
| `crm init` exits with code 4 | A database already exists. Use it: `crm where`. |
| `crm doctor` says `cloud_folder` warn | Move the database out of the synced folder, set `CRM_DB`, and back up into the synced folder with `crm backup`. |
| Download is HTTP 404 | No release is published for your platform yet. Use step 3b. |

## Update

Download the new archive (step 3), then replace the binary (step 4) and reopen the program. The database is backed up to `~/.crm/backups/` and migrated automatically. Run `crm doctor`.
