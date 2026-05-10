# FAQ

## Does carrier replace shell history?

No. Shell history records commands. `carrier` records command executions with metadata, output logs, duration, exit code, and Git context.

Shell history is still the fastest way to recall short commands. `carrier` is for runs you want to inspect, search, export, or rerun later.

## Does carrier send data anywhere?

No. Data is local by default: SQLite metadata plus log files under `storage.data_dir`.

Desktop notifications, when requested, go through your local OS notification tool: `notify-send` on Linux, `osascript` on macOS, and PowerShell on Windows.

## Are secrets safe?

Persisted logs are redacted by default. Redaction is regex-based and buffered, but no redactor can guarantee perfect secret removal.

Best practice:

- keep redaction enabled
- add project-specific patterns
- avoid printing secrets
- use `--no-redact` only for safe output

Environment capture is enabled by default. Values are redacted when displayed, but they are still stored in SQLite. Disable it with `storage.capture_env = false` if you do not want environment metadata persisted.

## Does terminal output get redacted?

No. Your terminal shows original output. Only persisted logs are redacted.

## Why are logs truncated?

`storage.max_output_mb` limits persisted log size to protect disk usage.

Terminal output still streams fully. Only persisted logs are capped.

## Does `carrier run` preserve exit codes?

Yes.

```bash
carrier run bash -c 'exit 7'
echo $?
```

prints:

```text
7
```

## Why is `carrier shell` alpha?

Shell mode needs PTY recording and shell hooks. Prompt plugins, aliases, tmux, shell differences, and interactive apps can affect tracking.

Use `carrier run` for important records.

## When should I use `watch`?

Use `carrier watch` for local feedback loops, such as rerunning tests on file changes.

```bash
carrier watch --pattern '*.go' go test ./...
```

Use your editor or build system's watcher if you need more complex include/exclude behavior.

## What is the difference between `run` and `rerun`?

`run` records a new command from arguments you type now.

`rerun` loads the original argv and cwd from a previous run, then creates a new run record.

## Why store argv separately from command text?

The command text is for humans and is shell-quoted for readability. `argv_json` is for reliable execution. `rerun` uses `argv_json`.

## Can I add notes to runs?

Use labels:

```bash
carrier label 42 prod deploy
carrier history --label prod
```

## Can I inspect data with SQLite?

Yes, but prefer read-only queries.

```bash
sqlite3 ~/.local/share/carrier/carrier.db 'select id,status,command from runs order by id desc limit 10;'
```

## How do I delete old data?

Preview:

```bash
carrier clean --older-than 30d --dry-run
```

Delete:

```bash
carrier clean --older-than 30d --yes
```

Keep a fixed number of recent runs:

```bash
carrier clean --keep-last 500 --yes
```

## How do I create a config file?

```bash
carrier config init
```

Overwrite existing config:

```bash
carrier config init --force
```

## How do I check my install?

```bash
carrier version
carrier doctor
```

## Where do I find every command and flag?

See [Commands](guide/commands.md).
