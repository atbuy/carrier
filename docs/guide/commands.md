# Commands

This page documents every `carrier` command and the flags users usually need.

Human-readable output is colorized on TTYs. Set `NO_COLOR=1` to disable colors or `CARRIER_COLOR=always` to force colors in non-TTY output.

!!! tip "Use IDs as handles"

    Most inspection commands take a run ID. Start with `carrier last`, `carrier history`, `carrier failed`, or `carrier search` to find one.

## Global flags

These flags apply to any command that runs or inspects commands.

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

| Flag                         | Meaning                                                               |
| ---------------------------- | --------------------------------------------------------------------- |
| `-t`, `--timeout <duration>` | Interrupt child after the duration, then kill if it does not exit.    |
| `-n`, `--notify`             | Notify only when command duration meets `notify.min_duration`.        |
| `-N`, `--notify-always`      | Always notify.                                                        |
| `-q`, `--quiet`              | Hide `carrier: run <id>` status output.                               |
| `--no-redact`                | Persist logs without redaction for this run.                          |

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

List recorded runs oldest-first:

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

### Session grouping

When runs belong to shell sessions, `history` displays them as a tree with session headers:

```text
   5  ┬──  2026-05-18 12:01  session: backend-debug
   3  ├──  failed   1.2s  make test
   2  └──  success  0.4s  go vet ./...
   1       failed   2.1s  make lint
```

Filter to a specific session by ID or label:

```bash title="Filter by session"
carrier history --session 5
carrier history --session backend-debug
```

Show only session headers, no individual runs:

```bash title="Sessions only"
carrier history --sessions-only
```

Use with `fzf`:

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

When a run belongs to a shell session, `label` also updates the session label. This keeps the history tree consistent: runs and their session share the same label.

## `watch`

Re-run a command when files in the current directory change:

```bash title="Watch a project"
carrier watch go test ./...
carrier watch --pattern '*.go' go test ./...
carrier watch --debounce 500ms go test ./...
```

`watch` recursively watches the current directory and skips `.git`, `node_modules`, and `vendor`.

## `shell`

Start an alpha tracked shell session:

```bash title="Shell mode"
carrier shell
```

Attach a label at start with `--label` or a positional argument:

```bash title="Labeled session"
carrier shell --label backend-debug
carrier shell 'backend-debug'
```

The label appears in `carrier history` session headers and in `carrier session list`.

Use `carrier run` when precise stdout/stderr capture matters. See [Shell mode](../advanced/shell-mode.md).

## `attach`

Attach to an existing shell session by session ID or label:

```bash title="Attach to a session"
carrier attach 5
carrier attach backend-debug
```

`attach` re-opens the session, starts a new PTY shell that records commands into it, then closes the session when you exit. Runs recorded during the attached shell appear grouped under the same session in `carrier history`.

This is useful when you need to return to a named session after disconnecting, or when you want to continue work in a labeled debugging context from a different terminal.

## `session`

Manage shell session labels and history.

### `session list`

List sessions newest-first:

```bash title="List sessions"
carrier session list
carrier session list --limit 20
```

Output includes session ID, start time, label, and duration (or `active` for open sessions).

### `session label`

Set or clear a session label:

```bash title="Label a session"
carrier session label 3 backend-debug
carrier session label 3               # clear label
```

When inside a tracked shell (`carrier shell` or `carrier attach`), the session ID is available as `$CARRIER_SESSION_ID`. Omit the ID argument to target the current session:

```bash title="Label current session from inside the shell"
carrier session label backend-debug
carrier session label               # clear current session label
```

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

## `version`

```bash title="Version"
carrier version
```
