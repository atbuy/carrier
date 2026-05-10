# Shell mode

`carrier shell` is alpha-quality.

It starts your shell inside a PTY and injects shell hooks so `carrier` can detect command start and end.

```bash title="Start shell mode"
carrier shell
```

!!! warning

    Shell mode is useful for experiments and interactive work. Use `carrier run` for precise stdout/stderr capture, reliable exit-code handling, and CI-like command records.

## When to use it

Use shell mode when:

- you want to try multi-command tracking
- commands are interactive
- stdout/stderr separation is not important
- convenience matters more than precision

Use `carrier run` when:

- exit code preservation is critical
- stdout and stderr must be separate
- you need reliable output capture
- you are recording CI-like commands

## Supported shells

Shell mode is best-effort for:

- zsh
- bash

`carrier doctor` reports whether the configured shell looks supported.

## How it works

Architecture:

```text
terminal/tmux
  -> carrier PTY recorder
    -> zsh/bash
      -> user commands
```

For zsh, `carrier` uses `preexec` and `precmd` hooks.

After sourcing your `.zshrc`, `carrier` disables zsh's `warn_create_global` option inside the tracked shell. This avoids prompt helper warnings from being printed on every command when prompt functions assign globals.

For bash, `carrier` starts bash with a generated `--rcfile`; support is best-effort through `DEBUG` and `PROMPT_COMMAND` hooks. Existing `PROMPT_COMMAND` content is preserved and run before `carrier` finishes the tracked command.

`carrier` guards its hook internals so internal `carrier internal ...` commands and `_carrier_*` helper functions are not recorded as user commands.

## What gets recorded

Shell mode creates one run per detected command. Shell runs use:

- mode: `shell`
- status: `running`, `success`, or `failed`
- command text from the shell hook
- cwd
- Git metadata
- terminal output log path

Unlike `carrier run`, shell mode stores terminal output instead of separate stdout and stderr logs.

## Limitations

- stdout and stderr may be merged
- prompts and shell plugins can affect detection
- aliases may change displayed commands
- interactive programs may produce noisy terminal logs
- hook internals may behave differently across shell versions
- bash hook behavior can vary when other tools also modify `DEBUG` or `PROMPT_COMMAND`
- `carrier shell` is not a drop-in replacement for shell history

## Inspect shell runs

From another terminal:

```bash title="Inspect shell runs"
carrier running
carrier tail 42
carrier show 42
```

Shell runs use `terminal_output_path` instead of separate stdout/stderr paths.

Show only terminal output:

```bash title="Tail terminal stream"
carrier tail 42 --stream terminal
```

## Ignore noisy commands

Configure commands that shell mode should ignore:

```toml title="Config"
[shell]
ignore_commands = ["nvim", "vim", "less", "man", "fzf", "yazi", "lazygit", "tmux"]
```

Interactive full-screen programs are ignored by default because their terminal output is usually noisy.

## Choose a shell

Leave `program` empty to use `$SHELL`:

```toml title="Use $SHELL"
[shell]
program = ""
```

Set a shell explicitly:

```toml title="Explicit shell"
[shell]
program = "/bin/zsh"
```

## Current recommendation

Use `carrier run` for important records. Treat `carrier shell` as an experimental convenience layer.
