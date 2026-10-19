#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
bin="$root/bench/bin"
results="$root/bench/results"
scenarios="$root/bench/scenarios"

mkdir -p "$bin" "$results"

cd "$root"
go build -o "$bin/bench-backend" ./bench/backend
go build -o "$bin/bench-loadbalancer" ./bench/loadbalancer
go build -o "$bin/bench-orchestrator" ./bench/orchestrator

for scenario in "$scenarios"/*.json; do
	echo "=== scenario: $(basename "$scenario") ==="
	"$bin/bench-orchestrator" \
		-scenario "$scenario" \
		-backend-bin "$bin/bench-backend" \
		-lb-bin "$bin/bench-loadbalancer" \
		-results "$results" \
		"$@"
done
