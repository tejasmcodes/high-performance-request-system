package metrics

import (
	"context"
	"log"
	"sync"
	"time"
)

const DefaultCPUSampleInterval = 2 * time.Second

type CPUSampler struct {
	sample func() (float64, error)

	mu    sync.RWMutex
	value float64
}

func NewCPUSampler() *CPUSampler {
	return &CPUSampler{
		sample: ProcessCPUUtilization,
	}
}

func (s *CPUSampler) Sample() {
	value, err := s.sample()
	if err != nil {
		log.Printf("CPU sample error: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.value = value
}

func (s *CPUSampler) Value() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.value
}

func (s *CPUSampler) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultCPUSampleInterval
	}

	s.Sample()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			s.Sample()
		}
	}
}
