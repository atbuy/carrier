# Upgrade fixtures

Each directory is a carrier data directory (`~/.local/share/carrier`) recorded by a real old release. `upgrade_test.go` copies one into a fresh home and checks that the current build upgrades it and still shows everything.

The fixtures hold only made-up data: the home is `/home/dev`, the hostname `example-host`, no environment is captured, and every timestamp is shifted to start at `2025-01-02 15:04:05 UTC`. Never commit data recorded on a real machine or in a real shell.

| Directory | Schema                      | Contains                                                                                       |
| --------- | --------------------------- | ---------------------------------------------------------------------------------------------- |
| `v0.1.0`  | before versioned migrations | a successful run and a failed run (exit 3)                                                     |
| `v0.27.0` | migration 7                 | a labeled run, a failed run with a note, and a labeled shell session with three runs and a note |

## Recording the fixtures

`tools/record.sh` records both directories:

1. Build each old release from its tag:

    ```bash
    for v in v0.1.0 v0.27.0; do
      git worktree add --detach /tmp/carrier-$v $v
      (cd /tmp/carrier-$v && go build -o /tmp/carrier-bin/carrier-$v ./cmd/carrier)
    done
    ```

2. Record, as root, in a separate UTS namespace so the hostname can be faked:

    ```bash
    sudo unshare --uts cmd/carrier/testdata/upgrade/tools/record.sh /tmp/carrier-bin /tmp/fixtures
    ```

    The script records in a throwaway `/home/dev` with an empty environment and `capture_env = false`. It runs shell sessions through `tools/shell.py`, a bash in a pseudo-terminal with no rc files. `tools/sanitize.py` then shifts the timestamps, merges the search index so deleted runs leave nothing behind, and compacts the database so no old values remain in free space.

3. Review everything before copying it here: every table, the log files, and the raw database bytes. Look for hostnames, paths, environment variables, and anything else from the machine.

To add a release, record it the same way and add a test for it in `upgrade_test.go`.
