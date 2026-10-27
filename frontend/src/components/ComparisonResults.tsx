import type { ComparisonResponse, MetricWinner } from "../api/types";

interface Props {
  comparison: ComparisonResponse | null;
  isComparing: boolean;
  error: string | null;
  onCompare: () => void;
  disabled: boolean;
}

const metrics: {
  key: keyof Omit<ComparisonResponse, "reference_algorithm">;
  title: string;
  unit: string;
  better: "higher" | "lower";
}[] = [
  { key: "best_throughput", title: "Best throughput", unit: "rps", better: "higher" },
  { key: "lowest_avg_latency", title: "Lowest avg latency", unit: "ms", better: "lower" },
  { key: "lowest_p95_latency", title: "Lowest P95 latency", unit: "ms", better: "lower" },
  { key: "lowest_p99_latency", title: "Lowest P99 latency", unit: "ms", better: "lower" },
  { key: "lowest_error_rate", title: "Lowest error rate", unit: "%", better: "lower" },
];

function renderValue(key: string, winner: MetricWinner, unit: string) {
  const value = key === "lowest_error_rate"
    ? `${(winner.value * 100).toFixed(2)}${unit}`
    : `${winner.value.toFixed(2)} ${unit}`;
  return value;
}

function improvementLabel(metric: (typeof metrics)[number], winner: MetricWinner) {
  if (winner.improvement_percent == null || !Number.isFinite(winner.improvement_percent)) {
    return null;
  }
  const percentage = Math.abs(winner.improvement_percent).toFixed(1);
  const sign =
    winner.improvement_percent === 0
      ? ""
      : winner.improvement_percent > 0
        ? "+"
        : "−";
  const descriptor = metric.better === "higher" ? "throughput" : "vs reference";
  return `${sign}${percentage}% ${descriptor}`;
}

export default function ComparisonResults({
  comparison,
  isComparing,
  error,
  onCompare,
  disabled,
}: Props) {
  return (
    <section className="comparison-section" aria-labelledby="comparison-heading">
      <div className="results-heading comparison-heading">
        <div>
          <span className="panel-kicker">CROSS-RUN ANALYSIS</span>
          <h2 id="comparison-heading">Compare algorithms</h2>
          <p>Metric winners are evaluated against the first run in the session.</p>
        </div>
        <button
          className="button button-secondary"
          type="button"
          onClick={onCompare}
          disabled={disabled || isComparing}
        >
          {isComparing ? (
            <>
              <span className="spinner spinner-dark" aria-hidden="true" />
              Comparing…
            </>
          ) : (
            "Compare Algorithms"
          )}
        </button>
      </div>
      {error && <div className="inline-error" role="alert">{error}</div>}
      {comparison ? (
        <>
          <div className="reference-note">
            Reference run: <strong>{comparison.reference_algorithm}</strong>
          </div>
          <div className="comparison-grid">
            {metrics.map((metric) => {
              const winner = comparison[metric.key];
              return (
                <article className="comparison-metric" key={metric.key}>
                  <span className="comparison-label">{metric.title}</span>
                  {winner ? (
                    <>
                      <strong className="comparison-winner">{winner.algorithm}</strong>
                      <span className="comparison-value">
                        {renderValue(metric.key, winner, metric.unit)}
                      </span>
                      {improvementLabel(metric, winner) && (
                        <span className="comparison-delta">
                          {improvementLabel(metric, winner)}
                        </span>
                      )}
                    </>
                  ) : (
                    <span className="comparison-unavailable">Metric unavailable</span>
                  )}
                </article>
              );
            })}
          </div>
          <p className="comparison-disclaimer">
            There is no combined score. Each metric has its own winner; percentages
            compare with the first result.
          </p>
        </>
      ) : (
        <div className="comparison-empty">
          Add at least two benchmark runs, then compare their measured metrics.
        </div>
      )}
    </section>
  );
}
