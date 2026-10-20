# Commands

This page documents every `carrier` command and the flags users usually need.

Human-readable output is colorized on TTYs. Set `NO_COLOR=1` to disable colors or `CARRIER_COLOR=always` to force colors in non-TTY output.

!!! tip "Use IDs as handles"

    Most inspection commands take a run ID. Start with `carrier last`, `carrier history`, `carrier failed`, or `carrier search` to find one, or use one of the shortcuts below.

## Referring to runs

`show`, `tail`, `export`, `rerun`, `diff`, `label`, `similar`, `note -r`, and `notes -r` accept any of these where they take a run:

| Reference      | Run                                                         |
| -------------- | ----------------------------------------------------------- |
| `42`           | run 42; if there is no run 42, a run whose label matches `42` |
| `last` or `-1` | the latest run                                              |
| `-2`, `-3`, …  | the run before the latest, and so on                        |
| `deploy`       | the newest run whose label matches `deploy`                 |

```bash title="Run shortcuts"
carrier show -1
carrier diff -2 -1
carrier rerun deploy
carrier note -r -2 "this one hung"
```

## Referring to sessions

`attach`, `history --session`, and `note --session` take a session ID or label. A number that is not a session ID is matched against labels too, so a session labeled `BUG-1234: fix login` can be reached with just `1234`:

```bash title="Attach by label"
carrier shell "BUG-1234: fix login"
carrier attach BUG-1234
carrier attach 1234      # no session 1234: matches the label, and says so
```

### How labels are matched

Matching ignores case and tries, in order:

1. the whole label: `deploy`
2. whole words of the label, in order, ignoring punctuation: `1234`, `bug-1234`, and `BUG 1234` all match `BUG-1234: fix login`, but `123` does not
3. part of the label, when only one label contains it: `fix log`

When several runs or sessions share the matching label, the newest is used. When the reference matches different labels, for example `1234` with both `BUG-1234` and `OPS-1234`, carrier lists them and asks for the ID or more of the label, so you never land in the wrong session.

## Empty results

When a list has nothing to show, `carrier` prints a short hint to stderr and exits `0`. Before anything is recorded, the hint suggests a first command:

```text
carrier: no runs recorded yet; try "carrier run echo hello" or "carrier shell"
```

Once runs exist, the hint says what was empty instead, for example `no failed runs` or `no runs match "timeout"`. Hints go to stderr, so piped output such as `carrier history | fzf` stays clean, and `--json` output is unchanged (`[]`).

Looking up a run or session that does not exist is an error, with a plain message such as `run 42 not found`.

## Global flags

These flags apply to any command that runs or inspects commands.

| Flag                    | Meaning                                                                       |
| ----------------------- | ----------------------------------------------------------------------------- |
| `-n`, `--notify`        | Request a desktop notification if duration is at least `notify.min_duration`. |
| `-N`, `--notify-always` | Request a desktop notification regardless of duration.                        |
| `-q`, `--quiet`         | Suppress carrier status messages.                                             |
| `--no-redact`           | Disable redaction for persisted logs or displayed captured environment.       |

!!! note

    `carrier run` passes arbitrary flags to child commands, so carrier flags must come before the child command:

    ```bash
    carrier run --timeout 30s go test ./...
    carrier run go test --timeout 30s
    ```

    In the second command, `--timeout` belongs to `go test`, not `carrier`.

## `run`

Run one command and record it:

```bash title="Record a command"
carrier run go test ./...
carrier run docker compose build
carrier run bash -c 'make clean && make'
```

Useful flags:

| Flag                         | Meaning                                                               |
| ---------------------------- | --------------------------------------------------------------------- |
| `-t`, `--timeout <duration>` | Interrupt child after the duration, then kill if it does not exit.    |
| `-n`, `--notify`             | Notify only when command duration meets `notify.min_duration`.        |
| `-N`, `--notify-always`      | Always notify.                                                        |
| `-q`, `--quiet`              | Hide `carrier: run <id>` status output.                               |
| `--no-redact`                | Persist logs without redaction for this run.                          |
| `-L`, `--label <text>`       | Attach a label to this run.                                           |

Behavior:

- streams stdout and stderr live
- stores stdout and stderr separately
- records metadata, Git context, and environment when enabled
- preserves child exit code
- redacts persisted logs by default
- caps persisted logs by `storage.max_output_mb`

## `last`

Show latest run:

```bash title="Show latest run"
carrier last        # show latest run
carrier last 5      # show latest 5 runs
carrier last 5 --json   # JSON array of latest 5
```

!!! note

    With N > 1, JSON output is an array.

## `tui`

Browse recorded runs in an interactive, full-screen viewer:

```bash title="Open the run browser"
carrier tui
carrier ui   # alias
```

The screen is split into a run list on the left and a live output preview on the
right. Move through the list to preview each run's details and captured output.
You can rerun, label, or delete runs without leaving the browser.

Rerun launches **after** the browser exits, so the command runs in your normal
terminal (not nested inside the viewer).

The preview shows `stdout`/`stderr` for `carrier run` commands. Shell-session
recordings rely on terminal control sequences and can't render faithfully in the
preview, so the browser points you to `carrier show <id>` or `carrier tail <id>`
to replay them in a real terminal instead.

### Keys

| Key                     | Action                                           |
| ----------------------- | ------------------------------------------------ |
| `↑` / `↓`, `k` / `j`    | move selection                                   |
| `g` / `G`               | jump to first / last run                         |
| `ctrl+u` / `ctrl+d`     | scroll the preview pane                          |
| `/`                     | filter by command, status, cwd, or label         |
| `r` / `enter`           | rerun the selected command (after the TUI exits) |
| `l`                     | set or clear the label on the selected run       |
| `d`                     | delete the selected run (asks `y/N` to confirm)  |
| `q` / `esc` / `ctrl+c`  | quit                                             |

While filtering, `enter` applies the filter and `esc` clears it. Deleting a run
also removes its log files and prunes any environment snapshot it no longer
shares.

## `history`

List recorded runs oldest-first:

```bash title="List recent runs"
carrier history
carrier history --limit 50
```

Filter history:

```bash title="Filter history"
carrier history --status failed
carrier history --since 24h
carrier history --cwd api
carrier history --branch main
carrier history --command 'go test'
carrier history --label deploy
carrier history --json
```

### Session grouping

List rows show a colored status glyph (`✓` success, `✗` failed, `●` running, `⊘` killed), a relative timestamp (`2m ago`), the duration, the command, and the working directory with your home directory collapsed to `~`. Absolute timestamps are shown in `carrier show`.

When runs belong to shell sessions, `history` displays them as a tree with session headers:

```text
   5  ┬──  active    12m ago   backend-debug
   3  ├──  ✗ failed   12m ago   1.2s   make test     ~/api
   2  └──  ✓ success  12m ago   0.4s   go vet ./...   ~/api
   1       ✗ failed   15m ago   2.1s   make lint      ~/api
```

Notes appear under the run they follow, and notes made before a session's first run appear right under the session header:

```text
   5  ┬──  active    12m ago   backend-debug
   3  ├──  ✗ failed   12m ago   1.2s   make test     ~/api
      │  note: fails only with the new cache layer
   2  └──  ✓ success  12m ago   0.4s   go vet ./...   ~/api
```

Filter to a specific session by ID or label:

```bash title="Filter by session"
carrier history --session 5
carrier history --session backend-debug
```

Show only session headers, no individual runs:

```bash title="Sessions only"
carrier history --sessions-only
```

Use with `fzf`:

```bash title="Pick a run and rerun it"
carrier history | fzf | awk '{print $1}' | xargs carrier rerun
```

## `show`

Show metadata and captured output:

```bash title="Show a run"
carrier show 42
carrier show 42 --json
```

Output controls:

```bash title="Show less output"
carrier show 42 --lines 100
carrier show 42 --stdout
carrier show 42 --stderr
carrier show 42 --env
```

`--stdout` and `--stderr` print only that stream, with no metadata header. `--env` prints captured environment variables when `storage.capture_env = true`.

Notes attached to the run are listed after the metadata. `--json` includes them in a `notes` array.

For runs captured in shell mode where a TUI application used the alternate screen (neovim, lazygit, noteui, etc.), the output section shows `(TUI session — terminal output suppressed)` instead of replaying the log. Run metadata is still fully recorded.

## `tail`

Stream captured output:

```bash title="Tail logs"
carrier tail 42
carrier tail 42 --stream stdout
carrier tail 42 --stream stderr
carrier tail 42 --stream terminal
```

For `carrier run`, the default `--stream both` prefixes lines:

```text
stdout | ...
stderr | ...
```

For `carrier shell`, terminal output is a single stream. Use `--stream terminal` or the default `both`.

## `failed`

List failed runs:

```bash title="List failed runs"
carrier failed
```

## `running`

List currently running commands:

```bash title="List active runs"
carrier running
carrier running --json
```

Use `tail` from another terminal to watch a running command:

```bash
carrier tail 42
```

## `search`

Search commands, cwd, and output:

```bash title="Search"
carrier search "connection refused"
carrier search --since 24h "timeout"
carrier search --since 7d --status failed "OOM"
carrier search --status running "migration"
carrier search --limit 25 "permission denied"
carrier search --json "timeout"
```

Search uses SQLite FTS over command text, working directory, and stored output snippets. A LIKE fallback catches command/cwd substring matches that token search may miss.

!!! note

    `--since` accepts durations like `30m`, `24h`, `7d` (days are supported). `--status` accepts `success`, `failed`, `killed`, `running`.

## `similar`

List earlier failed runs whose output contains the same error as a run:

```bash title="Find the same error before"
carrier similar 57
carrier similar 57 --limit 5
carrier similar 57 --json
```

```text
42  failed  docker compose up api  ~/api
    3 weeks ago · session "db-debug" · 2 notes
```

Runs with notes or a label are listed first. Matching ignores numbers, so the same error on a different port or line still matches; see [`hints.similar_failures`](../reference/configuration.md#similar_failures) for how errors are compared and to get a hint automatically when a run fails.

## `stats`

Show run totals, runs per active day, failure rate, average duration, and slowest commands:

```bash title="Stats"
carrier stats
carrier stats --slowest 10
carrier stats -c 'go test'          # filter to commands matching substring
carrier stats --command 'make'
carrier stats --json
```

!!! note

    When filtering with `-c`/`--command`, output includes min/max duration in addition to avg.

## `diff`

Diff the captured output of two runs:

```bash title="Diff two runs"
carrier diff 41 42
carrier diff --stream stderr 41 42
carrier diff --raw 41 42
```

By default, ANSI escape sequences are stripped before diffing so the output shows readable text. Use `--raw` to diff the log files directly.

`--stream` selects which output stream to compare:

| Value      | Meaning                                              |
| ---------- | ---------------------------------------------------- |
| `auto`     | terminal output for shell runs, stdout otherwise     |
| `stdout`   | captured stdout                                      |
| `stderr`   | captured stderr                                      |
| `terminal` | PTY recording (shell runs only)                      |

Colors follow `NO_COLOR` / `CARRIER_COLOR` / `--color` config.

## `export`

Export a run as Markdown by default:

```bash title="Export Markdown"
carrier export 42 > run-42.md
```

Other formats:

```bash title="Export JSON or CSV"
carrier export 42 --format json > run-42.json
carrier export --format csv > runs.csv
carrier export 42 --format csv > run-42.csv
```

## `rerun`

Run original argv from original cwd:

```bash title="Rerun"
carrier rerun 42
carrier -n rerun 42
```

Edit the argv JSON before rerunning:

```bash title="Edit before rerun"
carrier rerun 42 --edit
```

`rerun` creates a new run. It never overwrites the old record.

Useful flags:

| Flag                   | Meaning                    |
| ---------------------- | -------------------------- |
| `-L`, `--label <text>` | Attach a label to this run |

## `label`

Attach a short label to a run:

```bash title="Label runs"
carrier label 42 prod deploy
carrier history --label prod
```

Clear a label by omitting text:

```bash title="Clear a label"
carrier label 42
```

Labeling a run that does not exist fails with `run <id> not found`.

When a run belongs to a shell session, `label` also updates the session label. This keeps the history tree consistent: runs and their session share the same label.

## `note`

Write down what you were thinking, so you can find out later why you ran something:

```bash title="Add notes"
carrier note suspect stale DNS cache, flushing next
carrier note -r 42 "flaky: passes on rerun"
carrier note -s backend-debug "root cause: expired proxy cert"
```

Where the note goes:

- inside `carrier shell`: the current session, after the last command
- `--run <id>`: that run (and its session, if any)
- `--session <id-or-label>`: that session, after its last command
- otherwise: the latest run

Inside `carrier shell` the `carrier note` command itself is not recorded as a run. Command lines that combine a note with other commands, such as `make test && carrier note done`, are recorded as usual.

Quote the text when it contains shell characters like `;`, `|`, or `$`.

| Flag                             | Meaning                         |
| -------------------------------- | ------------------------------- |
| `-r`, `--run <id>`               | Attach to a run                 |
| `-s`, `--session <id-or-label>`  | Attach to a session             |

Notes show up in `history`, `show`, and `notes`, and their text is searchable with `carrier search`.

## `notes`

List notes oldest-first:

```bash title="List notes"
carrier notes
carrier notes --session backend-debug
carrier notes --run 42
carrier notes --json
```

Inside `carrier shell`, `notes` lists only the current session's notes unless `--session` or `--run` is given.

Delete a note by ID:

```bash title="Delete a note"
carrier notes rm 7
```

## `watch`

Re-run a command when files in the current directory change:

```bash title="Watch a project"
carrier watch go test ./...
carrier watch --pattern '*.go' go test ./...
carrier watch --debounce 500ms go test ./...
```

Useful flags:

| Flag                        | Meaning                                                |
| --------------------------- | ------------------------------------------------------ |
| `-p`, `--pattern <glob>`    | Only re-run when filename matches glob                 |
| `-d`, `--debounce <dur>`    | Debounce delay before re-running (default 200ms)       |
| `-L`, `--label <text>`      | Label to attach to each recorded run                   |
| `-q`, `--quiet`             | Suppress `carrier: changed: <file>` messages           |

`watch` recursively watches the current directory and skips `.git`, `node_modules`, and `vendor`. Before each re-run, it prints `carrier: changed: <file>` (suppressed by `--quiet`). Ctrl-C correctly stops both the watcher and any running child process. `--help` is supported.

## `shell`

Start an alpha tracked shell session:

```bash title="Shell mode"
carrier shell
```

Attach a label at start with `--label` or a positional argument:

```bash title="Labeled session"
carrier shell --label backend-debug
carrier shell 'backend-debug'
```

The label appears in `carrier history` session headers and in `carrier session list`.

Use `carrier run` when precise stdout/stderr capture matters. See [Shell mode](../advanced/shell-mode.md).

| Flag                | Meaning                                                              |
| ------------------- | -------------------------------------------------------------------- |
| `--label <text>`    | Label for the session                                                |
| `--program <shell>` | Shell to run instead of `shell.program` or `$SHELL`                  |
| `--auto`            | Start an automatic session; used by the line from [`init`](#init)    |

## `setup`

Set carrier up one step at a time: recording every terminal, similar-failure hints, desktop notifications, and the `c` alias.

```bash title="Setup"
carrier setup
carrier setup --dry-run      # show each change, write nothing
carrier setup --shell zsh    # set up a shell other than $SHELL
```

Each step checks what is already configured and says `already on` when there is nothing to do. When there is a change, it shows the exact diff for each file and asks before writing:

```text
~/.config/carrier/conf.d/hints.toml
  [hints]
- similar_failures = false # later
+ similar_failures = true # later
Apply this change? [Y/n]
```

How setup changes files:

- carrier config files are edited in place: one value is changed or added, and comments, ordering, and other settings are kept
- when a setting is set in an included file, that file is edited, since includes override the main config
- shell configs are only appended to, never rewritten
- every changed file is backed up first: shell configs next to the file (`~/.zshrc.bak-<time>`), carrier config files in `~/.config/carrier/backups/`, so backups are never loaded as includes
- a change that would make the config invalid is undone
- if the config does not load, setup stops without changing anything
- symlinked files, such as dotfiles from a repository, are edited at their target and stay symlinks

## `init`

Print the shell config snippet that records every terminal in an automatic session:

```bash title="Record every terminal"
# ~/.zshrc
eval "$(carrier init zsh)"

# ~/.bashrc
eval "$(carrier init bash)"

# ~/.config/fish/config.fish
carrier init fish | source
```

Without an argument, `init` uses the shell from `$SHELL`. See [Record every terminal](../advanced/shell-mode.md#record-every-terminal) for how it works and how to turn it off.

## `attach`

Attach to an existing shell session by session ID or label:

```bash title="Attach to a session"
carrier attach 5
carrier attach backend-debug
carrier attach 1234
```

See [Referring to sessions](#referring-to-sessions) for how labels are matched.

`attach` re-opens the session, starts a new PTY shell that records commands into it, then closes the session when you exit. Runs recorded during the attached shell appear grouped under the same session in `carrier history`.

This is useful when you need to return to a named session after disconnecting, or when you want to continue work in a labeled debugging context from a different terminal.

## `session`

Manage shell session labels and history.

### `session list`

List sessions newest-first:

```bash title="List sessions"
carrier session list
carrier session list --limit 20
```

Output includes session ID, start time, label, and duration (or `active` for open sessions).

### `session label`

Set or clear a session label:

```bash title="Label a session"
carrier session label 3 backend-debug
carrier session label 3               # clear label
```

When inside a tracked shell (`carrier shell` or `carrier attach`), the session ID is available as `$CARRIER_SESSION_ID`. Omit the ID argument to target the current session:

```bash title="Label current session from inside the shell"
carrier session label backend-debug
carrier session label               # clear current session label
```

A first word that is a number is read as a session ID, so `carrier session label 1234 fix` labels session 1234. To start a label with a number, pass the ID first: `carrier session label $CARRIER_SESSION_ID 1234 fix`.

Labeling an automatic session also keeps it from expiring.

### `session keep`

Keep an automatic session so it never expires, or let it expire again:

```bash title="Keep sessions"
carrier session keep              # the current session
carrier session keep 12
carrier session keep api-debug
carrier session unkeep 12
```

`carrier session list` marks automatic sessions with `auto` and kept ones with `kept`.

## `clean`

Preview deletion:

```bash title="Preview old records"
carrier clean --older-than 30d --dry-run
carrier clean --older-than 30d -d
```

Delete old records and logs:

```bash title="Delete old records"
carrier clean --older-than 30d --yes
carrier clean --older-than 30d -y
```

Keep only recent records:

```bash title="Retention by count"
carrier clean --keep-last 500 --dry-run
carrier clean --keep-last 500 --yes
```

Delete automatic sessions older than [`shell.auto_session_retention`](../reference/configuration.md#auto_session_retention), with their runs and logs:

```bash title="Expire automatic sessions"
carrier clean --sessions --dry-run
carrier clean --sessions --yes
```

Deletion requires `--yes`. Use `--dry-run` first.

Notes on deleted runs are kept when they belong to a session, and deleted otherwise.

## `doctor`

Check local setup:

```bash title="Doctor"
carrier doctor
```

Shows version, config path, storage paths, migration version, data size, redaction status, stale running runs, shell support, notification tool availability, and terminal status.

## `config`

Inspect and create config:

```bash title="Config commands"
carrier config path
carrier config show
carrier config check
carrier config init
carrier config init --force
```

## `completion`

Generate a shell completion script:

```bash title="Install completions"
# Bash — write once, restart shell
carrier completion bash > /etc/bash_completion.d/carrier

# Bash — per-session (add to ~/.bashrc for persistence)
source <(carrier completion bash)

# Zsh
carrier completion zsh > "${fpath[1]}/_carrier"

# Fish
carrier completion fish > ~/.config/fish/completions/carrier.fish

# PowerShell (add to $PROFILE)
carrier completion powershell | Out-String | Invoke-Expression
```

## `version`

```bash title="Version"
carrier version
```
