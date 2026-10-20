# Changelog

All notable changes to carrier are listed here, newest first. Run `carrier changelog` to read this from the command line.

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
