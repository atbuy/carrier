#!/bin/sh
# Records the upgrade fixtures with old carrier releases.
#
# Usage: unshare --uts tools/record.sh <dir-with-binaries> <out-dir>
#
# <dir-with-binaries> holds carrier-v0.1.0 and carrier-v0.27.0, built from
# their tags. Run as root inside "unshare --uts" so the hostname can be set to
# example-host without changing the machine's own. Data is recorded in a
# throwaway /home/dev with an empty environment, so nothing from the machine
# ends up in the fixtures; review the output before committing it anyway.
set -eu
BIN=$1
OUT=$2
TOOLS=$(cd "$(dirname "$0")" && pwd)

hostname example-host
rm -rf "$OUT"
mkdir -p "$OUT"

run() {
	b=$1
	shift
	(cd /home/dev/api && env -i HOME=/home/dev PATH=/usr/bin:/bin SHELL=/usr/bin/bash LANG=C.UTF-8 CARRIER_COLOR=never "$b" "$@")
}

fresh() {
	rm -rf /home/dev
	mkdir -p /home/dev/api /home/dev/.config/carrier
	# Never capture the environment.
	printf '[storage]\ncapture_env = false\n' >/home/dev/.config/carrier/config.toml
}

save() {
	D=/home/dev/.local/share/carrier
	python3 -I "$TOOLS/sanitize.py" "$D/carrier.db"
	if [ -f "$D/stale-check.timestamp" ]; then
		printf '2025-01-02T15:04:05Z' >"$D/stale-check.timestamp"
	fi
	mkdir -p "$OUT/$1"
	cp -r "$D/." "$OUT/$1/"
	rm -f "$OUT/$1"/carrier.db-*
}

fresh
B=$BIN/carrier-v0.1.0
run "$B" run sh -c 'echo build ok'
run "$B" run sh -c 'echo "error: connection refused" >&2; exit 3' || true
save v0.1.0

fresh
B=$BIN/carrier-v0.27.0
run "$B" run -L deploy sh -c 'echo deployed'
run "$B" run sh -c 'echo "error: connection refused" >&2; exit 3' || true
run "$B" note -r last "the database was restarting"
python3 -I "$TOOLS/shell.py" /home/dev "$B" \
	'carrier shell "BUG-1234: fix login"' \
	'echo hello-from-session' \
	'sh -c "echo error: connection refused >&2; exit 3"' \
	'carrier note "login fails because the database is down"' \
	'exit' 'exit'
save v0.27.0

rm -rf /home/dev
