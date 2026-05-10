# carrier

`carrier` is a local command logger for developers. It records what you ran, where you ran it, when it started and finished, how long it took, whether it failed, captured output, and useful Git context.

It is designed for normal terminal work: Ubuntu, Ghostty, tmux, zsh/bash, local projects, and long-running build/test commands.

!!! tip "New here?"

    Start with [Getting started](tutorial/getting-started.md). It walks through install, first run, inspection, and cleanup.

!!! warning "Shell mode is alpha"

    `carrier run` is the stable path for precise stdout/stderr capture. `carrier shell` is useful for experiments and long interactive sessions, but depends on PTY output and shell hooks.

## Common tasks

- Install a release binary: [Installation](tutorial/installation.md)
- Record your first command: [Getting started](tutorial/getting-started.md)
- Learn every command: [Commands](guide/commands.md)
- Configure storage, redaction, notifications, and shell mode: [Configuration reference](reference/configuration.md)
- Understand SQLite and log files: [Storage and state](reference/storage-and-state.md)
- Use JSON, rerun, export, and tail workflows: [Advanced workflows](advanced/power-user-workflows.md)
- Debug local setup: run `carrier doctor` or read [Troubleshooting](reference/troubleshooting.md)
- See release assets and checksums: [Release install reference](reference/release-install.md)

## Why use carrier?

- keep command history with stdout/stderr
- preserve child process exit codes
- search past commands and output
- export failed runs as Markdown
- rerun commands from their original working directory
- get optional desktop notifications for long commands
- keep logs on local disk, redacted by default

## Core concepts

### A run is one recorded command

Every recorded command gets an integer ID. Use that ID with `show`, `tail`, `export`, and `rerun`.

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
- [Storage and state](reference/storage-and-state.md)
- [Release install reference](reference/release-install.md)
- [Troubleshooting](reference/troubleshooting.md)
- [FAQ](faq.md)
