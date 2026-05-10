# First run tutorial

This tutorial uses a realistic test command and shows the full loop: record, inspect, search, export, rerun, and clean.

## Create a sample command

From any project directory, run:

```bash title="Record a failing command"
carrier run bash -c 'echo "build started"; echo "warning: example" >&2; sleep 1; exit 1'
```

The command exits with `1`, and `carrier` exits with `1` too.

!!! tip

    That exit-code behavior means `carrier run` can wrap commands in scripts without hiding failures.

## Inspect latest run

```bash title="Find the run ID"
carrier last
```

Output includes:

```text
ID:       42
Status:   failed
Command:  bash -c 'echo "build started"; echo "warning: example" >&2; sleep 1; exit 1'
CWD:      /home/alice/project
Exit:     1
Duration: 1s
```

## Show output

```bash title="Show metadata plus logs"
carrier show 42
```

For `carrier run`, stdout and stderr are shown separately.

## Tail output

Tail both streams with labels:

```bash title="Tail stdout and stderr"
carrier tail 42
```

Tail one stream:

```bash title="Tail one stream"
carrier tail 42 --stream stdout
carrier tail 42 --stream stderr
```

## Search logs

```bash title="Search stored output"
carrier search "warning"
```

Search checks command text, cwd, and stored output logs.

## Export report

```bash title="Create a Markdown report"
carrier export 42 > failed-run.md
```

The report is Markdown, with metadata plus output blocks.

## Rerun from original directory

```bash title="Rerun original argv"
carrier rerun 42
```

Rerun creates a new run record.

If you want to edit the command first:

```bash title="Edit before rerun"
carrier rerun 42 --edit
```

## Try JSON output

```bash title="JSON output"
carrier show 42 --json
carrier last --json
carrier running --json
```

Use JSON when integrating with scripts.

## Clean when done

Preview:

```bash title="Preview cleanup"
carrier clean --older-than 30d --dry-run
```

Delete:

```bash title="Delete old records and logs"
carrier clean --older-than 30d --yes
```
