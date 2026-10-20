# Compatibility

From 1.0, carrier aims to follow [semantic versioning](https://semver.org). This page describes what we try to keep the same within a major version, and what is likely to change in any release, so you know what is safe to build scripts and habits on.

This is how we intend to work, not a guarantee. If a 1.x release ever has to break something listed here, for example to fix a security problem, the changelog marks it **Breaking:** and explains what to do instead.

| Release          | Example         | Aims to contain                                     |
| ---------------- | --------------- | --------------------------------------------------- |
| Patch            | `1.2.0 → 1.2.1` | bug fixes only                                      |
| Minor            | `1.2.1 → 1.3.0` | new commands, flags, settings, and JSON fields      |
| Major            | `1.3.0 → 2.0.0` | changes that can break scripts, config, or habits   |

Changes are listed in the changelog. Run `carrier changelog --since <version>` before upgrading, or see the [release notes](https://github.com/atbuy/carrier/releases). Breaking changes are marked **Breaking:**.

## What we try to keep stable

Within 1.x, we aim to keep these working the same way. New things may be added next to them.

### Commands and flags

- Command names and their aliases, such as `carrier history` and `carrier h`.
- Flag names, short forms, and meaning, such as `-s` for `--session` and `-L` for `--label`.
- Run references: IDs, `last`, `-1`, `-2`, and labels, as described in [Referring to runs](../guide/commands.md#referring-to-runs).
- Session references: IDs and labels, as described in [Referring to sessions](../guide/commands.md#referring-to-sessions).
- The [duration format](configuration.md#durations), such as `30m`, `7d`, and `1d12h`.

### Exit codes

| Command                  | Exit code                                                       |
| ------------------------ | --------------------------------------------------------------- |
| `carrier run`, `rerun`   | the command's own exit code, or `127` when it cannot be started |
| everything else          | `0` on success, `1` on an error such as `run 42 not found`      |

A list with nothing to show, such as `carrier failed` with no failed runs, exits `0`.

### JSON output

- Field names, types, and meaning in `--json` output.
- The order of arrays: `history`, `history --sessions-only`, and `notes` list oldest first; `last N`, `failed`, `running`, and `session list` list newest first; `search` lists the best matches first.
- A list with nothing to show prints `[]`, and hints are never written to stdout.

New fields may be added in minor releases, so scripts should ignore fields they do not know. Fields such as `exit_code`, `label`, `session_id`, and `ended_at` are left out when they have no value, so treat a missing field as empty.

### Plain-text output for scripts

The first field of each line in `history`, `failed`, `running`, and `search` is the run ID, and we aim to keep it there so this keeps working:

```bash
carrier history | fzf | awk '{print $1}' | xargs carrier rerun
```

Everything else about human-readable output may change; see below.

### Configuration

- Setting names, sections, value types, and meaning in `config.toml`, including `includes` and `[[auto_label]]`.
- A config file that works with one 1.x release should keep working with later 1.x releases. When a setting is replaced, the old one is deprecated first (see below).
- Defaults try to stay the same. If one changes, the changelog says so, and setting the value explicitly keeps the old behavior.

### Environment variables

`NO_COLOR`, `CARRIER_COLOR`, and `CARRIER_NO_AUTO` should keep their meaning, and `CARRIER_SESSION_ID` is set inside a recorded shell. See [Environment variables](environment.md).

### Shell setup

- The line you add to your shell config for `carrier init`, such as `eval "$(carrier init zsh)"`, should keep working.
- zsh, bash, and fish are supported in shell mode.

### Your data

- A new release opens the data directory of earlier releases, including 0.x, and upgrades the database automatically, so nothing needs to be exported or migrated by hand.
- Upgrades are not meant to delete anything. Runs, sessions, notes, labels, and logs are deleted only by `carrier clean` and `shell.auto_session_retention`.

### Releases

The release asset names, such as `carrier-linux-amd64.tar.gz`, and the install scripts' options are meant to stay the same. See [Release install reference](release-install.md).

## What may change in any release

These are not kept stable and may change in any release, including patch releases.

- **Human-readable output.** Colors, columns, glyphs, wording, hints, and help text. Use `--json` in scripts.
- **The database schema and the files in the data directory.** Read runs with `--json` or `carrier export` instead of querying `carrier.db` directly.
- **Internals.** `carrier internal` commands, the generated shell hooks, `CARRIER_SHELL_STATE`, `CARRIER_HOOK_ACTIVE`, `CARRIER_NOTIFY`, `CARRIER_NOTIFY_ALWAYS`, `CARRIER_NO_REDACT`, `CARRIER_SESSION_LABEL`, and the sidecar files next to the database.
- **Heuristics.** Which runs similar-failure hints and `carrier similar` match, and how label matching orders candidates when the result is unambiguous.
- **Redaction.** Secret detection can become stricter in any release, so newly recorded logs may hide more than before. Logs already written are not changed.
- **The TUI.** Layout and key bindings in `carrier tui`.
- **Bug fixes.** Behavior that is clearly wrong, such as a command recorded twice, is fixed in patch releases even if something relied on it.

## Downgrades

Going back to an older release is not supported once a newer release has upgraded the database. Back up the data directory before trying a pre-release:

```bash
cp -r ~/.local/share/carrier ~/.local/share/carrier.backup
```

## Deprecation

When something from the stable list needs to go, we plan to deprecate it first:

1. The changelog lists it under **Deprecated**, with what to use instead.
2. It keeps working for the rest of 1.x where possible, and prints a warning to stderr when used.
3. It is removed or changed in the next major release, listed under **Removed** or **Changed**.

## Before 1.0

Releases before 1.0 made breaking changes in minor releases, such as `-s` becoming `--session` everywhere in 0.31.0. Each one is marked **Breaking:** in the [changelog](https://github.com/atbuy/carrier/blob/main/CHANGELOG.md).
