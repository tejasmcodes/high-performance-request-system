package models

import (
	"sync"
	"time"

	"github.com/tejasmcodes/high-performance-request-system/internal/metrics"
)

type Server struct {
	ID                    string
	URL                   string
	Weight                int
	Healthy               bool
	activeConnections     int
	cpuUtilization        float64
	cpuUpdatedAt          time.Time
	responseTime          *metrics.ResponseTime
	responseTimeUpdatedAt time.Time

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

func (s *Server) CPUUtilization() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cpuUtilization
}

func (s *Server) SetCPUUtilization(cpu float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cpuUtilization = cpu
	s.cpuUpdatedAt = time.Now()
}

func (s *Server) CPUUpdatedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cpuUpdatedAt
}

func (s *Server) ObserveResponseTime(duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.responseTime.Observe(duration)
	s.responseTimeUpdatedAt = time.Now()
}

func (s *Server) ResponseTime() float64 {
	return s.responseTime.Value()
}

func (s *Server) ResponseTimeSamples() int64 {
	return s.responseTime.Count()
}

func (s *Server) ResponseTimeSnapshot() (float64, int64, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.responseTime.Value(), s.responseTime.Count(), s.responseTimeUpdatedAt
}
