package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tejasmcodes/high-performance-request-system/internal/decisionlog"
	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

type testStrategy struct {
	next func([]*models.Server) *models.Server
}

func (s testStrategy) NextServer(servers []*models.Server) *models.Server {
	return s.next(servers)
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestBenchmarkProxyHandlerSnapshotsBeforeSelectionAndRecordsRequest(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	server := models.NewServer("backend-1", backend.URL, 1)
	var output bytes.Buffer

	handler := &benchmarkProxyHandler{
		strategy: testStrategy{next: func(servers []*models.Server) *models.Server {
			servers[0].SetHealthy(false)
			return servers[0]
		}},
		servers:      []*models.Server{server},
		recorder:     decisionlog.NewRecorder(&output),
		strategyName: "adaptive",
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://load-balancer/", nil))

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, response.Code)
	}

	record := decodeSingleRecord(t, output.Bytes())
	if len(record.Candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(record.Candidates))
	}
	if !record.Candidates[0].Healthy {
		t.Fatal("expected candidate snapshot from before strategy selection")
	}
	if record.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", record.SchemaVersion)
	}
	if record.DecisionID != "1" {
		t.Fatalf("expected decision ID 1, got %q", record.DecisionID)
	}
	if record.SelectedBackend != "backend-1" {
		t.Fatalf("expected selected backend backend-1, got %q", record.SelectedBackend)
	}
	if record.Strategy != "adaptive" {
		t.Fatalf("expected adaptive strategy, got %q", record.Strategy)
	}
	if record.SelectionTime.IsZero() || record.Outcome.CompletionTime.IsZero() {
		t.Fatal("expected selection and completion timestamps")
	}
	if record.Outcome.CompletionTime.Before(record.SelectionTime) {
		t.Fatal("expected completion after selection")
	}
	if record.Outcome.LatencyMs < 0 {
		t.Fatalf("expected non-negative latency, got %f", record.Outcome.LatencyMs)
	}
	if record.Outcome.HTTPStatus != http.StatusNoContent || !record.Outcome.Success {
		t.Fatalf("unexpected outcome: %+v", record.Outcome)
	}
}

func TestBenchmarkProxyHandlerDisabledLoggingPreservesProxyResponse(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer backend.Close()

	server := models.NewServer("backend-1", backend.URL, 1)
	handler := &benchmarkProxyHandler{
		strategy:     testStrategy{next: func(servers []*models.Server) *models.Server { return servers[0] }},
		servers:      []*models.Server{server},
		strategyName: "adaptive",
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://load-balancer/", nil))

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, response.Code)
	}
	if connections := server.ActiveConnections(); connections != 0 {
		t.Fatalf("expected active connections to return to zero, got %d", connections)
	}
}

func TestBenchmarkProxyHandlerRecorderErrorDoesNotFailRequest(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	server := models.NewServer("backend-1", backend.URL, 1)
	recorder := decisionlog.NewRecorder(failingWriter{err: errors.New("write failed")})
	handler := &benchmarkProxyHandler{
		strategy:     testStrategy{next: func(servers []*models.Server) *models.Server { return servers[0] }},
		servers:      []*models.Server{server},
		recorder:     recorder,
		strategyName: "adaptive",
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://load-balancer/", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if stats := recorder.Stats(); stats.WriteErrors != 1 {
		t.Fatalf("expected one recorder error, got %+v", stats)
	}
}

func decodeSingleRecord(t *testing.T, data []byte) decisionlog.Record {
	t.Helper()

	var record decisionlog.Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode decision record: %v", err)
	}

	return record
}
