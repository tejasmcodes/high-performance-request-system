package algorithms

import (
	"math"
	"sync"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

const (
	defaultV2ConnectionsWeight      = 0.45
	defaultV2ResponseTimeWeight     = 0.40
	defaultV2CPUWeight              = 0.15
	defaultV2MissingMetricScore     = 0.5
	defaultV2CPUStaleAfter          = 15 * time.Second
	defaultV2ResponseTimeStaleAfter = 15 * time.Second
	defaultV2ScoreEpsilon           = 1e-6
)

// AdaptiveV2Config is the frozen Adaptive V2 configuration, exposed under a
// separate name so Adaptive V1 remains available for comparisons.
type AdaptiveV2Config struct {
	ConnectionsWeight      float64
	ResponseTimeWeight     float64
	CPUWeight              float64
	MissingMetricScore     float64
	CPUStaleAfter          time.Duration
	ResponseTimeStaleAfter time.Duration
	ScoreEpsilon           float64
}

func DefaultAdaptiveV2Config() AdaptiveV2Config {
	return AdaptiveV2Config{
		ConnectionsWeight:      defaultV2ConnectionsWeight,
		ResponseTimeWeight:     defaultV2ResponseTimeWeight,
		CPUWeight:              defaultV2CPUWeight,
		MissingMetricScore:     defaultV2MissingMetricScore,
		CPUStaleAfter:          defaultV2CPUStaleAfter,
		ResponseTimeStaleAfter: defaultV2ResponseTimeStaleAfter,
		ScoreEpsilon:           defaultV2ScoreEpsilon,
	}
}

// AdaptiveV2 is a behavior-preserving copy of the frozen V2 strategy.
// Its distinct type allows V1 and V2 to be benchmarked together.
type AdaptiveV2 struct {
	cfg    AdaptiveV2Config
	mu     sync.Mutex
	cursor int
}

func NewAdaptiveV2(cfg AdaptiveV2Config) *AdaptiveV2 {
	return &AdaptiveV2{cfg: sanitizeAdaptiveV2Config(cfg)}
}

type adaptiveV2Candidate struct {
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

func (a *AdaptiveV2) NextServer(servers []*models.Server) *models.Server {
	if len(servers) == 0 {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	candidates := make([]adaptiveV2Candidate, 0, len(servers))
	for index, server := range servers {
		if !server.IsHealthy() || server.Weight <= 0 {
			continue
		}

		response, responseSamples, responseUpdatedAt := server.ResponseTimeSnapshot()
		cpu := server.CPUUtilization()
		cpuUpdatedAt := server.CPUUpdatedAt()
		candidates = append(candidates, adaptiveV2Candidate{
			server:      server,
			index:       index,
			weight:      server.Weight,
			connections: server.ActiveConnections(),
			response:    response,
			responseOK:  isFreshV2ResponseTime(response, responseSamples, responseUpdatedAt, now, a.cfg.ResponseTimeStaleAfter),
			cpu:         cpu,
			cpuOK:       isFreshV2CPU(cpu, cpuUpdatedAt, now, a.cfg.CPUStaleAfter),
		})
	}
	if len(candidates) == 0 {
		return nil
	}

	connMin := capacityAdjustedV2Connections(candidates[0].connections, candidates[0].weight)
	connMax := connMin
	var rtMin, rtMax float64
	hasResponse := false
	for _, candidate := range candidates {
		connections := capacityAdjustedV2Connections(candidate.connections, candidate.weight)
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
			rtMin, rtMax, hasResponse = candidate.response, candidate.response, true
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
		connectionScore := normalizeV2MinMax(capacityAdjustedV2Connections(candidate.connections, candidate.weight), connMin, connMax)
		responseScore := a.cfg.MissingMetricScore
		if candidate.responseOK && hasResponse {
			responseScore = normalizeV2MinMax(candidate.response, rtMin, rtMax)
		}
		cpuScore := a.cfg.MissingMetricScore
		if candidate.cpuOK {
			cpuScore = clampV2Unit(candidate.cpu / 100.0)
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

func isFreshV2CPU(cpu float64, updatedAt, now time.Time, staleAfter time.Duration) bool {
	if updatedAt.IsZero() || updatedAt.After(now) || now.Sub(updatedAt) > staleAfter {
		return false
	}
	return isFiniteV2(cpu) && cpu >= 0 && cpu <= 100
}

func isFreshV2ResponseTime(response float64, samples int64, updatedAt, now time.Time, staleAfter time.Duration) bool {
	if samples == 0 || updatedAt.IsZero() || updatedAt.After(now) || now.Sub(updatedAt) > staleAfter {
		return false
	}
	return isFiniteV2(response) && response >= 0
}

func capacityAdjustedV2Connections(connections, weight int) float64 {
	return float64(connections) / float64(weight)
}

func normalizeV2MinMax(value, min, max float64) float64 {
	if max <= min {
		return 0
	}
	return clampV2Unit((value - min) / (max - min))
}

func clampV2Unit(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func isFiniteV2(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func sanitizeAdaptiveV2Config(cfg AdaptiveV2Config) AdaptiveV2Config {
	cfg.ConnectionsWeight = sanitizeV2Weight(cfg.ConnectionsWeight)
	cfg.ResponseTimeWeight = sanitizeV2Weight(cfg.ResponseTimeWeight)
	cfg.CPUWeight = sanitizeV2Weight(cfg.CPUWeight)
	if cfg.ConnectionsWeight+cfg.ResponseTimeWeight+cfg.CPUWeight == 0 {
		defaults := DefaultAdaptiveV2Config()
		cfg.ConnectionsWeight = defaults.ConnectionsWeight
		cfg.ResponseTimeWeight = defaults.ResponseTimeWeight
		cfg.CPUWeight = defaults.CPUWeight
	}
	cfg.MissingMetricScore = sanitizeV2UnitScore(cfg.MissingMetricScore)
	if cfg.CPUStaleAfter <= 0 {
		cfg.CPUStaleAfter = defaultV2CPUStaleAfter
	}
	if cfg.ResponseTimeStaleAfter <= 0 {
		cfg.ResponseTimeStaleAfter = defaultV2ResponseTimeStaleAfter
	}
	if math.IsNaN(cfg.ScoreEpsilon) || math.IsInf(cfg.ScoreEpsilon, 0) || cfg.ScoreEpsilon < 0 {
		cfg.ScoreEpsilon = defaultV2ScoreEpsilon
	}
	return cfg
}

func sanitizeV2Weight(weight float64) float64 {
	if !isFiniteV2(weight) || weight < 0 {
		return 0
	}
	return weight
}

func sanitizeV2UnitScore(score float64) float64 {
	if !isFiniteV2(score) {
		return defaultV2MissingMetricScore
	}
	return clampV2Unit(score)
}
