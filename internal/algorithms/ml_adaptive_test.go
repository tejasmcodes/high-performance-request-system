package algorithms

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

func testMLModel() LinearLatencyModel {
	model := LinearLatencyModel{
		Version:      mlModelVersion,
		Type:         "linear",
		Target:       "predicted_latency_ms (lower is better, pick the minimum)",
		Features:     []string{"in_flight", "ema_ms", "cpu"},
		Coefficients: []float64{1, 2, 3},
		Intercept:    4,
	}
	model.Defaults.InFlight = 0
	model.Defaults.EMAMs = 50
	model.Defaults.CPU = 10
	return model
}

func TestMLAdaptivePrediction(t *testing.T) {
	server := models.NewServer("backend-1", "http://backend-1", 1)
	server.IncrementConnections()
	server.IncrementConnections()
	server.ObserveResponseTime(10 * time.Millisecond)
	server.SetCPUUtilization(20)

	strategy := NewMLAdaptive(testMLModel())
	if got, want := strategy.predict(server, time.Now()), 86.0; got != want {
		t.Fatalf("prediction = %v, want %v", got, want)
	}
}

func TestMLAdaptiveSelectsLowestPrediction(t *testing.T) {
	fast := models.NewServer("fast", "http://fast", 1)
	slow := models.NewServer("slow", "http://slow", 1)
	fast.ObserveResponseTime(10 * time.Millisecond)
	slow.ObserveResponseTime(100 * time.Millisecond)

	selected := NewMLAdaptive(testMLModel()).NextServer([]*models.Server{slow, fast})
	if selected != fast {
		t.Fatalf("selected = %v, want fast", selected)
	}
}

func TestMLAdaptiveSkipsUnhealthyAndDisabledServers(t *testing.T) {
	unhealthy := models.NewServer("unhealthy", "http://unhealthy", 1)
	unhealthy.SetHealthy(false)
	unhealthy.ObserveResponseTime(time.Millisecond)
	disabled := models.NewServer("disabled", "http://disabled", 0)
	disabled.ObserveResponseTime(time.Millisecond)
	healthy := models.NewServer("healthy", "http://healthy", 1)
	healthy.ObserveResponseTime(100 * time.Millisecond)

	selected := NewMLAdaptive(testMLModel()).NextServer([]*models.Server{unhealthy, disabled, healthy})
	if selected != healthy {
		t.Fatalf("selected = %v, want healthy", selected)
	}
}

func TestMLAdaptiveTieUsesConfiguredServerOrder(t *testing.T) {
	first := models.NewServer("first", "http://first", 1)
	second := models.NewServer("second", "http://second", 1)

	selected := NewMLAdaptive(testMLModel()).NextServer([]*models.Server{first, second})
	if selected != first {
		t.Fatalf("selected = %v, want first", selected)
	}
}

func TestMLAdaptiveMissingMetricsUseModelDefaults(t *testing.T) {
	server := models.NewServer("backend-1", "http://backend-1", 1)
	strategy := NewMLAdaptive(testMLModel())

	// 4 + (1 * 0) + (2 * 50 default EMA) + (3 * 10 default CPU).
	if got, want := strategy.predict(server, time.Now()), 134.0; got != want {
		t.Fatalf("prediction with defaults = %v, want %v", got, want)
	}
}

func TestMLMetricFreshnessRejectsStaleAndInvalidValues(t *testing.T) {
	now := time.Now()
	stale := now.Add(-mlResponseTimeStaleAfter - time.Nanosecond)
	if isFreshMLResponseTime(10, 1, stale, now) {
		t.Fatal("stale response time was accepted")
	}
	if isFreshMLResponseTime(math.NaN(), 1, now, now) {
		t.Fatal("NaN response time was accepted")
	}

	staleCPU := now.Add(-mlCPUStaleAfter - time.Nanosecond)
	if isFreshMLCPU(20, staleCPU, now) {
		t.Fatal("stale CPU was accepted")
	}
	if isFreshMLCPU(101, now, now) {
		t.Fatal("out-of-range CPU was accepted")
	}
}

func TestLoadLinearLatencyModelValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.json")
	model := testMLModel()
	data, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("marshal model: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	if _, err := LoadLinearLatencyModel(path); err != nil {
		t.Fatalf("load valid model: %v", err)
	}

	model.Features[0] = "wrong"
	data, err = json.Marshal(model)
	if err != nil {
		t.Fatalf("marshal invalid model: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write invalid model: %v", err)
	}
	if _, err := LoadLinearLatencyModel(path); err == nil {
		t.Fatal("invalid model loaded successfully")
	}
}

func TestMLAdaptiveFallsBackToAdaptiveV2WhenModelCannotLoad(t *testing.T) {
	fallback := NewAdaptiveV2(DefaultAdaptiveV2Config())
	strategy, err := NewMLAdaptiveFromFile("does-not-exist.json", fallback)
	if err == nil {
		t.Fatal("expected model load error")
	}
	if strategy != fallback {
		t.Fatalf("strategy = %T, want supplied Adaptive V2 fallback", strategy)
	}
}
