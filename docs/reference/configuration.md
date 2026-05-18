# Configuration reference

`carrier` reads config from:

```text
~/.config/carrier/config.toml
```

Show path:

```bash
carrier config path
```

Write default config:

```bash
carrier config init
```

Show active config:

```bash
carrier config show
```

Validate active config:

```bash
carrier config check
```

If the config file is missing, code defaults are used.

!!! tip

    Run `carrier config check` after editing config. It catches invalid regexes, invalid durations, blank paths, and risky output cap settings.

## Full example

```toml title="~/.config/carrier/config.toml"
[storage]
data_dir = "~/.local/share/carrier"
max_output_mb = 20
stale_run_threshold = "24h"
capture_env = true

[redaction]
enabled = true
patterns = [
  'Bearer [A-Za-z0-9._-]+',
  '(?i)(password|passwd|token|api[_-]?key|secret|access[_-]?token|refresh[_-]?token)\s*[:=]\s*\S+',
  'AKIA[0-9A-Z]{16}',
  '-----BEGIN PRIVATE KEY-----[\s\S]*?-----END PRIVATE KEY-----',
]

[notify]
min_duration = "10s"
success = true
failure = true

[shell]
program = ""
ignore_commands = ["nvim", "vim", "less", "man", "fzf", "yazi", "lazygit", "tmux"]
```

## `[storage]`

### `data_dir`

Type: string  
Default: `"~/.local/share/carrier"`

Directory for SQLite metadata and log files.

`~` expands to your home directory. Set `XDG_CONFIG_HOME` to move the config file itself.

### `max_output_mb`

Type: integer  
Default: `20`

Maximum persisted output size per log file, in MiB.

Terminal output is not capped. Only stored logs are truncated.

Set `0` to disable the cap. `carrier config check` warns when the cap is disabled.

### `stale_run_threshold`

Type: Go duration string
Default: `"24h"`

When `carrier` starts, runs stuck in `running` longer than this threshold are marked `killed`.

This protects the history after crashes, machine shutdowns, or killed parent processes.

### `capture_env`

Type: boolean
Default: `true`

Capture the child process environment in run metadata. Show it with:

```bash title="Show captured environment"
carrier show 42 --env
carrier show 42 --json
```

Values are **always redacted before being written to disk** using the builtin secret patterns plus any custom `redaction.patterns`. The `redaction.enabled` flag controls stdout/stderr redaction only; it does not affect environment storage.

Identical environments are stored once and shared across runs (SHA-256 deduplication).

## `[redaction]`

### `enabled`

Type: boolean  
Default: `true`

Controls whether persisted stdout/stderr logs are redacted before being written to disk. Terminal output is never redacted.

Environment snapshots (`capture_env`) are always redacted regardless of this setting.

### `patterns`

Type: list of regex strings

Patterns applied before writing logs to disk. Terminal output still shows original output.

Default patterns cover:

- bearer tokens
- password/token/api-key style assignments
- AWS access key IDs
- PEM private keys

Add project-specific patterns when your tools print secrets in custom formats:

```toml title="Custom redaction pattern"
[redaction]
patterns = [
  'Bearer [A-Za-z0-9._-]+',
  '(?i)(password|passwd|token|api[_-]?key|secret|access[_-]?token|refresh[_-]?token)\s*[:=]\s*\S+',
  'AKIA[0-9A-Z]{16}',
  '-----BEGIN PRIVATE KEY-----[\s\S]*?-----END PRIVATE KEY-----',
  'MY_SERVICE_TOKEN=\S+',
]
```

Setting `patterns` replaces the defaults, so keep any default patterns you still want.

!!! note

    Invalid regexes are ignored by the runtime redactor and reported by `carrier config check`.

## `[notify]`

Notifications are opt-in per command. Carrier uses `notify-send` on Linux, `osascript` on macOS, and PowerShell on Windows.

### `min_duration`

Type: Go duration string  
Default: `"10s"`

Used with `-n` / `--notify`.

```bash
carrier -n run docker compose build
```

`-N` / `--notify-always` ignores this threshold.

Valid duration examples:

```text
500ms
10s
5m
1h
```

### `success`

Type: boolean  
Default: `true`

Send notifications for successful commands when notification is requested.

### `failure`

Type: boolean  
Default: `true`

Send notifications for failed commands when notification is requested.

## `[shell]`

### `program`

Type: string  
Default: `""`

Shell to launch for `carrier shell`. Empty means use `$SHELL`.

If both config and `$SHELL` are empty, carrier falls back to `/bin/zsh`.

### `ignore_commands`

Type: list of strings

Commands that shell mode should not track.

Default:

```toml
["nvim", "vim", "less", "man", "fzf", "yazi", "lazygit", "tmux"]
```

Shell mode also ignores carrier's own internal hook commands.

## Validation rules

`carrier config check` reports:

| Field                     | Error or warning                                      |
| ------------------------- | ----------------------------------------------------- |
| `storage.data_dir`        | error if empty                                        |
| `storage.max_output_mb`   | error if negative, warning if zero                    |
| `redaction.patterns`      | warning if redaction is enabled with no patterns      |
| `redaction.patterns[]`    | error for invalid regex, warning for blank pattern    |
| `notify.min_duration`     | error if empty, invalid, or negative                  |
| `shell.program`           | error if blank whitespace                             |
| `shell.ignore_commands[]` | warning for blank command names                       |
