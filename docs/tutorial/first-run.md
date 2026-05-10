# First run tutorial

This tutorial uses a realistic test command and shows the full loop: record, inspect, search, export, rerun, and clean.

## Create a sample command

From any project directory, run:

```bash
carrier run bash -c 'echo "build started"; echo "warning: example" >&2; sleep 1; exit 1'
```

The command exits with `1`, and `carrier` exits with `1` too.

## Inspect latest run

```bash
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

```bash
carrier show 42
```

For `carrier run`, stdout and stderr are shown separately.

## Tail output

Tail both streams with labels:

```bash
carrier tail 42
```

Tail one stream:

```bash
carrier tail 42 --stream stdout
carrier tail 42 --stream stderr
```

## Search logs

```bash
carrier search "warning"
```

Search checks command text, cwd, and stored output logs.

## Export report

```bash
carrier export 42 > failed-run.md
```

The report is Markdown, with metadata plus output blocks.

## Rerun from original directory

```bash
carrier rerun 42
```

Rerun creates a new run record.

## Try JSON output

```bash
carrier show 42 --json
carrier last --json
carrier running --json
```

Use JSON when integrating with scripts.

## Clean when done

Preview:

```bash
carrier clean --older-than 30d --dry-run
```

Delete:

```bash
carrier clean --older-than 30d --yes
```
