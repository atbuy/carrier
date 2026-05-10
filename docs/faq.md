# FAQ

## Does carrier replace shell history?

No. Shell history records commands. `carrier` records command executions with metadata, output logs, duration, exit code, and Git context.

## Does carrier send data anywhere?

No. Data is local by default: SQLite metadata plus log files under `storage.data_dir`.

## Are secrets safe?

Persisted logs are redacted by default. Redaction is regex-based and buffered, but no redactor can guarantee perfect secret removal.

Best practice:

- keep redaction enabled
- add project-specific patterns
- avoid printing secrets
- use `--no-redact` only for safe output

## Does terminal output get redacted?

No. Your terminal shows original output. Only persisted logs are redacted.

## Why are logs truncated?

`storage.max_output_mb` limits persisted log size to protect disk usage.

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
