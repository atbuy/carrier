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
- persisted logs are redacted by default with buffered matching for split secrets.
- persisted logs are capped by `storage.max_output_mb` to protect disk usage.
- SQLite metadata is stored in `~/.local/share/carrier/carrier.db`.
- output logs are stored in `~/.local/share/carrier/runs/`.
- optional notifications use `notify-send` on Linux.
- `carrier shell` starts a PTY-backed tracked shell session. This mode is alpha-quality and best-effort for zsh/bash.

## Install

### From GitHub Releases

Download a prebuilt archive from [GitHub Releases](https://github.com/atbuy/carrier/releases).

Linux x86_64 example:

```bash
version=v0.1.0
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/carrier-linux-amd64.tar.gz"
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
tar -xzf carrier-linux-amd64.tar.gz
install -Dm755 carrier-linux-amd64 ~/.local/bin/carrier
carrier version
```

macOS arm64 example:

```bash
version=v0.1.0
curl -LO "https://github.com/atbuy/carrier/releases/download/${version}/carrier-darwin-arm64.tar.gz"
tar -xzf carrier-darwin-arm64.tar.gz
install -m 755 carrier-darwin-arm64 /usr/local/bin/carrier
carrier version
```

GitHub also shows a `sha256:<hash>` digest beside each release asset, similar to Neovim releases. Use `checksums.txt` for command-line verification.

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
carrier shell # alpha
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
carrier export 42
carrier rerun 42
carrier doctor
carrier config path
carrier config show
carrier config check
carrier config init
carrier version
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
```

## Shell Mode Status

`carrier shell` is experimental. It uses a PTY plus injected zsh/bash hooks to detect command boundaries, so prompts, tmux, shell plugins, aliases, and interactive programs may affect tracking. Use `carrier run` when precise stdout/stderr capture matters.

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
