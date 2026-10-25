# Configuration reference

`carrier` reads config from:

```text
~/.config/carrier/config.toml
```

Change settings without editing the file:

```bash
carrier config
```

This lists every setting, grouped like the file, with its value. Move with the arrow keys and press Enter to change the selected one: on/off settings toggle, settings with a fixed set of values move to the next one (←/→ pick either way), and other settings open for typing. Lists, such as `ignore_commands`, open on their own screen to add, edit, and delete items. Press `r` to reset a setting to its default, `/` to search, and `e` to open the file in `$EDITOR`.

Each change is checked like `carrier config check` does and saved to the file right away; an invalid value is not saved. Only the setting's line changes, so comments and the rest of the file stay as they are. carrier reads the file on every command, so a change applies from the next one, also in recorded shells. When an [include](#includes) sets the value, the include is changed, since its value is the one that takes effect. `[[auto_label]]` rules and `includes` are edited in the file.

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

    Run `carrier config check` after editing the file by hand. It catches invalid regexes, invalid durations, blank paths, and risky output cap settings.

## Durations

Every duration setting, and every duration flag such as `--since`, `--older-than`, `--timeout`, and `--debounce`, uses the same format: a number and a unit, like `30s`, `10m`, `1.5h`, `7d`, or `2w`. Units can be combined, as in `1d12h` or `1h30m`.

| Unit | Meaning |
| ---- | ------- |
| `ms`, `us`, `ns` | milliseconds, microseconds, nanoseconds |
| `s`, `m`, `h` | seconds, minutes, hours |
| `d` | days (24 hours) |
| `w` | weeks (7 days) |

`0` needs no unit. Negative durations are rejected.

## Full example

```toml title="~/.config/carrier/config.toml"
includes = ["conf.d/*.toml"]

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
auto_session_retention = "30d"
ask_outcome = false
branch_sessions = "suggest"

[hints]
similar_failures = false
similar_min_age = "1h"

[ui]
color = "auto"

[ui.theme]
muted = "#A8A8A8"
command = "#6AAF6A"
success = "#6AAF6A"
danger = "#D75F5F"
warning = "#D7AF5F"
accent = "#5B8DEF"
label = "#7AADF4"

[[auto_label]]
label = "feature - ${branch}"
git_branch = 'feat/(?P<branch>.+)'

[[auto_label]]
label = "tests"
cmd = 'go test.*'
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

Type: [duration](#durations)  
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

Type: [duration](#durations)  
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

Commands that shell mode records without output: the command line, working directory, timing, and exit code are kept, the terminal output is not. Matched by program name.

Default:

```toml
["nvim", "vim", "less", "man", "fzf", "yazi", "lazygit", "tmux"]
```

carrier's own hook commands are never recorded.

### `auto_session_retention`

Type: [duration](#durations)  
Default: `"30d"`

How long automatic sessions (started by the line from `carrier init`) are kept after they end. Expired sessions are deleted with their runs and logs. Sessions with a label, notes, or `carrier session keep` never expire, nor do sessions started with `carrier shell`. `"0"` keeps everything.

```toml
[shell]
auto_session_retention = "30d"
```

### `ask_outcome`

Type: boolean  
Default: `false`

When a shell recording into a labeled session exits and the session has no outcome yet, ask what it found:

```text
carrier: what did "BUG-1234: fix login" find? (Enter to skip) pool size was 0 after the config change
```

The answer is saved as the session's outcome, as with `carrier session end --note`. Unlabeled sessions are never asked about, and Enter skips. The question is only asked in a terminal; closing the terminal window skips it. In bash, it is not asked when your config sets its own `EXIT` trap.

```toml
[shell]
ask_outcome = true
```

### `branch_sessions`

Type: string  
Default: `"suggest"`

What a recorded shell does when a command, such as `git checkout`, moves it to a Git branch [linked to another session](../guide/commands.md#session-branch):

| Value       | Effect                                                                 |
| ----------- | ---------------------------------------------------------------------- |
| `"suggest"` | Print one line naming the branch's session and how to switch to it.    |
| `"follow"`  | Switch the shell to the branch's session, as `carrier attach` does.    |
| `"off"`     | Do nothing.                                                            |

```text
% git checkout BUG-1234-fix
Switched to branch 'BUG-1234-fix'
carrier: branch BUG-1234-fix has session 12 · BUG-1234: fix login; switch with carrier attach 12
```

The checkout itself stays in the session it was typed in. Branches without a link, a detached HEAD, and changing to another repository never switch or print anything.

```toml
[shell]
branch_sessions = "follow"
```

## `[hints]`

Optional hints printed after a run finishes. All hints are off by default.

### `similar_failures`

Type: boolean  
Default: `false`

When a run fails, look for an earlier failed run with the same error and point at it:

```text
dial tcp 10.0.0.7:6543: connect: connection refused
carrier: seen before in run 42 (3 weeks ago, session "db-debug", 2 notes): carrier show 42
```

Carrier takes the error lines near the end of the failed run's output (lines mentioning `error`, `failed`, `refused`, `panic`, and similar, or the last line if none do), drops words containing digits so ports, PIDs, line numbers, and hashes do not matter, and searches the output of earlier failed runs for the remaining words. Runs with notes or a label are preferred, since they record what happened. When more than one run matches, the hint ends with `carrier similar <id>` to list them all.

The hint is printed after `carrier run` (not with `--quiet`) and inside `carrier shell` before the next prompt. Commands stopped with Ctrl-C do not get a hint. `carrier similar <id>` works whether or not this is enabled.

### `similar_min_age`

Type: [duration](#durations)  
Default: `"1h"`

Ignore matches newer than this, so retrying a failing command does not point at the attempt you just made. Use `"0s"` to consider every earlier run.

## `[ui]`

Controls terminal presentation.

### `color`

Type: string (`auto`, `always`, or `never`)  
Default: `"auto"`

When to emit ANSI color:

- `auto` — color only when writing to a terminal. Honors the [`NO_COLOR`](https://no-color.org) environment variable: if `NO_COLOR` is set, output is plain text. Piping carrier into another command (non-TTY) also disables color automatically.
- `always` — force color even when output is not a terminal. Overrides `NO_COLOR`.
- `never` — never emit color.

```bash title="Disable color for one command"
NO_COLOR=1 carrier history
```

### `[ui.theme]`

Type: hex color strings (e.g. `"#6AAF6A"`), ANSI color names (e.g. `"red"`), or ANSI indices (e.g. `"9"`)

Override the color used for each semantic style. Any field left empty falls back to the builtin default.

| Field     | Used for                                  |
| --------- | ----------------------------------------- |
| `muted`   | secondary info: timestamps, cwd, hints    |
| `command` | command text and examples                 |
| `success` | successful run status                     |
| `danger`  | failed status and error messages          |
| `warning` | killed status and caution notices         |
| `accent`  | session IDs, tree connectors              |
| `label`   | user labels, shell-session labels, flag and command names |

```toml title="Custom palette"
[ui.theme]
danger = "#FF5555"
accent = "#BD93F9"
```

## `includes`

Split your config across multiple files using the `includes` key. Carrier merges
all listed files into the main config before applying it.

```toml title="~/.config/carrier/config.toml"
includes = [
  "labels.toml",          # relative to this file's directory
  "conf.d/*.toml",        # glob — loads all .toml files in conf.d/, alphabetically
  "~/shared/carrier.toml", # ~ expands to home directory
  "/etc/carrier/base.toml", # absolute path
]
```

### Path resolution

| Pattern type | Example                | Resolved as                                    |
| ------------ | ---------------------- | ---------------------------------------------- |
| Relative     | `"labels.toml"`        | same directory as the main config file         |
| Tilde        | `"~/shared/base.toml"` | `$HOME/shared/base.toml`                       |
| Absolute     | `"/etc/carrier/base.toml"` | used as-is                                 |
| Glob         | `"conf.d/*.toml"`      | all matching files, sorted alphabetically      |

A literal (non-glob) path that does not exist is an error. A glob pattern that
matches no files is silently ignored — useful for optional drop-in directories.

### Merge rules

| Type                                                    | Behaviour                         |
| ------------------------------------------------------- | --------------------------------- |
| `[[auto_label]]`                                        | appended in file order            |
| `redaction.patterns`                                    | appended                          |
| `shell.ignore_commands`                                 | appended                          |
| All scalar fields (`storage.*`, `notify.*`, `ui.*`, …) | included file overrides main only when the key is explicitly present in the include file |

This means an include file that only defines `[[auto_label]]` entries will never
accidentally clear your `notify.min_duration` or any other scalar setting.

### Recursion

Included files are **not** processed for their own `includes` keys. The merge is
one level deep only.

### Example: split auto-labels by project

```toml title="~/.config/carrier/config.toml"
includes = ["labels/*.toml"]

[storage]
data_dir = "~/.local/share/carrier"
```

```toml title="~/.config/carrier/labels/work.toml"
[[auto_label]]
label = "work - ${branch}"
dir = '/home/me/work'
git_branch = '(?P<branch>.+)'
```

```toml title="~/.config/carrier/labels/tests.toml"
[[auto_label]]
label = "tests"
cmd = 'go test.*'

[[auto_label]]
label = "lint"
cmd = 'golangci-lint.*'
```

### `carrier config show` and `carrier config check`

`carrier config show` displays the **merged** effective config, so you always see
the full set of active auto-label rules regardless of which file they came from.

`carrier config check` validates the merged config. Invalid regexes or empty
labels in any included file are reported as errors.

## `[[auto_label]]`

Automatically attach a label to every run whose command, working directory, or
git branch matches a set of patterns. This is useful for tagging runs by project,
workflow, or branch without passing `--label` on every invocation.

Each `[[auto_label]]` entry is a rule. Rules are evaluated in order; the **first
matching rule wins**. A rule fires when **all** of its non-empty fields match
(AND semantics).

### Fields

| Field        | Type   | Description                                                     |
| ------------ | ------ | --------------------------------------------------------------- |
| `label`      | string | **Required.** Label to assign. May contain `${name}` placeholders. |
| `cmd`        | regex  | Matches against the full display command string.                |
| `dir`        | regex  | Matches against the working directory (implicitly anchored at start). |
| `git_branch` | regex  | Matches against the current git branch name.                    |

Patterns are [Go regular expressions](https://pkg.go.dev/regexp/syntax). Named
capture groups (`(?P<name>...)`) and positional captures (`${1}`, `${2}`) from
matched fields can be referenced in the `label` template.

The `dir` pattern is implicitly anchored at the start of the path, so writing
`/home/me/project` matches `/home/me/project` and every subdirectory — you do
not need to write `^/home/me/project`.

### Examples

```toml title="Label by command"
[[auto_label]]
label = "tests"
cmd = 'go test.*'

[[auto_label]]
label = "build"
cmd = 'go build.*'
```

```toml title="Label by directory"
[[auto_label]]
label = "frontend"
dir = '/home/me/work/frontend'

[[auto_label]]
label = "backend"
dir = '/home/me/work/backend'
```

```toml title="Extract branch name into the label"
[[auto_label]]
label = "feature - ${branch}"
git_branch = 'feat/(?P<branch>.+)'

[[auto_label]]
label = "hotfix - ${ticket}"
git_branch = 'hotfix/(?P<ticket>[A-Z]+-\d+).*'
```

```toml title="Extract subcommand from command"
[[auto_label]]
label = "git ${subcmd}"
cmd = 'git (?P<subcmd>\w+).*'
```

```toml title="Combine fields (all must match)"
[[auto_label]]
label = "deploy"
cmd = 'make deploy.*'
dir = '/home/me/myproject'
```

```toml title="Positional capture groups"
[[auto_label]]
label = "make: ${1}"
cmd = 'make (\w+)'
```

### Interaction with `--label`

An explicit `--label` flag always takes precedence. Auto-labels are only applied
when no label is provided at invocation time.

### Validation

`carrier config check` reports:

- error if `label` is empty
- error if any pattern field contains an invalid regular expression
- warning if a rule has no `cmd`, `dir`, or `git_branch` (it would always fire)

## Validation rules

`carrier config check` reports:

| Field                     | Error or warning                                      |
| ------------------------- | ----------------------------------------------------- |
| `storage.data_dir`        | error if empty                                        |
| `storage.max_output_mb`   | error if negative, warning if zero                    |
| `redaction.patterns`      | warning if redaction is enabled with no patterns      |
| `redaction.patterns[]`    | error for invalid regex, warning for blank pattern    |
| `storage.stale_run_threshold` | error if invalid or negative                      |
| `notify.min_duration`     | error if empty, invalid, or negative                  |
| `shell.program`           | error if blank whitespace                             |
| `shell.ignore_commands[]` | warning for blank command names                       |
| `shell.auto_session_retention` | error if invalid or negative                     |
| `shell.branch_sessions`   | error if not `suggest`, `follow`, or `off`            |
| `hints.similar_min_age`   | error if invalid or negative                          |
| `ui.color`                | error if not `auto`, `always`, or `never`             |
| `ui.theme.*`              | error for invalid `#hex` color values                 |
| `auto_label[N].label`     | error if empty                                        |
| `auto_label[N]`           | warning if no match conditions (always fires)         |
| `auto_label[N].cmd`       | error for invalid regex                               |
| `auto_label[N].dir`       | error for invalid regex                               |
| `auto_label[N].git_branch`| error for invalid regex                               |
