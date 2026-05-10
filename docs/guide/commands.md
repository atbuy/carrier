# Commands

This page documents everyday commands.

## `run`

Run one command and record it:

```bash
carrier run go test ./...
carrier run docker compose build
carrier run bash -c 'make clean && make'
```

Behavior:

- streams stdout live
- streams stderr live
- stores stdout and stderr separately
- records metadata
- preserves child exit code
- redacts persisted logs
- caps persisted logs by `storage.max_output_mb`

## Notifications

Notifications are opt-in.

Notify only if command duration is at least `notify.min_duration`:

```bash
carrier -n run docker compose build
```

Notify regardless of duration:

```bash
carrier -N run ls
```

## `last`

Show latest run:

```bash
carrier last
carrier last --json
```

## `show`

Show metadata and output:

```bash
carrier show 42
carrier show 42 --json
```

## `tail`

Stream captured output:

```bash
carrier tail 42
carrier tail 42 --stream stdout
carrier tail 42 --stream stderr
```

For `carrier run`, `--stream both` prefixes lines:

```text
stdout | ...
stderr | ...
```

For `carrier shell`, output is terminal-level output.

## `failed`

List failed runs:

```bash
carrier failed
```

## `running`

List currently running commands:

```bash
carrier running
carrier running --json
```

## `search`

Search commands, cwd, and output:

```bash
carrier search "connection refused"
carrier search "gcov data not found"
```

Search is file-scan based in current versions.

## `export`

Export a run as Markdown:

```bash
carrier export 42 > run-42.md
```

## `rerun`

Run original argv from original cwd:

```bash
carrier rerun 42
carrier -n rerun 42
```

This creates a new run.

## `clean`

Preview:

```bash
carrier clean --older-than 30d --dry-run
carrier clean --older-than 30d -d
```

Delete:

```bash
carrier clean --older-than 30d --yes
carrier clean --older-than 30d -y
```

Deletion requires `--yes`.

## `doctor`

Check local setup:

```bash
carrier doctor
```

Shows config path, storage paths, migration version, data size, redaction status, shell status, and optional tool availability.

## `config`

```bash
carrier config path
carrier config show
carrier config check
carrier config init
carrier config init --force
```

## `version`

```bash
carrier version
```
