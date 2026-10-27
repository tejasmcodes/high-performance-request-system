package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/bench/scenario"
	"github.com/tejasmcodes/high-performance-request-system/bench/workload"
	"github.com/tejasmcodes/high-performance-request-system/internal/benchmark"
)

type BenchmarkRunner interface {
	Run(context.Context, benchmark.Config) (*workload.Result, error)
}

type API struct {
	scenarioDir string
	runner      BenchmarkRunner
	corsOrigin  string
}

func New(scenarioDir, corsOrigin string, runner BenchmarkRunner) http.Handler {
	api := &API{scenarioDir: scenarioDir, corsOrigin: corsOrigin, runner: runner}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/algorithms", api.algorithms)
	mux.HandleFunc("GET /api/scenarios", api.scenarios)
	mux.HandleFunc("POST /api/benchmarks", api.runBenchmark)
	mux.HandleFunc("POST /api/benchmarks/compare", api.compare)
	return api.cors(mux)
}

type algorithm struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var algorithms = []algorithm{
	{ID: "round-robin", Name: "Round Robin"},
	{ID: "weighted-round-robin", Name: "Weighted Round Robin"},
	{ID: "least-connections", Name: "Least Connections"},
	{ID: "adaptive", Name: "Adaptive"},
}

func (a *API) algorithms(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"algorithms": algorithms})
}

type scenarioInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a *API) scenarios(w http.ResponseWriter, _ *http.Request) {
	entries, err := os.ReadDir(a.scenarioDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scenario_load_failed", "Unable to load benchmark scenarios")
		return
	}

	scenarios := make([]scenarioInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		sc, err := scenario.LoadScenario(filepath.Join(a.scenarioDir, entry.Name()))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "scenario_load_failed", "Unable to load benchmark scenarios")
			return
		}
		scenarios = append(scenarios, scenarioInfo{ID: sc.Name, Name: sc.Name, Description: sc.Description})
	}
	sort.Slice(scenarios, func(i, j int) bool { return scenarios[i].ID < scenarios[j].ID })
	writeJSON(w, http.StatusOK, map[string]any{"scenarios": scenarios})
}

type serverInput struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Weight int    `json:"weight"`
}

type benchmarkRequest struct {
	Algorithm       string        `json:"algorithm"`
	Scenario        string        `json:"scenario"`
	Servers         []serverInput `json:"servers"`
	DurationSeconds int           `json:"duration_seconds"`
	WarmupSeconds   int           `json:"warmup_seconds"`
	Concurrency     int           `json:"concurrency"`
	RatePerSecond   float64       `json:"rate_per_second"`
}

func (a *API) runBenchmark(w http.ResponseWriter, r *http.Request) {
	var request benchmarkRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	cfg, err := a.validateBenchmark(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_configuration", err.Error())
		return
	}

	result, err := a.runner.Run(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, "benchmark_failed", "Benchmark execution failed")
		return
	}
	if result == nil {
		writeError(w, http.StatusInternalServerError, "benchmark_failed", "Benchmark execution returned no result")
		return
	}

	elapsed := result.FinishedAt.Sub(result.StartedAt).Seconds()
	throughput := 0.0
	if elapsed > 0 {
		throughput = float64(result.TotalRequests) / elapsed
	}
	completed := result.TotalRequests - result.TotalErrors
	writeJSON(w, http.StatusOK, map[string]any{
		"algorithm":           result.Strategy,
		"scenario":            result.Scenario,
		"duration_seconds":    result.DurationSeconds,
		"concurrency":         result.Concurrency,
		"rate_per_second":     result.RatePerSecond,
		"total_requests":      result.TotalRequests,
		"completed_requests":  completed,
		"failed_requests":     result.TotalErrors,
		"throughput_rps":      throughput,
		"error_rate":          result.ErrorRate,
		"latency":             result.Latency,
		"server_distribution": result.Backends,
		"windows":             result.Windows,
	})
}

func (a *API) validateBenchmark(request benchmarkRequest) (benchmark.Config, error) {
	if !knownAlgorithm(request.Algorithm) {
		return benchmark.Config{}, fmt.Errorf("unknown algorithm %q", request.Algorithm)
	}
	scenarioPath, err := a.findScenario(request.Scenario)
	if err != nil {
		return benchmark.Config{}, err
	}
	if _, err := scenario.LoadScenario(scenarioPath); err != nil {
		return benchmark.Config{}, fmt.Errorf("invalid scenario %q", request.Scenario)
	}
	if request.DurationSeconds <= 0 {
		return benchmark.Config{}, errors.New("duration_seconds must be positive")
	}
	if request.WarmupSeconds < 0 {
		return benchmark.Config{}, errors.New("warmup_seconds must not be negative")
	}
	if request.Concurrency <= 0 {
		return benchmark.Config{}, errors.New("concurrency must be positive")
	}
	if !finite(request.RatePerSecond) || request.RatePerSecond < 0 {
		return benchmark.Config{}, errors.New("rate_per_second must be finite and non-negative")
	}
	if len(request.Servers) == 0 {
		return benchmark.Config{}, errors.New("at least one server is required")
	}

	servers := make([]benchmark.ServerConfig, len(request.Servers))
	seen := make(map[string]bool, len(request.Servers))
	for i, server := range request.Servers {
		if strings.TrimSpace(server.ID) == "" || seen[server.ID] {
			return benchmark.Config{}, errors.New("server IDs must be non-empty and unique")
		}
		seen[server.ID] = true
		if server.Weight <= 0 {
			return benchmark.Config{}, fmt.Errorf("server %q weight must be positive", server.ID)
		}
		parsed, err := url.ParseRequestURI(server.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
			return benchmark.Config{}, fmt.Errorf("server %q must have a valid http or https URL", server.ID)
		}
		servers[i] = benchmark.ServerConfig{ID: server.ID, URL: strings.TrimRight(server.URL, "/"), Weight: server.Weight}
	}

	return benchmark.Config{
		Algorithm:      request.Algorithm,
		Scenario:       request.Scenario,
		Servers:        servers,
		Duration:       time.Duration(request.DurationSeconds) * time.Second,
		Warmup:         time.Duration(request.WarmupSeconds) * time.Second,
		Concurrency:    request.Concurrency,
		RatePerSecond:  request.RatePerSecond,
		RequestTimeout: 5 * time.Second,
	}, nil
}

func (a *API) findScenario(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("scenario is required")
	}
	entries, err := os.ReadDir(a.scenarioDir)
	if err != nil {
		return "", errors.New("unable to load benchmark scenarios")
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(a.scenarioDir, entry.Name())
		sc, err := scenario.LoadScenario(path)
		if err == nil && sc.Name == name {
			return path, nil
		}
	}
	return "", fmt.Errorf("unknown scenario %q", name)
}

func knownAlgorithm(id string) bool {
	for _, item := range algorithms {
		if item.ID == id {
			return true
		}
	}
	return false
}

type compareInput struct {
	Results []comparisonResult `json:"results"`
}

type comparisonResult struct {
	Algorithm     string   `json:"algorithm"`
	ThroughputRPS *float64 `json:"throughput_rps"`
	AvgLatencyMS  *float64 `json:"avg_latency_ms"`
	P95LatencyMS  *float64 `json:"p95_latency_ms"`
	P99LatencyMS  *float64 `json:"p99_latency_ms"`
	ErrorRate     *float64 `json:"error_rate"`
}

type metricWinner struct {
	Algorithm          string   `json:"algorithm"`
	Value              float64  `json:"value"`
	ImprovementPercent *float64 `json:"improvement_percent,omitempty"`
}

type compareResponse struct {
	ReferenceAlgorithm string        `json:"reference_algorithm"`
	BestThroughput     *metricWinner `json:"best_throughput,omitempty"`
	LowestAvgLatency   *metricWinner `json:"lowest_avg_latency,omitempty"`
	LowestP95Latency   *metricWinner `json:"lowest_p95_latency,omitempty"`
	LowestP99Latency   *metricWinner `json:"lowest_p99_latency,omitempty"`
	LowestErrorRate    *metricWinner `json:"lowest_error_rate,omitempty"`
}

func (a *API) compare(w http.ResponseWriter, r *http.Request) {
	var input compareInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(input.Results) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_comparison", "results must not be empty")
		return
	}
	for _, result := range input.Results {
		if strings.TrimSpace(result.Algorithm) == "" {
			writeError(w, http.StatusBadRequest, "invalid_comparison", "each result must include an algorithm")
			return
		}
		hasMetric := false
		for _, metric := range []*float64{result.ThroughputRPS, result.AvgLatencyMS, result.P95LatencyMS, result.P99LatencyMS, result.ErrorRate} {
			if metric == nil {
				continue
			}
			hasMetric = true
			if !finite(*metric) || *metric < 0 {
				writeError(w, http.StatusBadRequest, "invalid_comparison", "metrics must be finite and non-negative")
				return
			}
		}
		if !hasMetric || (result.ErrorRate != nil && *result.ErrorRate > 1) {
			writeError(w, http.StatusBadRequest, "invalid_comparison", "each result must include metrics and error_rate must not exceed 1")
			return
		}
	}

	response := compareResponse{ReferenceAlgorithm: input.Results[0].Algorithm}
	response.BestThroughput = chooseWinner(input.Results, func(r comparisonResult) *float64 { return r.ThroughputRPS }, true)
	response.LowestAvgLatency = chooseWinner(input.Results, func(r comparisonResult) *float64 { return r.AvgLatencyMS }, false)
	response.LowestP95Latency = chooseWinner(input.Results, func(r comparisonResult) *float64 { return r.P95LatencyMS }, false)
	response.LowestP99Latency = chooseWinner(input.Results, func(r comparisonResult) *float64 { return r.P99LatencyMS }, false)
	response.LowestErrorRate = chooseWinner(input.Results, func(r comparisonResult) *float64 { return r.ErrorRate }, false)
	writeJSON(w, http.StatusOK, response)
}

func chooseWinner(results []comparisonResult, value func(comparisonResult) *float64, higherIsBetter bool) *metricWinner {
	bestIndex := -1
	for i, result := range results {
		current := value(result)
		if current == nil {
			continue
		}
		if bestIndex < 0 {
			bestIndex = i
			continue
		}
		best := value(results[bestIndex])
		if (higherIsBetter && *current > *best) || (!higherIsBetter && *current < *best) {
			bestIndex = i
		}
	}
	if bestIndex < 0 {
		return nil
	}

	winnerValue := *value(results[bestIndex])
	winner := &metricWinner{Algorithm: results[bestIndex].Algorithm, Value: winnerValue}
	reference := value(results[0])
	if reference != nil && *reference != 0 {
		improvement := (winnerValue - *reference) / *reference * 100
		if !higherIsBetter {
			improvement = -improvement
		}
		winner.ImprovementPercent = &improvement
	}
	return winner
}

type apiError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Error: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode API response: %v", err)
	}
}

func (a *API) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.corsOrigin != "" && r.Header.Get("Origin") == a.corsOrigin {
			w.Header().Set("Access-Control-Allow-Origin", a.corsOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
