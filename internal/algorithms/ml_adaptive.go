package algorithms

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

const (
	mlModelVersion           = 1
	mlModelTarget            = "predicted_latency_ms (lower is better, pick the minimum)"
	mlCPUStaleAfter          = 15 * time.Second
	mlResponseTimeStaleAfter = 15 * time.Second
)

var mlFeatures = []string{"in_flight", "ema_ms", "cpu"}

// LinearLatencyModel is the portable model format produced by ml/train.py.
// Its coefficients operate on raw feature values in the documented order.
type LinearLatencyModel struct {
	Version      int       `json:"version"`
	Type         string    `json:"type"`
	Target       string    `json:"target"`
	Features     []string  `json:"features"`
	Coefficients []float64 `json:"coefficients"`
	Intercept    float64   `json:"intercept"`
	Defaults     struct {
		InFlight float64 `json:"in_flight"`
		EMAMs    float64 `json:"ema_ms"`
		CPU      float64 `json:"cpu"`
	} `json:"defaults"`
}

// LoadLinearLatencyModel loads and validates the fixed, standard-library JSON
// representation. It never invokes Python or a remote model service.
func LoadLinearLatencyModel(path string) (LinearLatencyModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LinearLatencyModel{}, fmt.Errorf("read ML model %q: %w", path, err)
	}

	var model LinearLatencyModel
	if err := json.Unmarshal(data, &model); err != nil {
		return LinearLatencyModel{}, fmt.Errorf("decode ML model %q: %w", path, err)
	}
	if err := model.Validate(); err != nil {
		return LinearLatencyModel{}, fmt.Errorf("validate ML model %q: %w", path, err)
	}

	return model, nil
}

func (m LinearLatencyModel) Validate() error {
	if m.Version != mlModelVersion {
		return fmt.Errorf("unsupported version %d", m.Version)
	}
	if m.Type != "linear" {
		return fmt.Errorf("unsupported type %q", m.Type)
	}
	if m.Target != mlModelTarget {
		return fmt.Errorf("unsupported target %q", m.Target)
	}
	if len(m.Features) != len(mlFeatures) || len(m.Coefficients) != len(mlFeatures) {
		return fmt.Errorf("expected %d features and coefficients", len(mlFeatures))
	}
	for i, feature := range mlFeatures {
		if m.Features[i] != feature {
			return fmt.Errorf("feature %d = %q, want %q", i, m.Features[i], feature)
		}
	}
	if !isFiniteML(m.Intercept) || !isFiniteML(m.Defaults.InFlight) || !isFiniteML(m.Defaults.EMAMs) || !isFiniteML(m.Defaults.CPU) {
		return fmt.Errorf("intercept and defaults must be finite")
	}
	if m.Defaults.InFlight < 0 || m.Defaults.EMAMs < 0 || m.Defaults.CPU < 0 || m.Defaults.CPU > 100 {
		return fmt.Errorf("defaults are outside supported metric ranges")
	}
	for _, coefficient := range m.Coefficients {
		if !isFiniteML(coefficient) {
			return fmt.Errorf("coefficient must be finite")
		}
	}
	return nil
}

// MLAdaptive predicts each eligible backend's latency and picks the minimum.
// Equal predictions retain configured server order as deterministic tie-breaking.
type MLAdaptive struct {
	model LinearLatencyModel
}

func NewMLAdaptive(model LinearLatencyModel) *MLAdaptive {
	return &MLAdaptive{model: model}
}

// NewMLAdaptiveFromFile returns fallback on model load or validation failure.
// The returned error is intended for startup logging; routing remains available.
func NewMLAdaptiveFromFile(path string, fallback Strategy) (Strategy, error) {
	model, err := LoadLinearLatencyModel(path)
	if err != nil {
		return fallback, err
	}
	return NewMLAdaptive(model), nil
}

func (a *MLAdaptive) NextServer(servers []*models.Server) *models.Server {
	now := time.Now()
	var selected *models.Server
	bestPrediction := math.Inf(1)

	for _, server := range servers {
		if !server.IsHealthy() || server.Weight <= 0 {
			continue
		}

		prediction := a.predict(server, now)
		if prediction < bestPrediction {
			selected = server
			bestPrediction = prediction
		}
	}

	return selected
}

func (a *MLAdaptive) predict(server *models.Server, now time.Time) float64 {
	response, samples, responseUpdatedAt := server.ResponseTimeSnapshot()
	ema := a.model.Defaults.EMAMs
	if isFreshMLResponseTime(response, samples, responseUpdatedAt, now) {
		ema = response
	}

	cpu := server.CPUUtilization()
	if !isFreshMLCPU(cpu, server.CPUUpdatedAt(), now) {
		cpu = a.model.Defaults.CPU
	}

	inFlight := float64(server.ActiveConnections())
	return a.model.Intercept +
		a.model.Coefficients[0]*inFlight +
		a.model.Coefficients[1]*ema +
		a.model.Coefficients[2]*cpu
}

func isFreshMLCPU(cpu float64, updatedAt, now time.Time) bool {
	if updatedAt.IsZero() || updatedAt.After(now) || now.Sub(updatedAt) > mlCPUStaleAfter {
		return false
	}
	return isFiniteML(cpu) && cpu >= 0 && cpu <= 100
}

func isFreshMLResponseTime(response float64, samples int64, updatedAt, now time.Time) bool {
	if samples == 0 || updatedAt.IsZero() || updatedAt.After(now) || now.Sub(updatedAt) > mlResponseTimeStaleAfter {
		return false
	}
	return isFiniteML(response) && response >= 0
}

func isFiniteML(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
