# carrier

`carrier` records local command executions for developer workflows. It captures command metadata, timing, exit status, output logs, Git context, and optional desktop notifications.

## Quick Start

Install with Go:

```bash
go install github.com/atbuy/carrier/cmd/carrier@latest
```

Make sure Go's bin directory is on your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Or build from source:

```bash
git clone https://github.com/atbuy/carrier.git
cd carrier
make build
./bin/carrier --help
```

Most examples use this alias:

```bash
alias c='carrier'
```

Run and record a command:

```bash
carrier run go test ./...
carrier -n run docker compose build
carrier -N run ls
```

Inspect recorded runs:

```bash
carrier last
carrier last --json
carrier failed
carrier show 42
carrier show 42 --json
carrier tail 42
carrier search "connection refused"
carrier export 42
carrier rerun 42
carrier doctor
carrier config path
carrier config show
carrier config init
carrier version
carrier clean --older-than 30d --dry-run
carrier clean --older-than 30d -d
```

Use `--json` with `last`, `show`, and `running` when scripting against recorded runs.

## Shell Mode

`carrier shell` is alpha-quality. It is useful for trying tracked interactive sessions, but `carrier run` is the stable path for precise command capture. Shell tracking depends on PTY output and injected zsh/bash hooks, so stdout and stderr may be merged and some prompts or shell plugins can affect behavior.

## Storage

Metadata is stored in SQLite at:

```text
~/.local/share/carrier/carrier.db
```

Command output is stored under:

```text
~/.local/share/carrier/runs/
```

Persisted logs are capped by `storage.max_output_mb` from the config file. Terminal output still streams normally; only on-disk logs are truncated.

Logs are redacted before they are written to disk. Redaction uses a buffered writer so common split secrets and multiline private keys can still be matched.

## Development

Use the repository `Makefile`:

```bash
make fmt
make lint
make test
make build
```
