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
| `notes`            | Notes attached to sessions and runs          |
| `session_branches` | Git branches linked to sessions              |
| `run_search`       | FTS5 full-text search index                  |
| `goose_db_version` | Applied migration versions                   |

Migrations are embedded in the binary and applied on startup. After the first successful migration, carrier writes a `schema.version` sidecar file to the data directory and skips the migration check on subsequent startups as long as the version matches.

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
- Git metadata (root, branch, commit, dirty flag)
- output log paths
- notification flags
- label
- reference to a deduplicated environment snapshot (when enabled)

Git metadata and environment capture are collected concurrently with the child process, so they do not delay command startup. When the command finishes, `carrier run` writes the git metadata, environment link, status, exit code, and duration back in a single SQLite UPDATE to keep per-command overhead low.

In shell mode, the Git root, branch, and commit are read from the files in `.git` before each command, without starting `git`, so they describe the repository as the command starts. The dirty flag (`git status`) and the environment are added by a background process, so commands never wait for them; `carrier show` may lack them for the first moment of a run.

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

Redacting runs every value through the secret-detection rules, which takes time. A recorded shell remembers the last environment it stored, by a hash of the variables, in a file next to its state file, and reuses it while the environment is unchanged; only a changed environment is redacted again.

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
- text of notes attached to the run

The index stores up to 256 KiB of combined output per run. The original log files remain on disk under `runs/`.

## Stale running runs

Runs can get stuck in `running` if the parent `carrier` process is killed or the machine shuts down. On startup, carrier marks old running runs as `killed` when they are older than:

```toml title="Stale run threshold"
[storage]
stale_run_threshold = "24h"
```

The cleanup runs at most once every 5 minutes regardless of how often you invoke carrier, so it does not add overhead to every command.

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

Automatic sessions expire on their own after `shell.auto_session_retention`; see [Keep or expire automatic sessions](../advanced/shell-mode.md#keep-or-expire-automatic-sessions).

## Temporary files

Each recorded shell keeps a few files in the temp directory (`$TMPDIR`, usually `/tmp`) while it runs: its state file `carrier-shell-<pid>.json`, the session file `.json.session` its prompt reads, the environment cache `.json.env`, and a `carrier-shell-<pid>-*` directory with its shell hooks. They are removed when the shell exits.

A shell that is killed instead, for example by `tmux kill-server`, leaves them behind. At most once a day, when a recorded shell starts, carrier removes such files when their carrier process has ended and they are at least an hour old. Only your own files are touched.
