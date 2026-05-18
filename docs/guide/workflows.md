# Workflows

This page shows practical workflows.

## Test loop

```bash title="Record tests"
carrier run go test ./...
carrier last
carrier failed
```

If a test fails:

```bash title="Inspect failure"
carrier show 42
carrier export 42 > failed-tests.md
```

After fixing:

```bash title="Run the same command again"
carrier rerun 42
```

If you need to change package paths or flags before rerunning:

```bash title="Edit before rerun"
carrier rerun 42 --edit
```

## Docker build

```bash title="Notify on long build"
carrier -n run docker compose build
```

`-n` requests a notification if the command runs longer than `notify.min_duration`.

Force notification:

```bash title="Notify regardless of duration"
carrier -N run docker compose build
```

## Long-running service

For precise capture:

```bash title="Record a service command"
carrier run npm run dev
```

From another terminal:

```bash title="Inspect active command"
carrier running
carrier tail 42
```

For interactive terminal sessions, try alpha shell mode:

```bash title="Start tracked shell"
carrier shell
```

!!! warning

    Use `carrier run` for reliable stdout/stderr separation. Shell mode is a convenience layer around PTY output and shell hooks.

## Shell session workflow

Use a labeled session to group related exploratory work:

```bash title="Start a named session"
carrier shell 'incident-123'
```

Inside the shell, all commands are recorded under that session. Check the current session label anytime:

```bash title="Check sessions"
carrier session list
```

If you need to return to the session from a different terminal, use `attach`:

```bash title="Re-join a session"
carrier attach incident-123
```

Review the full session tree in history:

```bash title="Review session history"
carrier history --session incident-123
```

See only session headers across all history:

```bash title="Overview of all sessions"
carrier history --sessions-only
```

Label a session retroactively (from outside the shell):

```bash title="Label by ID"
carrier session label 5 incident-123
```

## Failure report

```bash title="Create a report"
carrier failed
carrier show 42
carrier export 42 > run-42.md
```

Attach `run-42.md` to a ticket, pull request, or chat thread.

## Label important runs

Use labels when a run represents something worth finding later:

```bash title="Label a deployment"
carrier label 42 prod deploy
carrier history --label prod
```

Clear the label:

```bash title="Clear label"
carrier label 42
```

## Search old output

```bash title="Search"
carrier search "connection refused"
carrier search "permission denied"
carrier search "timeout"
```

Use a smaller result set when piping to other tools:

```bash title="Search JSON"
carrier search --limit 20 --json "timeout" | jq '.[].id'
```

## Review command health

```bash title="Review stats"
carrier stats
carrier stats --slowest 10
```

Use this to check total runs, failure rate, runs per active day, and slow commands.

## Watch a project

Re-run a command when files change:

```bash title="Watch Go files"
carrier watch --pattern '*.go' go test ./...
```

Use a longer debounce for tools that write many files:

```bash title="Debounce noisy writes"
carrier watch --debounce 750ms make test
```

`watch` runs once immediately, then runs again after matching changes.

## Keep storage clean

Preview cleanup:

```bash title="Preview age cleanup"
carrier clean --older-than 30d --dry-run
```

Delete:

```bash title="Delete old records"
carrier clean --older-than 30d --yes
```

Keep only the latest records:

```bash title="Count-based retention"
carrier clean --keep-last 500 --dry-run
carrier clean --keep-last 500 --yes
```

## Script against carrier

Use JSON:

```bash title="JSON entry points"
carrier last --json
carrier show 42 --json
carrier running --json
carrier history --json
carrier search --json "panic"
carrier stats --json
```

Example:

```bash title="Read an exit code"
carrier show 42 --json | jq -r '.exit_code'
```

Use captured log paths directly:

```bash title="Open stderr log"
less "$(carrier show 42 --json | jq -r '.stderr_path')"
```

## Capture and inspect environment

Environment capture is enabled by default with `storage.capture_env = true`.

```bash title="Show captured environment"
carrier show 42 --env
carrier show 42 --json | jq '.env'
```

Values are redacted when displayed unless you pass `--no-redact`.
