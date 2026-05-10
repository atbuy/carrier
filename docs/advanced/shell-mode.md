# Shell mode

`carrier shell` is alpha-quality.

It starts your shell inside a PTY and injects shell hooks so `carrier` can detect command start and end.

```bash
carrier shell
```

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

## How it works

Architecture:

```text
terminal/tmux
  -> carrier PTY recorder
    -> zsh/bash
      -> user commands
```

For zsh, `carrier` uses `preexec` and `precmd` hooks.

For bash, support is best-effort through shell hooks.

## Limitations

- stdout and stderr may be merged
- prompts and shell plugins can affect detection
- aliases may change displayed commands
- interactive programs may produce noisy terminal logs
- hook internals may behave differently across shell versions

## Inspect shell runs

From another terminal:

```bash
carrier running
carrier tail 42
carrier show 42
```

Shell runs use `terminal_output_path` instead of separate stdout/stderr paths.

## Current recommendation

Use `carrier run` for important records. Treat `carrier shell` as an experimental convenience layer.
