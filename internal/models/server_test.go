package models

import (
	"sync"
	"testing"
	"time"
)

func TestServerCPUUtilization(t *testing.T) {
	server := NewServer("backend-1", "http://localhost:8081", 1)

	if got := server.CPUUtilization(); got != 0 {
		t.Fatalf("expected initial CPU utilization 0, got %f", got)
	}

	if got := server.CPUUpdatedAt(); !got.IsZero() {
		t.Fatalf("expected zero CPU update time before first sample, got %v", got)
	}

	before := time.Now()
	server.SetCPUUtilization(37.5)
	after := time.Now()

	if got := server.CPUUtilization(); got != 37.5 {
		t.Fatalf("expected CPU utilization 37.5, got %f", got)
	}

	updatedAt := server.CPUUpdatedAt()
	if updatedAt.Before(before) || updatedAt.After(after) {
		t.Fatalf("expected CPU update time between %v and %v, got %v", before, after, updatedAt)
	}
}

func TestServerResponseTimeSamples(t *testing.T) {
	server := NewServer("backend-1", "http://localhost:8081", 1)

	if got := server.ResponseTimeSamples(); got != 0 {
		t.Fatalf("expected 0 initial response time samples, got %d", got)
	}

	server.ObserveResponseTime(50 * time.Millisecond)
	server.ObserveResponseTime(100 * time.Millisecond)

	if got := server.ResponseTimeSamples(); got != 2 {
		t.Fatalf("expected 2 response time samples, got %d", got)
	}
}

func TestServerCPUUtilizationConcurrentAccess(t *testing.T) {
	server := NewServer("backend-1", "http://localhost:8081", 1)

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func(value float64) {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				server.SetCPUUtilization(value)
				_ = server.CPUUtilization()
				_ = server.CPUUpdatedAt()
				_ = server.ResponseTimeSamples()
			}
		}(float64(i))
	}

	wg.Wait()
}
