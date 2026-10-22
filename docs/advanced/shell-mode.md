# Shell mode

`carrier shell` starts your shell inside a PTY and adds shell hooks so carrier knows when each command line starts and ends. Every command line becomes a run, with its terminal output, exit code, timing, and Git details, grouped under the current session.

```bash title="Start shell mode"
carrier shell
```

To record every terminal without starting `carrier shell` yourself, see [Record every terminal](#record-every-terminal).

## Shell mode or `carrier run`

Both record the command, exit code, timing, working directory, and Git details. They differ in how output is kept:

- `carrier shell` records the terminal as you saw it, with stdout and stderr together. Use it for everyday work and debugging sessions, where what happened and in what order matters.
- `carrier run` keeps stdout and stderr in separate logs. Use it when a single command's streams matter on their own, for example in scripts or CI-like checks.

## Supported shells

- zsh
- bash (4.4 or newer recommended)
- fish

`carrier doctor` reports whether the configured shell looks supported.

## How it works

Architecture:

```text
terminal/tmux
  -> carrier PTY recorder
    -> zsh/bash/fish
      -> user commands
```

For zsh, `carrier` uses `preexec` and `precmd` hooks.

After sourcing your `.zshrc`, `carrier` disables zsh's `warn_create_global` option inside the tracked shell. This avoids prompt helper warnings from being printed on every command when prompt functions assign globals.

For bash, `carrier` starts bash with a generated `--rcfile` and uses `PS0`, a `DEBUG` trap, and `PROMPT_COMMAND`. Each command line becomes one run, including compound lines like `make && make test`, pipelines, and subshells like `(cd api; make)`. The full line is taken from bash history, so it is recorded as typed. Existing `PROMPT_COMMAND` and `PS0` content is preserved, and `PROMPT_COMMAND` runs before `carrier` finishes the tracked command.

Bash 4.4 or newer is recommended. Older versions (such as the bash 3.2 that ships with macOS) have no `PS0`, so lines made only of a subshell are not recorded there.

`carrier` guards its hook internals so internal `carrier internal ...` commands and `_carrier_*` helper functions are not recorded as user commands.

## Sessions

Every `carrier shell` invocation creates a shell session. All runs recorded during that shell are linked to the session.

### Label a session at start

Pass a label as a positional argument or with `--label`:

```bash title="Named session"
carrier shell 'backend-debug'
carrier shell --label backend-debug
```

### View sessions

```bash title="List sessions"
carrier session list
```

Output includes session ID, start time, label, and duration:

```text
     5  2026-05-18 12:01  backend-debug  42m15s
     3  2026-05-17 09:30  (unlabeled)    1h02m33s
```

### Label a session after starting

From inside the tracked shell, `$CARRIER_SESSION_ID` is set automatically. Omit the ID argument to target the current session:

```bash title="Label current session"
carrier session label backend-debug
carrier session label               # clear the label
```

From outside, pass the session ID explicitly:

```bash title="Label a session by ID"
carrier session label 5 backend-debug
```

Labeling a run with `carrier label` also propagates the label to the run's session automatically.

### Grouped history

`carrier history` shows session runs as a tree:

```text
   5  ┬──  2026-05-18 12:01  session: backend-debug
   3  ├──  failed   1.2s  make test
   2  └──  success  0.4s  go vet ./...
   1       failed   2.1s  make lint
```

Filter to one session:

```bash title="Filter history by session"
carrier history --session 5
carrier history --session backend-debug
```

Show only session headers:

```bash title="Sessions only"
carrier history --sessions-only
```

### Notes

Use `carrier note` inside the shell to record what you were thinking. The note is added to the current session after the last command, so `history` shows why you ran the next one:

```bash title="Add a note"
carrier note suspect stale DNS cache, flushing next
```

The `carrier note` command itself is not recorded as a run. List the session's notes with `carrier notes`.

### Review a session

When you are done, record what you found and read the session back:

```bash title="End and review a session"
carrier session end --note "pool size was 0 after the config change"
carrier session show BUG-1234
carrier session export BUG-1234 > bug-1234.md
```

`session show` replays the session as a timeline of commands, failed commands' error lines, and notes, with the outcome at the top. `session export` writes the same as Markdown to paste into a pull request or ticket. Inside a recorded shell, `session end` moves the shell to a new automatic session. Set [`shell.ask_outcome`](../reference/configuration.md#ask_outcome) to be asked for the outcome when a labeled session's shell exits. See [`session show`](../guide/commands.md#session-show).

## Record every terminal

Add one line to your shell config, after `PATH` is set, to start every interactive terminal in an automatic session:

```bash title="~/.zshrc"
eval "$(carrier init zsh)"
```

Use `carrier init bash` in `~/.bashrc`, or `carrier init fish | source` in `~/.config/fish/config.fish`. `carrier doctor` shows whether it is set up.

How it behaves:

- it only runs in interactive terminals, never in scripts, `ssh host cmd`, or pipes
- it never runs inside an existing carrier session, so `carrier shell` loading your config again does not loop
- exiting the recorded shell closes the terminal, as usual
- if carrier cannot start, it prints why and the terminal continues as a normal shell
- a terminal opened and closed without running anything leaves no session behind
- each terminal, and each tmux pane, is its own session, unless its directory is [pinned to a session](#one-session-for-every-terminal)

Turn it off for one terminal with `CARRIER_NO_AUTO=1`, for example `CARRIER_NO_AUTO=1 zsh`, or remove the line from your config.

### Keep or expire automatic sessions

Automatic sessions expire [`shell.auto_session_retention`](../reference/configuration.md#auto_session_retention) after they end (30 days by default), with their runs and logs. A session is kept for good when it has:

- a label: `carrier session label api-debug` from inside the session
- a note: `carrier note ...`
- a pin: `carrier session keep`

Sessions you start yourself with `carrier shell` never expire. Expired sessions are cleaned up at most once a day when a shell starts; `carrier clean --sessions --dry-run` shows what would go.

### One session for every terminal

When you work on one task across several terminals or tmux windows, pin the repository to the task's session:

```bash title="Pin the repository"
cd ~/src/api
carrier session label "BUG-1234: fix login"
carrier session pin
```

This writes a `.carrier-session` file. Every automatic session that starts in `~/src/api` or below it now joins `BUG-1234: fix login` instead of starting its own, so a new tmux window in the repository records into the same session. Moving to another repository for testing keeps you in the session, even when that repository has its own pin; run `carrier session reload` to switch to the session pinned where you are. See [`session pin`](../guide/commands.md#session-pin).

## Switching sessions

Inside a recorded shell, `carrier attach` switches the shell to another session without starting a new one:

```bash title="Switch by ID or label"
carrier attach 5
carrier attach backend-debug
carrier attach BUG-1234   # matches the label "BUG-1234: fix login"
```

From the next command on, runs are recorded into that session, and the prompt indicator, terminal title, and `CARRIER_SESSION_ID` follow. The session you leave is ended; an automatic session you leave without running anything is deleted. `carrier attach 1234` also works: when there is no session 1234, carrier matches the number against labels. See [Referring to sessions](../guide/commands.md#referring-to-sessions).

Outside a recorded shell, or with `--new-shell`, `attach` starts a new shell that records into the session, which is useful to continue a session from another terminal or after a disconnect.

## What gets recorded

Shell mode creates one run per command line. Shell runs use:

- mode: `shell`
- status: `running`, `success`, `failed`, or `killed`
- command text from the shell hook
- cwd
- Git metadata
- terminal output log path
- session ID

Unlike `carrier run`, shell mode stores terminal output instead of separate stdout and stderr logs.

The command that ends the shell, such as `exit`, is finished with the shell's exit status when the shell exits. If the shell is killed by a signal, that run is marked `killed`.

## TUI applications

TUI programs (alternate-screen apps like neovim, lazygit, noteui) run normally inside a carrier shell session — their output appears on screen as usual. Carrier detects the alternate-screen escape sequences (`\x1b[?1049h` / `\x1b[?1049l`) and suppresses the enclosed content from the terminal log, because cursor-movement-heavy TUI output cannot be replayed as plain text. When you run `carrier show` on a run that involved a TUI application, the output section displays `(TUI session — terminal output suppressed)` instead of replaying the log. Run metadata (command, exit code, duration, etc.) is fully recorded regardless.

## Limitations

- stdout and stderr are recorded together, as shown in the terminal
- full-screen (alternate-screen) output is left out of terminal logs, since it cannot be replayed as text
- in bash, other tools that also set a `DEBUG` trap or `PROMPT_COMMAND` can interfere with the hooks
- in bash, lines hidden from history with `HISTCONTROL=ignorespace` are recorded by their first command only, for example ` export A=1 && make` is recorded as `export A=1`
- `carrier shell` is not a drop-in replacement for shell history

## Inspect shell runs

From another terminal:

```bash title="Inspect shell runs"
carrier running
carrier tail 42
carrier show 42
```

Shell runs use `terminal_output_path` instead of separate stdout/stderr paths.

Show only terminal output:

```bash title="Tail terminal stream"
carrier tail 42 --stream terminal
```

## Commands recorded without output

Commands in `shell.ignore_commands` are recorded with their command line, working directory, timing, and exit code, but without their terminal output:

```toml title="Config"
[shell]
ignore_commands = ["nvim", "vim", "less", "man", "fzf", "yazi", "lazygit", "tmux"]
```

Full-screen programs are listed by default: knowing that you opened `vim config.yaml` or `less app.log` matters when you look back, but their screen output cannot be replayed as text. `carrier show` notes that the output was not recorded.

## Choose a shell

Leave `program` empty to use `$SHELL`:

```toml title="Use $SHELL"
[shell]
program = ""
```

Set a shell explicitly:

```toml title="Explicit shell"
[shell]
program = "/bin/zsh"
```

## Recommendation

Record every terminal with the line from `carrier init`, label sessions you will want to find again, and write down what you were thinking with `carrier note`. Use `carrier run` when you need a single command's stdout and stderr kept apart.
