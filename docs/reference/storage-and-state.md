# Storage and state

`carrier` stores metadata in SQLite and command output in log files.

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

Main table:

```text
runs
```

Migration table:

```text
goose_db_version
```

Search index:

```text
run_search
```

Migrations are embedded in the binary and applied on startup.

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

```bash
carrier --no-redact run ./script-that-prints-safe-output
```

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

## Cleanup

Preview:

```bash
carrier clean --older-than 30d --dry-run
```

Delete:

```bash
carrier clean --older-than 30d --yes
```
