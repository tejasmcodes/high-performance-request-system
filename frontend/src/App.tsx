import { useCallback, useEffect, useMemo, useState } from "react";
import {
  API_BASE_URL,
  compareBenchmarks,
  getAlgorithms,
  getScenarios,
  runBenchmark,
} from "./api/client";
import type {
  Algorithm,
  BenchmarkRequest,
  BenchmarkResult,
  ComparisonResponse,
  ComparisonResultInput,
  Scenario,
  ServerConfig,
} from "./api/types";
import BenchmarkResults from "./components/BenchmarkResults";
import ComparisonResults from "./components/ComparisonResults";
import ConfigurationPanel, {
  type WorkloadValues,
} from "./components/ConfigurationPanel";

interface RunRecord {
  id: string;
  result: BenchmarkResult;
}

const initialServers: ServerConfig[] = [
  { id: "backend-1", url: "http://127.0.0.1:18081", weight: 3 },
  { id: "backend-2", url: "http://127.0.0.1:18082", weight: 1 },
];

const initialWorkload: WorkloadValues = {
  duration_seconds: 10,
  warmup_seconds: 2,
  concurrency: 32,
  rate_per_second: 0,
};

function validateConfiguration(
  algorithm: string,
  scenario: string,
  servers: ServerConfig[],
  workload: WorkloadValues,
): string[] {
  const errors: string[] = [];
  if (!algorithm) errors.push("Choose an algorithm.");
  if (!scenario) errors.push("Choose a scenario label.");
  if (servers.length === 0) errors.push("Add at least one backend server.");

  const ids = new Set<string>();
  servers.forEach((server, index) => {
    const label = `Server ${index + 1}`;
    const id = server.id.trim();
    if (!id) errors.push(`${label}: enter a server ID.`);
    else if (ids.has(id)) errors.push(`${label}: server IDs must be unique.`);
    ids.add(id);

    try {
      const url = new URL(server.url);
      if (!["http:", "https:"].includes(url.protocol) || !url.hostname) {
        errors.push(`${label}: enter a valid HTTP or HTTPS URL.`);
      }
    } catch {
      errors.push(`${label}: enter a valid HTTP or HTTPS URL.`);
    }

    if (!Number.isInteger(server.weight) || server.weight <= 0) {
      errors.push(`${label}: weight must be a positive whole number.`);
    }
  });

  if (!Number.isInteger(workload.duration_seconds) || workload.duration_seconds <= 0) {
    errors.push("Duration must be a positive whole number of seconds.");
  }
  if (!Number.isInteger(workload.warmup_seconds) || workload.warmup_seconds < 0) {
    errors.push("Warmup must be a non-negative whole number of seconds.");
  }
  if (!Number.isInteger(workload.concurrency) || workload.concurrency <= 0) {
    errors.push("Concurrency must be a positive whole number.");
  }
  if (!Number.isFinite(workload.rate_per_second) || workload.rate_per_second < 0) {
    errors.push("Target rate must be a non-negative number.");
  }
  return errors;
}

export default function App() {
  const [algorithms, setAlgorithms] = useState<Algorithm[]>([]);
  const [scenarios, setScenarios] = useState<Scenario[]>([]);
  const [algorithm, setAlgorithm] = useState("");
  const [scenario, setScenario] = useState("");
  const [servers, setServers] = useState(initialServers);
  const [workload, setWorkload] = useState(initialWorkload);
  const [runs, setRuns] = useState<RunRecord[]>([]);
  const [comparison, setComparison] = useState<ComparisonResponse | null>(null);
  const [optionsLoading, setOptionsLoading] = useState(true);
  const [optionsError, setOptionsError] = useState<string | null>(null);
  const [runError, setRunError] = useState<string | null>(null);
  const [comparisonError, setComparisonError] = useState<string | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [isComparing, setIsComparing] = useState(false);

  const loadOptions = useCallback(async (signal?: AbortSignal) => {
    setOptionsLoading(true);
    setOptionsError(null);
    try {
      const [algorithmOptions, scenarioOptions] = await Promise.all([
        getAlgorithms(),
        getScenarios(),
      ]);
      if (signal?.aborted) return;
      setAlgorithms(algorithmOptions);
      setScenarios(scenarioOptions);
      setAlgorithm((current) =>
        algorithmOptions.some((item) => item.id === current)
          ? current
          : algorithmOptions[0]?.id ?? "",
      );
      setScenario((current) =>
        scenarioOptions.some((item) => item.id === current)
          ? current
          : scenarioOptions[0]?.id ?? "",
      );
    } catch (error) {
      if (!signal?.aborted) {
        setOptionsError(
          error instanceof Error ? error.message : "Could not load API options.",
        );
      }
    } finally {
      if (!signal?.aborted) setOptionsLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void loadOptions(controller.signal);
    return () => controller.abort();
  }, [loadOptions]);

  const validationMessages = useMemo(
    () => validateConfiguration(algorithm, scenario, servers, workload),
    [algorithm, scenario, servers, workload],
  );
  const hasOptions = algorithms.length > 0 && scenarios.length > 0;
  const controlsDisabled = isRunning || isComparing || optionsLoading;

  async function handleRun() {
    if (validationMessages.length > 0 || !hasOptions || isRunning) return;
    setRunError(null);
    setComparison(null);
    setComparisonError(null);
    setIsRunning(true);
    const request: BenchmarkRequest = {
      algorithm,
      scenario,
      servers: servers.map((server) => ({
        ...server,
        id: server.id.trim(),
        url: server.url.trim(),
      })),
      ...workload,
    };

    try {
      const result = await runBenchmark(request);
      setRuns((current) => [...current, { id: crypto.randomUUID(), result }]);
    } catch (error) {
      setRunError(error instanceof Error ? error.message : "Benchmark failed.");
    } finally {
      setIsRunning(false);
    }
  }

  async function handleCompare() {
    if (runs.length < 2 || isComparing || isRunning) return;
    setComparison(null);
    setComparisonError(null);
    setIsComparing(true);
    const results: ComparisonResultInput[] = runs.map(({ result }) => ({
      algorithm: result.algorithm,
      throughput_rps: result.throughput_rps,
      avg_latency_ms: result.latency.mean_ms,
      p95_latency_ms: result.latency.p95_ms,
      p99_latency_ms: result.latency.p99_ms,
      error_rate: result.error_rate,
    }));

    try {
      setComparison(await compareBenchmarks(results));
    } catch (error) {
      setComparisonError(
        error instanceof Error ? error.message : "Comparison failed.",
      );
    } finally {
      setIsComparing(false);
    }
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <a className="brand" href="/" aria-label="Adaptive Load Balancer console">
          <span className="brand-mark" aria-hidden="true">
            <span />
            <span />
            <span />
          </span>
          <span className="brand-copy">
            <strong>Adaptive Load Balancer</strong>
            <span>BENCHMARKING CONSOLE</span>
          </span>
        </a>
        <div className="topbar-right">
          <span className="environment-tag">LOCAL ENVIRONMENT</span>
          <div
            className={`api-status ${optionsError ? "api-status-offline" : optionsLoading ? "api-status-pending" : "api-status-online"}`}
            title={`API: ${API_BASE_URL}`}
          >
            <span className="status-dot" />
            {optionsLoading ? "Connecting" : optionsError ? "API unavailable" : "API connected"}
          </div>
        </div>
      </header>

      <main className="page-content">
        <section className="page-intro">
          <div>
            <div className="eyebrow"><span /> BENCHMARK WORKSPACE</div>
            <h1>Benchmarking Console</h1>
            <p>
              Configure a workload, measure routing performance, and compare
              algorithms side by side.
            </p>
          </div>
          <div className="session-indicator">
            <span className="session-indicator-line" />
            <span>
              <strong>{runs.length.toString().padStart(2, "0")}</strong>
              <small>RUNS THIS SESSION</small>
            </span>
          </div>
        </section>

        {optionsError && (
          <div className="banner-error" role="alert">
            <div>
              <strong>Unable to connect to the benchmark API</strong>
              <p>{optionsError}</p>
              <span>Confirm the API is running at {API_BASE_URL}.</span>
            </div>
            <button
              className="button button-small button-quiet"
              type="button"
              onClick={() => void loadOptions()}
              disabled={optionsLoading || isRunning}
            >
              Retry
            </button>
          </div>
        )}

        {runError && (
          <div className="banner-error" role="alert">
            <div>
              <strong>Benchmark could not be completed</strong>
              <p>{runError}</p>
            </div>
            <button
              className="icon-button"
              type="button"
              aria-label="Dismiss benchmark error"
              onClick={() => setRunError(null)}
            >
              ×
            </button>
          </div>
        )}

        <div className="workspace-grid">
          <ConfigurationPanel
            algorithms={algorithms}
            scenarios={scenarios}
            algorithm={algorithm}
            scenario={scenario}
            servers={servers}
            workload={workload}
            disabled={controlsDisabled}
            canRun={hasOptions && validationMessages.length === 0}
            validationMessages={hasOptions ? validationMessages : []}
            isRunning={isRunning}
            onAlgorithmChange={setAlgorithm}
            onScenarioChange={setScenario}
            onServersChange={setServers}
            onWorkloadChange={setWorkload}
            onRun={() => void handleRun()}
          />

          <section className="results-panel" aria-labelledby="results-heading">
            <div className="results-heading">
              <div>
                <span className="panel-kicker">MEASURED OUTPUT</span>
                <h2 id="results-heading">Benchmark results</h2>
                <p>Run another algorithm to build a side-by-side comparison.</p>
              </div>
              {isRunning && (
                <div className="running-indicator" role="status" aria-live="polite">
                  <span className="spinner spinner-dark" />
                  Running benchmark…
                </div>
              )}
            </div>
            <BenchmarkResults runs={runs} />

            <ComparisonResults
              comparison={comparison}
              isComparing={isComparing}
              error={comparisonError}
              onCompare={() => void handleCompare()}
              disabled={runs.length < 2 || isRunning || isComparing}
            />
          </section>
        </div>

        <footer className="page-footer">
          <span>Adaptive Load Balancer <span className="footer-divider">/</span> Benchmarking Console</span>
          <span>Results are held in memory and cleared when this page is refreshed.</span>
        </footer>
      </main>
    </div>
  );
}
