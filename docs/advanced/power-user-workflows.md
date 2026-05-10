# Power-user workflows

This page is for users who want to automate or inspect `carrier` more deeply.

## Use JSON output

Most inspection commands support JSON.

```bash title="JSON commands"
carrier last --json
carrier show 42 --json
carrier running --json
carrier history --json
carrier search --json "connection refused"
carrier stats --json
```

Examples:

```bash title="Extract fields"
carrier last --json | jq -r '.id'
carrier show 42 --json | jq -r '.stdout'
carrier running --json | jq '.[].command'
```

`show --json` includes log paths, captured output, metadata, and captured environment when enabled.

## Keep original argv reliable

`carrier` stores:

- display command in `command`
- original argv in `argv_json`

Display strings are shell-quoted for readability. `rerun` uses `argv_json`, not the display string.

## Use rerun safely

```bash title="Rerun"
carrier rerun 42
```

Rerun uses original cwd. If that directory was deleted or changed, behavior may differ.

Edit before rerun:

```bash title="Edit argv JSON"
carrier rerun 42 --edit
```

`--edit` opens a JSON array in `$EDITOR` or `$VISUAL`. The command is aborted if the file is unchanged, invalid JSON, or an empty array.

## Build interactive pickers

Use `history` with `fzf`:

```bash title="Pick and show"
carrier history | fzf | awk '{print $1}' | xargs carrier show
```

Use JSON when you need stable parsing:

```bash title="Pick failed run IDs"
carrier history --status failed --json | jq -r '.[].id'
```

## Label important runs

Labels are useful for deploys, incidents, benchmark baselines, and manual checkpoints.

```bash title="Label and filter"
carrier label 42 prod deploy
carrier history --label prod
```

Labels are stored in SQLite and appear in `history`, JSON views, and `show`.

## Inspect storage directly

Metadata DB:

```text
~/.local/share/carrier/carrier.db
```

Example:

```bash title="SQLite query"
sqlite3 ~/.local/share/carrier/carrier.db \
  'select id,status,command,cwd,exit_code from runs order by id desc limit 10;'
```

!!! warning

    Direct DB edits can break `carrier`. Read-only queries are safest.

## Use output paths

`show --json` includes log paths:

```bash title="Read path"
carrier show 42 --json | jq -r '.stdout_path'
```

Use paths to pass logs to other tools:

```bash title="Open stderr log"
less "$(carrier show 42 --json | jq -r '.stderr_path')"
```

Compare two command outputs:

```bash title="Diff stdout logs"
diff -u \
  "$(carrier show 41 --json | jq -r '.stdout_path')" \
  "$(carrier show 42 --json | jq -r '.stdout_path')"
```

## Tune output retention

Set log cap:

```toml title="Config"
[storage]
max_output_mb = 20
```

Set cleanup policy in a cron job:

```bash title="Cron-friendly cleanup"
carrier clean --older-than 30d --yes
```

Or keep a fixed history size:

```bash title="Keep last 500"
carrier clean --keep-last 500 --yes
```

## Disable redaction for one run

```bash title="No redaction for one run"
carrier --no-redact run env
```

Use this only when you are certain output does not contain secrets.

## Inspect captured environment

Environment capture is enabled by default.

```bash title="Environment output"
carrier show 42 --env
carrier show 42 --json | jq '.env'
```

Values are redacted on display. To disable capture entirely:

```toml title="Config"
[storage]
capture_env = false
```

## Use timeouts

`carrier run` can stop commands that hang:

```bash title="Timeout"
carrier run --timeout 10m make integration-test
```

Carrier sends an interrupt first. If the process does not exit, it is killed.

## Watch with filters

`watch` runs once immediately, then re-runs on matching file changes:

```bash title="Go test watcher"
carrier watch --pattern '*.go' --debounce 500ms go test ./...
```

It recursively watches the current directory and skips `.git`, `node_modules`, and `vendor`.
