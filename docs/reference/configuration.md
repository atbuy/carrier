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

## Full example

```toml
[storage]
data_dir = "~/.local/share/carrier"
max_output_mb = 20

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

### `max_output_mb`

Type: integer  
Default: `20`

Maximum persisted output size per log file, in MiB.

Terminal output is not capped. Only stored logs are truncated.

## `[redaction]`

### `enabled`

Type: boolean  
Default: `true`

Controls whether persisted logs are redacted.

### `patterns`

Type: list of regex strings

Patterns applied before writing logs to disk. Terminal output still shows original output.

Default patterns cover:

- bearer tokens
- password/token/api-key style assignments
- AWS access key IDs
- PEM private keys

## `[notify]`

### `min_duration`

Type: Go duration string  
Default: `"10s"`

Used with `-n` / `--notify`.

```bash
carrier -n run docker compose build
```

`-N` / `--notify-always` ignores this threshold.

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

### `ignore_commands`

Type: list of strings

Commands that shell mode should not track.

Default:

```toml
["nvim", "vim", "less", "man", "fzf", "yazi", "lazygit", "tmux"]
```
