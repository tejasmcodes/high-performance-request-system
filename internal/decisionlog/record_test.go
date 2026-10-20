package decisionlog

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRecordJSONSerialization(t *testing.T) {
	probability := 0.25

	record := Record{
		SchemaVersion:        SchemaVersion,
		DecisionID:           "17",
		RunID:                "run-1",
		SelectionTime:        time.Date(2026, 10, 20, 12, 0, 0, 123000000, time.UTC),
		Scenario:             "baseline",
		Strategy:             "adaptive",
		Run:                  3,
		SelectedBackend:      "backend-2",
		SelectionProbability: &probability,
		PolicyVersion:        "policy-v1",
		Candidates: []CandidateSnapshot{
			{
				BackendID:           "backend-2",
				Healthy:             true,
				Weight:              3,
				ActiveConnections:   2,
				ResponseTimeEMAMs:   12.5,
				ResponseTimeSamples: 4,
				CPUUtilization:      37.5,
				CPUUpdatedAt:        time.Date(2026, 10, 20, 11, 59, 59, 0, time.UTC),
				CPUAgeMs:            1000,
			},
		},
		Outcome: Outcome{
			CompletionTime:  time.Date(2026, 10, 20, 12, 0, 0, 173000000, time.UTC),
			LatencyMs:       12.345,
			HTTPStatus:      200,
			Success:         true,
			Error:           "",
			Timeout:         false,
			ContextCanceled: false,
			ProxyError:      false,
		},
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}

	assertKeys(t, raw, []string{
		"schema_version",
		"decision_id",
		"run_id",
		"selection_time",
		"scenario",
		"strategy",
		"run",
		"selected_backend",
		"selection_probability",
		"policy_version",
		"candidates",
		"outcome",
	})

	var candidates []map[string]json.RawMessage
	if err := json.Unmarshal(raw["candidates"], &candidates); err != nil {
		t.Fatalf("unmarshal candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}

	assertKeys(t, candidates[0], []string{
		"backend_id",
		"healthy",
		"weight",
		"active_connections",
		"response_time_ema_ms",
		"response_time_samples",
		"cpu_utilization",
		"cpu_updated_at",
		"cpu_age_ms",
	})

	var outcome map[string]json.RawMessage
	if err := json.Unmarshal(raw["outcome"], &outcome); err != nil {
		t.Fatalf("unmarshal outcome: %v", err)
	}

	assertKeys(t, outcome, []string{
		"completion_time",
		"latency_ms",
		"http_status",
		"success",
		"error",
		"timeout",
		"context_canceled",
		"proxy_error",
	})

	var got Record
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal round trip: %v", err)
	}

	if got.SchemaVersion != record.SchemaVersion {
		t.Fatalf("expected schema version %d, got %d", record.SchemaVersion, got.SchemaVersion)
	}
	if got.DecisionID != record.DecisionID {
		t.Fatalf("expected decision id %q, got %q", record.DecisionID, got.DecisionID)
	}
	if !got.SelectionTime.Equal(record.SelectionTime) {
		t.Fatalf("expected selection time %v, got %v", record.SelectionTime, got.SelectionTime)
	}
	if got.SelectionProbability == nil {
		t.Fatal("expected selection probability, got nil")
	}
	if *got.SelectionProbability != probability {
		t.Fatalf("expected selection probability %f, got %f", probability, *got.SelectionProbability)
	}
	if got.Outcome.LatencyMs != record.Outcome.LatencyMs {
		t.Fatalf("expected latency %f, got %f", record.Outcome.LatencyMs, got.Outcome.LatencyMs)
	}
	if got.Candidates[0].CPUAgeMs != record.Candidates[0].CPUAgeMs {
		t.Fatalf("expected CPU age %d, got %d", record.Candidates[0].CPUAgeMs, got.Candidates[0].CPUAgeMs)
	}
	if !got.Candidates[0].CPUUpdatedAt.Equal(record.Candidates[0].CPUUpdatedAt) {
		t.Fatalf("expected CPU updated at %v, got %v", record.Candidates[0].CPUUpdatedAt, got.Candidates[0].CPUUpdatedAt)
	}
}

func TestRecordSelectionProbabilityNull(t *testing.T) {
	data, err := json.Marshal(Record{})
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}

	if string(raw["selection_probability"]) != "null" {
		t.Fatalf("expected null selection probability, got %s", raw["selection_probability"])
	}
}

func assertKeys(t *testing.T, values map[string]json.RawMessage, expected []string) {
	t.Helper()

	if len(values) != len(expected) {
		t.Fatalf("expected %d keys, got %d: %v", len(expected), len(values), keys(values))
	}

	for _, key := range expected {
		if _, ok := values[key]; !ok {
			t.Fatalf("missing key %q", key)
		}
	}
}

func keys(values map[string]json.RawMessage) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
