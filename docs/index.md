# carrier

`carrier` is a local command logger for developers. It records what you ran, where you ran it, when it started and finished, how long it took, whether it failed, captured output, and useful Git context.

It is designed for normal terminal work: local projects, long-running build/test commands, deploys, migrations, Docker tasks, zsh/bash, tmux, and terminal emulators such as Ghostty.

!!! tip "New here?"

    Start with [Getting started](tutorial/getting-started.md). It walks through install, first run, inspection, and cleanup.

!!! example "Try it in one command"

    Already installed?

    ```bash
    carrier run bash -c 'echo "build started"; echo "warning: example" >&2; sleep 1; exit 1'
    ```

    Then inspect it:

    ```bash
    carrier last
    carrier show 1
    ```

!!! warning "Shell mode is alpha"

    `carrier run` is the stable path for precise stdout/stderr capture. `carrier shell` is useful for experiments and long interactive sessions, but depends on PTY output and shell hooks.

## Common tasks

- Install carrier: [Installation](tutorial/installation.md)
- Record your first command: [Getting started](tutorial/getting-started.md)
- Learn every command and flag: [Commands](guide/commands.md)
- Build daily workflows with history, labels, export, rerun, and search: [Workflows](guide/workflows.md)
- Script against JSON output and log paths: [Power-user workflows](advanced/power-user-workflows.md)
- Understand alpha shell mode, sessions, and `carrier attach`: [Shell mode](advanced/shell-mode.md)
- Configure storage, redaction, notifications, and shell behavior: [Configuration reference](reference/configuration.md)
- Check environment variables: [Environment variables](reference/environment.md)
- Understand SQLite and log files: [Storage and state](reference/storage-and-state.md)
- Debug local setup: run `carrier doctor` or read [Troubleshooting](reference/troubleshooting.md)

## Why use carrier?

- command history includes stdout, stderr, duration, exit code, cwd, Git branch, and Git dirty state
- child process exit codes are preserved, so `carrier run` works in scripts
- search covers commands, cwd, and stored output snippets
- failed runs can be exported as Markdown or JSON for issues, pull requests, and chat threads
- original argv is stored separately from the display string, so `rerun` does not parse shell-looking text
- logs stay local on disk and are redacted before persistence by default
- optional notifications help with slow builds without making every command noisy
- shell sessions group related commands under a named label, visible as a tree in `carrier history`
- `carrier attach` re-opens a labeled session from any terminal

## Core concepts

### A run is one recorded command

Every recorded command gets an integer ID. Use that ID with `show`, `tail`, `export`, `label`, and `rerun`.

```bash
carrier run go test ./...
carrier last
carrier show 1
```

### Metadata lives in SQLite

Run metadata is stored in:

```text
~/.local/share/carrier/carrier.db
```

### Output lives in log files

Command output is stored under:

```text
~/.local/share/carrier/runs/
```

`carrier run` stores stdout and stderr separately. `carrier shell` stores terminal output.

### Logs are protected

Persisted logs are:

- redacted before writing to disk
- capped by `storage.max_output_mb`
- left on disk until you clean them

Terminal output still streams normally. Redaction and truncation only affect persisted logs.

### Search is local

`carrier search` uses SQLite FTS for command/cwd/output matches, then falls back to substring matching for command and cwd text. No hosted service is involved.

## Choose your path

### First-time users

- [Getting started](tutorial/getting-started.md)
- [Installation](tutorial/installation.md)
- [First run tutorial](tutorial/first-run.md)

### Daily usage

- [Commands](guide/commands.md)
- [Workflows](guide/workflows.md)

### Advanced usage

- [Power-user workflows](advanced/power-user-workflows.md)
- [Shell mode](advanced/shell-mode.md)

### Exact reference

- [Configuration reference](reference/configuration.md)
- [Environment variables](reference/environment.md)
- [Storage and state](reference/storage-and-state.md)
- [Release install reference](reference/release-install.md)
- [Troubleshooting](reference/troubleshooting.md)
- [FAQ](faq.md)
