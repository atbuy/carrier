# Getting started

This tutorial gets you from a fresh install to a useful `carrier` workflow.

## What carrier is

`carrier` records command executions on your machine. It captures:

- command and arguments
- working directory
- start and finish time
- duration
- exit code
- stdout and stderr logs
- Git root, branch, commit, and dirty state

It is most useful for test runs, builds, migrations, deploy commands, long Docker tasks, and debugging sessions.

## What carrier is not

`carrier` is not a hosted service, terminal replacement, shell history replacement, or CI system.

!!! note

    Everything is local. Metadata is stored in SQLite, and output logs are stored as files under your local data directory.

## Install carrier

=== "Linux/macOS"

    ```bash
    curl -fsSL https://raw.githubusercontent.com/atbuy/carrier/main/install.sh | sh
    ```

=== "Windows"

    ```powershell
    irm https://raw.githubusercontent.com/atbuy/carrier/main/install.ps1 | iex
    ```

Specific versions and Go installs are covered in [Installation](installation.md).

Verify:

```bash title="Verify install"
carrier version
carrier doctor
```

## Add a short alias

Most examples use:

```bash title="Shell alias"
alias c='carrier'
```

Add that to `~/.zshrc` or `~/.bashrc`, then reload your shell:

```bash title="Reload zsh config"
source ~/.zshrc
```

Use `source ~/.bashrc` if you use bash.

## Record your first command

Run a simple command:

```bash title="Record a simple command"
c run echo "hello from carrier"
```

You should see normal terminal output. `carrier` records metadata and saves logs.

Show latest run:

```bash title="Show latest run"
c last
```

Inspect full details:

```bash title="Show full details"
c show 1
```

If your first run ID is not `1`, use ID shown by `last`.

## Record a command that fails

Run:

```bash title="Record a failing command"
c run bash -c 'echo stdout; echo stderr >&2; exit 7'
echo $?
```

The final `echo $?` should print `7`. `carrier run` preserves child exit code.

List failed runs:

```bash title="List failed runs"
c failed
```

## Search output

Search command text, cwd, stdout, stderr, and terminal logs:

```bash title="Search output"
c search "stderr"
```

## Export a run

Export run details as Markdown:

```bash title="Export Markdown"
c export 1 > run-1.md
```

This is useful when sharing a failure with teammates or adding context to an issue.

## Rerun a command

Run again from original working directory:

```bash title="Rerun original argv"
c rerun 1
```

This creates a new run record. It does not overwrite the old run.

## Clean old runs safely

Preview deletion:

```bash title="Preview cleanup"
c clean --older-than 30d --dry-run
```

Actually delete:

```bash title="Delete old records and logs"
c clean --older-than 30d --yes
```

## Next steps

- Read [First run tutorial](first-run.md) for a realistic build/test example.
- Read [Commands](../guide/commands.md) for full command coverage.
- Read [Configuration reference](../reference/configuration.md) before changing storage, redaction, or notifications.
