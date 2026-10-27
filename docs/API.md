# Benchmark API

Start from the repository root:

```sh
go run ./cmd/api
```

By default the API listens on `127.0.0.1:8082`, reads scenarios from
`bench/scenarios`, and allows CORS from `http://localhost:5173`. Configure these
with `-addr`, `-scenario-dir`, and `-cors-origin`.

The API runs the existing time-based workload runner. It does not accept a
request-count limit or store results.

## Discover options

### `GET /api/algorithms`

Returns the implemented routing algorithms:

```sh
curl http://127.0.0.1:8082/api/algorithms
```

```json
{
  "algorithms": [
    {"id":"round-robin","name":"Round Robin"},
    {"id":"weighted-round-robin","name":"Weighted Round Robin"},
    {"id":"least-connections","name":"Least Connections"},
    {"id":"adaptive","name":"Adaptive"}
  ]
}
```

### `GET /api/scenarios`

Returns IDs, names, and descriptions loaded from the JSON files in the scenario
directory. A scenario ID is included with the run as its label; backends and
load settings are supplied explicitly in the benchmark request.

```sh
curl http://127.0.0.1:8082/api/scenarios
```

## Run a benchmark

### `POST /api/benchmarks`

Each configured server must be reachable and return `200` from its `/health`
endpoint. Adaptive routing also consumes the existing CPU metric endpoint when
available.

Request:

```json
{
  "algorithm": "adaptive",
  "scenario": "baseline",
  "servers": [
    {"id": "backend-1", "url": "http://127.0.0.1:18081", "weight": 3},
    {"id": "backend-2", "url": "http://127.0.0.1:18082", "weight": 1}
  ],
  "duration_seconds": 10,
  "warmup_seconds": 2,
  "concurrency": 32,
  "rate_per_second": 0
}
```

Example:

```sh
curl -X POST http://127.0.0.1:8082/api/benchmarks \
  -H 'Content-Type: application/json' \
  -d '{"algorithm":"adaptive","scenario":"baseline","servers":[{"id":"backend-1","url":"http://127.0.0.1:18081","weight":3}],"duration_seconds":10,"warmup_seconds":2,"concurrency":32,"rate_per_second":0}'
```

`rate_per_second: 0` uses the runner's closed-loop workload; a positive value
uses its open-loop workload. Each request is a `GET /` through the selected
backends.

Response metrics come from `workload.Result`. `completed_requests` is total
requests minus failures. Throughput is total measured requests divided by the
runner's measured elapsed time.

```json
{
  "algorithm": "adaptive",
  "scenario": "baseline",
  "duration_seconds": 10,
  "concurrency": 32,
  "rate_per_second": 0,
  "total_requests": 998,
  "completed_requests": 998,
  "failed_requests": 0,
  "throughput_rps": 99.8,
  "error_rate": 0,
  "latency": {
    "count": 998,
    "mean_ms": 3.2,
    "p50_ms": 3.1,
    "p95_ms": 4.8,
    "p99_ms": 6.2,
    "min_ms": 1.0,
    "max_ms": 8.0
  },
  "server_distribution": {"backend-1": 698, "backend-2": 300},
  "windows": []
}
```

The values above are illustrative. `windows` contains the workload runner's
per-second request, error, latency, backend, and available CPU measurements.

## Compare results

### `POST /api/benchmarks/compare`

Send completed results from the frontend; the API keeps no result history.
Metrics may be omitted when unavailable. `error_rate` is a fraction from `0` to
`1`. Percentage improvement is measured against the first result: higher
throughput is better; lower latency and error rate are better. The percentage
is omitted when the reference metric is zero or missing.

```json
{
  "results": [
    {
      "algorithm": "round-robin",
      "throughput_rps": 1820,
      "avg_latency_ms": 5.4,
      "p95_latency_ms": 8.2,
      "p99_latency_ms": 12.1,
      "error_rate": 0
    },
    {
      "algorithm": "adaptive",
      "throughput_rps": 2310,
      "avg_latency_ms": 4.2,
      "p95_latency_ms": 6.1,
      "p99_latency_ms": 9.2,
      "error_rate": 0
    }
  ]
}
```

```sh
curl -X POST http://127.0.0.1:8082/api/benchmarks/compare \
  -H 'Content-Type: application/json' \
  -d '{"results":[{"algorithm":"round-robin","throughput_rps":1820,"avg_latency_ms":5.4},{"algorithm":"adaptive","throughput_rps":2310,"avg_latency_ms":4.2}]}'
```

Example response:

```json
{
  "reference_algorithm": "round-robin",
  "best_throughput": {
    "algorithm": "adaptive",
    "value": 2310,
    "improvement_percent": 26.92
  },
  "lowest_avg_latency": {
    "algorithm": "adaptive",
    "value": 4.2,
    "improvement_percent": 22.22
  },
  "lowest_p95_latency": {
    "algorithm": "adaptive",
    "value": 6.1,
    "improvement_percent": 25.61
  },
  "lowest_p99_latency": {
    "algorithm": "adaptive",
    "value": 9.2,
    "improvement_percent": 23.97
  },
  "lowest_error_rate": {
    "algorithm": "round-robin",
    "value": 0
  }
}
```

The response identifies the best throughput and lowest average/P95/P99 latency
and error rate, with improvement percentages where calculable. There is no
combined or weighted overall score.

Malformed input returns a JSON object with `error` and `message`. Invalid
configuration returns `400`; benchmark backend/execution failures return
`502`.
