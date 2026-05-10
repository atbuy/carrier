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

If you already have Go installed:

```bash
go install github.com/atbuy/carrier/cmd/carrier@latest
```

If you prefer release binaries, see [Installation](installation.md).

Verify:

```bash
carrier version
carrier doctor
```

## Add a short alias

Most examples use:

```bash
alias c='carrier'
```

Add that to `~/.zshrc` or `~/.bashrc`, then reload your shell:

```bash
source ~/.zshrc
```

Use `source ~/.bashrc` if you use bash.

## Record your first command

Run a simple command:

```bash
c run echo "hello from carrier"
```

You should see normal terminal output. `carrier` records metadata and saves logs.

Show latest run:

```bash
c last
```

Inspect full details:

```bash
c show 1
```

If your first run ID is not `1`, use ID shown by `last`.

## Record a command that fails

Run:

```bash
c run bash -c 'echo stdout; echo stderr >&2; exit 7'
echo $?
```

The final `echo $?` should print `7`. `carrier run` preserves child exit code.

List failed runs:

```bash
c failed
```

## Search output

Search command text, cwd, stdout, stderr, and terminal logs:

```bash
c search "stderr"
```

## Export a run

Export run details as Markdown:

```bash
c export 1 > run-1.md
```

This is useful when sharing a failure with teammates or adding context to an issue.

## Rerun a command

Run again from original working directory:

```bash
c rerun 1
```

This creates a new run record. It does not overwrite the old run.

## Clean old runs safely

Preview deletion:

```bash
c clean --older-than 30d --dry-run
```

Actually delete:

```bash
c clean --older-than 30d --yes
```

## Next steps

- Read [First run tutorial](first-run.md) for a realistic build/test example.
- Read [Commands](../guide/commands.md) for full command coverage.
- Read [Configuration reference](../reference/configuration.md) before changing storage, redaction, or notifications.
