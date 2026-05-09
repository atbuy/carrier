# carrier

`carrier` records local command executions for developer workflows. It captures command metadata, timing, exit status, output logs, Git context, and optional desktop notifications.

## Quick Start

Build from source:

```bash
make build
./bin/carrier --help
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
carrier failed
carrier show 42
carrier tail 42
carrier search "connection refused"
carrier export 42
carrier rerun 42
carrier doctor
```

## Storage

Metadata is stored in SQLite at:

```text
~/.local/share/carrier/carrier.db
```

Command output is stored under:

```text
~/.local/share/carrier/runs/
```

## Development

Use the repository `Makefile`:

```bash
make fmt
make lint
make test
make build
```
