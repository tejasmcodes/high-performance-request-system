package models

import (
	"sync"
	"testing"
)

func TestServerCPUUtilization(t *testing.T) {
	server := NewServer("backend-1", "http://localhost:8081", 1)

	if got := server.CPUUtilization(); got != 0 {
		t.Fatalf("expected initial CPU utilization 0, got %f", got)
	}

	server.SetCPUUtilization(37.5)

	if got := server.CPUUtilization(); got != 37.5 {
		t.Fatalf("expected CPU utilization 37.5, got %f", got)
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
			}
		}(float64(i))
	}

	wg.Wait()
}
