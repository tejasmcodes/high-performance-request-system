package models

import (
	"sync"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/metrics"
)

type Server struct {
	ID                string
	URL               string
	Weight            int
	Healthy           bool
	activeConnections int
	responseTime      *metrics.ResponseTime

	mu sync.Mutex
}

func NewServer(id, url string, weight int) *Server {
	return &Server{
		ID:           id,
		URL:          url,
		Weight:       weight,
		Healthy:      true,
		responseTime: metrics.NewResponseTime(0.2),
	}
}

func (s *Server) ActiveConnections() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.activeConnections

}

func (s *Server) IncrementConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.activeConnections++
}

func (s *Server) DecrementConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeConnections > 0 {
		s.activeConnections--
	}

}

func (s *Server) IsHealthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Healthy
}

func (s *Server) SetHealthy(healthy bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Healthy = healthy
}

func (s *Server) ObserveResponseTime(duration time.Duration) {
	s.responseTime.Observe(duration)
}

func (s *Server) ResponseTime() float64 {
	return s.responseTime.Value()
}
