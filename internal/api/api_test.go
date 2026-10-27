package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/bench/workload"
	"github.com/tejasmcodes/high-performance-request-system/internal/benchmark"
)

type stubRunner struct {
	config benchmark.Config
	err    error
}

func (s *stubRunner) Run(_ context.Context, cfg benchmark.Config) (*workload.Result, error) {
	s.config = cfg
	if s.err != nil {
		return nil, s.err
	}
	now := time.Now()
	return &workload.Result{
		Scenario:        cfg.Scenario,
		Strategy:        cfg.Algorithm,
		Concurrency:     cfg.Concurrency,
		RatePerSecond:   cfg.RatePerSecond,
		DurationSeconds: cfg.Duration.Seconds(),
		StartedAt:       now.Add(-time.Second),
		FinishedAt:      now,
		TotalRequests:   10,
		TotalErrors:     2,
		ErrorRate:       0.2,
		Latency:         workload.HistogramSnapshot{Count: 10, MeanMS: 2, P50MS: 2, P95MS: 3, P99MS: 4},
		Backends:        map[string]uint64{"one": 10},
	}, nil
}

func newTestAPI(t *testing.T, runner BenchmarkRunner) http.Handler {
	t.Helper()
	dir := t.TempDir()
	data := `{"name":"baseline","description":"Equal latency","backends":[{"id":"backend-1","port":18081,"weight":1}],"load":{"duration_s":1}}`
	if err := os.WriteFile(filepath.Join(dir, "baseline.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return New(dir, "http://localhost:5173", runner)
}

func TestGetAlgorithms(t *testing.T) {
	response := httptest.NewRecorder()
	newTestAPI(t, &stubRunner{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/algorithms", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var body struct {
		Algorithms []algorithm `json:"algorithms"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Algorithms) != 4 {
		t.Fatalf("expected four supported algorithms, got %d", len(body.Algorithms))
	}
}

func TestGetScenarios(t *testing.T) {
	response := httptest.NewRecorder()
	newTestAPI(t, &stubRunner{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/scenarios", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"baseline"`) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
}

func TestRunBenchmark(t *testing.T) {
	runner := &stubRunner{}
	handler := newTestAPI(t, runner)
	request := `{"algorithm":"adaptive","scenario":"baseline","servers":[{"id":"one","url":"http://127.0.0.1:8081","weight":2}],"duration_seconds":3,"warmup_seconds":1,"concurrency":4,"rate_per_second":10}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/benchmarks", strings.NewReader(request)))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if runner.config.Algorithm != "adaptive" || runner.config.Duration != 3*time.Second || runner.config.Servers[0].Weight != 2 {
		t.Fatalf("unexpected runner config: %+v", runner.config)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["completed_requests"] != float64(8) || body["failed_requests"] != float64(2) || body["throughput_rps"] != float64(10) {
		t.Fatalf("unexpected benchmark response: %+v", body)
	}
}

func TestRunBenchmarkRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "algorithm", body: `{"algorithm":"unknown","scenario":"baseline","servers":[{"id":"one","url":"http://localhost:1","weight":1}],"duration_seconds":1,"concurrency":1}`},
		{name: "scenario", body: `{"algorithm":"adaptive","scenario":"unknown","servers":[{"id":"one","url":"http://localhost:1","weight":1}],"duration_seconds":1,"concurrency":1}`},
		{name: "configuration", body: `{"algorithm":"adaptive","scenario":"baseline","servers":[{"id":"one","url":"http://localhost:1","weight":1}],"duration_seconds":1,"concurrency":0}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			newTestAPI(t, &stubRunner{}).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/benchmarks", strings.NewReader(test.body)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), `"error"`) {
				t.Fatalf("expected structured error response, got %s", response.Body.String())
			}
		})
	}
}

func TestCompare(t *testing.T) {
	body := `{"results":[{"algorithm":"round-robin","throughput_rps":1820,"avg_latency_ms":5.4,"p95_latency_ms":8.2,"p99_latency_ms":12.1,"error_rate":0},{"algorithm":"adaptive","throughput_rps":2310,"avg_latency_ms":4.2,"p95_latency_ms":6.1,"p99_latency_ms":9.2,"error_rate":0}]}`
	response := httptest.NewRecorder()
	newTestAPI(t, &stubRunner{}).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/benchmarks/compare", strings.NewReader(body)))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var result compareResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.BestThroughput == nil || result.BestThroughput.Algorithm != "adaptive" {
		t.Fatalf("unexpected throughput winner: %+v", result.BestThroughput)
	}
	if result.BestThroughput.ImprovementPercent == nil || *result.BestThroughput.ImprovementPercent < 26.9 {
		t.Fatalf("unexpected throughput improvement: %+v", result.BestThroughput)
	}
	if result.LowestP99Latency == nil || result.LowestP99Latency.Algorithm != "adaptive" {
		t.Fatalf("unexpected P99 winner: %+v", result.LowestP99Latency)
	}
}

func TestCompareRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: `{"results":[]}`},
		{name: "negative", body: `{"results":[{"algorithm":"adaptive","throughput_rps":-1}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			newTestAPI(t, &stubRunner{}).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/benchmarks/compare", strings.NewReader(test.body)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCORSPreflight(t *testing.T) {
	request := httptest.NewRequest(http.MethodOptions, "/api/algorithms", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()
	newTestAPI(t, &stubRunner{}).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("unexpected CORS response: %d %+v", response.Code, response.Header())
	}
}

func TestBenchmarkExecutionErrorReturnsGatewayError(t *testing.T) {
	runner := &stubRunner{err: fmt.Errorf("backend unavailable")}
	request := `{"algorithm":"adaptive","scenario":"baseline","servers":[{"id":"one","url":"http://127.0.0.1:8081","weight":1}],"duration_seconds":1,"concurrency":1}`
	response := httptest.NewRecorder()
	newTestAPI(t, runner).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/benchmarks", strings.NewReader(request)))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", response.Code)
	}
}
