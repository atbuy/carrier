# Power-user workflows

This page is for users who want to automate or inspect `carrier` more deeply.

## Use JSON output

`last`, `show`, and `running` support JSON.

```bash
carrier last --json
carrier show 42 --json
carrier running --json
```

Examples:

```bash
carrier last --json | jq -r '.id'
carrier show 42 --json | jq -r '.stdout'
carrier running --json | jq '.[].command'
```

## Keep original argv reliable

`carrier` stores:

- display command in `command`
- original argv in `argv_json`

Display strings are shell-quoted for readability. `rerun` uses `argv_json`, not the display string.

## Use rerun safely

```bash
carrier rerun 42
```

Rerun uses original cwd. If that directory was deleted or changed, behavior may differ.

## Inspect storage directly

Metadata DB:

```text
~/.local/share/carrier/carrier.db
```

Example:

```bash
sqlite3 ~/.local/share/carrier/carrier.db \
  'select id,status,command,cwd,exit_code from runs order by id desc limit 10;'
```

!!! warning

    Direct DB edits can break `carrier`. Read-only queries are safest.

## Use output paths

`show --json` includes log paths:

```bash
carrier show 42 --json | jq -r '.stdout_path'
```

Use paths to pass logs to other tools:

```bash
less "$(carrier show 42 --json | jq -r '.stderr_path')"
```

## Tune output retention

Set log cap:

```toml
[storage]
max_output_mb = 20
```

Set cleanup policy in a cron job:

```bash
carrier clean --older-than 30d --yes
```

## Disable redaction for one run

```bash
carrier --no-redact run env
```

Use this only when you are certain output does not contain secrets.
