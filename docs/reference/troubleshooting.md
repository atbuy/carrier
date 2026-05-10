# Troubleshooting

Start with:

```bash
carrier doctor
```

## `command not found`

If `carrier run foo` says:

```text
command not found: foo
```

then `foo` is not on `PATH` for this shell.

Check:

```bash
which foo
echo "$PATH"
```

## Notifications do not show

Check:

```bash
carrier doctor
which notify-send
```

Notifications are opt-in:

```bash
carrier -n run long-command
carrier -N run short-command
```

`-n` respects `notify.min_duration`. `-N` always notifies.

## Logs are truncated

Increase:

```toml
[storage]
max_output_mb = 50
```

Only persisted logs are capped. Terminal output still streams fully.

## Output contains `[REDACTED]`

Persisted logs are redacted. Terminal output is not.

To disable for one run:

```bash
carrier --no-redact run safe-command
```

## `clean` refuses to delete

Deletion requires confirmation:

```bash
carrier clean --older-than 30d --yes
```

Preview first:

```bash
carrier clean --older-than 30d --dry-run
```

## Shell mode records unexpected output

`carrier shell` is alpha. Use `carrier run` for precise capture.

Shell mode depends on:

- PTY output
- zsh/bash hooks
- prompt behavior
- shell plugins

## Database problems

Check paths:

```bash
carrier doctor
carrier config show
```

Default DB:

```text
~/.local/share/carrier/carrier.db
```

If you move `storage.data_dir`, make sure the new directory is writable.
