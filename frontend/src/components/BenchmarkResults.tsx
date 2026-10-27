import type { BenchmarkResult } from "../api/types";

interface RunRecord {
  id: string;
  result: BenchmarkResult;
}

interface Props {
  runs: RunRecord[];
}

function number(value: number | undefined, digits = 2): string {
  return value == null || !Number.isFinite(value) ? "—" : value.toFixed(digits);
}

export default function BenchmarkResults({ runs }: Props) {
  if (runs.length === 0) {
    return (
      <div className="empty-results">
        <div className="empty-glyph" aria-hidden="true">⌁</div>
        <p className="empty-title">Your benchmark results will appear here</p>
        <p>Configure a run and start your first experiment to begin comparing.</p>
      </div>
    );
  }

  return (
    <div className="table-scroll">
      <table className="results-table">
        <thead>
          <tr>
            <th scope="col">Run / Algorithm</th>
            <th scope="col">Scenario</th>
            <th scope="col">Duration</th>
            <th scope="col">Concurrency</th>
            <th scope="col">Throughput</th>
            <th scope="col">Avg latency</th>
            <th scope="col">P50</th>
            <th scope="col">P95</th>
            <th scope="col">P99</th>
            <th scope="col">Completed</th>
            <th scope="col">Failed</th>
            <th scope="col">Error rate</th>
          </tr>
        </thead>
        <tbody>
          {runs.map(({ id, result }, index) => (
            <tr key={id}>
              <td>
                <span className="run-name">
                  <span className="run-number">{String(index + 1).padStart(2, "0")}</span>
                  {result.algorithm}
                </span>
              </td>
              <td className="muted-cell">{result.scenario || "—"}</td>
              <td>{number(result.duration_seconds, 0)}s</td>
              <td>{result.concurrency ?? "—"}</td>
              <td className="metric-strong">{number(result.throughput_rps)} <small>rps</small></td>
              <td>{number(result.latency?.mean_ms)} <small>ms</small></td>
              <td>{number(result.latency?.p50_ms)} <small>ms</small></td>
              <td>{number(result.latency?.p95_ms)} <small>ms</small></td>
              <td>{number(result.latency?.p99_ms)} <small>ms</small></td>
              <td>{result.completed_requests ?? "—"}</td>
              <td>{result.failed_requests ?? "—"}</td>
              <td>{number(result.error_rate * 100)}<small>%</small></td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="table-footer">
        <span>{runs.length} {runs.length === 1 ? "run" : "runs"} in this session</span>
        <span>Latency values in milliseconds · error rate as percent</span>
      </div>
    </div>
  );
}
