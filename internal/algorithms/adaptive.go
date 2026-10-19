package algorithms

import (
	"math"
	"sync"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

const (
	defaultConnectionsWeight  = 0.45
	defaultResponseTimeWeight = 0.40
	defaultCPUWeight          = 0.15
	defaultMissingMetricScore = 0.5
	defaultCPUStaleAfter      = 15 * time.Second
	defaultScoreEpsilon       = 1e-6
)

type AdaptiveConfig struct {
	ConnectionsWeight  float64
	ResponseTimeWeight float64
	CPUWeight          float64
	MissingMetricScore float64
	CPUStaleAfter      time.Duration
	ScoreEpsilon       float64
}

func DefaultAdaptiveConfig() AdaptiveConfig {
	return AdaptiveConfig{
		ConnectionsWeight:  defaultConnectionsWeight,
		ResponseTimeWeight: defaultResponseTimeWeight,
		CPUWeight:          defaultCPUWeight,
		MissingMetricScore: defaultMissingMetricScore,
		CPUStaleAfter:      defaultCPUStaleAfter,
		ScoreEpsilon:       defaultScoreEpsilon,
	}
}

type Adaptive struct {
	cfg    AdaptiveConfig
	mu     sync.Mutex
	cursor int
}

func NewAdaptive(cfg AdaptiveConfig) *Adaptive {
	return &Adaptive{
		cfg: sanitizeAdaptiveConfig(cfg),
	}
}

type adaptiveCandidate struct {
	server      *models.Server
	index       int
	weight      int
	connections int
	response    float64
	responseOK  bool
	cpu         float64
	cpuOK       bool
	score       float64
}

func (a *Adaptive) NextServer(servers []*models.Server) *models.Server {
	if len(servers) == 0 {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	candidates := make([]adaptiveCandidate, 0, len(servers))

	for index, server := range servers {
		if !server.IsHealthy() || server.Weight <= 0 {
			continue
		}

		response := server.ResponseTime()
		responseSamples := server.ResponseTimeSamples()
		cpu := server.CPUUtilization()
		cpuUpdatedAt := server.CPUUpdatedAt()

		candidates = append(candidates, adaptiveCandidate{
			server:      server,
			index:       index,
			weight:      server.Weight,
			connections: server.ActiveConnections(),
			response:    response,
			responseOK:  responseSamples > 0 && isFinite(response) && response >= 0,
			cpu:         cpu,
			cpuOK:       isFreshCPU(cpu, cpuUpdatedAt, now, a.cfg.CPUStaleAfter),
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	connMin := float64(candidates[0].connections)
	connMax := connMin

	var (
		rtMin       float64
		rtMax       float64
		hasResponse bool
	)

	for _, candidate := range candidates {
		connections := float64(candidate.connections)
		if connections < connMin {
			connMin = connections
		}
		if connections > connMax {
			connMax = connections
		}

		if !candidate.responseOK {
			continue
		}

		if !hasResponse {
			rtMin = candidate.response
			rtMax = candidate.response
			hasResponse = true
			continue
		}

		if candidate.response < rtMin {
			rtMin = candidate.response
		}
		if candidate.response > rtMax {
			rtMax = candidate.response
		}
	}

	for i := range candidates {
		candidate := &candidates[i]

		connectionScore := normalizeMinMax(float64(candidate.connections), connMin, connMax)

		responseScore := a.cfg.MissingMetricScore
		if candidate.responseOK && hasResponse {
			responseScore = normalizeMinMax(candidate.response, rtMin, rtMax)
		}

		cpuScore := a.cfg.MissingMetricScore
		if candidate.cpuOK {
			cpuScore = clampUnit(candidate.cpu / 100.0)
		}

		candidate.score = a.cfg.ConnectionsWeight*connectionScore +
			a.cfg.ResponseTimeWeight*responseScore +
			a.cfg.CPUWeight*cpuScore
	}

	minScore := candidates[0].score
	for _, candidate := range candidates[1:] {
		if candidate.score < minScore {
			minScore = candidate.score
		}
	}

	maxTiedWeight := math.MinInt
	for _, candidate := range candidates {
		if candidate.score <= minScore+a.cfg.ScoreEpsilon && candidate.weight > maxTiedWeight {
			maxTiedWeight = candidate.weight
		}
	}

	tied := make([]bool, len(servers))
	for _, candidate := range candidates {
		if candidate.score <= minScore+a.cfg.ScoreEpsilon && candidate.weight == maxTiedWeight {
			tied[candidate.index] = true
		}
	}

	start := a.cursor % len(servers)
	for offset := 0; offset < len(servers); offset++ {
		index := (start + offset) % len(servers)

		if tied[index] {
			a.cursor = (index + 1) % len(servers)
			return servers[index]
		}
	}

	return nil
}

func isFreshCPU(cpu float64, updatedAt, now time.Time, staleAfter time.Duration) bool {
	if updatedAt.IsZero() || updatedAt.After(now) {
		return false
	}

	if now.Sub(updatedAt) > staleAfter {
		return false
	}

	return isFinite(cpu) && cpu >= 0 && cpu <= 100
}

func normalizeMinMax(value, min, max float64) float64 {
	if max <= min {
		return 0
	}

	return clampUnit((value - min) / (max - min))
}

func clampUnit(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func sanitizeAdaptiveConfig(cfg AdaptiveConfig) AdaptiveConfig {
	cfg.ConnectionsWeight = sanitizeWeight(cfg.ConnectionsWeight)
	cfg.ResponseTimeWeight = sanitizeWeight(cfg.ResponseTimeWeight)
	cfg.CPUWeight = sanitizeWeight(cfg.CPUWeight)

	if cfg.ConnectionsWeight+cfg.ResponseTimeWeight+cfg.CPUWeight == 0 {
		defaults := DefaultAdaptiveConfig()
		cfg.ConnectionsWeight = defaults.ConnectionsWeight
		cfg.ResponseTimeWeight = defaults.ResponseTimeWeight
		cfg.CPUWeight = defaults.CPUWeight
	}

	cfg.MissingMetricScore = sanitizeUnitScore(cfg.MissingMetricScore)

	if cfg.CPUStaleAfter <= 0 {
		cfg.CPUStaleAfter = defaultCPUStaleAfter
	}

	if math.IsNaN(cfg.ScoreEpsilon) || math.IsInf(cfg.ScoreEpsilon, 0) || cfg.ScoreEpsilon < 0 {
		cfg.ScoreEpsilon = defaultScoreEpsilon
	}

	return cfg
}

func sanitizeWeight(weight float64) float64 {
	if !isFinite(weight) || weight < 0 {
		return 0
	}

	return weight
}

func sanitizeUnitScore(score float64) float64 {
	if !isFinite(score) {
		return defaultMissingMetricScore
	}

	return clampUnit(score)
}
