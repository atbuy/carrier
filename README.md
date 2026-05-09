# carrier

[![CI](https://github.com/atbuy/carrier/actions/workflows/ci.yml/badge.svg)](https://github.com/atbuy/carrier/actions/workflows/ci.yml)
[![Docs](https://github.com/atbuy/carrier/actions/workflows/docs.yml/badge.svg)](https://github.com/atbuy/carrier/actions/workflows/docs.yml)
[![codecov](https://codecov.io/gh/atbuy/carrier/graph/badge.svg)](https://codecov.io/gh/atbuy/carrier)

`carrier` is a local developer command logger for Go-based single-binary installs. It records command executions, working directory, timing, exit code, output logs, Git metadata, and optional desktop notifications.

Documentation: [https://atbuy.github.io/carrier/](https://atbuy.github.io/carrier/)

Most users will alias it:

```bash
alias c='carrier'
```

Quick usage:

```bash
c run go test ./...
c last
c show 1
```

## Features

- `carrier run <command...>` records one command while preserving the child exit code.
- stdout and stderr stream live to the terminal and are stored separately.
- persisted logs are redacted by default.
- persisted logs are capped by `storage.max_output_mb` to protect disk usage.
- SQLite metadata is stored in `~/.local/share/carrier/carrier.db`.
- output logs are stored in `~/.local/share/carrier/runs/`.
- optional notifications use `notify-send` on Linux.
- `carrier shell` starts a PTY-backed tracked shell session.

## Install

From this checkout:

```bash
make build
./bin/carrier --help
```

For a module release:

```bash
go install github.com/atbuy/carrier/cmd/carrier@latest
```

## Common Commands

```bash
carrier run go test ./...
carrier -n run docker compose build
carrier -N run ls
carrier shell
carrier last
carrier show 42
carrier tail 42
carrier failed
carrier running
carrier search "connection refused"
carrier export 42
carrier rerun 42
carrier doctor
carrier clean --older-than 30d
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
  '(?i)(password|token|api_key)=\S+',
]

[notify]
min_duration = "10s"
success = true
failure = true

[shell]
program = ""
ignore_commands = ["nvim", "vim", "less", "man", "fzf", "yazi", "lazygit", "tmux"]
```

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
