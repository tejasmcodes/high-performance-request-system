package decisionlog

import "time"

const SchemaVersion = 1

type Record struct {
	SchemaVersion        int                 `json:"schema_version"`
	DecisionID           string              `json:"decision_id"`
	RunID                string              `json:"run_id"`
	SelectionTime        time.Time           `json:"selection_time"`
	Scenario             string              `json:"scenario"`
	Strategy             string              `json:"strategy"`
	Run                  int                 `json:"run"`
	SelectedBackend      string              `json:"selected_backend"`
	SelectionProbability *float64            `json:"selection_probability"`
	PolicyVersion        string              `json:"policy_version"`
	Candidates           []CandidateSnapshot `json:"candidates"`
	Outcome              Outcome             `json:"outcome"`
}

type Outcome struct {
	CompletionTime  time.Time `json:"completion_time"`
	LatencyMs       float64   `json:"latency_ms"`
	HTTPStatus      int       `json:"http_status"`
	Success         bool      `json:"success"`
	Error           string    `json:"error"`
	Timeout         bool      `json:"timeout"`
	ContextCanceled bool      `json:"context_canceled"`
	ProxyError      bool      `json:"proxy_error"`
}
