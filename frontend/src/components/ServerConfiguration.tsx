import type { ServerConfig } from "../api/types";

interface Props {
  servers: ServerConfig[];
  disabled: boolean;
  onChange: (servers: ServerConfig[]) => void;
}

export default function ServerConfiguration({
  servers,
  disabled,
  onChange,
}: Props) {
  function update(index: number, patch: Partial<ServerConfig>) {
    onChange(
      servers.map((server, current) =>
        current === index ? { ...server, ...patch } : server,
      ),
    );
  }

  function addServer() {
    onChange([
      ...servers,
      { id: `backend-${servers.length + 1}`, url: "", weight: 1 },
    ]);
  }

  return (
    <section className="config-section" aria-labelledby="servers-heading">
      <div className="section-heading">
        <div>
          <span className="step-label">02 / BACKENDS</span>
          <h3 id="servers-heading">Backend servers</h3>
        </div>
        <button
          className="button button-small button-quiet"
          type="button"
          onClick={addServer}
          disabled={disabled}
        >
          <span aria-hidden="true">＋</span> Add server
        </button>
      </div>

      <div className="server-list">
        {servers.map((server, index) => (
          <div className="server-row" key={`${index}-${server.id}`}>
            <div className="server-row-top">
              <span className="server-index">
                <span className="status-dot" />
                Server {String(index + 1).padStart(2, "0")}
              </span>
              {servers.length > 1 && (
                <button
                  type="button"
                  className="icon-button remove-button"
                  aria-label={`Remove ${server.id || `server ${index + 1}`}`}
                  title="Remove server"
                  disabled={disabled}
                  onClick={() => onChange(servers.filter((_, i) => i !== index))}
                >
                  ×
                </button>
              )}
            </div>
            <div className="server-fields">
              <label>
                <span>Server ID</span>
                <input
                  value={server.id}
                  onChange={(event) => update(index, { id: event.target.value })}
                  placeholder="backend-1"
                  autoComplete="off"
                  disabled={disabled}
                />
              </label>
              <label className="server-url">
                <span>Base URL</span>
                <input
                  type="url"
                  value={server.url}
                  onChange={(event) => update(index, { url: event.target.value })}
                  placeholder="http://127.0.0.1:18081"
                  autoComplete="url"
                  disabled={disabled}
                />
              </label>
              <label className="weight-field">
                <span>Weight</span>
                <input
                  type="number"
                  min="1"
                  step="1"
                  value={server.weight}
                  onChange={(event) =>
                    update(index, { weight: Number(event.target.value) })
                  }
                  disabled={disabled}
                />
              </label>
            </div>
          </div>
        ))}
      </div>
      <p className="field-note">
        Servers must be reachable and respond successfully at <code>/health</code>.
      </p>
    </section>
  );
}
