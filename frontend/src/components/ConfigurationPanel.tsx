import type { Algorithm, Scenario, ServerConfig } from "../api/types";
import ServerConfiguration from "./ServerConfiguration";
import WorkloadConfiguration from "./WorkloadConfiguration";

export interface WorkloadValues {
  duration_seconds: number;
  warmup_seconds: number;
  concurrency: number;
  rate_per_second: number;
}

interface Props {
  algorithms: Algorithm[];
  scenarios: Scenario[];
  algorithm: string;
  scenario: string;
  servers: ServerConfig[];
  workload: WorkloadValues;
  disabled: boolean;
  canRun: boolean;
  validationMessages: string[];
  isRunning: boolean;
  onAlgorithmChange: (value: string) => void;
  onScenarioChange: (value: string) => void;
  onServersChange: (value: ServerConfig[]) => void;
  onWorkloadChange: (value: WorkloadValues) => void;
  onRun: () => void;
}

export default function ConfigurationPanel(props: Props) {
  const selectedScenario = props.scenarios.find(
    (scenario) => scenario.id === props.scenario,
  );

  return (
    <aside className="configuration-panel">
      <div className="panel-title">
        <span className="panel-kicker">NEW EXPERIMENT</span>
        <h2>Run configuration</h2>
        <p>Set your routing strategy and workload parameters.</p>
      </div>

      <section className="config-section" aria-labelledby="strategy-heading">
        <div className="section-heading">
          <div>
            <span className="step-label">01 / STRATEGY</span>
            <h3 id="strategy-heading">Experiment setup</h3>
          </div>
        </div>
        <label className="form-field">
          <span className="input-label">Routing algorithm</span>
          <select
            value={props.algorithm}
            onChange={(event) => props.onAlgorithmChange(event.target.value)}
            disabled={props.disabled || props.algorithms.length === 0}
          >
            {props.algorithms.length === 0 && <option value="">Loading…</option>}
            {props.algorithms.map((algorithm) => (
              <option value={algorithm.id} key={algorithm.id}>
                {algorithm.name}
              </option>
            ))}
          </select>
        </label>
        <label className="form-field">
          <span className="input-label">Scenario label</span>
          <select
            value={props.scenario}
            onChange={(event) => props.onScenarioChange(event.target.value)}
            disabled={props.disabled || props.scenarios.length === 0}
          >
            {props.scenarios.length === 0 && <option value="">Loading…</option>}
            {props.scenarios.map((scenario) => (
              <option value={scenario.id} key={scenario.id}>
                {scenario.name}
              </option>
            ))}
          </select>
        </label>
        {selectedScenario && (
          <p className="scenario-description">{selectedScenario.description}</p>
        )}
        <div className="notice">
          <span className="notice-icon" aria-hidden="true">i</span>
          <p>
            Scenarios are validated labels. Their backend phases and settings
            are not applied automatically.
          </p>
        </div>
      </section>

      <ServerConfiguration
        servers={props.servers}
        disabled={props.disabled}
        onChange={props.onServersChange}
      />

      <WorkloadConfiguration
        values={props.workload}
        disabled={props.disabled}
        onChange={props.onWorkloadChange}
      />

      {props.validationMessages.length > 0 && (
        <ul className="validation-list" aria-live="polite">
          {props.validationMessages.map((message) => (
            <li key={message}>{message}</li>
          ))}
        </ul>
      )}

      <button
        className="button button-primary run-button"
        type="button"
        onClick={props.onRun}
        disabled={props.disabled || !props.canRun}
      >
        {props.isRunning ? (
          <>
            <span className="spinner" aria-hidden="true" />
            Running benchmark…
          </>
        ) : (
          <>
            <span aria-hidden="true">▶</span>
            Run Benchmark
          </>
        )}
      </button>
      <p className="run-footnote">Runs synchronously · results stay in this session</p>
    </aside>
  );
}
