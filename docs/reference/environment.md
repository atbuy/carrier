# Environment variables

This page lists environment variables that affect `carrier`.

## User-facing variables

| Variable          | Used by                   | Meaning                                                                                         |
| ----------------- | ------------------------- | ----------------------------------------------------------------------------------------------- |
| `CARRIER_COLOR`   | all human-readable output | Set `always`, `1`, or `true` to force colors; `never`, `0`, or `false` to disable.              |
| `NO_COLOR`        | all human-readable output | Any non-empty value disables colors.                                                            |
| `TERM`            | all human-readable output | `TERM=dumb` disables colors.                                                                    |
| `XDG_CONFIG_HOME` | config loading            | Changes config path to `$XDG_CONFIG_HOME/carrier/config.toml`.                                  |
| `HOME`            | config and default paths  | Used for `~` expansion and default XDG-style locations.                                         |
| `SHELL`           | `run`, `rerun`, `shell`   | Used for metadata, shell fallback, and default shell mode program.                              |
| `EDITOR`          | `rerun --edit`            | Editor used to modify argv JSON before rerun.                                                   |
| `VISUAL`          | `rerun --edit`            | Fallback editor if `EDITOR` is not set.                                                         |

!!! tip "Color examples"

    ```bash title="Disable colors"
    NO_COLOR=1 carrier history
    ```

    ```bash title="Force colors through a pipe"
    CARRIER_COLOR=always carrier failed | less -R
    ```

## Captured command environment

By default, `carrier run` stores the command environment as JSON metadata:

```toml title="Config"
[storage]
capture_env = true
```

Inspect captured environment:

```bash title="Show environment"
carrier show 42 --env
carrier show 42 --json | jq '.env'
```

Values are redacted when displayed unless you pass `--no-redact`.

!!! warning

    Environment variables often contain credentials. Keep redaction enabled when environment capture is enabled.

Disable capture:

```toml title="Disable environment capture"
[storage]
capture_env = false
```

## Internal shell-mode variables

`carrier shell` uses internal environment variables to coordinate generated shell hooks and child `carrier internal ...` commands:

| Variable                 | Purpose                                               |
| ------------------------ | ----------------------------------------------------- |
| `CARRIER_SHELL_STATE`    | Path to temporary shell state JSON.                   |
| `CARRIER_NOTIFY`         | Passes notification request into shell hook runs.     |
| `CARRIER_NOTIFY_ALWAYS`  | Passes always-notify request into shell hook runs.    |
| `CARRIER_NO_REDACT`      | Passes redaction override into shell hook runs.       |

These variables are implementation details. Do not set them manually unless you are debugging shell mode internals.
