# Commands

This page documents everyday `carrier` commands and the flags users usually need.

Human-readable output is colorized on TTYs. Set `NO_COLOR=1` to disable colors or `CARRIER_COLOR=always` to force colors in non-TTY output.

!!! tip "Use IDs as handles"

    Most inspection commands take a run ID. Start with `carrier last`, `carrier history`, `carrier failed`, or `carrier search` to find one.

## Global flags

These flags can be used with commands that execute or inspect runs.

| Flag                    | Meaning                                                                       |
| ----------------------- | ----------------------------------------------------------------------------- |
| `-n`, `--notify`        | Request a desktop notification if duration is at least `notify.min_duration`. |
| `-N`, `--notify-always` | Request a desktop notification regardless of duration.                        |
| `-q`, `--quiet`         | Suppress carrier status messages.                                             |
| `--no-redact`           | Disable redaction for persisted logs or displayed captured environment.       |

!!! note

    `carrier run` passes arbitrary flags to child commands, so carrier flags must come before the child command:

    ```bash
    carrier run --timeout 30s go test ./...
    carrier run go test --timeout 30s
    ```

    In the second command, `--timeout` belongs to `go test`, not `carrier`.

## `run`

Run one command and record it:

```bash title="Record a command"
carrier run go test ./...
carrier run docker compose build
carrier run bash -c 'make clean && make'
```

Useful flags:

| Flag                           | Meaning                                                               |
| ------------------------------ | --------------------------------------------------------------------- |
| `-t`, `--timeout <duration>`   | Interrupt the child after the duration, then kill if it does not exit. |
| `-n`, `--notify`               | Notify only when command duration meets `notify.min_duration`.        |
| `-N`, `--notify-always`        | Always notify.                                                        |
| `-q`, `--quiet`                | Hide `carrier: run <id>` status output.                               |
| `--no-redact`                  | Persist logs without redaction for this run.                          |

Behavior:

- streams stdout and stderr live
- stores stdout and stderr separately
- records metadata, Git context, and environment when enabled
- preserves child exit code
- redacts persisted logs by default
- caps persisted logs by `storage.max_output_mb`

## `last`

Show latest run:

```bash title="Show latest run"
carrier last
carrier last --json
```

## `history`

List recorded runs newest-first:

```bash title="List recent runs"
carrier history
carrier history --limit 50
```

Filter history:

```bash title="Filter history"
carrier history --status failed
carrier history --since 24h
carrier history --cwd api
carrier history --branch main
carrier history --command 'go test'
carrier history --label deploy
carrier history --json
```

Use it with `fzf`:

```bash title="Pick a run and rerun it"
carrier history | fzf | awk '{print $1}' | xargs carrier rerun
```

## `show`

Show metadata and captured output:

```bash title="Show a run"
carrier show 42
carrier show 42 --json
```

Output controls:

```bash title="Show less output"
carrier show 42 --lines 100
carrier show 42 --stdout
carrier show 42 --stderr
carrier show 42 --env
```

`--stdout` and `--stderr` print only that stream, with no metadata header. `--env` prints captured environment variables when `storage.capture_env = true`.

## `tail`

Stream captured output:

```bash title="Tail logs"
carrier tail 42
carrier tail 42 --stream stdout
carrier tail 42 --stream stderr
carrier tail 42 --stream terminal
```

For `carrier run`, the default `--stream both` prefixes lines:

```text
stdout | ...
stderr | ...
```

For `carrier shell`, terminal output is a single stream. Use `--stream terminal` or the default `both`.

## `failed`

List failed runs:

```bash title="List failed runs"
carrier failed
```

## `running`

List currently running commands:

```bash title="List active runs"
carrier running
carrier running --json
```

Use `tail` from another terminal to watch a running command:

```bash
carrier tail 42
```

## `search`

Search commands, cwd, and output:

```bash title="Search"
carrier search "connection refused"
carrier search "gcov data not found"
carrier search --limit 25 "permission denied"
carrier search --json "timeout"
```

Search uses SQLite FTS over command text, working directory, and stored output snippets. A LIKE fallback catches command/cwd substring matches that token search may miss.

## `stats`

Show run totals, runs per active day, failure rate, average duration, and slowest commands:

```bash title="Stats"
carrier stats
carrier stats --slowest 10
carrier stats --json
```

## `export`

Export a run as Markdown by default:

```bash title="Export Markdown"
carrier export 42 > run-42.md
```

Other formats:

```bash title="Export JSON or CSV"
carrier export 42 --format json > run-42.json
carrier export --format csv > runs.csv
carrier export 42 --format csv > run-42.csv
```

## `rerun`

Run original argv from original cwd:

```bash title="Rerun"
carrier rerun 42
carrier -n rerun 42
```

Edit the argv JSON before rerunning:

```bash title="Edit before rerun"
carrier rerun 42 --edit
```

`rerun` creates a new run. It never overwrites the old record.

## `label`

Attach a short label to a run:

```bash title="Label runs"
carrier label 42 prod deploy
carrier history --label prod
```

Clear a label by omitting text:

```bash title="Clear a label"
carrier label 42
```

## `watch`

Re-run a command when files in the current directory change:

```bash title="Watch a project"
carrier watch go test ./...
carrier watch --pattern '*.go' go test ./...
carrier watch --debounce 500ms go test ./...
```

`watch` recursively watches the current directory and skips `.git`, `node_modules`, and `vendor`.

## `clean`

Preview deletion:

```bash title="Preview old records"
carrier clean --older-than 30d --dry-run
carrier clean --older-than 30d -d
```

Delete old records and logs:

```bash title="Delete old records"
carrier clean --older-than 30d --yes
carrier clean --older-than 30d -y
```

Keep only recent records:

```bash title="Retention by count"
carrier clean --keep-last 500 --dry-run
carrier clean --keep-last 500 --yes
```

Deletion requires `--yes`. Use `--dry-run` first.

## `doctor`

Check local setup:

```bash title="Doctor"
carrier doctor
```

Shows version, config path, storage paths, migration version, data size, redaction status, stale running runs, shell support, notification tool availability, and terminal status.

## `config`

Inspect and create config:

```bash title="Config commands"
carrier config path
carrier config show
carrier config check
carrier config init
carrier config init --force
```

## `shell`

Start alpha tracked shell mode:

```bash title="Shell mode"
carrier shell
```

Use `carrier run` when precise stdout/stderr capture matters. See [Shell mode](../advanced/shell-mode.md).

## `version`

```bash title="Version"
carrier version
```
