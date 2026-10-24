# Changelog

All notable changes to carrier are listed here, newest first. Run `carrier self changelog` to read this from the command line.

## [1.5.0] - 2026-10-24

### Added

- carrier has a logo: a shipping container with a prompt on its side. It is at the top of the README and the documentation, and is the documentation's header logo and favicon.
- `carrier version` and `carrier setup` show the logo in terminals with 256 or more colors. It is left out when the output is not a terminal and when colors are off. The install scripts print the version when they finish, so new installs show it too.

### Changed

- `carrier version` is a top-level command again and is no longer deprecated. `carrier self version` does the same and keeps working. `carrier changelog` stays deprecated in favor of `carrier self changelog`.
- Deprecation warnings, such as the one for `carrier changelog`, are orange so they are not missed. They stay plain when colors are off.
- carrier is built with Go 1.27.3. `go install` needs Go 1.27 or newer, which the `go` command downloads by itself when it is older.

## [1.4.0] - 2026-10-24

### Added

- `carrier self update` replaces the installed binary with the latest carrier release, so updating no longer means finding the install command again. `--check` reports what is available without installing it, `--version` installs one release, including an older one, and `--force` reinstalls the version already installed. The archive is checked against the release's `checksums.txt` before anything is replaced, the old binary stays in place if any step fails, and shells that are already recording keep working and use the new binary from their next command. A successful update prints what changed since the version you had.
- `carrier self update` is the only command that reaches the network, and only when you run it. carrier does not check for new versions on its own, so `carrier self update --check` is how you ask.
- `carrier self update` leaves alone a binary that did not come from a release archive, and names the command to use instead: `go install` for one in `GOBIN` or `GOPATH/bin`, `make install` for a build from source, and the `--system` installer line for a directory this user cannot write.

### Deprecated

- `carrier version` is now `carrier self version`, and `carrier changelog` is now `carrier self changelog` (`carrier self news` stays an alias). The old names still work and print a warning to stderr, and are removed in the next major release.

## [1.3.1] - 2026-10-23

### Fixed

- Patched vulnerabilities in the `golang.org/x/text` package. Specifically [CVE-2026-56851](https://app.opencve.io/cve/CVE-2026-56851).

## [1.3.0] - 2026-10-23

### Added

- `carrier status` shows whether this terminal is recorded and into which session, with a summary of the session and its outcome, and the session pinned for the directory. It points out a tmux pane that only inherited another terminal's session, a pin that names another session or a missing one, a shell too old for its prompt to follow switches, and a tmux server holding variables from an older carrier. `--json` gives the same.
- `carrier doctor` checks this terminal (`this shell`), the pin for this directory, the tmux server's environment inside tmux, and counts leftover temp files.

## [1.2.4] - 2026-10-22

### Changed

- Commands in a recorded shell start about 4 times sooner: carrier's hook before each command took about 110 ms and now takes about 25 ms. The environment is redacted again only when it changes, and `git status` and the environment are collected in the background, so slow `git status` in large repositories no longer delays commands.
- The Git branch and commit of a shell run are read from `.git` as the command starts.

### Fixed

- Temporary files of recorded shells that were killed instead of exiting, for example by `tmux kill-server`, are removed at most once a day when a shell starts.

## [1.2.3] - 2026-10-22

### Fixed

- Relabeling a session with `carrier session label` or `carrier label` updates the prompt of every terminal recording into it, such as all tmux windows joined to a pinned session, from their next prompt. Only the terminal the label was set in showed it.

## [1.2.2] - 2026-10-22

### Fixed

- tmux panes started from a recorded shell get their own session. They inherited the first shell's variables, so they did not start a session, and `carrier session reload`, `attach`, `note`, or `session end` in a pane acted on the first terminal's session instead. Restart a tmux server started before upgrading (`tmux kill-server`) for this to apply.
- In zsh, shells started from a recorded shell, such as tmux panes or a nested `zsh`, load your own config again instead of carrier's hooks for the first shell, and the hooks load `.zshrc` from your `ZDOTDIR` when you set one.

## [1.2.1] - 2026-10-22

### Fixed

- With powerlevel10k, the session is shown as a `carrier` segment on the right of the prompt, and follows session switches. powerlevel10k rebuilds the right prompt before every prompt, so the indicator was not shown there.
- Switching sessions in a shell opened before 1.1.0, whose prompt cannot follow, now says so; its commands are recorded into the new session.

## [1.2.0] - 2026-10-22

### Added

- `carrier session show` replays a session as a timeline of commands, the error lines of failed commands, notes, and the outcome, with `--failed`, `--output`, `--lines`, and `--json`.
- `carrier session export` writes a session as Markdown, or JSON, to paste into a pull request, ticket, or incident write-up. Environments and log paths are left out, and home paths are shown as `~`.
- `carrier session end` ends a session and records its outcome with `--note`. Inside a recorded shell it moves the shell to a new automatic session, and `--unpin` removes the session's pin.
- `[shell] ask_outcome = true` asks for a labeled session's outcome when its shell exits.
- Notes have a `kind` field in JSON output; outcomes have kind `outcome` and are marked in `carrier notes` and `carrier history`.

## [1.1.0] - 2026-10-21

### Added

- Switch a recorded shell to another session without starting a new shell: `carrier attach <session>` inside a recorded shell records into that session from the next command on, and the prompt, title, and `CARRIER_SESSION_ID` follow. An automatic session left with nothing in it is deleted.
- `carrier session pin` writes a `.carrier-session` file, like `.python-version`, so every new terminal or tmux window started in that directory or below it joins the pinned session instead of starting its own automatic session. `carrier session unpin` removes it.
- `carrier session reload` switches the shell to the session pinned for the current directory. Changing directory never switches sessions on its own.
- `carrier attach --new-shell` starts a new shell even inside a recorded shell.

### Changed

- `carrier attach` inside a recorded shell switches sessions instead of starting a shell inside the shell.
- Relabeling the current session with `carrier session label` updates the prompt.
- A session several terminals record into stays active while any of them runs commands.

## [1.0.1] - 2026-10-21

### Added

- Tests that keep 1.x compatible: every `--json` output is checked field by field, and data recorded by 0.1.0 and 0.27.0 is upgraded and checked on every change. The Compatibility page describes both.

## [1.0.0] - 2026-10-20

### Changed

- carrier 1.0 is the first stable release. From now on, commands, flags, exit codes, JSON output, config settings, and your data are meant to stay compatible across 1.x releases, and anything that has to break is marked **Breaking:** here. See the Compatibility page in the docs for what is covered.
- Since 0.1.0, carrier has grown from recording single commands into a record of whole debugging sessions: shell mode for zsh, bash, and fish, automatic sessions for every terminal, notes, labels and run shortcuts, similar-failure hints, full-text search, a TUI, and the `carrier setup` wizard. Run `carrier changelog --since 0.1.0` for the full history.

### Added

- A Compatibility page in the docs describing what we aim to keep stable from 1.0, such as commands, flags, exit codes, JSON fields, config settings, and data upgrades, and what may change in any release.

## [0.34.0] - 2026-10-20

### Changed

- Every duration flag and setting accepts the same format, including `d` for days and `w` for weeks, combined as in `1d12h`. `--timeout`, `--debounce`, `notify.min_duration`, `storage.stale_run_threshold`, and `hints.similar_min_age` now accept days and weeks, and `--since` and `--older-than` accept weeks and combined units.
- Negative durations are rejected everywhere, and invalid durations are reported with the accepted format.
- `carrier config check` reports an invalid `storage.stale_run_threshold`.

## [0.33.0] - 2026-10-20

### Added

- `--json` in `carrier failed`, `carrier session list`, and `carrier history --sessions-only`.
- Run JSON includes `session_id` for runs recorded in a shell session, so `history --json` keeps the session grouping.

## [0.32.0] - 2026-10-20

### Changed

- Shell mode is no longer experimental: the alpha warning, the `(alpha)` label in help, and the `doctor` warning are gone. zsh, bash, and fish are tested in a real shell for every change.
- Commands in `shell.ignore_commands`, such as `vim` and `less`, are recorded with their command line, timing, and exit code instead of being skipped; only their output is left out, and `carrier show` says so.
- `carrier doctor` reports fish as a supported shell.

### Fixed

- In zsh, the session indicator on the right of the prompt no longer repeats after every command.
- In zsh, pressing Enter on an empty line no longer starts an extra carrier process before the prompt.
- With fish, closing an automatic session now closes the terminal instead of leaving a second, unrecorded fish prompt.
- In fish automatic sessions, a terminal opened and closed without running anything no longer leaves a session behind.

## [0.31.0] - 2026-10-20

### Added

- `carrier shell -L` as the short form of `--label`, like `run`, `rerun`, and `watch`.

### Changed

- **Breaking:** `-s` now means `--session` in every command. `carrier history -s` filters by session, as in `note` and `notes`; `--short` no longer has a short form, so use `carrier history --short` or `carrier hs`.

## [0.30.0] - 2026-10-20

### Added

- `carrier changelog` shows what changed in each version: the installed version by default, one version, everything `--since` a version, or `--all`, with `--json` and `--markdown` output.
- A changelog covering every release since 0.1.0.

## [0.29.0] - 2026-10-20

### Added

- `carrier setup` walks through the main settings one step at a time: recording every terminal, similar-failure hints, desktop notifications, and the `c` alias. Each step checks what is already set up, shows the exact change to each file, and asks before writing. `carrier setup --dry-run` shows the changes without writing anything.
- Setup edits existing config files in place, keeping comments and other settings, edits the included file when a setting is set there, and backs up every file before changing it.

## [0.28.0] - 2026-10-20

### Added

- `carrier init zsh|bash|fish` prints a shell config line that starts every interactive terminal in an automatic session. Set `CARRIER_NO_AUTO=1` to skip it for one terminal.
- Automatic sessions expire after `shell.auto_session_retention` (30 days by default). Sessions with a label, notes, or `carrier session keep` never expire.
- `carrier session keep` and `carrier session unkeep`.
- `carrier clean --sessions` deletes expired automatic sessions on demand.
- `carrier shell --program` chooses the shell to run.
- `carrier doctor` shows whether automatic sessions are set up.

### Changed

- `carrier session list` marks automatic and kept sessions.
- Labeling an automatic session reports that it will be kept.

## [0.27.0] - 2026-10-20

### Added

- Refer to runs as `last`, `-1` (latest), `-2` (the one before), and so on, or by label, in `show`, `tail`, `export`, `rerun`, `diff`, `label`, `similar`, `note -r`, and `notes -r`.
- Attach to sessions by label as well as ID. A number that is not a session ID is matched against labels, so `carrier attach 1234` finds a session labeled `BUG-1234: fix login`. References matching several different labels are reported instead of guessed.

## [0.26.0] - 2026-10-20

### Added

- Optional similar-failure hints (`[hints] similar_failures = true`): when a command fails with an error seen before, carrier points at the earlier run and its notes.
- `carrier similar <run>` lists earlier failed runs with the same error.

### Fixed

- Shell mode output is now fully searchable. Runs were indexed before their terminal log was completely written, so search often matched shell runs by command only.

## [0.25.0] - 2026-10-20

### Added

- Helpful messages when there is nothing to show, for example `no runs recorded yet; try "carrier run echo hello" or "carrier shell"`. Hints are written to stderr, so piped and `--json` output is unchanged.

### Changed

- Missing runs and sessions are reported as `run 7 not found` instead of a database error.

### Fixed

- `carrier label` no longer reports success for a run that does not exist.

## [0.24.1] - 2026-10-20

### Fixed

- In bash shell mode, each command line is now recorded as one run, including `a && b`, pipelines, and subshells. Shell startup commands are no longer recorded.
- The command that closes a shell, such as `exit`, is no longer left as running.

## [0.24.0] - 2026-10-20

### Added

- `carrier note <text>` records what you were thinking in the current shell session or on a run. Notes appear in `history` and `show` and are searchable.
- `carrier notes` lists notes, and `carrier notes rm` deletes one.

## [0.23.0] - 2026-05-28

### Added

- Auto-label rules (`[[auto_label]]`) label runs automatically by command, directory, or Git branch.
- `includes` splits the config across multiple files.

## [0.22.0] - 2026-05-28

### Added

- `carrier diff` compares the output of two runs.
- `--label/-L` labels a run when it is recorded.
- `carrier last N` shows the N most recent runs.
- `carrier completion` generates shell completions.
- `--command/-c` filter and minimum and maximum durations in `carrier stats`.
- `--since` and `--status` filters in `carrier search`.
- `r` reruns the selected run in `carrier tui`.

### Changed

- `carrier watch` stops the running command when a file changes, and prints the file that triggered the run.

### Fixed

- `--help` works for `carrier watch` and `carrier run`.

## [0.21.1] - 2026-05-23

### Changed

- Session and run status text use the theme colors.

## [0.21.0] - 2026-05-22

### Added

- `carrier tui`, an interactive run browser.

## [0.20.0] - 2026-05-22

### Added

- `NO_COLOR` and `CARRIER_COLOR` control colored output, and `[ui]` theme colors can be configured.

### Changed

- Run lists show status glyphs, relative times, and shortened paths.

## [0.19.5] - 2026-05-22

### Changed

- Faster run recording: run metadata is saved in a single database update.

## [0.19.4] - 2026-05-22

### Changed

- Faster startup: secret-detection patterns are compiled once, in the background.

## [0.19.3] - 2026-05-22

### Changed

- Faster runs and shell sessions: Git and environment details are collected in parallel, the database uses write-ahead logging, and repeated startup checks are skipped.

## [0.19.2] - 2026-05-20

### Changed

- Documentation for how shell mode handles full-screen programs.

## [0.19.1] - 2026-05-20

### Fixed

- Output from full-screen programs such as editors is no longer saved to shell session logs or replayed by `carrier show`.

## [0.19.0] - 2026-05-20

### Added

- `carrier history --short` collapses session runs.

## [0.18.1] - 2026-05-19

### Fixed

- Stronger redaction of captured environment variables.
- `carrier history` lists runs oldest-first.

## [0.18.0] - 2026-05-19

### Added

- Shell sessions show an indicator in the prompt and the terminal title.

## [0.17.0] - 2026-05-19

### Added

- Command aliases, such as `carrier h` for `history`.
- `help` is listed in the help menu.
- Each run records a redacted snapshot of its environment, stored once per distinct environment.

### Changed

- New color palette.
- `carrier clean` removes environment snapshots no longer used by any run.

## [0.16.0] - 2026-05-18

### Added

- `carrier attach` rejoins an existing shell session.
- `carrier history` groups session runs in a tree, and `--sessions-only` lists sessions.
- `carrier history --session` accepts a session label.
- `carrier shell` accepts the session label as an argument, and labeling a run also labels its session.

## [0.15.0] - 2026-05-17

### Added

- Shell sessions: runs recorded in `carrier shell` are grouped by session, and sessions can be labeled.

## [0.14.7] - 2026-05-17

### Fixed

- Shell mode resizes correctly on every platform.

## [0.14.6] - 2026-05-17

### Fixed

- Shell mode starts at the right terminal size and follows resizes.
- Shell mode sets `TERM` and `COLORTERM`.
- zsh no longer prints `WARN_CREATE_GLOBAL` warnings in shell mode.

## [0.14.5] - 2026-05-17

### Added

- Built-in secret detection based on the gitleaks rule set, used to redact persisted logs.

## [0.14.4] - 2026-05-12

### Changed

- `carrier watch` passes any flags through to the child command.

## [0.14.3] - 2026-05-11

### Added

- Installer scripts for Linux, macOS, and Windows.

## [0.14.2] - 2026-05-11

### Changed

- Colored run status output.

## [0.14.1] - 2026-05-11

### Changed

- Internal test improvements only.

## [0.14.0] - 2026-05-10

### Added

- `carrier watch` reruns a command when files change.
- Runs record their environment variables.

## [0.13.0] - 2026-05-10

### Added

- `carrier label` labels a run, and `carrier history --label` filters by label.

## [0.12.0] - 2026-05-10

### Added

- `--command/-c` filter in `carrier history`.
- `--stdout` and `--stderr` in `carrier show`.
- `--format` in `carrier export`.

### Changed

- Runs left running after a crash are marked as killed on startup.

## [0.11.0] - 2026-05-10

### Added

- `--limit/-l` in `carrier search`.
- `--lines` in `carrier show` limits output length.
- `--edit/-e` in `carrier rerun` edits the command before running it.
- Desktop notifications on Windows.

### Fixed

- `carrier run` passes stdin to the child command.
- `carrier history --since` accepts days, such as `7d`.

## [0.10.0] - 2026-05-10

### Added

- `--status`, `--since`, `--cwd`, and `--branch` filters in `carrier history`.
- `--json` in `carrier search`, and search accepts multiple words.
- `--timeout/-t` in `carrier run`.
- `--keep-last` in `carrier clean`.
- Desktop notifications on macOS.

### Changed

- `carrier tail` follows output as it is written instead of polling.

## [0.9.2] - 2026-05-10

### Changed

- Internal code cleanup only.

## [0.9.1] - 2026-05-10

### Changed

- Search also matches parts of words in commands and directories.

## [0.9.0] - 2026-05-10

### Added

- `carrier history`.

### Changed

- Improved shell mode.

## [0.8.0] - 2026-05-10

### Added

- Full-text search over commands and output.
- Colored command output.

## [0.7.1] - 2026-05-10

### Added

- `carrier stats`.

## [0.7.0] - 2026-05-10

### Added

- `carrier config check` validates the config.

## [0.6.0] - 2026-05-10

### Added

- `carrier tail` can show only stdout or stderr.

### Changed

- `carrier clean` asks for confirmation before deleting.
- Clearer errors when a command cannot be started.
- `carrier doctor` checks more of the setup.

## [0.5.1] - 2026-05-10

### Added

- SHA-256 checksums for release downloads.

## [0.5.0] - 2026-05-10

### Added

- `carrier config init`, `carrier config show`, and `carrier config path`.

## [0.4.0] - 2026-05-10

### Changed

- The database schema is upgraded automatically with versioned migrations.

## [0.3.0] - 2026-05-10

### Added

- `--dry-run` in `carrier clean`.

## [0.2.0] - 2026-05-10

### Added

- `--json` output for run inspection commands.

## [0.1.1] - 2026-05-10

### Added

- `carrier version` shows release details.

### Changed

- Recorded commands are displayed with shell-safe quoting.

## [0.1.0] - 2026-05-10

### Added

- `carrier run` records a command's output, exit code, timing, working directory, and Git details, and keeps the command's exit code.
- `last`, `show`, `tail`, `failed`, `running`, `search`, `export`, `rerun`, and `clean` to inspect and manage runs.
- `carrier shell`, an experimental tracked shell for zsh and bash.
- `carrier doctor` checks the setup.
- Persisted logs are redacted and size-capped.
- Desktop notifications on Linux.
