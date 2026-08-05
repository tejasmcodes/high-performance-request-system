package algorithms

import (
	"sync"

	"github.com/tejasmcodes/high-performance-request-system/internal/models"
)

// RoundRobin cycles through the server list in order, skipping any
// server that isn't Healthy. State (current) is shared across every
// incoming request's goroutine, so it's protected by a mutex.
type RoundRobin struct {
	current int
	mu      sync.Mutex
}

// NextServer implements the Strategy interface.
func (r *RoundRobin) NextServer(servers []*models.Server) *models.Server {
	if len(servers) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for i := 0; i < len(servers); i++ {
		idx := r.current % len(servers)
		r.current++
		if servers[idx].Healthy {
			return servers[idx]
		}
	}

	return nil
}

// compile-time check: RoundRobin must satisfy Strategy
var _ Strategy = (*RoundRobin)(nil)
