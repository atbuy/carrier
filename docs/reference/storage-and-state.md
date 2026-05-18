# Storage and state

`carrier` stores metadata in SQLite and command output in log files.

!!! note

    All runtime data is local. `carrier` does not upload runs, logs, environment variables, or Git metadata.

## Default layout

```text
~/.local/share/carrier/
  carrier.db
  runs/
    000001.stdout.log
    000001.stderr.log
    000002.terminal.log
```

Change base directory:

```toml
[storage]
data_dir = "~/.local/share/carrier"
```

## SQLite database

Metadata lives in:

```text
carrier.db
```

Tables:

| Table              | Purpose                                      |
| ------------------ | -------------------------------------------- |
| `runs`             | One row per recorded command                 |
| `environments`     | Deduplicated env snapshots (SHA-256 indexed) |
| `shell_sessions`   | Shell session groupings                      |
| `run_search`       | FTS5 full-text search index                  |
| `goose_db_version` | Applied migration versions                   |

Migrations are embedded in the binary and applied on startup.

Check migration state:

```bash title="Check migration"
carrier doctor
```

## Run metadata

Each run stores:

- ID
- status
- mode
- command display string
- original argv JSON
- cwd
- start and finish timestamps
- duration
- exit code
- hostname
- shell
- Git metadata
- output log paths
- notification flags
- label
- reference to a deduplicated environment snapshot (when enabled)

## Output logs

`carrier run` writes:

```text
000001.stdout.log
000001.stderr.log
```

`carrier shell` writes:

```text
000002.terminal.log
```

## Redaction

Terminal output remains original. Persisted logs are redacted before writing.

Disable for one run:

```bash title="Disable redaction for one safe run"
carrier --no-redact run ./script-that-prints-safe-output
```

!!! warning

    `--no-redact` affects persisted logs for that run. Use it only when you know the command cannot print secrets.

## Output cap

Persisted logs are capped by:

```toml
[storage]
max_output_mb = 20
```

Truncated logs include:

```text
[carrier: output truncated at configured max_output_mb]
```

Set `max_output_mb = 0` to disable the cap. `carrier config check` warns about this because unbounded logs can grow quickly.

## Captured environment

When `storage.capture_env = true`, `carrier run` and `carrier shell` store the process environment in SQLite.

Environment values are **always redacted before being written to disk** using the builtin secret patterns plus any custom patterns from your config. The `redaction.enabled` flag controls stdout/stderr redaction; it does not affect environment storage.

Identical environments are stored only once. Runs share a row in the `environments` table, keyed by SHA-256 hash of the redacted JSON. This keeps the database small even when the same environment is used thousands of times.

Inspect a captured environment:

```bash title="Inspect environment"
carrier show 42 --env
carrier show 42 --json | jq '.env'
```

Orphaned environment rows (no longer referenced by any run) are removed automatically when you run `carrier clean`.

### Existing data

When upgrading from a version that stored env snapshots inline in the `runs` table, `carrier` migrates them to the `environments` table on first startup, redacting values in the process.

## Search index

`carrier search` indexes:

- command display string
- working directory
- stdout and stderr logs from `carrier run`
- terminal logs from `carrier shell`

The index stores up to 256 KiB of combined output per run. The original log files remain on disk under `runs/`.

## Stale running runs

Runs can get stuck in `running` if the parent `carrier` process is killed or the machine shuts down. On startup, carrier marks old running runs as `killed` when they are older than:

```toml title="Stale run threshold"
[storage]
stale_run_threshold = "24h"
```

`carrier doctor` reports how many stale runs remain.

## Cleanup

Preview:

```bash title="Preview cleanup"
carrier clean --older-than 30d --dry-run
```

Delete:

```bash title="Delete old data"
carrier clean --older-than 30d --yes
```

Keep only the latest N runs:

```bash title="Keep latest records"
carrier clean --keep-last 500 --dry-run
carrier clean --keep-last 500 --yes
```
