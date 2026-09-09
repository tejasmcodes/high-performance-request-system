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

	var healthy []*models.Server

	for _, server := range servers {
		if server.IsHealthy() && server.Weight > 0 {
			healthy = append(healthy, server)
		}
	}

	if len(healthy) == 0 {
		return nil
	}

	totalWeight := 0
	for _, server := range healthy {
		totalWeight += server.Weight
	}

	wrr.current %= totalWeight

	position := 0

	for _, server := range healthy {
		position += server.Weight

		if wrr.current < position {
			selected := server
			wrr.current++
			return selected
		}
	}

	return nil
}