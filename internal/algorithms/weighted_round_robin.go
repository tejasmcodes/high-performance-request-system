package algorithms

import (
	"sync"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

type WeightedRoundRobin struct {
	current int
	mu      sync.Mutex
}

func (wrr *WeightedRoundRobin) NextServer(servers []*models.Server) *models.Server {
	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	if len(servers) == 0 {
		return nil
	}

	totalWeight := 0

	for _, server := range servers {
		if !server.IsHealthy() {
			continue
		}

		if server.Weight <= 0 {
			continue
		}

		totalWeight += server.Weight
	}

	if totalWeight == 0 {
		return nil
	}

	wrr.current = wrr.current % totalWeight
	position := 0

	for _, server := range servers {
		if !server.IsHealthy() || server.Weight <= 0 {
			continue
		}

		position += server.Weight
		if wrr.current < position {
			wrr.current++

			return server
		}
	}

	return nil
}
