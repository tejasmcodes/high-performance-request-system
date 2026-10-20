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

exec "$bin/bench-orchestrator" \
	-scenario "$scenarios/baseline.json" \
	-backend-bin "$bin/bench-backend" \
	-lb-bin "$bin/bench-loadbalancer" \
	-results "$results" \
	-duration 3 \
	-warmup 2 \
	-concurrency 8 \
	-repeats 1 \
	-cooldown 1s \
	"$@"
