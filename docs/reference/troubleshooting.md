# Troubleshooting

Start with:

```bash title="Run diagnostics"
carrier doctor
```

`doctor` checks config, storage directories, migrations, Git, notification support, shell support, stale runs, and terminal status.

## `command not found`

If `carrier run foo` says:

```text
command not found: foo
```

then `foo` is not on `PATH` for this shell.

Check:

```bash title="Check PATH"
which foo
echo "$PATH"
```

If `carrier` itself is not found after installing with the script, restart your terminal or add the install directory to `PATH`:

```bash title="Linux/macOS PATH"
export PATH="${HOME}/.local/bin:${PATH}"
```

On Windows, the installer updates your user `PATH`. Restart PowerShell after install.

## Notifications do not show

Check:

```bash title="Check notifications"
carrier doctor
which notify-send
```

Notifications are opt-in:

```bash title="Request notifications"
carrier -n run long-command
carrier -N run short-command
```

`-n` respects `notify.min_duration`. `-N` always notifies.

On Linux, install a desktop notification tool that provides `notify-send`. On macOS, `osascript` is expected. On Windows, carrier uses PowerShell.

## Logs are truncated

Increase:

```toml title="Config"
[storage]
max_output_mb = 50
```

Only persisted logs are capped. Terminal output still streams fully.

## Output contains `[REDACTED]`

Persisted logs are redacted. Terminal output is not.

To disable for one run:

```bash title="Disable redaction for one safe run"
carrier --no-redact run safe-command
```

If the redaction is unexpected, check your configured regexes:

```bash title="Validate config"
carrier config check
```

## `clean` refuses to delete

Deletion requires confirmation:

```bash title="Delete with confirmation"
carrier clean --older-than 30d --yes
```

Preview first:

```bash title="Preview first"
carrier clean --older-than 30d --dry-run
```

At least one retention selector is required:

```bash title="Retention selectors"
carrier clean --older-than 30d --dry-run
carrier clean --keep-last 500 --dry-run
```

## Shell mode records unexpected output

`carrier shell` is alpha. Use `carrier run` for precise capture.

Shell mode depends on:

- PTY output
- zsh/bash hooks
- prompt behavior
- shell plugins

## Shell mode shows "(TUI session — terminal output suppressed)"

This message appears when `carrier show` detects that the terminal log contains alternate-screen sequences from a TUI application (neovim, noteui, lazygit, etc.). Replaying TUI output corrupts the terminal, so carrier suppresses it intentionally. The run metadata — command, exit code, duration, cwd, and Git context — is still fully recorded and visible in `carrier show` and `carrier history`.

## Database problems

Check paths:

```bash title="Check paths"
carrier doctor
carrier config show
```

Default DB:

```text
~/.local/share/carrier/carrier.db
```

If you move `storage.data_dir`, make sure the new directory is writable.

## Runs are stuck in `running`

This can happen if a parent `carrier` process is killed. Carrier marks stale running runs as `killed` on startup when they are older than `storage.stale_run_threshold`.

Check:

```bash title="Check stale runs"
carrier doctor
carrier running
```

Tune the threshold:

```toml title="Config"
[storage]
stale_run_threshold = "24h"
```

## Colors look wrong

Disable color:

```bash title="Disable colors"
NO_COLOR=1 carrier history
```

Force color in a pipe:

```bash title="Force colors"
CARRIER_COLOR=always carrier failed | less -R
```

## `rerun --edit` fails

`rerun --edit` needs `EDITOR` or `VISUAL`.

```bash title="Set editor"
export EDITOR=nvim
carrier rerun 42 --edit
```

The edited file must remain a JSON array, for example:

```json
["go", "test", "./..."]
```

## Search misses expected output

Search indexes persisted logs. If output was truncated by `storage.max_output_mb`, text past the cap may not be searchable.

Search command and cwd terms too:

```bash title="Search command and cwd"
carrier search "go test"
carrier search "project-name"
```
