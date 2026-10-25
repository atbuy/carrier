[![CI](https://github.com/atbuy/carrier/actions/workflows/ci.yml/badge.svg)](https://github.com/atbuy/carrier/actions/workflows/ci.yml)
[![Docs](https://github.com/atbuy/carrier/actions/workflows/docs.yml/badge.svg)](https://github.com/atbuy/carrier/actions/workflows/docs.yml)
[![codecov](https://codecov.io/gh/atbuy/carrier/graph/badge.svg)](https://codecov.io/gh/atbuy/carrier)

<img src="docs/assets/logo_full.svg" alt="carrier logo" width="420" />

# carrier

`carrier` is a local developer command logger. It records command executions, working directory, timing, exit code, output logs, Git metadata, and optional desktop notifications.

Documentation: [https://atbuy.github.io/carrier/](https://atbuy.github.io/carrier/) · Changes: [CHANGELOG.md](CHANGELOG.md) or `carrier self changelog`

## Quick start

Install, set up, and record one command:

```bash
curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh
carrier setup
carrier run go test ./...
carrier last
```

`carrier setup` walks through the main settings one step at a time: recording every terminal, similar-failure hints, notifications, and the short `c` alias. Recording every terminal also sets up tab completion for `carrier` and `c`, so `c show <Tab>` lists recent runs and `c a <Tab>` your sessions. It shows each change before making it, edits existing config files in place, and backs them up first. `carrier setup --dry-run` only shows what it would change.

To set the alias by hand instead:

```bash
alias c='carrier'
```

Then daily usage becomes:

```bash
c run go test ./...
c last
c show <run-id>     # or -1 for the latest run, or a label
c rerun -1
c attach BUG-1234  # attach to a session by its label
c a                # or pick one from a list
c search "connection refused"
```

New users should start with the [Getting started guide](https://atbuy.github.io/carrier/tutorial/getting-started/). Advanced users should see [Power-user workflows](https://atbuy.github.io/carrier/advanced/power-user-workflows/) and [Shell mode](https://atbuy.github.io/carrier/advanced/shell-mode/).

## Features

- `carrier run <command...>` records one command while preserving the child exit code.
- stdout and stderr stream live to the terminal and are stored separately.
- persisted logs are redacted by default with buffered matching for split secrets.
- persisted logs are capped by `storage.max_output_mb` to protect disk usage.
- search uses SQLite FTS over commands, working directories, and output snippets.
- SQLite metadata is stored in `~/.local/share/carrier/carrier.db`.
- output logs are stored in `~/.local/share/carrier/runs/`.
- optional notifications use `notify-send` on Linux.
- human-readable output is colorized on TTYs; use `NO_COLOR=1` to disable or `CARRIER_COLOR=always` to force.
- `carrier shell` starts a recorded shell session for zsh, bash, or fish. Each shell session groups its runs in `carrier history`, `carrier history --session my-session`, and `carrier history --sessions-only`.
- `carrier attach <id-or-label>` switches a recorded shell to an existing session, or opens it in a new shell from any other terminal. Without a session it lists recent ones to pick from.
- `carrier session pin` pins a directory to a session, so every new terminal or tmux window started there joins it.
- `carrier session list` lists shell sessions; `carrier session label` sets or clears a session label.
- `eval "$(carrier init zsh)"` in your shell config records every terminal in an automatic session; unlabeled ones expire after 30 days (`shell.auto_session_retention`).
- optional similar-failure hints point at an earlier run with the same error (`[hints] similar_failures = true`); `carrier similar <id>` lists them.
- `carrier note <text>` adds a note to the current shell session or a run, so you can record why you ran something. Notes show up in `history` and `show` and are searchable.

## Install

### Installer Script

Linux and macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh
```

Install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh -s -- --version v1.0.0
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1 | iex
```

Install a specific version on Windows:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1))) -Version v1.0.0"
```

Installers download prebuilt binaries from [GitHub Releases](https://github.com/atbuy/carrier/releases). Release notes include checksum verification details for users who want manual validation.

### With Go

Install the latest released module with:

```bash
go install github.com/atbuy/carrier/cmd/carrier@latest
```

Make sure Go's bin directory is on your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Then verify the install:

```bash
carrier --help
```

### From Source

Clone the repository and build the local binary:

```bash
git clone https://github.com/atbuy/carrier.git
cd carrier
make build
./bin/carrier --help
```

To install from a source checkout:

```bash
make install
```

### Recommended Alias

Most examples assume this shell alias:

```bash
alias c='carrier'
```

Add it to your shell config, for example `~/.zshrc` or `~/.bashrc`.

## Common Commands

```bash
carrier run go test ./...
carrier -n run docker compose build
carrier -N run ls
carrier shell                      # start a recorded shell session
carrier shell 'my-session'         # start a named session
carrier attach my-session          # switch to an existing session
carrier attach                     # pick a session to switch to
carrier session pin my-session     # new terminals in this directory join it
carrier status                     # is this terminal recorded, and into which session?
carrier session show my-session    # replay a session as a timeline
carrier session export my-session  # the same as Markdown, for a PR or ticket
carrier session end -m "fixed it"  # end a session and record its outcome
carrier session list               # list sessions
carrier session label 5 my-label   # label a session
carrier session keep               # keep the current automatic session
carrier init zsh                   # shell config line to record every terminal
carrier note suspect DNS cache     # note in the current session
carrier notes                      # list notes
carrier last
carrier last --json
carrier show 42
carrier show 42 --json
carrier tail 42
carrier tail 42 --stream stdout
carrier tail 42 --stream stderr
carrier failed
carrier running
carrier running --json
carrier search "connection refused"
carrier similar 42
carrier stats
carrier stats --json
carrier export 42
carrier rerun 42
carrier doctor
carrier config
carrier config set shell.branch_sessions follow
carrier config path
carrier config show
carrier config check
carrier config init
carrier version
carrier self update
carrier self changelog
carrier clean --older-than 30d --dry-run
carrier clean --older-than 30d -d
carrier clean --older-than 30d --yes
```

## Configuration

Config is read from `~/.config/carrier/config.toml`. Defaults are used when the file is absent.

```toml
[storage]
data_dir = "~/.local/share/carrier"
max_output_mb = 20

[redaction]
enabled = true
patterns = [
  'Bearer [A-Za-z0-9._-]+',
  '(?i)(password|passwd|token|api[_-]?key|secret|access[_-]?token|refresh[_-]?token)\s*[:=]\s*\S+',
  'AKIA[0-9A-Z]{16}',
  '-----BEGIN PRIVATE KEY-----[\s\S]*?-----END PRIVATE KEY-----',
]

[notify]
min_duration = "10s"
success = true
failure = true

[shell]
program = ""
ignore_commands = ["nvim", "vim", "less", "man", "fzf", "yazi", "lazygit", "tmux"]
auto_session_retention = "30d"
```

## Shell mode

`carrier shell` records every command line in a session, with its terminal output, exit code, timing, and Git details, for zsh, bash, and fish. `carrier setup` can start every new terminal in a recorded session. `carrier run` records a single command and keeps its stdout and stderr in separate logs.

## Development

```bash
make fmt
make lint
make test
make build
make run ARGS="run echo hello"
```

Documentation is built with Zensical:

```bash
make docs-build
make docs-serve
```

If Go VCS stamping fails in a checkout with dubious Git ownership, use:

```bash
go build -buildvcs=false ./cmd/carrier
```
