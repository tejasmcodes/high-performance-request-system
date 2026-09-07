package algorithms

import (
	"github.com/tejasmcodes/high-performance-request-system/internal/models"
	"sync"
)

type RoundRobin struct {
	mu      sync.Mutex
	current int
}

func (rr *RoundRobin) NextServer(servers []*models.Server) *models.Server {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	if len(servers) == 0 {
		return nil
	}

	for i := range servers {
		index := (rr.current + i) % len(servers)

		if servers[index].Healthy {
			rr.current = (index + 1) % len(servers)
			return servers[index]
		}
	}

	return nil
}
