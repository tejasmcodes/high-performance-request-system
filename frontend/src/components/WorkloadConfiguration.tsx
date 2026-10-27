interface WorkloadValues {
  duration_seconds: number;
  warmup_seconds: number;
  concurrency: number;
  rate_per_second: number;
}

interface Props {
  values: WorkloadValues;
  disabled: boolean;
  onChange: (values: WorkloadValues) => void;
}

const fields: {
  key: keyof WorkloadValues;
  label: string;
  helper: string;
  min: number;
  step: number;
}[] = [
  {
    key: "duration_seconds",
    label: "Duration",
    helper: "Measurement time",
    min: 1,
    step: 1,
  },
  {
    key: "warmup_seconds",
    label: "Warmup",
    helper: "Before measurement",
    min: 0,
    step: 1,
  },
  {
    key: "concurrency",
    label: "Concurrency",
    helper: "Concurrent clients",
    min: 1,
    step: 1,
  },
  {
    key: "rate_per_second",
    label: "Target rate",
    helper: "0 = closed loop",
    min: 0,
    step: 1,
  },
];

export default function WorkloadConfiguration({
  values,
  disabled,
  onChange,
}: Props) {
  return (
    <section className="config-section" aria-labelledby="workload-heading">
      <div className="section-heading">
        <div>
          <span className="step-label">03 / WORKLOAD</span>
          <h3 id="workload-heading">Load profile</h3>
        </div>
      </div>
      <div className="workload-grid">
        {fields.map((field) => (
          <label className="number-field" key={field.key}>
            <span className="input-label">{field.label}</span>
            <input
              type="number"
              min={field.min}
              step={field.step}
              value={values[field.key]}
              onChange={(event) =>
                onChange({ ...values, [field.key]: Number(event.target.value) })
              }
              disabled={disabled}
            />
            <span className="field-note">{field.helper}</span>
          </label>
        ))}
      </div>
      <p className="field-note rate-note">
        Benchmarks run for a duration; request count is measured, not pre-set.
      </p>
    </section>
  );
}
