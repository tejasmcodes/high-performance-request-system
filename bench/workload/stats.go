package workload

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type WindowReport struct {
	Second   int                `json:"second"`
	Requests uint64             `json:"requests"`
	Errors   uint64             `json:"errors"`
	Latency  HistogramSnapshot  `json:"latency"`
	Backends map[string]uint64  `json:"backend_requests"`
	CPU      map[string]float64 `json:"backend_cpu"`
}

type Result struct {
	Scenario        string            `json:"scenario"`
	Strategy        string            `json:"strategy"`
	Run             int               `json:"run"`
	URL             string            `json:"url"`
	Concurrency     int               `json:"concurrency"`
	RatePerSecond   float64           `json:"rate_per_second"`
	WarmupSeconds   float64           `json:"warmup_seconds"`
	DurationSeconds float64           `json:"duration_seconds"`
	StartedAt       time.Time         `json:"started_at"`
	FinishedAt      time.Time         `json:"finished_at"`
	TotalRequests   uint64            `json:"total_requests"`
	TotalErrors     uint64            `json:"total_errors"`
	ErrorRate       float64           `json:"error_rate"`
	Latency         HistogramSnapshot `json:"latency"`
	Backends        map[string]uint64 `json:"backend_requests"`
	Windows         []WindowReport    `json:"windows"`
}

func (r *Result) WriteJSON(path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

type Summary struct {
	Scenario  string    `json:"scenario"`
	CreatedAt time.Time `json:"created_at"`
	Results   []*Result `json:"results"`
}

func WriteSummary(path string, summary *Summary) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
