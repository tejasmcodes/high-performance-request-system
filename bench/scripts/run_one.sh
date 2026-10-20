#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bin="$root/bench/bin"
results="$root/bench/results"

if [[ $# -lt 1 ]]; then
	echo "usage: $0 <scenario.json> [orchestrator flags...]" >&2
	exit 2
fi

scenario="$1"
shift

mkdir -p "$bin" "$results"

cd "$root"
go build -o "$bin/bench-backend" ./bench/backend
go build -o "$bin/bench-loadbalancer" ./bench/loadbalancer
go build -o "$bin/bench-orchestrator" ./bench/orchestrator

exec "$bin/bench-orchestrator" \
	-scenario "$scenario" \
	-backend-bin "$bin/bench-backend" \
	-lb-bin "$bin/bench-loadbalancer" \
	-results "$results" \
	"$@"
