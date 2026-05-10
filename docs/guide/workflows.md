# Workflows

This page shows practical workflows.

## Test loop

```bash
carrier run go test ./...
carrier last
carrier failed
```

If a test fails:

```bash
carrier show 42
carrier export 42 > failed-tests.md
```

After fixing:

```bash
carrier rerun 42
```

## Docker build

```bash
carrier -n run docker compose build
```

`-n` requests a notification if the command runs longer than `notify.min_duration`.

Force notification:

```bash
carrier -N run docker compose build
```

## Long-running service

For precise capture:

```bash
carrier run npm run dev
```

From another terminal:

```bash
carrier running
carrier tail 42
```

For interactive terminal sessions, try alpha shell mode:

```bash
carrier shell
```

## Failure report

```bash
carrier failed
carrier show 42
carrier export 42 > run-42.md
```

Attach `run-42.md` to a ticket, pull request, or chat thread.

## Search old output

```bash
carrier search "connection refused"
carrier search "permission denied"
carrier search "timeout"
```

## Keep storage clean

Preview cleanup:

```bash
carrier clean --older-than 30d --dry-run
```

Delete:

```bash
carrier clean --older-than 30d --yes
```

## Script against carrier

Use JSON:

```bash
carrier last --json
carrier show 42 --json
carrier running --json
```

Example:

```bash
carrier show 42 --json | jq -r '.exit_code'
```
